package ui

// "Loaded dev-project" state machine — the persistence + hydration
// glue for the per-org Scope feature. Surfaces (records / objects /
// flows / reports) consult m.activeScope() during render to inject
// the auto-pinned project chip; the user toggles load/unload via the
// `_` key on /dev-projects.
//
// Ownership of state:
//   orgData.LoadedDevProjectID — the persisted choice ("which dev
//                                project is loaded for this org?").
//   orgData.LoadedScope        — the hydrated lookup-friendly snapshot
//                                of the project's items, filtered to
//                                this org.
// settings.toml stores the id, never the items — the items live in
// the SQLite store and are refetched on load / re-hydrate.
//
// When the user K-collects an item into the loaded project, the
// caller should also call m.refreshLoadedScope(d) so subsequent
// renders see the addition without a sf-deck restart.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/orgproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// activeScope returns the current org's loaded Scope, or nil when
// nothing's loaded. nil-safe for surfaces — they can call
// scope.Loaded() without checking the pointer.
func (m Model) activeScope() *orgproject.Scope {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil
	}
	return d.LoadedScope
}

// loadDevProject sets the loaded dev-project id for the given org,
// hydrates a Scope (filtered to that org's items), and persists the
// choice. Empty id unloads (clears state + settings entry).
//
// Errors from the store are logged but not surfaced to the user as
// a hard failure — the load completes with an empty Scope so the
// rest of the UI doesn't get stuck. The user can retry.
func (m *Model) loadDevProject(orgUser, devProjectID string, label string) {
	if orgUser == "" {
		return
	}
	d := m.ensureOrgData(orgUser)
	d.LoadedDevProjectID = devProjectID
	if devProjectID == "" {
		d.LoadedScope = nil
		// Project mode on /reports doesn't make sense without a
		// loaded project — drop the flag so the next /reports paint
		// shows folders again rather than an empty list.
		d.ReportsProjectMode = false
	} else {
		scope, err := orgproject.Hydrate(m.devProjects, devProjectID, orgUser, orgproject.ScopeOptions{
			ProjectName: label,
		})
		if err != nil {
			applog.Error("orgproject.hydrate", map[string]any{
				"err":     err.Error(),
				"project": devProjectID,
				"org":     orgUser,
			})
		}
		d.LoadedScope = scope
	}
	if m.settings != nil {
		m.settings.SetLoadedDevProjectForOrg(orgUser, devProjectID)
		m.saveSettings("")
	}
	// Drop chip cursors back to 0 so when the user returns to a
	// chip-shaped surface they land on the freshly-prepended project
	// chip (now at index 0). On unload (devProjectID == ""), 0 just
	// becomes whatever the first favourite is — also fine.
	m.setObjectsChipIdx(0)
	m.setFlowsChipIdx(0)
	m.setRecordsChipIdx(0)
	m.applySelectedChipMatcher(d)
}

// refreshLoadedScope re-hydrates the active org's Scope. Call after
// a K-collect into the loaded project so the new item appears in
// future renders without restarting sf-deck. Cheap (one SQL query +
// a few maps); safe to call on every collect.
//
// Side-effect: when the collect promoted the project chip from
// "not-visible" (no items of any chip-shaped kind yet) to "visible"
// for some kinds, bump every affected chip-cursor by +1 to keep the
// user on the chip they had selected. Without this the prepend of
// the project chip silently shifts the user from "All" to "📁
// project", which surprises users.
func (m *Model) refreshLoadedScope(d *orgData) {
	if d == nil || d.LoadedDevProjectID == "" {
		return
	}
	if len(m.orgs) == 0 {
		return
	}
	orgUser := m.orgs[m.selected].Username
	name := ""
	if d.LoadedScope != nil {
		name = d.LoadedScope.ProjectName
	}
	prev := d.LoadedScope
	scope, err := orgproject.Hydrate(m.devProjects, d.LoadedDevProjectID, orgUser, orgproject.ScopeOptions{
		ProjectName: name,
	})
	if err != nil {
		applog.Error("orgproject.refresh", map[string]any{"err": err.Error()})
		return
	}
	d.LoadedScope = scope
	m.compensateChipCursorsForPrepend(d, prev, scope)
}

