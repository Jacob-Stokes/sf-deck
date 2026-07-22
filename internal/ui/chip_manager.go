package ui

// Unified chip manager. One opener + one dispatcher serve every
// surface (records / objects / flows). The shared modal_chip_manager.go
// owns the menu shape; this file owns the per-surface specifics:
// which registry to read, which scope, which import flow, and how to
// describe the chip in the menu hint.
//
// Replaces modal_lens_manager.go + modal_filter_manager.go (both
// removed in the unified-query-ast cutover).

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/services/chips"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// chipDomain identifies which surface a manager invocation targets.
// Drives registry lookup + import flow + scope.
type chipDomain string

const (
	domainRecords     chipDomain = "records"
	domainObjects     chipDomain = "objects"
	domainFlows       chipDomain = "flows"
	domainApex        chipDomain = "apex"
	domainTriggers    chipDomain = "triggers"
	domainLWC         chipDomain = "lwc"
	domainAura        chipDomain = "aura"
	domainPermSets    chipDomain = "permsets"
	domainPSGs        chipDomain = "psgs"
	domainProfiles    chipDomain = "profiles"
	domainQueues      chipDomain = "queues"
	domainPublicGroup chipDomain = "publicgroups"
	domainSOQLSaved   chipDomain = "soql-saved"
	domainSOQLHistory chipDomain = "soql-history"
	domainRecent      chipDomain = "recent"
	domainUsers       chipDomain = "users"
	domainDashboards  chipDomain = "dashboards"
	domainDeploys     chipDomain = "deploys"
	domainReportTypes chipDomain = "reporttypes"
	domainActiveUsers chipDomain = "active-users"
)

// registryFor returns the live registry pointer for a domain.
// Straight map lookup since every domain (Records included) lives in
// chipRegistries.
func (m *Model) registryFor(d chipDomain) *qchip.Registry {
	return m.chipRegistry(d)
}

// allChipRegistries returns every chip Registry on the model. Used
// to broadcast cross-cutting state changes (active-org filter,
// settings reload) to every domain at once. Derived from the
// registries map, so a new domain can't be forgotten — the old
// hand-maintained list silently omitted the schema-fields registry.
func (m *Model) allChipRegistries() []*qchip.Registry {
	out := make([]*qchip.Registry, 0, len(m.chipRegistries))
	for _, r := range m.chipRegistries {
		out = append(out, r)
	}
	return out
}

// setActiveOrgOnChipRegistries gates every Registry's ChipsFor output
// by the given org's username so user-stored chips only appear when the
// user is on an org their scope allows. Empty orgUser falls back to
// "global only" — strictest safe default if there's no org selected
// (no active session). Also re-injects the group-membership resolver
// so chips shared with an OrgGroup resolve correctly; the closure
// reads settings live, so post-startup group edits take effect on the
// next render without re-registering.
func (m *Model) setActiveOrgOnChipRegistries(orgUser string) {
	groupMembers := m.chipGroupMembersResolver()
	for _, r := range m.allChipRegistries() {
		if r == nil {
			continue
		}
		r.SetActiveOrg(orgUser)
		r.SetGroupMembers(groupMembers)
	}
}

// chipGroupMembersResolver builds the closure registries use to answer
// "is this username a member of this OrgGroup?" for ChipShareGroup
// resolution. Reads settings live so a renamed/edited group is honoured
// immediately. Returns nil when there's no settings store (group-shared
// chips then fail closed, which matches the registry's documented
// "nil = fail closed" contract).
func (m Model) chipGroupMembersResolver() func(groupID, username string) bool {
	if m.settings == nil {
		return nil
	}
	st := m.settings
	return func(groupID, username string) bool {
		return st.OrgGroupForUsername(username) == groupID
	}
}

// activeOrgUserForChips returns the username to stamp on a newly
// created or imported chip. Empty when no org is selected — caller
// should reject the save in that case (creating a chip without an
// org would silently leak into other orgs after the next launch).
func (m Model) activeOrgUserForChips() string {
	if len(m.orgs) == 0 {
		return ""
	}
	return m.orgs[m.selected].Username
}

