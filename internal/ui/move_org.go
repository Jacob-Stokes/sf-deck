package ui

// "Find the same resource in another org."
//
// The gesture lives inside the open (o / ctrl+o) menu as a synthetic
// "Find in another org…" row (see menu.go). Picking it swaps the menu
// for an org sub-picker; picking an org there looks up the SAME
// resource in that org and, ONLY if it exists there, switches to that
// org and drills into it. If the resource isn't in the target org, the
// user stays put and gets a "not in <org>" flash — we never strand
// them in another org just to report the miss.
//
// Why this is more than "switch org + keep the tab": tab/subtab state
// is already per-org (orgData), so a bare switch lands the user on
// whatever they last looked at in that org. This carries the cursored
// resource across, re-resolves it by its STABLE name in the target org
// (record Ids and metadata Ids are org-local and don't map across
// orgs), and drills into its detail view.
//
// Cross-org matching key: for API-name-keyed kinds (sObject, field)
// the identity Ref is already stable and used directly. For Id-keyed
// kinds (flow, apex, LWC/Aura, permset, …) we match on the developer/
// API name and read the TARGET org's org-local Id back out of its
// loaded list. That list may still be fetching at switch time, so the
// resolve is deferred: a pendingMove rides on the model until the
// relevant resource lands (applyResourceMsg calls resolvePendingMove).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// orgMoveLabel is the user-visible name for an org in move flashes and
// the picker — the configured label/alias, falling back to username.
func orgMoveLabel(o sf.Org) string {
	if l := o.Display(); l != "" {
		return l
	}
	return o.Username
}

// pendingMove is a Move-to-org request awaiting the target org's data.
//
// Set when the user picks a destination org; cleared once we either
// drill into the matched resource or give up (list loaded, no match).
type pendingMove struct {
	kind     devproject.ItemKind
	name     string // stable match key (developer/API name, or sobj.field)
	typeHint string // secondary key: parent sObject for fields/triggers, bundle name, etc.
	label    string // user-visible resource label for flashes
	fromTab  Tab    // originator tab, for drill return-tab tracking
	target   string // target org username (sanity guard against races)
}

// movableKind reports whether a cursored kind can be moved across orgs.
//
// v1 scope — the kinds that (a) have a stable cross-org name and (b)
// re-resolve cleanly from a single list this file knows how to fetch
// directly (moveEnsureCmd):
//
//   - sObject / field — Ref is already the API name; nothing to
//     re-resolve, so these work even offline-from-cache.
//   - flow / apex class / LWC bundle — Id-keyed, matched by developer
//     name against the target org's loaded list.
//
// Deliberately excluded for now: records (org-local Id, no cross-org
// identity); flow versions (a version Id, not the flow); apex
// triggers, Aura bundles, permsets / PSGs / profiles / queues / public
// groups (their lists load per-subtab or via a combined resource —
// a clean fast-follow once the core gesture is proven); and local-only
// kinds (SOQL / apex snippets live in the tag store, not an org).
func movableKind(k devproject.ItemKind) bool {
	switch k {
	case devproject.KindSObject,
		devproject.KindField,
		devproject.KindFlow,
		devproject.KindApexClass,
		devproject.KindLWC:
		return true
	}
	return false
}

// moveEnsureCmd fires the fetch for the target org's list that a move
// of this kind will re-resolve against, independent of which tab/subtab
// is active. Returns nil for kinds whose Ref needs no re-resolution
// (sObject / field), or when there's nothing to fetch.
func (m *Model) moveEnsureCmd(d *orgData, k devproject.ItemKind) tea.Cmd {
	switch k {
	case devproject.KindSObject, devproject.KindField:
		return d.SObjects.Ensure(m.cache)
	case devproject.KindFlow:
		return d.Flows.Ensure(m.cache)
	case devproject.KindApexClass:
		return d.ApexClasses.Ensure(m.cache)
	case devproject.KindLWC:
		return d.LWCBundles.Ensure(m.cache)
	}
	return nil
}

// moveNameOf returns the stable cross-org match key for an identity,
// plus a type hint (secondary key) where the kind needs one.
//
// For sObjects and fields the Ref is already the API name, so it's
// returned verbatim. For Id-keyed kinds the Label carries the
// developer/API name (that's how the identity resolvers populate it).
func moveNameOf(it ItemIdentity) (name, typeHint string) {
	switch it.Kind {
	case devproject.KindSObject:
		return it.Ref, ""
	case devproject.KindField:
		// Ref is "<sobj>.<field>"; typeHint carries the parent sObject
		// so the target-org resolver can jump straight to the describe.
		if i := indexOfRune(it.Ref, '.'); i >= 0 {
			return it.Ref, it.Ref[:i]
		}
		return it.Ref, ""
	default:
		// Id-keyed kinds: Label is the developer/API name captured by
		// the identity resolver.
		return it.Label, ""
	}
}

