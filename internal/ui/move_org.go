package ui

// "Find the same resource in another org."

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func orgMoveLabel(o sf.Org) string {
	if l := o.Display(); l != "" {
		return l
	}
	return o.Username
}

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

func moveNameOf(it ItemIdentity) (name, typeHint string) {
	switch it.Kind {
	case devproject.KindSObject:
		return it.Ref, ""
	case devproject.KindField:
		if i := indexOfRune(it.Ref, '.'); i >= 0 {
			return it.Ref, it.Ref[:i]
		}
		return it.Ref, ""
	default:
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

	cmds := []tea.Cmd{}
	if td := m.orgDataFor(targetUser); td != nil {
		if c := m.moveEnsureCmd(td, it.Kind); c != nil {
			cmds = append(cmds, c)
		}
	}
	if navCmd := m.resolvePendingMove(); navCmd != nil {
		cmds = append(cmds, navCmd)
	}
	return tea.Batch(cmds...)
}

func (m Model) orgIndexByUser(username string) (int, bool) {
	for i, o := range m.orgs {
		if o.Username == username {
			return i, true
		}
	}
	return 0, false
}

func mustOrgIndex(m *Model, username string) int {
	i, _ := m.orgIndexByUser(username)
	return i
}

func (m *Model) orgDataFor(username string) *orgData {
	if username == "" {
		return nil
	}
	return m.ensureOrgData(username)
}

// moveOrgPickerTargetID is the sentinel on the "Find in another org…"
// row that requestOpenMenu injects. fireMenuTarget intercepts it to
// open the org sub-picker rather than opening a URL.
const moveOrgPickerTargetID = "__find_in_org_picker__"

const moveOrgChoiceIDPrefix = "__find_in_org__:"

func moveOrgChoiceID(username string) string { return moveOrgChoiceIDPrefix + username }

func parseMoveOrgChoiceID(id string) (string, bool) {
	if !strings.HasPrefix(id, moveOrgChoiceIDPrefix) {
		return "", false
	}
	return strings.TrimPrefix(id, moveOrgChoiceIDPrefix), true
}

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
		m.move = nil
		m.flash(fmt.Sprintf("%q not in %s", mv.label, orgMoveLabel(m.orgs[targetIdx])))
		return nil
	}
	m.move = nil
	m.setSelectedOrg(targetIdx)
	m.flash("opening " + mv.label + " in " + orgMoveLabel(m.orgs[targetIdx]))
	cmd, ok := drillByKind(m, string(mv.kind), ref, mv.typeHint, mv.label, mv.fromTab)
	if !ok {
		m.setTab(moveListTabFor(mv.kind))
		return m.onTabChanged()
	}
	return cmd
}
