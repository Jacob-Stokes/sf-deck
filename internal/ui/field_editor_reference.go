package ui

// Reference (lookup) field editor. References point at another
// record by Id; the user almost never knows the Id but does know
// the name. Phase 1 keeps the editor simple:
//
//   - On Init, populate Raw with the current name (when present)
//     and ChosenID with the current Id.
//   - HandleKey treats the buffer like a search query — every
//     keystroke updates it; the user can submit a SOSL search by
//     pressing Enter, which fires a referenceSearchMsg.
//   - SearchResults land back via referenceSearchResultMsg and
//     are surfaced as a list under the cell so the user picks
//     one with arrow + Enter.
//   - Polymorphic references (referenceTo has >1 entry) are
//     handled with a prefix nudge: the user has to type the
//     target type's prefix first (e.g. "Account: Acme"). We
//     accept the form and pass the first matching entry; SF
//     resolves by Id anyway, so the prefix is just for the
//     user's search filter.
//
// For phase 1 we render the inline list of hits in the cell;
// future work could route through the anchored picker. Keeping it
// flat means search-as-you-type works without an extra modal
// layer.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// referenceEditor handles reference (lookup) field edits. Owns a
// search query buffer + list of recent hits + cursor into them.
type referenceEditor struct{}

func init() {
	registerFieldEditor(&referenceEditor{}, "reference")
}

// We extend EditState through a tiny package-private struct stored
// in Picker — keeps the discriminated-union footprint small while
// giving reference editor its own state shape.
type referenceState struct {
	Hits      []sf.SearchHit
	HitCursor int
	Loading   bool
	LastQuery string
}

// referenceStateOf retrieves (or initialises) the per-editor
// state attached to an EditState. We piggyback on the Picker
// field which is otherwise unused on text-shape editors.
func referenceStateOf(s *EditState) *referenceState {
	if s.Picker == nil {
		s.Picker = &PickerState{}
	}
	if s.Picker.Reference == nil {
		s.Picker.Reference = &referenceState{}
	}
	return s.Picker.Reference
}

// PickerState is the holder for editor-specific extra state. Lives
// on EditState.Picker so the union stays a flat struct without
// type-discriminated subfields. Only allocated when an editor
// actually needs richer state.
type PickerState struct {
	Reference *referenceState
}

func (e *referenceEditor) CanEdit(f sf.Field) bool {
	if !f.Updateable || len(f.ReferenceTo) == 0 {
		return false
	}
	return true
}

func (e *referenceEditor) Init(f sf.Field, current any) EditState {
	raw := stringifyFieldValue(current)
	return EditState{Field: f, Raw: "", ChosenID: raw, ChosenName: ""}
}

func (e *referenceEditor) RenderEditCell(s *EditState, width int, focused bool) string {
	fg := theme.Fg
	if !focused {
		fg = theme.FgDim
	}
	state := referenceStateOf(s)
	var label string
	if s.ChosenName != "" {
		label = s.ChosenName + " (" + s.ChosenID + ")"
	} else if s.ChosenID != "" {
		label = s.ChosenID
	} else {
		label = "(empty)"
	}
	body := lipgloss.NewStyle().Foreground(fg).Render(label)
	if focused {
		body += "\n  query: "
		body += lipgloss.NewStyle().Foreground(theme.Yellow).Render(s.Raw)
		body += lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("▌")
		switch {
		case state.Loading:
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("(searching…)")
		case len(state.Hits) > 0:
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).
				Render(fmt.Sprintf("(%d hits · ↑↓ pick · ↵ select)", len(state.Hits)))
		case state.LastQuery != "":
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("(no matches)")
		default:
			body += "  " + lipgloss.NewStyle().Foreground(theme.Muted).
				Render("(type ↵ to search "+strings.Join(s.Field.ReferenceTo, "/")+")")
		}
		// List the hits below the query line.
		for i, hit := range state.Hits {
			arrow := "  "
			if i == state.HitCursor {
				arrow = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌ ")
			}
			body += "\n  " + arrow + hit.Name + "  " +
				lipgloss.NewStyle().Foreground(theme.Muted).Render("· "+hit.ID)
		}
	}
	if s.Error != "" {
		body += "  " + lipgloss.NewStyle().Foreground(theme.Red).Render("· "+s.Error)
	}
	return body
}

// referenceSearchMsg fires when the user hits Enter on the query
// line. The orchestrator (recordEditSession in stage 7) catches
// it, runs sf.SearchRecords, and posts a referenceSearchResultMsg
// back.
type referenceSearchMsg struct {
	Field string // field API name
	Query string
}

// referenceSearchResultMsg lands when the SOSL search completes.
type referenceSearchResultMsg struct {
	Field string
	Hits  []sf.SearchHit
	Err   error
}