// compensateChipCursorsForPrepend looks at each chip-bearing surface
// and, for any kind where the project chip just transitioned from
// hidden→visible, bumps that surface's chip cursor by +1 so the
// user's previous selection stays under the cursor.
//
// Walks every chipSurface that exposes ScopeCount + a project-chip
// predicate; bumps that surface's cursor by one when its scope just
// transitioned from empty → non-empty. Records is exempt — its chip
// strip is per-sObject and resets on subtab change anyway.
func (m *Model) compensateChipCursorsForPrepend(d *orgData, prev, next *orgproject.Scope) {
	if d == nil || next == nil {
		return
	}
	for _, surf := range allChipSurfaces() {
		if surf.ScopeCount == nil || surf.ApplyProjectChip == nil {
			continue
		}
		prevN := 0
		if prev != nil {
			prevN = surf.ScopeCount(prev)
		}
		if prevN == 0 && surf.ScopeCount(next) > 0 {
			// Previous selection shifts one slot to the right when
			// the project chip prepends. Reading cursor on a value
			// receiver and writing it back is safe because each
			// surface's setter operates on the active org's data.
			surf.SetChipIdx(m, surf.ChipIdx(*m)+1)
		}
	}
	// Re-apply the matcher for the active surface so the cursor's
	// new position (potentially the freshly-prepended project chip)
	// drives a fresh predicate. Without this the cursor visually
	// lands on the project chip but the underlying ListView.Extra
	// still holds whatever matcher the user had before — manifesting
	// as "project chip selected, but list shows All."
	m.applySelectedChipMatcher(d)
}

// hydrateLoadedProjectFromSettings is called once per org per session
// (via ensureOrgData's lazy-init) to pull the persisted loaded-id
// out of settings and build the Scope. Safe when devProjects is nil
// — leaves Scope nil so surfaces just don't render the project chip.
func (m *Model) hydrateLoadedProjectFromSettings(d *orgData, orgUser string) {
	if d == nil || m.settings == nil {
		return
	}
	id := m.settings.LoadedDevProjectForOrg(orgUser)
	if id == "" {
		return
	}
	d.LoadedDevProjectID = id
	if m.devProjects == nil {
		return
	}
	label := loadedProjectLabel(m.devProjects, id)
	scope, err := orgproject.Hydrate(m.devProjects, id, orgUser, orgproject.ScopeOptions{
		ProjectName: label,
	})
	if err != nil {
		applog.Error("orgproject.startup_hydrate", map[string]any{
			"err":     err.Error(),
			"project": id,
			"org":     orgUser,
		})
		// Keep id around so the user can re-load explicitly to
		// clear / repair; only Scope is dropped.
		return
	}
	if !scope.Loaded() {
		// Stale id — project was deleted between sessions. Drop the
		// settings entry so the next render doesn't re-attempt.
		d.LoadedDevProjectID = ""
		m.settings.SetLoadedDevProjectForOrg(orgUser, "")
		m.saveSettings("")
		return
	}
	d.LoadedScope = scope
}

// toggleLoadDevProject is the `_` keypress handler on /dev-projects.
// Loads the cursored project for the active org, or unloads if it
// was already loaded. No-op (with a flash) when nothing's cursored
// or the dev-project store isn't open.
//
// Returns the (Model, tea.Cmd) pair the dispatcher expects.
func (m Model) toggleLoadDevProject() (Model, tea.Cmd) {
	if len(m.orgs) == 0 || m.devProjects == nil {
		m.flash("can't load: no org or store")
		return m, nil
	}
	p, ok := m.devProjectList.Selected()
	if !ok {
		m.flash("no dev project selected")
		return m, nil
	}
	orgUser := m.orgs[m.selected].Username
	d := m.ensureOrgData(orgUser)
	// Toggle: pressing _ on the already-loaded project unloads it.
	if d.LoadedDevProjectID == p.ID {
		m.loadDevProject(orgUser, "", "")
		m.flash("unloaded project")
		return m, nil
	}
	label := p.Name
	m.loadDevProject(orgUser, p.ID, label)
	if label != "" {
		m.flash("loaded: " + label)
	} else {
		m.flash("loaded project")
	}
	return m, nil
}