// openChipManagerFor opens the manager modal for the given domain.
// `scope` is the chip's applicability — for records-shaped surfaces
// it's the active sObject API name; for the universal surfaces
// (/objects, /flows) it's "*".
func (m *Model) openChipManagerFor(d chipDomain, scope, title string, withImport bool) tea.Cmd {
	reg := m.registryFor(d)
	if reg == nil {
		return nil
	}
	chips := reg.ChipsFor(scope)
	rows := make([]chipMenuRow, 0, len(chips))
	for _, c := range chips {
		rows = append(rows, chipMenuRow{
			ID:              c.ID,
			Label:           c.Label,
			Hint:            chipManagerHint(*m, c),
			Origin:          c.Origin,
			Share:           c.Share,
			Favourite:       c.Favourite,
			LockedFavourite: c.LockedFavourite,
		})
	}
	importLabel := ""
	if withImport {
		importLabel = "Import from Salesforce…"
	}
	return m.openChipManagerMenu(chipManagerSpec{
		Kind:        string(d),
		Title:       title,
		Scope:       scope,
		Chips:       rows,
		OtherOrgs:   m.chipsFromOtherOrgs(d, scope),
		Ephemerals:  m.ephemeralChipsFor(d, scope),
		NewLabel:    "New view…",
		ImportLabel: importLabel,
	})
}

// ephemeralChipsFor returns the IPC-spawned session-only chips
// registered at (domain, scope), shaped as chipMenuRows for the
// manager modal. Cross-org "Preview here" entries (which share the
// chipPreviews storage) are filtered out — they get their own
// "other orgs" section.
func (m Model) ephemeralChipsFor(d chipDomain, scope string) []chipMenuRow {
	previews := m.chipPreviewsFor(d, scope)
	if len(previews) == 0 {
		return nil
	}
	out := make([]chipMenuRow, 0, len(previews))
	for _, p := range previews {
		if p.OriginOrgUser != chipPreviewOriginIPC {
			continue
		}
		out = append(out, chipMenuRow{
			ID:     p.Chip.ID,
			Label:  p.Chip.Label,
			Hint:   "session-only · e to save or dismiss",
			Origin: qchip.OriginUser,
		})
	}
	return out
}

// chipsFromOtherOrgs returns chips that match the current (domain, scope)
// but whose Share excludes the active org — these are surfaced in the
// manager modal's "chips from your other orgs" section so users can
// preview or widen scope without leaving the current view.
//
// Reads directly from settings (rather than from registries) because
// the registries are pre-filtered to the active org — other orgs' chips
// aren't in them.
func (m Model) chipsFromOtherOrgs(d chipDomain, scope string) []otherOrgChipRow {
	if m.settings == nil {
		return nil
	}
	active := m.activeOrgUserForChips()
	groupMembers := m.chipGroupMembersResolver()
	var out []otherOrgChipRow
	for _, c := range m.settings.ChipsForDomain(string(d)) {
		if !chipScopeApplies(c.Scope, scope) {
			continue
		}
		share := c.EffectiveShare()
		if share.Allows(active, groupMembers) {
			continue // already visible for this org — handled by the main list
		}
		// Identify the origin org so the preview tag / sub-modal can show it.
		origin := chipOriginOrgFromShare(share)
		out = append(out, otherOrgChipRow{
			ID:            c.ID,
			Label:         c.Label,
			OriginOrgUser: origin,
			Hint:          fmt.Sprintf("from %s", chipShareFriendlyOrg(m, origin)),
			Chip:          qchip.FromConfig(c),
		})
	}
	return out
}

// chipScopeApplies mirrors qchip.scopeApplies (private to the qchip
// package) — empty/"*" matches everything, otherwise exact match.
func chipScopeApplies(chipScope, querScope string) bool {
	if chipScope == "" || chipScope == "*" {
		return true
	}
	return chipScope == querScope
}

