package ui

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
// created or imported chip. Empty when no org is selected; callers reject
// the save so the chip cannot cross org boundaries.
func (m Model) activeOrgUserForChips() string {
	if len(m.orgs) == 0 {
		return ""
	}
	return m.orgs[m.selected].Username
}

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

type otherOrgChipRow struct {
	ID            string
	Label         string
	OriginOrgUser string
	Hint          string
	Chip          qchip.Chip
}

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
		id := strings.TrimPrefix(pick, "actions:")
		return m.openChipActionsModal(d, id)
	case strings.HasPrefix(pick, "otherorg:"):
		// Enter on a row from the "other orgs" section opens the
		// preview / widen-scope sub-modal — no destructive defaults.
		id := strings.TrimPrefix(pick, "otherorg:")
		return m.openOtherOrgChipActions(d, id)
	case strings.HasPrefix(pick, "eph:"):
		id := strings.TrimPrefix(pick, "eph:")
		if scope == "" {
			scope = chipScopeFor(m, d)
		}
		return m.applyChipSelection(d, scope, id)
	case strings.HasPrefix(pick, "ephactions:"):
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

func primaryImportTarget(m *Model, d chipDomain) (*orgData, string) {
	dd, targets := importTargets(m, d)
	if len(targets) == 0 {
		return dd, ""
	}
	return dd, targets[0]
}

type chipImportDoneMsg struct {
	label    string
	err      error
	parseErr error
}

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

func importChipID(scope, name string) string {
	id := strings.ToLower(scope) + "-" + slugify(name)
	if id == "-" || id == "" {
		id = "imported-" + slugify(name)
	}
	return id
}

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
		m.flash("loading Salesforce list views…")
		alias := targetArg(o)
		domain := d
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
