package ui

// Leader-key chords. Pressing q (when NOT typing in an input) enters
// "chord mode": the next letter fires a q-<letter> chord and exits.
// Pressing q again — or esc — cancels. While active, the status bar
// shows a CHORD alert with the available next letters, so the whole
// namespace is discoverable without a cheat sheet.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"

	"strings"

	"github.com/charmbracelet/x/ansi"
)

type chordSpec struct {
	Letter    string // the second key, e.g. "s"
	Label     string // short hint shown in the CHORD alert ("s sort by modified")
	Available func(m Model) bool
	Do        func(m Model) (Model, tea.Cmd)
}

var (
	sortColsModified = []string{"Modified", "LastModifiedDate", "SystemModstamp"}
	sortColsCreated  = []string{"Created", "CreatedDate"}
	sortColsName     = []string{"Name", "Label", "MasterLabel", "DeveloperName"}
	sortColsModBy    = []string{"ModifiedBy", "LastModifiedBy", "LastModifiedByName", "CreatedBy"}
	sortColsStatus   = []string{"Status", "Valid", "Type"}
	sortColsSize     = []string{"Size", "LengthWithoutComments", "BodyLength"}
)

func chordRegistry() []chordSpec {
	modified := semanticSortChord("s", "Last Modified", sortColsModified, true)
	modified.Do = func(m Model) (Model, tea.Cmd) {
		name := m.firstSortableColumn(sortColsModified)
		if name == "" {
			m.flash("no Last Modified column in this view")
			return m, nil
		}
		return m.sortByColumnNameDir(name, m.settings.ChordSortModifiedDesc())
	}
	return []chordSpec{
		modified,
		semanticSortChord("c", "Created date", sortColsCreated, true),
		semanticSortChord("l", "Name", sortColsName, false),
		semanticSortChord("b", "Modified by", sortColsModBy, false),
		semanticSortChord("t", "Status", sortColsStatus, false),
		semanticSortChord("z", "Size", sortColsSize, true),
		noteChord(),
		resetViewChord(),
		allViewChord(),
		clearRecentChord(),
		yankListTableChord(),
		yankIDListChord(),
		homeSubtabChord("1", "Landing", SubtabHomeLanding),
		homeSubtabChord("2", "Recently Viewed", SubtabHomeRecent),
		homeSubtabChord("3", "Notifications", SubtabHomeNotifications),
		homeSubtabChord("4", "Limits", SubtabHomeLimits),
		homeSubtabChord("5", "Licenses", SubtabHomeLicenses),
	}
}

func homeSubtabChord(letter, label string, sub Subtab) chordSpec {
	return chordSpec{
		Letter: letter,
		Label:  letter + " " + label,
		Do: func(m Model) (Model, tea.Cmd) {
			return m, (&m).jumpToHomeSubtab(sub)
		},
	}
}

func (m Model) allChipIndex() (surf *chipSurface, idx int) {
	s := m.resolveChipSurface()
	if s == nil || len(m.orgs) == 0 {
		return nil, -1
	}
	reg := s.Registry(&m)
	if reg == nil {
		return nil, -1
	}
	strip := m.stripRows(domainFromRegistry(m, reg), "*")
	for i, c := range strip {
		if c.ID == "all" {
			return s, i
		}
	}
	return nil, -1
}

func allViewChord() chordSpec {
	return chordSpec{
		Letter:    "a",
		Label:     "a All view",
		Available: func(m Model) bool { _, idx := m.allChipIndex(); return idx >= 0 },
		Do: func(m Model) (Model, tea.Cmd) {
			surf, idx := m.allChipIndex()
			if surf == nil || idx < 0 {
				m.flash("no All view here")
				return m, nil
			}
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			surf.SetChipIdx(&m, idx)
			surf.ResetList(d)
			m.applySelectedChipMatcher(d)
			return m, m.ensureDataFor(m.tab())
		},
	}
}

func (m Model) viewStateDirty() bool {
	if st := (&m).activeListTableState(); st != nil && st.SortColumn != "" {
		return true
	}
	if s := (&m).currentSearch(); s != nil && s.Applied() {
		return true
	}
	return false
}

func resetViewChord() chordSpec {
	return chordSpec{
		Letter:    "x",
		Label:     "x reset view (sort + search)",
		Available: func(m Model) bool { return m.viewStateDirty() },
		Do: func(m Model) (Model, tea.Cmd) {
			mm := &m
			cleared := false
			if st := mm.activeListTableState(); st != nil && st.SortColumn != "" {
				st.SortColumn = ""
				st.SortDesc = false
				cleared = true
			}
			if mm.clearCommittedSearch() {
				cleared = true
			}
			if cleared {
				mm.resetCursorForCurrentView()
				mm.flash("view reset")
			}
			return *mm, nil
		},
	}
}

func (m Model) currentSurfaceChipID() string {
	s := m.resolveChipSurface()
	if s == nil || len(m.orgs) == 0 {
		return ""
	}
	return m.activeChipID(s.Domain, "*")
}

func (m Model) activeViewShowsLocalRecent() bool {
	if len(m.orgs) == 0 {
		return false
	}
	if m.tab() == TabHome && m.currentSubtab() == SubtabHomeRecent {
		if d := m.data[m.orgs[m.selected].Username]; d != nil {
			return d.HomeRecentMode == ChipModeLocal
		}
		return false
	}
	return m.currentSurfaceChipID() == recentlyViewedChipID
}