// beginFindInOrg captures the cursored resource and arms a lookup in
// the target org — WITHOUT switching to it. The target org's data is
// fetched in the background (its Resource fetchers capture their own
// alias, so no org switch is needed); resolvePendingMove then either
// navigates on a confirmed hit or flashes "not found" and stays put.
//
// This ordering is deliberate: we never strand the user in another org
// only to discover the resource isn't there. The switch happens ONLY
// once we've confirmed the resource exists in the target.
//
// targetUser must be a currently-loaded, usable org other than the
// active one; the caller (the org sub-picker) guarantees this.
func (m *Model) beginFindInOrg(targetUser string) tea.Cmd {
	it, ok := m.resolveItemIdentity()
	if !ok || !movableKind(it.Kind) {
		m.flash("nothing to find here")
		return nil
	}
	name, typeHint := moveNameOf(it)
	if name == "" {
		m.flash("can't match this resource across orgs")
		return nil
	}
	if _, ok := m.orgIndexByUser(targetUser); !ok {
		m.flash("org not loaded")
		return nil
	}

	m.move = &pendingMove{
		kind:     it.Kind,
		name:     name,
		typeHint: typeHint,
		label:    it.Label,
		fromTab:  m.tab(),
		target:   targetUser,
	}
	m.flash("finding " + it.Label + " in " + orgMoveLabel(m.orgs[mustOrgIndex(m, targetUser)]) + "…")

	// Fetch the target org's list in the background. The fetch closure
	// carries its own alias, so this loads the RIGHT org's data even
	// though that org isn't selected. applyResourceMsg routes the
	// result by scope → resolvePendingMove picks it up.
	cmds := []tea.Cmd{}
	if td := m.orgDataFor(targetUser); td != nil {
		if c := m.moveEnsureCmd(td, it.Kind); c != nil {
			cmds = append(cmds, c)
		}
	}
	// Immediate resolve when the target list is already in memory
	// (common when the user has visited that org this session).
	if navCmd := m.resolvePendingMove(); navCmd != nil {
		cmds = append(cmds, navCmd)
	}
	return tea.Batch(cmds...)
}

// orgIndexByUser returns the index of the org with the given username.
func (m Model) orgIndexByUser(username string) (int, bool) {
	for i, o := range m.orgs {
		if o.Username == username {
			return i, true
		}
	}
	return 0, false
}

// mustOrgIndex is orgIndexByUser for call sites that have already
// validated the username exists (returns 0 on miss — the caller
// guarantees a hit).
func mustOrgIndex(m *Model, username string) int {
	i, _ := m.orgIndexByUser(username)
	return i
}

// orgDataFor returns the orgData for a username, allocating it if
// needed — so a background fetch can populate a not-yet-visited org.
func (m *Model) orgDataFor(username string) *orgData {
	if username == "" {
		return nil
	}
	return m.ensureOrgData(username)
}

// --- open-menu integration -------------------------------------------
//
// Find-in-org lives as a synthetic row in the open (o) menu. Picking
// it swaps the menu for an org sub-picker (same openMenuStack push the
// browser picker uses); picking an org there fires beginFindInOrg.

// moveOrgPickerTargetID is the sentinel on the "Find in another org…"
// row that requestOpenMenu injects. fireMenuTarget intercepts it to
// open the org sub-picker rather than opening a URL.
const moveOrgPickerTargetID = "__find_in_org_picker__"

// moveOrgChoiceIDPrefix marks a synthetic sub-picker row that, when
// fired, searches that org for the cursored resource (username follows).
const moveOrgChoiceIDPrefix = "__find_in_org__:"

func moveOrgChoiceID(username string) string { return moveOrgChoiceIDPrefix + username }

func parseMoveOrgChoiceID(id string) (string, bool) {
	if !strings.HasPrefix(id, moveOrgChoiceIDPrefix) {
		return "", false
	}
	return strings.TrimPrefix(id, moveOrgChoiceIDPrefix), true
}

// moveOrgTargets returns the other connected orgs the cursored resource
// could move to (excludes the active org and any unusable/disconnected
// one). Empty when there's nowhere to move.
func (m Model) moveOrgTargets() []sf.Org {
	var out []sf.Org
	cur := ""
	if len(m.orgs) > 0 {
		cur = m.orgs[m.selected].Username
	}
	for _, o := range m.orgs {
		if o.Username == cur || !canUseOrg(o) {
			continue
		}
		out = append(out, o)
	}
	return out
}

// moveOrgOpenTarget returns the synthetic "Move to org…" row for the
// open menu, or nil when the cursored resource isn't movable or there's
// no other org to move it to. Injected by requestOpenMenu (open mode
// only — moving is not a yank).
func (m Model) moveOrgOpenTarget() *sf.OpenTarget {
	it, ok := m.resolveItemIdentity()
	if !ok || !movableKind(it.Kind) {
		return nil
	}
	if len(m.moveOrgTargets()) == 0 {
		return nil
	}
	return &sf.OpenTarget{
		ID:    moveOrgPickerTargetID,
		Label: "Find in another org…",
		// No Shortcut — avoids colliding with a real target's
		// accelerator; the row is reachable via j/k + enter.
	}
}