// toggleReportsProjectMode flips the /reports surface's project-pin
// activation. When ON: the report list shows only the loaded
// project's reports, ignoring the folder breadcrumb. When OFF: the
// strip's "All" / breadcrumb folders take over again. No-op when no
// project is loaded — flashes a hint instead.
func (m Model) toggleReportsProjectMode() (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	scope := m.activeScope()
	if !scope.Loaded() {
		m.flash("no project loaded — _ on /dev-projects to load")
		return m, nil
	}
	d.ReportsProjectMode = !d.ReportsProjectMode
	if d.ReportsProjectMode {
		// Reset the report-row cursor so we don't index past the new
		// (smaller) list.
		d.Cursors.Reset(cursorKindReportRow, "__project__")
		m.flash("📁 " + scope.ProjectName)
	} else {
		m.flash("project mode off")
	}
	return m, nil
}

// projectRecordsChip synthesises a qchip.Chip whose Query selects
// the records currently in the loaded project for the given sObject.
// Returns ok=false when no project is loaded or the project has no
// records for that sObject — caller should not fire a fetch in that
// case (the strip wouldn't show the chip either; this is defensive).
func (m Model) projectRecordsChip(d *orgData, sobject string) (qchip.Chip, bool) {
	scope := m.activeScope()
	if !scope.Loaded() {
		return qchip.Chip{}, false
	}
	ids := scope.RecordIDsFor(sobject)
	if len(ids) == 0 {
		return qchip.Chip{}, false
	}
	values := make([]any, len(ids))
	for i, id := range ids {
		values[i] = id
	}
	return qchip.Chip{
		ID:    projectChipID,
		Label: scope.ProjectName,
		Query: query.Query{
			Where: query.Cmp("Id", query.OpIn, values),
		},
	}, true
}

// visitedRecordsChip synthesises a qchip.Chip whose Query selects
// the records the user has recently visited for the given sObject —
// pulled from the merged sf-deck + Salesforce stream. Same shape
// as projectRecordsChip; the visited-set IDs become a SOQL
// `WHERE Id IN (...)` clause.
//
// Returns ok=false when there are no visited records for this
// sObject, in which case callers don't fire a fetch.
func (m Model) visitedRecordsChip(d *orgData, sobject string, orgUser string) (qchip.Chip, bool) {
	visited := m.recentVisitedRecordIDs(orgUser, sobject)
	if len(visited) == 0 {
		return qchip.Chip{}, false
	}
	values := make([]any, 0, len(visited))
	for id := range visited {
		values = append(values, id)
	}
	return qchip.Chip{
		ID:    recentlyViewedChipID,
		Label: "Recently viewed",
		Query: query.Query{
			Where: query.Cmp("Id", query.OpIn, values),
		},
	}, true
}

// salesforceVisitedRecordsChip is the SF-mode counterpart of
// visitedRecordsChip — produces a chip whose query selects the
// records Salesforce considers recently viewed for this sObject.
// Sources IDs from d.RecentlyViewed (the server-side payload)
// only — does NOT include sf-deck's local visit log — so the
// SF-mode chip reflects exactly what Lightning would show at
// `/lightning/o/<X>/list?filterName=Recent`.
//
// Returns ok=false when SF reports no recently-viewed records for
// this sObject (callers render an empty-state hint instead of
// firing a doomed fetch).
func (m Model) salesforceVisitedRecordsChip(d *orgData, sobject string, orgUser string) (qchip.Chip, bool) {
	visited := m.salesforceVisitedRecordIDs(orgUser, sobject)
	if len(visited) == 0 {
		return qchip.Chip{}, false
	}
	values := make([]any, 0, len(visited))
	for id := range visited {
		values = append(values, id)
	}
	return qchip.Chip{
		ID:    sfRecentlyViewedChipID,
		Label: "Recently Viewed",
		Query: query.Query{
			Where: query.Cmp("Id", query.OpIn, values),
		},
	}, true
}

// loadedProjectLabel resolves the user-visible label for a dev
// project: just the DevProject name. Used both at startup-hydrate
// and load-toggle so the Scope's ProjectName is consistent.
func loadedProjectLabel(store *devproject.Store, devProjectID string) string {
	if store == nil || devProjectID == "" {
		return ""
	}
	dp, err := store.GetDevProject(devProjectID)
	if err != nil || dp == nil {
		return ""
	}
	return dp.Name
}