// chipOriginOrgFromShare picks a representative origin org for display.
// For ChipShareOrg/Orgs it's the first listed username; for Group it's
// the group id (the friendly-name resolver will translate); for global
// it's "" (the section won't list these — they're always allowed —
// but the helper is defensive).
func chipOriginOrgFromShare(s settings.ChipShare) string {
	switch s.Kind {
	case settings.ChipShareOrg, settings.ChipShareOrgs:
		if len(s.Orgs) > 0 {
			return s.Orgs[0]
		}
	case settings.ChipShareGroup:
		return s.Group // friendly-name lookup happens in the renderer
	}
	return ""
}

// otherOrgChipRow is one entry in the manager modal's cross-org section.
type otherOrgChipRow struct {
	ID            string
	Label         string
	OriginOrgUser string
	Hint          string
	Chip          qchip.Chip
}

// chipHint summarises a chip for the right-hand column of the menu.
// Imported chips show their posterity link; user chips show a SOQL
// snippet of the predicate; built-ins fall through to "built-in".
// chipManagerHint composes the manager-row hint: the predicate / source
// summary (chipHint) plus a "· <who>" share summary when the chip is
// shared with other orgs. Single-org chips get just the predicate hint
// (the row carries no ⇄ marker, so no need to mention scope).
func chipManagerHint(m Model, c qchip.Chip) string {
	base := chipHint(c)
	if !c.Share.IsShared() {
		return base
	}
	share := chipWizardShareSummary(m, c.Share)
	if share == "" {
		return base
	}
	if base == "" {
		return share
	}
	return base + " · " + share
}

func chipHint(c qchip.Chip) string {
	if c.Origin == qchip.OriginBuiltIn {
		return "built-in"
	}
	if c.Origin == qchip.OriginImported && c.SourceName != "" {
		when := ""
		if len(c.ImportedAt) >= 10 {
			when = " on " + c.ImportedAt[:10]
		}
		return "imported from \"" + c.SourceName + "\"" + when
	}
	if where := query.ToSOQLWhere(c.Query.Where); where != "" {
		if len(where) > 50 {
			where = ansi.Truncate(where, 48, "…")
		}
		return where
	}
	if len(c.Query.OrderBy) > 0 {
		return "ORDER BY " + c.Query.OrderBy[0].Field
	}
	return "(no filter)"
}

