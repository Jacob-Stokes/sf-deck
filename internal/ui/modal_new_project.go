package ui

// "New project" / edit / delete / remove-item modals for the
// flattened DevProject layout. The pre-flatten variant chained two
// modals (DevProject + OrgProject) and surfaced a Reparent flow
// because OrgProjects could move umbrella; with a single tier those
// flows disappear entirely.

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func (m *Model) triggerNewProject() tea.Cmd {
	if m.devProjects == nil {
		m.flash("dev-projects unavailable")
		return nil
	}
	if !projectsContext(*m) {
		return nil
	}
	return m.openNewDevProjectModal()
}

func (m *Model) openNewDevProjectModal() tea.Cmd {
	return m.openEditModal(editModalState{
		Title:       "New dev project",
		Hint:        "Line 1 = name. Line 2+ = description. Enter to save · Esc to cancel.",
		InitialBody: "",
		Multiline:   true,
		Save: func(val string, _ any) error {
			name, desc := splitNameDescription(val)
			if name == "" {
				return fmt.Errorf("name required")
			}
			p := devproject.DevProject{
				ID:          newID(),
				Name:        name,
				Description: desc,
			}
			return m.devProjects.CreateDevProject(p)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return devProjectsChangedMsg{} }
		},
	})
}

func projectsContext(m Model) bool {
	if m.tab() == TabDevProjects {
		return true
	}
	return railDevProjectsActive(m)
}

func railDevProjectsActive(m Model) bool {
	return m.focus == focusOrgs && m.currentUtility().ID == utilityBookmarks
}

func (m *Model) triggerEditProject() tea.Cmd {
	if m.devProjects == nil || !projectsContext(*m) {
		return nil
	}
	dp, ok := m.devProjectList.Selected()
	if !ok {
		m.flash("no project selected")
		return nil
	}
	initial := dp.Name
	if dp.Description != "" {
		initial += "\n" + dp.Description
	}
	return m.openEditModal(editModalState{
		Title:       "Edit dev project",
		Hint:        "First line is the name; the rest is description. Enter to save.",
		InitialBody: initial,
		Multiline:   true,
		Save: func(val string, _ any) error {
			name, desc := splitNameDescription(val)
			if name == "" {
				return fmt.Errorf("name required")
			}
			return m.devProjects.UpdateDevProject(dp.ID, name, desc)
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg { return devProjectsChangedMsg{} }
		},
	})
}

// triggerDeleteProject removes the cursored DevProject. force=false
// refuses non-empty projects (returns ErrNotEmpty); force=true
// (shift+D) cascades items. Auto-unloads the loaded project on every
// org if it's the one being deleted.
func (m *Model) triggerDeleteProject(force bool) tea.Cmd {
	if m.devProjects == nil || !projectsContext(*m) {
		return nil
	}
	dp, ok := m.devProjectList.Selected()
	if !ok {
		m.flash("no project selected")
		return nil
	}
	if err := m.devProjects.DeleteDevProject(dp.ID, force); err != nil {
		if errors.Is(err, devproject.ErrNotEmpty) {
			m.flash("not empty — " + firstPretty(Keys.DeleteProjectForce) + " to force-delete with cascade")
			return nil
		}
		m.flash("delete: " + err.Error())
		return nil
	}
	for user, d := range m.data {
		if d != nil && d.LoadedDevProjectID == dp.ID {
			m.loadDevProject(user, "", "")
		}
	}
	m.flash("deleted " + dp.Name)
	return func() tea.Msg { return devProjectsChangedMsg{} }
}

func (m *Model) triggerDeleteProjectItem() tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	if m.tab() != TabDevProjectDetail {
		return nil
	}
	if m.devProjectCur == "" || len(m.devProjectItemsView()) == 0 {
		m.flash("nothing to remove")
		return nil
	}
	row, _, ok := m.rowAtCursor()
	if !ok {
		m.flash("nothing to remove")
		return nil
	}
	it := row.Item
	if err := m.devProjects.RemoveItem(it.DevProjectID, it.OrgUser, it.Kind, it.Ref); err != nil {
		m.flash("remove: " + err.Error())
		return nil
	}
	label := it.Name
	if label == "" {
		label = it.Ref
	}
	m.flash("removed " + label + " — ctrl+k to add it back")
	if m.devProjectKindChip != "" && it.Kind == m.devProjectKindChip {
		m.devProjectKindChip = ""
		m.devProjectKindChipCursor = 0
	}
	return func() tea.Msg { return devProjectsChangedMsg{} }
}

func splitNameDescription(s string) (name, desc string) {
	parts := strings.SplitN(s, "\n", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		desc = strings.TrimSpace(parts[1])
	}
	return name, desc
}

type devProjectsChangedMsg struct{}