func (e *referenceEditor) HandleKey(s *EditState, msg tea.KeyMsg) (bool, tea.Cmd) {
	state := referenceStateOf(s)
	key := msg.String()
	switch key {
	case "backspace":
		if len(s.Raw) > 0 {
			s.Raw = s.Raw[:len(s.Raw)-1]
		}
		s.Error = ""
		return true, nil
	case "ctrl+u":
		s.Raw = ""
		state.Hits = nil
		state.LastQuery = ""
		state.HitCursor = 0
		s.Error = ""
		return true, nil
	case "up", "k":
		if state.HitCursor > 0 {
			state.HitCursor--
		}
		return true, nil
	case "down", "j":
		if state.HitCursor < len(state.Hits)-1 {
			state.HitCursor++
		}
		return true, nil
	case "enter", "↵":
		// Two modes: (a) hits already shown → pick the cursored
		// one; (b) no hits yet → fire a search with the current
		// query.
		if len(state.Hits) > 0 {
			h := state.Hits[state.HitCursor]
			s.ChosenID = h.ID
			s.ChosenName = h.Name
			s.Raw = ""
			state.Hits = nil
			state.LastQuery = ""
			state.HitCursor = 0
			s.Error = ""
			return true, nil
		}
		query := strings.TrimSpace(s.Raw)
		if query == "" {
			return true, nil
		}
		state.Loading = true
		state.LastQuery = query
		return true, func() tea.Msg {
			return referenceSearchMsg{Field: s.Field.Name, Query: query}
		}
	case "space", " ":
		s.Raw += " "
		s.Error = ""
		return true, nil
	}
	if r, ok := singleRune(key); ok {
		s.Raw += string(r)
		s.Error = ""
		return true, nil
	}
	return false, nil
}

func (e *referenceEditor) Commit(s *EditState) (CommitMode, any, error) {
	if s.ChosenID == "" {
		if !s.Field.Nillable {
			s.Error = "required"
			return CommitNone, nil, fmt.Errorf("required")
		}
		return CommitNull, nil, nil
	}
	return CommitValue, s.ChosenID, nil
}

// referenceSearchFor is the per-org search dispatcher. Called by
// the recordEditSession (stage 7) when a referenceSearchMsg lands.
// Resolves the field's first referenceTo entry, runs SOSL, and
// returns a result message.
//
// Polymorphic refs (referenceTo has >1) use the first entry — phase
// 1 simplification. If the user really needs to pick across multiple
// types they can type the type name into the query and SOSL's
// IN NAME FIELDS clause won't help, but `FIND {term}` would —
// future enhancement.
func referenceSearchFor(alias string, m Model, msg referenceSearchMsg) tea.Cmd {
	return func() tea.Msg {
		// Resolve referenceTo + the target object's nameField from
		// the cached describe.
		field, target, nameField := resolveReferenceTarget(m, msg.Field)
		_ = field
		if target == "" {
			return referenceSearchResultMsg{Field: msg.Field, Err: fmt.Errorf("no referenceTo target")}
		}
		hits, err := sf.SearchRecordsAlias(alias, target, nameField, msg.Query, m.settings.LimitReferencePicker())
		return referenceSearchResultMsg{Field: msg.Field, Hits: hits, Err: err}
	}
}

// resolveReferenceTarget walks the describe of the record being edited
// to find the field's first referenceTo target + that target's
// nameField. Defaults nameField to "Name" if the describe is
// unavailable — the SOSL helper layers the same default in.
//
// The describe MUST come from the drilled record's sObject
// (RecordDetailCur), NOT the last-browsed /objects describe
// (DescribeCur): browsing object A then drilling a record of object B
// leaves DescribeCur=A while the edit targets B, so resolving from
// DescribeCur would search the wrong sObject and could write a foreign
// Id into the field.
func resolveReferenceTarget(m Model, fieldName string) (sf.Field, string, string) {
	d := m.activeOrgData()
	if d == nil {
		return sf.Field{}, "", "Name"
	}
	sobject, _ := splitRecordKey(d.RecordDetailCur)
	desc, ok := d.Describes[sobject]
	if !ok || desc.FetchedAt().IsZero() {
		return sf.Field{}, "", "Name"
	}
	for _, f := range desc.Value().Fields {
		if f.Name == fieldName {
			if len(f.ReferenceTo) == 0 {
				return f, "", "Name"
			}
			return f, f.ReferenceTo[0], targetNameField(m, f.ReferenceTo[0])
		}
	}
	return sf.Field{}, "", "Name"
}

// targetNameField looks up the nameField (the field marked
// NameField=true on the target's describe). Defaults to "Name".
func targetNameField(m Model, target string) string {
	d := m.activeOrgData()
	if d == nil {
		return "Name"
	}
	desc, ok := d.Describes[target]
	if !ok || desc.FetchedAt().IsZero() {
		return "Name"
	}
	for _, f := range desc.Value().Fields {
		if f.NameField {
			return f.Name
		}
	}
	return "Name"
}