// dispatchChipManagerAction routes a manager-menu pick. Replaces the
// per-surface dispatchLensManagerAction / dispatchObjectsFilterAction
// / dispatchFlowsFilterAction that the old code had.
func (m *Model) dispatchChipManagerAction(d chipDomain, scope, pick string) tea.Cmd {
	switch {
	case pick == "new":
		return m.openChipWizard(d, qchip.Chip{Scope: chipScopeFor(m, d)})
	case pick == "import":
		return m.openChipImportPicker(d)
	case strings.HasPrefix(pick, "apply:"):
		// Enter on a chip row: make it the active view, exactly like
		// picking it from the M overflow modal.
		id := strings.TrimPrefix(pick, "apply:")
		if scope == "" {
			scope = chipScopeFor(m, d)
		}
		return m.applyChipSelection(d, scope, id)
	case strings.HasPrefix(pick, "actions:"):
		// Enter on a chip row opens the per-chip action picker
		// (Pin/Unpin · Edit · Delete · Cancel) instead of
		// committing immediately. Keeps the manager list one-row-
		// per-chip while preserving every action.
		id := strings.TrimPrefix(pick, "actions:")
		return m.openChipActionsModal(d, id)
	case strings.HasPrefix(pick, "otherorg:"):
		// Enter on a row from the "other orgs" section opens the
		// preview / widen-scope sub-modal — no destructive defaults.
		id := strings.TrimPrefix(pick, "otherorg:")
		return m.openOtherOrgChipActions(d, id)
	case strings.HasPrefix(pick, "eph:"):
		// Enter on a session-only chip row: apply it (same as Enter
		// on any other chip — the strip already shows it, we just
		// move the cursor onto it).
		id := strings.TrimPrefix(pick, "eph:")
		if scope == "" {
			scope = chipScopeFor(m, d)
		}
		return m.applyChipSelection(d, scope, id)
	case strings.HasPrefix(pick, "ephactions:"):
		// e on an ephemeral row opens the Save/Dismiss sub-modal
		// instead of the usual rename/delete/scope set — those don't
		// apply to a session-only chip.
		id := strings.TrimPrefix(pick, "ephactions:")
		return m.openEphemeralChipActions(d, id)
	case strings.HasPrefix(pick, "ephsave:"):
		id := strings.TrimPrefix(pick, "ephsave:")
		return m.openEphemeralSavePrompt(d, id)
	case strings.HasPrefix(pick, "ephdismiss:"):
		id := strings.TrimPrefix(pick, "ephdismiss:")
		p, ok := m.findChipPreview(id)
		if !ok {
			m.flash("session chip not found")
			return nil
		}
		m.removeChipPreview(p.Domain, p.Scope, id)
		m.flash("dismissed " + p.Chip.Label)
		return nil
	case strings.HasPrefix(pick, "otherpreview:"):
		id := strings.TrimPrefix(pick, "otherpreview:")
		cfg, ok := m.findChipConfigByID(d, id)
		if !ok {
			m.flash("chip not found")
			return nil
		}
		c := qchip.FromConfig(cfg)
		originOrg := chipOriginOrgFromShare(cfg.EffectiveShare())
		if scope == "" {
			scope = chipScopeFor(m, d)
		}
		m.addChipPreview(d, scope, c, originOrg)
		m.flash("previewing " + c.Label + " (session only)")
		// Make the preview the ACTIVE view immediately — the preview
		// row is already on the strip, so this just moves the cursor
		// onto it (same path as Enter-to-apply on a local chip).
		return m.applyChipSelection(d, scope, id)
	case strings.HasPrefix(pick, "otherscope:"):
		id := strings.TrimPrefix(pick, "otherscope:")
		cfg, ok := m.findChipConfigByID(d, id)
		if !ok {
			m.flash("chip not found")
			return nil
		}
		scope := chipScopeFor(m, d)
		return m.openChipScopeChooser("Widen scope · "+cfg.Label, cfg.EffectiveShare(), chipScopeTarget{
			kind:   chipScopeTargetOtherOrg,
			domain: d,
			chipID: id,
			scope:  scope,
		})
	case strings.HasPrefix(pick, "edit:"):
		id := strings.TrimPrefix(pick, "edit:")
		reg := m.registryFor(d)
		if reg == nil {
			return nil
		}
		c, ok := reg.FindByID(id)
		if !ok || c.Origin == qchip.OriginBuiltIn {
			return nil
		}
		return m.openChipWizard(d, c)
	case strings.HasPrefix(pick, "delete:"):
		id := strings.TrimPrefix(pick, "delete:")
		return m.openChipDeleteConfirm(d, id)
	case strings.HasPrefix(pick, "fav:"):
		id := strings.TrimPrefix(pick, "fav:")
		reg := m.registryFor(d)
		if reg == nil {
			return nil
		}
		c, ok := reg.FindByID(id)
		if !ok {
			return nil
		}
		if !reg.SetFavourite(id, !c.Favourite) {
			m.flash(c.Label + " can't be unpinned")
			return nil
		}
		if !c.Favourite {
			m.flash("★ pinned " + c.Label)
		} else {
			m.flash("☆ unpinned " + c.Label)
		}
		if m.settings != nil {
			reg.PersistUser(m.settings)
			m.saveSettings("")
		}
		return m.onTabChanged()
	case strings.HasPrefix(pick, "importpick:"):
		listViewID := strings.TrimPrefix(pick, "importpick:")
		return m.importSalesforceListView(d, listViewID)
	}
	return nil
}

// chipScopeFor returns the right scope value for a new chip on the
// given domain. Records uses the active sObject; the universal
// surfaces use "*".
func chipScopeFor(m *Model, d chipDomain) string {
	if d != domainRecords {
		return "*"
	}
	_, sobj := m.activeRecordsSObject()
	if sobj == "" {
		return "*"
	}
	return sobj
}

