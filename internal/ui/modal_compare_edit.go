package ui

// compareEditModal — a focused modal for editing/cloning a SAVED
// comparison before re-running it. Mirrors the New setup form (source /
// target / scope / method) plus an overwrite-vs-clone toggle, but lives
// as transient modal state rather than on the persistent per-org run.
//
// Why a modal, not the in-subtab setup form: editing a saved comparison
// in-place on the New subtab leaked origin-link state — leave + return
// and the "Editing" row lingered, because d.Run persisted with no
// owner of "am I still editing a saved one?". A modal owns that state
// for its lifetime and disposes of it on close, so the leak is
// impossible by construction. The New subtab reverts to purely
// "compose fresh / show active result".
//
// On Compare: the modal seeds d.Run (source/target/scope/method +
// OriginSavedID + SaveAsNew), closes, and starts the comparison — the
// user lands on the New subtab watching progress → inventory, clean.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// compareEditRow identifies one navigable row of the edit modal.
type compareEditRow int

const (
	editRowMode compareEditRow = iota // overwrite / clone toggle
	editRowSource
	editRowTarget
	editRowScope
	editRowMethod
	editRowCompare
)

func compareEditRows() []compareEditRow {
	return []compareEditRow{editRowMode, editRowSource, editRowTarget, editRowScope, editRowMethod, editRowCompare}
}

// compareEditModalState is the live modal. It holds an editable copy of
// the comparison config; nothing persists until Compare.
type compareEditModalState struct {
	OriginID   string // saved-comparison id being edited
	OriginName string
	SaveAsNew  bool // false = overwrite origin, true = clone → new

	Source endpoint
	Target endpoint
	Scope  []string
	Method compareMethod

	Cursor int
	Err    string
}

// openCompareEditModal opens the modal seeded from a saved comparison.
func (m *Model) openCompareEditModal(sc compareEditSeed) {
	m.compareEdit = &compareEditModalState{
		OriginID:   sc.OriginID,
		OriginName: sc.OriginName,
		SaveAsNew:  false,
		Source:     orgEndpoint(sc.Source),
		Target:     orgEndpoint(sc.Target),
		Scope:      sc.Scope,
		Method:     sc.Method,
		Cursor:     editRowCompareIndex(), // land on Compare
	}
}

// compareEditSeed is the config the modal opens with.
type compareEditSeed struct {
	OriginID   string
	OriginName string
	Source     string
	Target     string
	Scope      []string
	Method     compareMethod
}

func editRowCompareIndex() int {
	rows := compareEditRows()
	return len(rows) - 1
}

// --- render ---------------------------------------------------------------

func (m Model) renderCompareEditModal() string {
	st := m.compareEdit
	if st == nil {
		return ""
	}
	w := modalWidth(m.width, 60, 96)
	inner := w - 4

	title := "Edit comparison"
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(title))
	lines = append(lines, theme.Subtle.Render("based on saved '"+st.OriginName+"'"))
	lines = append(lines, strings.Repeat("─", inner))

	rowVal := map[compareEditRow]struct{ label, val string }{
		editRowMode:   {"Save", m.compareEditModeValue(st)},
		editRowSource: {"Source", endpointDisplay(m, st.Source)},
		editRowTarget: {"Target", endpointDisplay(m, st.Target)},
		editRowScope:  {"Scope", scopeLabelOrNone(st.Scope)},
		editRowMethod: {"Method", st.Method.String() + "  " + theme.Subtle.Render(methodHint(st.Method))},
	}
	rows := compareEditRows()
	for i, rk := range rows {
		if rk == editRowCompare {
			continue
		}
		r := rowVal[rk]
		prefix := "   "
		labelStyle := theme.Subtle
		if i == st.Cursor {
			prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" ▌ ")
			labelStyle = lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
		}
		lines = append(lines, prefix+labelStyle.Render(padRight(r.label, 8))+"▸  "+r.val)
	}
	lines = append(lines, "")

	onCompare := st.Cursor == len(rows)-1
	act := theme.Subtle.Render("  Compare")
	if onCompare {
		act = lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("❮ Compare ❯")
	}
	lines = append(lines, "     "+act)
	if st.Err != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("  "+st.Err))
	}
	lines = append(lines, "")
	lines = append(lines, theme.Subtle.Render("  ↑↓ move · enter change/run · esc cancel"))
	return modalBox(strings.Join(lines, "\n"), w)
}

func (m Model) compareEditModeValue(st *compareEditModalState) string {
	if st.SaveAsNew {
		return "new copy of '" + st.OriginName + "'" + theme.Subtle.Render("   (enter: → overwrite)")
	}
	return "overwrite '" + st.OriginName + "'" + theme.Subtle.Render("   (enter: → new copy)")
}

func scopeLabelOrNone(scope []string) string {
	if len(scope) == 0 {
		return theme.Subtle.Render("(none — enter to pick)")
	}
	return scopeLabel(scope)
}

// --- keys -----------------------------------------------------------------

