package ui

// "Loaded dev-project" state machine — the persistence + hydration
// glue for the per-org Scope feature. Surfaces (records / objects /
// flows / reports) consult m.activeScope() during render to inject
// the auto-pinned project chip; the user toggles load/unload via the
// `_` key on /dev-projects.

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

func (m *Model) loadDevProject(orgUser, devProjectID string, label string) {
	if orgUser == "" {
		return
	}
	d := m.ensureOrgData(orgUser)
	d.LoadedDevProjectID = devProjectID
	if devProjectID == "" {
		d.LoadedScope = nil
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
		return
	}
	if !scope.Loaded() {
		d.LoadedDevProjectID = ""
		m.settings.SetLoadedDevProjectForOrg(orgUser, "")
		m.saveSettings("")
		return
	}
	d.LoadedScope = scope
}

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
		d.Cursors.Reset(cursorKindReportRow, "__project__")
		m.flash("📁 " + scope.ProjectName)
	} else {
		m.flash("project mode off")
	}
	return m, nil
}

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