// openChipDeleteConfirm pops a yes/no confirm. Same UX as before; one
// implementation now serves all three domains.
func (m *Model) openChipDeleteConfirm(d chipDomain, id string) tea.Cmd {
	reg := m.registryFor(d)
	if reg == nil {
		return nil
	}
	c, ok := reg.FindByID(id)
	if !ok || c.Origin == qchip.OriginBuiltIn {
		return nil
	}
	state := choiceModalState{
		Title: "Delete view",
		Hint:  fmt.Sprintf("Remove %q? This cannot be undone.", c.Label),
		Options: []choiceOption{
			{Label: "Cancel", Value: "cancel", Cancel: true},
			{Label: "Delete", Hint: c.Label, Value: "ok"},
		},
		Cursor: 0,
		Save: func(val any) error {
			if val != "ok" {
				return nil
			}
			if m.settings != nil {
				m.settings.DeleteChip(string(d), id)
			}
			// Drop from runtime registry too — UpsertChip's settings call
			// rebuilt by LoadFromSettings would also work, but reloading
			// is heavier than a direct mutation here.
			user := reg.User()
			out := user[:0]
			for _, x := range user {
				if x.ID != id {
					out = append(out, x)
				}
			}
			reg.SetUser(out)
			if m.settings != nil {
				return m.settings.Save()
			}
			return nil
		},
		SuccessMsg: "chip deleted",
		OnSuccess:  func() tea.Cmd { return m.onTabChanged() },
	}
	return m.openChoiceModal(state)
}

