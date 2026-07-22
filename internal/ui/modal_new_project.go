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

// triggerNewProject is the entry point for `n` on /dev-projects (or
// the rail's Dev Projects panel). One-step modal: name + optional
// description.
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

// openNewDevProjectModal asks for the DevProject's name + description.
// First line of the editor buffer is the name; rest is description.
// On save, creates the row and refreshes the list view.
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

// projectsContext reports whether the active surface is the dev-
// projects tab or the rail's Dev Projects panel — gates the n / e /
// d / shift+D dispatch so they don't fire on unrelated tabs.
func projectsContext(m Model) bool {
	if m.tab() == TabDevProjects {
		return true
	}
	return railDevProjectsActive(m)
}

// railDevProjectsActive returns true when keys should treat the
// cursor as if it were on /dev-projects — used by triggers (new /
// edit / delete) to disambiguate the "no tab is /dev-projects but
// the rail is" case.
func railDevProjectsActive(m Model) bool {
	return m.focus == focusOrgs && m.currentUtility().ID == utilityBookmarks
}

// triggerEditProject opens the rename-and-description editor for the
// cursored DevProject. The buffer is multiline: name on line 1,
// description on line 2+.
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
	// Sweep every org's loaded-project state and clear any reference
	// to the just-deleted DevProject so chips don't render against
	// dead ids.
	for user, d := range m.data {
		if d != nil && d.LoadedDevProjectID == dp.ID {
			m.loadDevProject(user, "", "")
		}
	}
	m.flash("deleted " + dp.Name)
	return func() tea.Msg { return devProjectsChangedMsg{} }
}

// triggerDeleteProjectItem removes the cursored row from the active
// DevProject's items (TabDevProjectDetail). Items are removed by
// (DevID, OrgUser, Kind, Ref) so the same kind+ref collected from
// two different orgs are independent rows.
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
	// Kind-chip filter could have just lost its last item. The
	// reloadDevProjectItems triggered by devProjectsChangedMsg will
	// drive devProjectKindChips() to recompute the strip — but the
	// stored devProjectKindChip kind would still filter on the
	// vanished kind, leaving a "no items in this view" panel that
	// surprises the user. Snap back to "All" when the active kind
	// matches the row that just got removed.
	if m.devProjectKindChip != "" && it.Kind == m.devProjectKindChip {
		m.devProjectKindChip = ""
		m.devProjectKindChipCursor = 0
	}
	return func() tea.Msg { return devProjectsChangedMsg{} }
}

// splitNameDescription parses the multiline editor buffer used by
// triggerEditProject. First line is the name; remaining lines (joined
// back with "\n") are the description. Trailing whitespace per-segment
// is trimmed.
func splitNameDescription(s string) (name, desc string) {
	parts := strings.SplitN(s, "\n", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		desc = strings.TrimSpace(parts[1])
	}
	return name, desc
}

// devProjectsChangedMsg is the synthetic message that re-loads the
// dev-project list view after a mutation. Update folds it back into
// the model.
type devProjectsChangedMsg struct{}
