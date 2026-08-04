package ui

// Picklist + multipicklist editors. Two implementations because the
// UI shape diverges meaningfully:
//
//   - picklistEditor: single value, cycle via arrow keys when the
//     picklist has ≤ picklistCycleThreshold (8) values. Larger
//     picklists open the anchored picker on Enter so the user
//     doesn't have to arrow through 30 options.
//   - multipicklistEditor: many values selected together. Always
//     opens the picker. Selected values are shown as comma-joined
//     in the cell when collapsed.
//
// Both editors honour the Active flag on each describe option —
// inactive values still exist on the record but can't be picked
// fresh. (We surface the existing inactive value as the current
// pick but exclude it from the picker.)

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// picklistCycleThreshold is the option-count cutoff below which the
// editor cycles inline with arrow keys. Above it, Enter opens the
// anchored picker. Tuned to "you can hold the values in your head"
// — bigger lists need a search-as-you-type picker.
const picklistCycleThreshold = 8

// picklistEditor handles single-value picklists.
type picklistEditor struct{}

// multipicklistEditor handles multi-value picklists. Values are
// stored as the API expects (semicolon-separated on the wire) and
// rendered comma-separated in the cell.
type multipicklistEditor struct{}

func init() {
	registerFieldEditor(&picklistEditor{}, "picklist")
	registerFieldEditor(&multipicklistEditor{}, "multipicklist")
}

// ---- picklistEditor ------------------------------------------

func (e *picklistEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && len(f.PicklistValues) > 0
}

func (e *picklistEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
	// -1 = the stored value isn't among the ACTIVE options (e.g. an
	// admin deactivated it after this record was set). RenderEditCell
	// shows Raw in that case rather than masking it with options[0] —
	// otherwise a single ↑/↓ would jump two values away from what the
	// user believes is selected and PATCH the wrong value.
	cursor := -1
	for i, pv := range activePicklistValues(f) {
		if pv.Value == raw {
			cursor = i
			break
		}
	}
	return EditState{Field: f, Raw: raw, Cursor: cursor}
}

func (e *picklistEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	options := activePicklistValues(s.Field)
	label := s.Raw
	if s.Cursor >= 0 && s.Cursor < len(options) {
		label = options[s.Cursor].Label
		if options[s.Cursor].Value != options[s.Cursor].Label {
			label += " (" + options[s.Cursor].Value + ")"
		}
	}
	fg := theme.Fg
	if !focused {
		fg = theme.FgDim
	}
	body := lipgloss.NewStyle().Foreground(fg).Render(label)
	if focused {
		if len(options) <= picklistCycleThreshold {
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).
				Render(fmt.Sprintf("(%d/%d · ↑↓ cycle)", s.Cursor+1, len(options)))
		} else {
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).
				Render(fmt.Sprintf("(%d options · ↵ picker · ↑↓ cycle)", len(options)))
		}
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

func (e *picklistEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	options := activePicklistValues(s.Field)
	if len(options) == 0 {
		return false, nil
	}
	switch msg.String() {
	case "up", "k":
		if s.Cursor > 0 {
			s.Cursor--
		}
		s.Raw = options[s.Cursor].Value
		s.Error = ""
		return true, nil
	case "down", "j":
		if s.Cursor < len(options)-1 {
			s.Cursor++
		}
		s.Raw = options[s.Cursor].Value
		s.Error = ""
		return true, nil
	case "ctrl+u":
		// Clear (sets to no selection — nillable only).
		s.Raw = ""
		s.Cursor = -1
		s.Error = ""
		return true, nil
	}
	return false, nil
}

func (e *picklistEditor) Commit(s *EditState) (CommitMode, any, error) {
	if s.Raw == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	return CommitValue, s.Raw, nil
}

// ---- multipicklistEditor -------------------------------------

func (e *multipicklistEditor) CanEdit(f sf.Field) bool {
	return f.Updateable && len(f.PicklistValues) > 0
}

func (e *multipicklistEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
	// SF returns multipicklist values as semicolon-joined.
	var selected []string
	for _, v := range strings.Split(raw, ";") {
		if v = strings.TrimSpace(v); v != "" {
			selected = append(selected, v)
		}
	}
	return EditState{Field: f, Raw: raw, Selected: selected}
}

func (e *multipicklistEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	fg := theme.Fg
	if !focused {
		fg = theme.FgDim
	}
	label := "(none)"
	if len(s.Selected) > 0 {
		label = strings.Join(s.Selected, ", ")
	}
	body := lipgloss.NewStyle().Foreground(fg).Render(label)
	if focused {
		options := activePicklistValues(s.Field)
		body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).
			Render(fmt.Sprintf("(%d/%d selected · space toggles · ↑↓ navigate)",
				len(s.Selected), len(options)))
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

func (e *multipicklistEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	options := activePicklistValues(s.Field)
	if len(options) == 0 {
		return false, nil
	}
	switch msg.String() {
	case "up", "k":
		if s.Cursor > 0 {
			s.Cursor--
		}
		s.Error = ""
		return true, nil
	case "down", "j":
		if s.Cursor < len(options)-1 {
			s.Cursor++
		}
		s.Error = ""
		return true, nil
	case "space", " ", "x", "X":
		// Toggle the cursored option in the selected slice.
		val := options[s.Cursor].Value
		idx := -1
		for i, v := range s.Selected {
			if v == val {
				idx = i
				break
			}
		}
		if idx >= 0 {
			s.Selected = append(s.Selected[:idx], s.Selected[idx+1:]...)
		} else {
			s.Selected = append(s.Selected, val)
		}
		s.Error = ""
		return true, nil
	case "ctrl+u":
		s.Selected = nil
		s.Error = ""
		return true, nil
	}
	return false, nil
}

func (e *multipicklistEditor) Commit(s *EditState) (CommitMode, any, error) {
	if len(s.Selected) == 0 {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	return CommitValue, strings.Join(s.Selected, ";"), nil
}

// ---- helpers ----------------------------------------------------

// activePicklistValues filters out the inactive options. Inactive
// values stay on the record (an admin disabled them after the
// record was created) but the editor shouldn't offer them as a
// fresh pick. Order of remaining values is preserved.
func activePicklistValues(f sf.Field) []sf.PicklistValue {
	out := make([]sf.PicklistValue, 0, len(f.PicklistValues))
	for _, pv := range f.PicklistValues {
		if pv.Active {
			out = append(out, pv)
		}
	}
	return out
}
