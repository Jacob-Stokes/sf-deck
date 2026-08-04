package ui

// Helpers connecting the UI to the devproject store. Refreshes the
// in-memory ListView wrappers from disk so the renderers can stay
// dumb (read whatever's in the wrapper). All work is best-effort —
// store errors get flashed but don't block UI.

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// reloadDevProjects pulls every dev project off the store and pushes
// it into the model's ListView. Called on entry to TabDevProjects,
// after any mutation, and on the left-rail panel render.
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

// reloadDevProjectItems loads the items for the drilled-in dev
// project (TabDevProjectDetail). orgUser="" returns items from
// every org; pass an org username to filter to that org's
// contributions only — the detail view defaults to the active org
// but offers a toggle to see "all orgs" too.
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

// devProjectByID looks up a dev project in the cached list. Used by
// the renderers and by /dev-projects drill to render the header
// without a per-paint store hit.
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

// openItemForOrigin routes a devproject.Item to its canonical detail
// tab. The Kind tells us which tab; Ref + Type carry the (sobject,
// id) pair where relevant. origin is the tab the user came from so
// Esc-back lands on the right surface (TabDevProjectDetail for the
// /dev-projects drill, TabTagDetail for the /tags drill, etc.).
//
// If the item came from a different org than the active one, switches
// the active org first so the detail tab opens in the right context.
func (m *Model) openItemForOrigin(it devproject.Item, origin Tab) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	// Switch to the item's origin org if we're not already there.
	if it.OrgUser != "" && it.OrgUser != m.orgs[m.selected].Username {
		for i, o := range m.orgs {
			if o.Username == it.OrgUser {
				m.setSelectedOrg(i)
				break
			}
		}
	}
	// Try the shared per-kind dispatcher first.  Handles every kind
	// that has a regular detail tab; falls through to the
	// devproject-specific SOQL/Apex snippet cases below for the few
	// kinds that aren't drillable from /home Recent.  See
	// docs/recent-kinds-drill-audit.md.
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
		// Pinned query: load body into the editor, jump to /soql,
		// land on the Editor subtab. Touch updates "most recent."
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
	// Unhandled item kind. Flash an explanation rather than silently
	// no-op so the user knows the keystroke landed but the kind
	// doesn't have a drill destination yet. Apex snippets are the
	// known fall-through today.
	m.flash(notDrillableMessage(it.Kind))
	return nil
}

// notDrillableMessage renders the user-facing flash when a drill
// fires on a kind that has no detail tab. Keep messages short;
// the status bar truncates long text.
func notDrillableMessage(kind devproject.ItemKind) string {
	switch kind {
	case devproject.KindApexSnippet:
		return "apex snippets aren't drillable — open via /apex"
	}
	return "no detail view for " + string(kind) + " yet"
}

// indexOfRune is a minimal strings.IndexByte for the "<sobject>.<field>"
// split — kept inline so we don't pull strings into this file just for
// one call.
func indexOfRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}