// importSalesforceListView fetches a SF list view's SOQL via
// /listviews/<id>/describe, parses it through query.Parse, and saves
// the result as a chip with origin=imported. Works for any sObject-
// backed surface — records pulls Account list views; flows pulls
// FlowDefinition list views. The parser does the heavy lifting.
func (m *Model) importSalesforceListView(d chipDomain, listViewID string) tea.Cmd {
	d2, sobj := primaryImportTarget(m, d)
	if sobj == "" {
		m.flash("nothing to import — no sObject context for this surface")
		return nil
	}
	// Look up the source name + the list view's actual SobjectType
	// in the cached list. The describe endpoint requires the right
	// sObject per ID — flow imports may sit on either FlowDefinition
	// or FlowDefinitionView, so we trust the row's SobjectType field.
	var sourceName, listViewSObject string
	if r, ok := d2.ListViewsPerSObject[sobj]; ok && !r.FetchedAt().IsZero() {
		for _, lv := range r.Value() {
			if lv.ID == listViewID {
				sourceName = lv.Name
				listViewSObject = lv.SobjectType
				break
			}
		}
	}
	if sourceName == "" {
		m.flash("list view not found in cache — try opening the picker again")
		return nil
	}
	if listViewSObject == "" {
		listViewSObject = sobj
	}
	o, hasOrg := m.currentOrg()
	if !hasOrg {
		return nil
	}
	alias := targetArg(o)
	domain := d
	registry := m.registryFor(domain)
	settingsRef := m.settings
	scope := sobj
	if domain != domainRecords {
		scope = "*"
	}

	// Build the describe fallback chain. The list view's SobjectType
	// is the natural first try, but Salesforce sometimes returns
	// INVALID_TYPE for it (e.g. FlowDefinitionView is queryable as
	// a SobjectType column on ListView but the describe endpoint
	// only accepts FlowDefinition). When that happens, fall through
	// to the domain's other targets.
	_, allTargets := importTargets(m, d)
	describeChain := []string{listViewSObject}
	for _, t := range allTargets {
		if t == listViewSObject {
			continue
		}
		describeChain = append(describeChain, t)
	}

	return func() tea.Msg {
		c, err := sf.RESTClient(alias)
		if err != nil {
			return chipImportDoneMsg{err: err}
		}
		var desc sf.ListViewDescribe
		var lastErr error
		for _, target := range describeChain {
			d2, derr := c.DescribeListView(target, listViewID)
			if derr == nil {
				desc = d2
				lastErr = nil
				break
			}
			lastErr = derr
		}
		if lastErr != nil {
			return chipImportDoneMsg{err: lastErr}
		}
		q, _, perr := query.Parse(desc.Query)
		// perr is non-fatal — Parse returns a best-effort Query even
		// when it rejects an unsupported clause. The user gets a flash
		// alongside the success message.
		newChip := qchip.Chip{
			ID:         importChipID(sobj, sourceName),
			Label:      sourceName,
			Scope:      scope,
			Origin:     qchip.OriginImported,
			OrgUser:    o.Username,
			Query:      q,
			SourceID:   listViewID,
			SourceName: sourceName,
			ImportedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if settingsRef != nil {
			settingsRef.UpsertChip(qchip.ToConfig(newChip, string(domain)))
			if saveErr := settingsRef.Save(); saveErr != nil {
				return chipImportDoneMsg{err: saveErr}
			}
			registry.LoadFromSettings(settingsRef)
		}
		return chipImportDoneMsg{label: sourceName, parseErr: perr}
	}
}

// importTargets returns the candidate sObjects backing a domain's
// list-view import. /records picks the active sObject; /users
// always targets User. /flows is excluded — the modern
// FlowDefinitionView list views can't be imported (Salesforce
// rejects the describe endpoint), so the chip surface's
// ImportFromSF flag is false for /flows and this function never
// gets called for that domain.
//
// Callers query each entry, merge results, and dedupe by ListView Id.
func importTargets(m *Model, d chipDomain) (*orgData, []string) {
	switch d {
	case domainObjects:
		return nil, nil
	case domainRecords:
		dd, sobj := m.activeRecordsSObject()
		if sobj == "" {
			return dd, nil
		}
		return dd, []string{sobj}
	case domainUsers:
		o, ok := m.currentOrg()
		if !ok {
			return nil, nil
		}
		return m.ensureOrgData(o.Username), []string{"User"}
	}
	return nil, nil
}

// primaryImportTarget returns the canonical sObject we cache list-view
// metadata under. For multi-target domains (flows) we use the first
// entry as the cache key; the merged results live there.
func primaryImportTarget(m *Model, d chipDomain) (*orgData, string) {
	dd, targets := importTargets(m, d)
	if len(targets) == 0 {
		return dd, ""
	}
	return dd, targets[0]
}

// chipImportDoneMsg lands on the main loop after the import finishes.
type chipImportDoneMsg struct {
	label    string
	err      error
	parseErr error
}

// chipImportListViewsReadyMsg fires when an auto-load of the list-view
// catalog (kicked off by openChipImportPicker) finishes. Update reopens
// the picker so the user sees the results without any extra keystrokes.
type chipImportListViewsReadyMsg struct {
	Domain chipDomain
}

// chipImportListViewsFetchedMsg lands when the SF list-view auto-load
// goroutine has merged its payload across every candidate sobject.
// Update applies the slice to the Resource on the main goroutine —
// the goroutine MUST NOT touch Resource.Set itself, since that races
// with renders reading .Value(). Apply returns a follow-up
// chipImportListViewsReadyMsg to re-open the picker.
type chipImportListViewsFetchedMsg struct {
	Domain  chipDomain
	Sobject string
	Views   []sf.ListView
}

// applyChipImportListViews is the Update-side handler for
// chipImportListViewsFetchedMsg. Resolves the live Resource on the
// active org and writes the payload — see "async discipline" in
// docs/architecture.md for the rule this enforces.
func (m Model) applyChipImportListViews(msg chipImportListViewsFetchedMsg) (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if r, ok := d.ListViewsPerSObject[msg.Sobject]; ok {
		r.Set(msg.Views)
	}
	return m, func() tea.Msg {
		return chipImportListViewsReadyMsg{Domain: msg.Domain}
	}
}

// applyChipImportDone is the Update-side handler.
func (m Model) applyChipImportDone(msg chipImportDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.flash("import failed: " + msg.err.Error())
		return m, nil
	}
	switch {
	case msg.parseErr != nil:
		m.flash("imported " + msg.label + " (partial: " + msg.parseErr.Error() + ")")
	default:
		m.flash("imported " + msg.label)
	}
	return m, m.onTabChanged()
}

// importChipID builds a stable id for an imported chip from the
// scope + source name. Re-importing the same view overwrites cleanly.
func importChipID(scope, name string) string {
	id := strings.ToLower(scope) + "-" + slugify(name)
	if id == "-" || id == "" {
		id = "imported-" + slugify(name)
	}
	return id
}

