package ui

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m *Model) reloadDevProjects() {
	if m.devProjects == nil {
		m.devProjectList.Set(nil)
		return
	}
	list, err := m.devProjects.ListDevProjects()
	if err != nil {
		m.flash("dev projects: " + err.Error())
		m.devProjectList.Set(nil)
		return
	}
	m.devProjectList.Set(list)
}

// setActiveDevProject pins the given DevProject id as the drilled-in
// project and clears per-project state that wouldn't make sense
// after a switch. Kind-filter chip resets to "All" because a chip
// stored from project A is meaningless against project B's item set.
//
// Doesn't load items — call reloadDevProjectItems() (or trigger a
// devProjectsChangedMsg) after this so the panel paints with fresh
// data.
func (m *Model) setActiveDevProject(id string) {
	m.devProjectCur = id
	m.devProjectKindChip = ""
	m.devProjectKindChipCursor = 0
}

// devProjectItemsView returns a read-only snapshot of the active
// org's loaded items. Returns empty when no org is active or the
// org has no data. Caller MUST NOT mutate the returned slice.
func (m Model) devProjectItemsView() []devproject.Item {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	return d.DevProjectItems.Items()
}

func (m *Model) reloadDevProjectItems() {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if m.devProjects == nil || m.devProjectCur == "" {
		d.DevProjectItems.Set(nil)
		return
	}
	orgFilter := ""
	if !m.devProjectShowAllOrgs {
		orgFilter = m.orgs[m.selected].Username
	}
	items, err := m.devProjects.ListItems(m.devProjectCur, orgFilter)
	if err != nil {
		m.flash("dev project: " + err.Error())
		d.DevProjectItems.Set(nil)
		return
	}
	// Install the search machinery once per org — lazy idempotent so
	// re-installing on each load is cheap.
	if !d.DevProjectItems.HasMatch() {
		installSearch(&d.DevProjectItems, uilayout.MatchSpec[devproject.Item]{
			Any: func(it devproject.Item) string {
				return strings.ToLower(it.Name + " " + it.Ref + " " + it.Type)
			},
			Field: func(it devproject.Item, field string) string {
				v, _ := it.Field(field)
				if s, ok := v.(string); ok {
					return strings.ToLower(s)
				}
				return ""
			},
			Fields:  []string{"Name", "Ref", "Type", "Kind"},
			Primary: "Name",
		})
	}
	d.DevProjectItems.Set(items)
}

func (m Model) devProjectByID(id string) (devproject.DevProject, bool) {
	for _, p := range m.devProjectList.Items() {
		if p.ID == id {
			return p, true
		}
	}
	return devproject.DevProject{}, false
}

// newID generates a short hex ID for new dev projects. Doesn't need
// to be UUID-grade — local-only DB, ~16 bits of randomness is plenty
// to avoid collision over a user's lifetime.
func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (m *Model) openItemForOrigin(it devproject.Item, origin Tab) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	if it.OrgUser != "" && it.OrgUser != m.orgs[m.selected].Username {
		for i, o := range m.orgs {
			if o.Username == it.OrgUser {
				m.setSelectedOrg(i)
				break
			}
		}
	}
	if cmd, ok := drillByKind(m, string(it.Kind), it.Ref, it.Type, it.Name, origin); ok {
		return cmd
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	// Devproject-only fall-through: KindSOQLQuery loads the saved
	// query body into the editor.  Lives here (not in drillByKind)
	// because it needs the devProjects store + soql editor state
	// that the dispatcher doesn't want as a dependency.
	switch it.Kind {
	case devproject.KindSOQLQuery:
		if m.devProjects == nil {
			return nil
		}
		q, err := m.devProjects.GetSavedQuery(it.Ref)
		if err != nil {
			m.flash("saved query missing — may have been deleted")
			return nil
		}
		_ = m.devProjects.TouchSavedQuery(q.ID)
		resolved := substituteSOQL(q.Body, m.substitutionsFor(d))
		m.soqlInput.SetValue(resolved)
		m.soqlSubtabIdx = 0
		m.soqlEditing = true
		m.soqlInput.Focus()
		m.invalidateSOQLSaved()
		m.soqlEditingSavedID = q.ID
		m.setTab(TabSOQL)
		return m.onTabChanged()
	}
	m.flash(notDrillableMessage(it.Kind))
	return nil
}

func notDrillableMessage(kind devproject.ItemKind) string {
	switch kind {
	case devproject.KindApexSnippet:
		return "apex snippets aren't drillable — open via /apex"
	}
	return "no detail view for " + string(kind) + " yet"
}

func indexOfRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}