func (m Model) handleCompareEditKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	st := m.compareEdit
	if st == nil {
		return m, nil
	}
	rows := compareEditRows()
	switch msg.String() {
	case "esc", "ctrl+c":
		m.compareEdit = nil
		return m, nil
	case "up", "k":
		if st.Cursor > 0 {
			st.Cursor--
		}
		return m, nil
	case "down", "j":
		if st.Cursor < len(rows)-1 {
			st.Cursor++
		}
		return m, nil
	case "enter":
		return (&m).activateCompareEditRow(rows[st.Cursor])
	}
	return m, nil
}

func (m *Model) activateCompareEditRow(row compareEditRow) (Model, tea.Cmd) {
	st := m.compareEdit
	switch row {
	case editRowMode:
		st.SaveAsNew = !st.SaveAsNew
		return *m, nil
	case editRowSource:
		return *m, m.openCompareEditOrgPicker(true)
	case editRowTarget:
		return *m, m.openCompareEditOrgPicker(false)
	case editRowScope:
		return *m, m.openCompareEditScopePicker()
	case editRowMethod:
		return *m, m.openCompareEditMethodPicker()
	case editRowCompare:
		return m.runCompareEdit()
	}
	return *m, nil
}

// --- modal-scoped pickers (write back to the modal, not d.Run) -----------

func (m *Model) openCompareEditOrgPicker(isSource bool) tea.Cmd {
	st := m.compareEdit
	if st == nil {
		return nil
	}
	role := "target"
	current := st.Target.OrgRef()
	if isSource {
		role, current = "source", st.Source.OrgRef()
	}
	opts := make([]choiceOption, 0, len(m.orgs))
	cursor := 0
	for i, o := range m.orgs {
		label := o.Username
		if o.Alias != "" {
			label = o.Alias
		}
		opts = append(opts, choiceOption{Label: label, Hint: o.Username, Value: o.Username})
		if o.Username == current {
			cursor = i
		}
	}
	if len(opts) == 0 {
		return nil
	}
	return m.openChoiceModal(choiceModalState{
		Title:      "Compare " + role + " org",
		Hint:       "pick the " + role + " org",
		Options:    opts,
		Cursor:     cursor,
		Searchable: true,
		Save: func(val any) error {
			if u, _ := val.(string); u != "" && m.compareEdit != nil {
				if isSource {
					m.compareEdit.Source = orgEndpoint(u)
				} else {
					m.compareEdit.Target = orgEndpoint(u)
				}
				m.compareEdit.Err = ""
			}
			return nil
		},
	})
}

func (m *Model) openCompareEditScopePicker() tea.Cmd {
	st := m.compareEdit
	if st == nil {
		return nil
	}
	return m.openCompareScopeModal(st.Scope, func(selected []string) {
		if m.compareEdit != nil {
			m.compareEdit.Scope = selected
		}
	})
}

func (m *Model) openCompareEditMethodPicker() tea.Cmd {
	st := m.compareEdit
	if st == nil {
		return nil
	}
	methods := []compareMethod{compareMethodAuto, compareMethodTooling, compareMethodMetadataAPI}
	opts := make([]choiceOption, 0, len(methods))
	cursor := 0
	for i, cm := range methods {
		opts = append(opts, choiceOption{Label: cm.String(), Hint: methodHint(cm), Value: int(cm)})
		if cm == st.Method {
			cursor = i
		}
	}
	return m.openChoiceModal(choiceModalState{
		Title:   "Retrieval method",
		Hint:    "speed vs API-call count",
		Options: opts,
		Cursor:  cursor,
		Save: func(val any) error {
			if i, ok := val.(int); ok && m.compareEdit != nil {
				m.compareEdit.Method = compareMethod(i)
			}
			return nil
		},
	})
}

// runCompareEdit validates, seeds d.Run from the modal, closes the
// modal, and starts the comparison (user lands on New watching it).
func (m *Model) runCompareEdit() (Model, tea.Cmd) {
	st := m.compareEdit
	if st.Target.IsZero() {
		st.Err = "choose a target org first"
		return *m, nil
	}
	if st.Target.Equal(st.Source) {
		st.Err = "source and target are the same"
		return *m, nil
	}
	if len(st.Scope) == 0 {
		st.Err = "pick at least one metadata type (Scope)"
		return *m, nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		m.compareEdit = nil
		return *m, nil
	}
	run := &compareRun{
		Source: st.Source,
		Target: st.Target,
		Scope:  st.Scope,
		Method: st.Method,
		Phase:  comparePhaseSetup,
	}
	if !st.SaveAsNew {
		run.OriginSavedID = st.OriginID
		run.OriginSavedName = st.OriginName
	} else {
		// Clone → new: keep the name to seed the "save as new" default,
		// but no origin id so saving inserts a fresh row.
		run.OriginSavedName = st.OriginName
		run.SaveAsNew = true
	}
	d.Run = run
	m.compareEdit = nil
	// startCompare switches to the Result subtab itself.
	return *m, m.startCompare(d)
}