// openChipImportPicker opens the list-view picker. When the catalog
// hasn't loaded yet — common on /flows since the user hasn't visited
// /records · Salesforce mode — we fire the fetch transparently and
// re-open the picker once the data lands. Some domains (flows) span
// multiple sObjects, so we query each candidate and merge results.
func (m *Model) openChipImportPicker(d chipDomain) tea.Cmd {
	dd, targets := importTargets(m, d)
	if dd == nil || len(targets) == 0 {
		m.flash("import not supported on this surface")
		return nil
	}
	o, hasOrg := m.currentOrg()
	if !hasOrg {
		return nil
	}
	primary := targets[0]
	r, ok := dd.ListViewsPerSObject[primary]
	if !ok || r.FetchedAt().IsZero() {
		// Auto-fetch and re-open. Query every candidate sObject and
		// merge so the user sees their list views regardless of
		// which underlying object Salesforce parked them on.
		m.flash("loading Salesforce list views…")
		alias := targetArg(o)
		domain := d
		// Touch (lazy-allocate) the Resource on the main goroutine so
		// the apply path in Update can find it. The Fetch closure
		// itself isn't used here — the cache miss path lands via the
		// chipImportListViewsFetchedMsg we emit below, applied via
		// applyChipImportListViews on the Update goroutine.
		_ = dd.EnsureListViews(alias, primary)
		queries := append([]string(nil), targets...)
		return func() tea.Msg {
			seen := map[string]bool{}
			merged := []sf.ListView{}
			var firstErr error
			for _, t := range queries {
				views, err := sf.ListViews(alias, t)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				for _, lv := range views {
					if seen[lv.ID] {
						continue
					}
					seen[lv.ID] = true
					if lv.SobjectType == "" {
						lv.SobjectType = t
					}
					merged = append(merged, lv)
				}
			}
			if len(merged) == 0 && firstErr != nil {
				return chipImportDoneMsg{err: firstErr}
			}
			// Hand the merged payload back to Update; the applier
			// writes it to the Resource on the main goroutine,
			// preventing races against renders that read .Value().
			return chipImportListViewsFetchedMsg{
				Domain:  domain,
				Sobject: primary,
				Views:   merged,
			}
		}
	}
	views := r.Value()
	if len(views) == 0 {
		m.flash("no Salesforce list views found for " + primary)
		return nil
	}
	opts := make([]choiceOption, 0, len(views))
	for _, lv := range views {
		hint := "import as sf-deck chip"
		if lv.SobjectType != "" && lv.SobjectType != primary {
			hint = lv.SobjectType + " · " + hint
		}
		if !lv.IsSoqlCompatible {
			hint = "not SOQL-compatible — cannot import"
		}
		opts = append(opts, choiceOption{
			Label:  lv.Name,
			Hint:   hint,
			Value:  "importpick:" + lv.ID,
			Cancel: !lv.IsSoqlCompatible,
		})
	}
	title := "Import from Salesforce · " + primary
	if len(targets) > 1 {
		title = "Import from Salesforce · " + strings.Join(targets, " + ")
	}
	domainKind := string(d)
	state := choiceModalState{
		Title:      title,
		Hint:       "Pick a list view to copy into a sf-deck chip · / to filter",
		Searchable: true,
		Options:    opts,
		Cursor:     0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: domainKind, pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

// slugify is the same kebab-cased id helper the old lens import used.
// Centralised here so chip ids look the same across surfaces.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// openOtherOrgChipActions is the sub-modal for a row in the manager's
// "chips from your other orgs" section. Three actions, no defaults that
// silently modify the chip:
//
//   - Preview here (session)  → drop an ephemeral preview onto the strip
//     that vanishes on relaunch.
//   - Add to scope…           → run the scope chooser; on commit, the
//     chip is rewritten with the new Share
//     and now permanently visible per its
//     widened scope.
//   - Cancel                  → close, no state change.
func (m *Model) openOtherOrgChipActions(d chipDomain, chipID string) tea.Cmd {
	cfg, ok := m.findChipConfigByID(d, chipID)
	if !ok {
		m.flash("chip not found")
		return nil
	}
	c := qchip.FromConfig(cfg)
	originOrg := chipOriginOrgFromShare(cfg.EffectiveShare())

	opts := []choiceOption{
		{
			Label: "Preview here (session)",
			Hint:  "show on the strip until you relaunch sf-deck",
			Value: "otherpreview:" + chipID,
		},
		{
			Label: "Add to scope…",
			Hint:  "permanently widen this chip's scope (chooser)",
			Value: "otherscope:" + chipID,
		},
		{Label: "Cancel", Cancel: true},
	}
	domainKind := string(d)
	state := choiceModalState{
		Title:   c.Label + " · from " + chipShareFriendlyOrg(*m, originOrg),
		Hint:    "Pick an action  ·  Esc to cancel",
		Options: opts,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: domainKind, pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

// openEphemeralChipActions opens the Save / Dismiss sub-modal for
// an IPC-spawned session-only chip. Save promotes the chip to a
// persisted entry via the same chips.Create service the CLI uses;
// Dismiss drops the entry from m.chipPreviews.
func (m *Model) openEphemeralChipActions(d chipDomain, chipID string) tea.Cmd {
	p, ok := m.findChipPreview(chipID)
	if !ok || p.OriginOrgUser != chipPreviewOriginIPC {
		m.flash("session chip not found")
		return nil
	}
	opts := []choiceOption{
		{
			Label: "Save (promote to persistent)",
			Hint:  "give it a name so it survives restart",
			Value: "ephsave:" + chipID,
		},
		{
			Label: "Dismiss",
			Hint:  "drop this session chip from the strip",
			Value: "ephdismiss:" + chipID,
		},
		{Label: "Cancel", Cancel: true},
	}
	domainKind := string(d)
	state := choiceModalState{
		Title:   chipEphemeralGlyph + " " + p.Chip.Label + " · session",
		Hint:    "Pick an action  ·  Esc to cancel",
		Options: opts,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return chipManagerInvokeMsg{kind: domainKind, pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

// openEphemeralSavePrompt asks the user for a stable id to promote
// the ephemeral chip under. Routes through chips.Create so every
// validation (id shape, collision, column shape) runs through the
// same code path the CLI uses.
func (m *Model) openEphemeralSavePrompt(d chipDomain, chipID string) tea.Cmd {
	p, ok := m.findChipPreview(chipID)
	if !ok || p.OriginOrgUser != chipPreviewOriginIPC {
		m.flash("session chip not found")
		return nil
	}
	if m.settings == nil {
		m.flash("settings unavailable")
		return nil
	}
	domain := p.Domain
	scope := p.Scope
	previewID := chipID
	return m.openEditModal(editModalState{
		Title:       "Save chip · " + p.Chip.Label,
		Hint:        "Enter a chip id (lowercase, kebab-case). Enter to save · Esc to cancel.",
		InitialBody: "",
		Save: func(val string, _ any) error {
			newID := strings.TrimSpace(val)
			if newID == "" {
				return fmt.Errorf("id required")
			}
			cur, ok := (*m).findChipPreview(previewID)
			if !ok {
				return fmt.Errorf("session chip vanished")
			}
			in := chips.CreateInput{
				ID:        newID,
				Domain:    string(domain),
				Scope:     scope,
				Label:     cur.Chip.Label,
				Favourite: true, // user took deliberate save action; default to strip
				Columns:   cur.Columns,
				Limit:     cur.Limit,
				Clauses:   cur.Clauses,
			}
			persist := func() error { m.saveSettings(""); return nil }
			if _, err := chips.Create(m.settings, in, persist); err != nil {
				return err
			}
			(*m).removeChipPreview(domain, scope, previewID)
			(*m).flash("saved as " + newID)
			return nil
		},
		OnSuccess: nil,
	})
}

// findChipConfigByID locates a chip in settings by (domain, id). Used
// for the cross-org preview/widen flows where the chip is NOT in the
// active registry (it belongs to another org's scope).
func (m Model) findChipConfigByID(d chipDomain, id string) (settings.ChipConfig, bool) {
	if m.settings == nil {
		return settings.ChipConfig{}, false
	}
	for _, c := range m.settings.ChipsForDomain(string(d)) {
		if c.ID == id {
			return c, true
		}
	}
	return settings.ChipConfig{}, false
}
