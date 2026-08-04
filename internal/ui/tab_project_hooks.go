package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) ensureDevProjectsData(_ *orgData, _ sf.Org) tea.Cmd {
	m.reloadDevProjects()
	return nil
}

// activateDevProjects drills into the cursored DevProject. Loads
// items filtered to the active org (the default detail view).
func (m *Model) activateDevProjects() tea.Cmd {
	p, ok := m.devProjectList.Selected()
	if !ok {
		return nil
	}
	m.setActiveDevProject(p.ID)
	if s := m.devProjectList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	// Reset scope to "this org" on entry — Tab toggles to "all orgs"
	// after the user has had a chance to read what's visible by
	// default. Stops the user from being surprised by other-org items
	// the first time they open a project.
	m.devProjectShowAllOrgs = false
	m.reloadDevProjectItems()
	m.setTab(TabDevProjectDetail)
	return nil
}

func (m Model) devProjectsSearchPtr() *searchState { return m.devProjectList.SearchPtr() }

func (m *Model) moveDevProjectsCursor(delta int) { m.devProjectList.MoveBy(delta) }

func (m *Model) resetDevProjectsCursor() { m.devProjectList.ResetCursor() }

// moveDevProjectDetailCursor walks the Items subtab list-table on
// TabDevProjectDetail. Routes through d.DevProjectItems.MoveBy so
// the cursor stays in sync with the underlying ListView the list
// surface uses for rendering.
func (m *Model) moveDevProjectDetailCursor(delta int) {
	if len(m.orgs) == 0 || m.devProjectCur == "" {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.DevProjectItems.MoveBy(delta)
}

func (m *Model) ensureDevProjectDetailData(_ *orgData, _ sf.Org) tea.Cmd {
	// Auto-reconcile the drilled-in project on nav-in: merge duplicate
	// refs (e.g. a flow stored under its DeveloperName vs DefinitionId)
	// and drop items whose resource is confirmed missing in its org.
	// No-op when nothing needs fixing; reloads the views itself.
	m.reconcileDevProject(m.devProjectCur)
	m.reloadDevProjects()
	m.reloadDevProjectItems()
	return nil
}

// activateDevProjectDetail handles Enter on TabDevProjectDetail.
// On a parent row: toggle expand/collapse. On any other row: open
// the cursored item via openItemFromDevProject.
func (m *Model) activateDevProjectDetail() tea.Cmd {
	if m.devProjectCur == "" || len(m.devProjectItemsView()) == 0 {
		return nil
	}
	row, _, ok := m.rowAtCursor()
	if !ok {
		return nil
	}
	// Flat list table now — no parent rows / org headers to special-
	// case. Every selectable row drills into its item.
	return m.openItemForOrigin(row.Item, TabDevProjectDetail)
}

func (m *Model) ensureProjectsData(_ *orgData, _ sf.Org) tea.Cmd {
	return m.projectsRes.Ensure(m.cache)
}

func (m Model) refreshProjectsData(_ *orgData) tea.Cmd {
	return m.projectsRes.Refresh(m.cache)
}

func (m Model) projectsSearchPtr() *searchState { return m.projectList.SearchPtr() }

func (m *Model) moveProjectsCursor(delta int) { m.projectList.MoveBy(delta) }

func (m *Model) resetProjectsCursor() { m.projectList.ResetCursor() }