// openMoveOrgSubPicker swaps the active open menu for an org chooser.
// Pushes the current menu so esc pops back to it (mirrors the browser
// sub-picker). Rows are the other connected orgs; firing one moves the
// cursored resource there.
func (m *Model) openMoveOrgSubPicker() tea.Cmd {
	if m.openMenu == nil {
		return nil
	}
	orgs := m.moveOrgTargets()
	if len(orgs) == 0 {
		m.flash("no other connected org to search")
		return nil
	}
	rows := make([]sf.OpenTarget, 0, len(orgs))
	for _, o := range orgs {
		label := orgMoveLabel(o)
		if o.IsSandbox {
			label += " · sandbox"
		} else if o.IsScratch {
			label += " · scratch"
		}
		rows = append(rows, sf.OpenTarget{ID: moveOrgChoiceID(o.Username), Label: label})
	}
	prev := *m.openMenu
	label := prev.title
	if src := prev.source; src != nil {
		label = cursorLabel(src)
	}
	m.openMenuStack = append(m.openMenuStack, prev)
	m.openMenu = &openMenuState{
		title:               "Find in org · " + label,
		mode:                menuOpen,
		org:                 prev.org,
		source:              prev.source,
		targets:             rows,
		cursor:              0,
		restoreGlobalSearch: prev.restoreGlobalSearch,
	}
	return nil
}

// fireMoveOrgChoice completes a move to the org encoded in the selected
// sub-picker row. Unwinds the whole open-menu stack, then arms the move.
func (m Model) fireMoveOrgChoice(idx int) (Model, tea.Cmd) {
	if m.openMenu == nil || idx < 0 || idx >= len(m.openMenu.targets) {
		return m, nil
	}
	username, ok := parseMoveOrgChoiceID(m.openMenu.targets[idx].ID)
	if !ok {
		return m, nil
	}
	m.openMenu = nil
	m.openMenuStack = nil
	mm := m
	cmd := (&mm).beginFindInOrg(username)
	return mm, cmd
}

// moveListTabFor maps a movable kind to the list Tab that hosts it, so
// the target org lands on the right surface (and fires the right
// EnsureData) before the resolve completes.
func moveListTabFor(k devproject.ItemKind) Tab {
	switch k {
	case devproject.KindSObject, devproject.KindField:
		return TabObjects
	case devproject.KindFlow:
		return TabFlows
	case devproject.KindApexClass:
		return TabApex
	case devproject.KindLWC:
		return TabLWC
	}
	return TabHome
}

// resolvePendingMove tries to complete an armed find against the
// TARGET org's data — WITHOUT having switched to it. It resolves the
// resource in the target org's (background-loaded) list and only
// switches + drills once existence is confirmed. Returns a nav command
// on a hit; nil otherwise.
//
// Three terminal outcomes clear m.move:
//   - found  → switch to the target org and drill into the resource
//   - absent → target list loaded but no match; flash in the CURRENT
//     org and stay put (no switch — we never strand the user)
//   - gone   → target org no longer loaded (removed mid-flight); drop
//
// While the target list is still fetching, returns nil and keeps
// m.move armed so the next resource msg retries.
func (m *Model) resolvePendingMove() tea.Cmd {
	mv := m.move
	if mv == nil {
		return nil
	}
	targetIdx, ok := m.orgIndexByUser(mv.target)
	if !ok {
		m.move = nil // target org disappeared; abandon quietly
		return nil
	}
	td := m.data[mv.target]
	if td == nil {
		return nil // background fetch hasn't allocated/populated it yet
	}
	ref, found, ready := resolveMoveRef(td, mv.kind, mv.name, mv.typeHint)
	if !ready {
		return nil // target list still fetching; try again next msg
	}
	if !found {
		// Resource genuinely absent in the target org. Stay in the
		// current org — do NOT switch — and tell the user.
		m.move = nil
		m.flash(fmt.Sprintf("%q not in %s", mv.label, orgMoveLabel(m.orgs[targetIdx])))
		return nil
	}
	// Confirmed present — NOW switch to the target org and drill into
	// the resource's detail. drillByKind operates on the selected org,
	// so select first.
	m.move = nil
	m.setSelectedOrg(targetIdx)
	m.flash("opening " + mv.label + " in " + orgMoveLabel(m.orgs[targetIdx]))
	cmd, ok := drillByKind(m, string(mv.kind), ref, mv.typeHint, mv.label, mv.fromTab)
	if !ok {
		// Kind has no detail surface (shouldn't happen for movable
		// kinds) — fall back to its list so the user at least lands
		// on the right surface in the target org.
		m.setTab(moveListTabFor(mv.kind))
		return m.onTabChanged()
	}
	return cmd
}
