package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) ensureDevProjectsData(_ *orgData, _ sf.Org) tea.Cmd {
	m.reloadDevProjects()
	return nil
}

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
	m.devProjectShowAllOrgs = false
	m.reloadDevProjectItems()
	m.setTab(TabDevProjectDetail)
	return nil
}

func (m Model) devProjectsSearchPtr() *searchState { return m.devProjectList.SearchPtr() }

func (m *Model) moveDevProjectsCursor(delta int) { m.devProjectList.MoveBy(delta) }

func (m *Model) resetDevProjectsCursor() { m.devProjectList.ResetCursor() }

func (m *Model) moveDevProjectDetailCursor(delta int) {
	if len(m.orgs) == 0 || m.devProjectCur == "" {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.DevProjectItems.MoveBy(delta)
}

func (m *Model) ensureDevProjectDetailData(_ *orgData, _ sf.Org) tea.Cmd {
	m.reconcileDevProject(m.devProjectCur)
	m.reloadDevProjects()
	m.reloadDevProjectItems()
	return nil
}

func (m *Model) activateDevProjectDetail() tea.Cmd {
	if m.devProjectCur == "" || len(m.devProjectItemsView()) == 0 {
		return nil
	}
	row, _, ok := m.rowAtCursor()
	if !ok {
		return nil
	}
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