// clearRecentChord (q-r) wipes sf-deck's LOCAL recently-viewed log for
// the active org — the visit history sf-deck records itself, NOT
// Salesforce's server-side RecentlyViewed (which sf-deck can't clear).
// Available only on views that display the local log. On chip surfaces
// the "Recently viewed" lens re-filters immediately; entries sourced
// from Salesforce's RecentlyViewed union remain, by design.
func clearRecentChord() chordSpec {
	return chordSpec{
		Letter:    "r",
		Label:     "r clear recently viewed (sf-deck log)",
		Available: func(m Model) bool { return m.activeViewShowsLocalRecent() },
		Do: func(m Model) (Model, tea.Cmd) {
			if len(m.orgs) == 0 {
				return m, nil
			}
			orgUser := m.orgs[m.selected].Username
			d := m.ensureOrgData(orgUser)
			n := len(d.Recent)
			if n == 0 {
				m.flash("recently viewed is already empty")
				return m, nil
			}
			d.Recent = nil
			d.recentGen++
			d.RecentList.Set(nil)
			persistRecent(&m, orgUser, nil)
			if s := m.resolveChipSurface(); s != nil {
				if reset := s.ResetList; reset != nil {
					reset(d)
				}
				m.applySelectedChipMatcher(d)
			}
			m.flash(fmt.Sprintf("cleared %d recently-viewed entries (sf-deck log)", n))
			return m, m.ensureDataFor(m.tab())
		},
	}
}

func (m *Model) jumpToHomeSubtab(id Subtab) tea.Cmd {
	idx := 0
	for i, s := range homeSubtabs() {
		if s.ID == id {
			idx = i
			break
		}
	}
	m.setTab(TabHome)
	m.setHomeSubtab(idx)
	m.focus = focusMain
	return m.onTabChanged()
}

func semanticSortChord(letter, label string, cols []string, descFirst bool) chordSpec {
	return chordSpec{
		Letter:    letter,
		Label:     letter + " sort by " + label,
		Available: func(m Model) bool { return m.firstSortableColumn(cols) != "" },
		Do: func(m Model) (Model, tea.Cmd) {
			name := m.firstSortableColumn(cols)
			if name == "" {
				m.flash("no " + label + " column in this view")
				return m, nil
			}
			return m.sortByColumnNameDir(name, descFirst)
		},
	}
}

func (m Model) enterChordMode() (Model, tea.Cmd) {
	m.chordActive = true
	return m, nil
}

func (m Model) handleChordKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.chordActive = false // every path exits chord mode
	key := msg.String()
	switch key {
	case "q":
		(&m).showInfoModal(m.chordListModal())
		return m, nil
	case "esc", "ctrl+c":
		return m, nil // cancelled
	}
	for _, c := range chordRegistry() {
		if c.Letter != key {
			continue
		}
		if c.Available != nil && !c.Available(m) {
			m.flash("q-" + key + " isn't available here")
			return m, nil
		}
		return c.Do(m)
	}
	m.flash("no chord q-" + key)
	return m, nil
}

func (m Model) availableChords() []chordSpec {
	var out []chordSpec
	for _, c := range chordRegistry() {
		if c.Available == nil || c.Available(m) {
			out = append(out, c)
		}
	}
	return out
}

// chordDescription strips the leading "<letter> " from a chord's Label
// so the cheat-sheet can show the key in its own column. Labels are
// authored as "<letter> <description>" (e.g. "s sort by Last Modified").
func chordDescription(c chordSpec) string {
	return strings.TrimPrefix(c.Label, c.Letter+" ")
}

// chordListModal builds the read-only "all chords" cheat-sheet opened by
// q-q. Every registered chord is listed with its key; chords not
// available on the current surface are dimmed with a "(n/a here)" note
// so the user sees the full set but knows what fires right now.
func (m Model) chordListModal() infoModalState {
	rows := []infoRow{
		{Body: "Press q then the key. Esc cancels the leader."},
		{Body: ""},
	}
	for _, c := range chordRegistry() {
		desc := chordDescription(c)
		if c.Available != nil && !c.Available(m) {
			desc += "  (n/a here)"
		}
		rows = append(rows, infoRow{Label: "q " + c.Letter, Body: desc})
	}
	return infoModalState{
		Title: "Chords (q + key)",
		Rows:  rows,
	}
}

func (m Model) renderChordBar() string {
	badge := lipgloss.NewStyle().
		Foreground(theme.Bg).Background(theme.Magenta).Bold(true).
		Render(" CHORD ")

	chords := m.availableChords()
	var hints []string
	for _, c := range chords {
		hints = append(hints, c.Label)
	}
	body := "no chords on this surface"
	if len(hints) > 0 {
		body = strings.Join(hints, "  ·  ")
	}

	sep := lipgloss.NewStyle().Foreground(theme.FgDim)
	content := " " + badge + "  " +
		lipgloss.NewStyle().Foreground(theme.Fg).Render(body) + "  " +
		sep.Render("· q all chords · esc cancel") + " "

	if lipgloss.Width(content) > m.width {
		content = ansi.Truncate(content, m.width, "…")
	}
	return lipgloss.NewStyle().
		Background(theme.Panel).
		Width(m.width).
		MaxHeight(1).
		Render(content)
}

func (m Model) firstSortableColumn(candidates []string) string {
	_, cols := (&m).activeListTable()
	for _, want := range candidates {
		for _, c := range cols {
			if c.Name == want && !c.Unsortable {
				return c.Name
			}
		}
	}
	return ""
}
