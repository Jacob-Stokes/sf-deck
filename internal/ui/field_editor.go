package ui

// Per-field-type editor abstraction backing inline record edit on
// /record. The model is:
//
//	1. Each Salesforce field type (string, picklist, reference, …)
//	   has a FieldEditor implementation registered in fieldEditors.
//	2. EditState carries the per-field live edit data — current
//	   buffer / cursor / picker state / error message — keyed by
//	   field API name on recordEditSession.
//	3. The /record renderer calls the editor's RenderEditCell for
//	   the focused field; everything else renders its display value.
//	4. Keystrokes route through the editor's HandleKey while it
//	   owns focus.
//	5. Commit converts the edit state into a value the PATCH body
//	   accepts. Commit failure means a format problem the editor
//	   wants to surface locally; the field stays in edit mode.
//
// Adding a new field type = one struct + one registry entry.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// EditState is the live per-field edit data. A single discriminated
// union covers every editor — text-ish ones use Raw, the cycle-style
// ones use Cursor, the picker-style ones use Picker / ChosenID /
// ChosenName / Selected. Editors only touch the fields they care
// about; unused fields stay zero-valued.
type EditState struct {
	// Field is the describe metadata for this row. Immutable for the
	// life of the EditState. Editors read referenceTo, picklistValues,
	// length, precision, nillable, etc. from here.
	Field sf.Field

	// Raw is the textinput / textarea buffer for text-shaped editors.
	// For pickers it can hold the visible label (e.g. picklist label,
	// reference Name) while ChosenID / ChosenName / Selected carry
	// the values that ultimately get PATCHed.
	Raw string

	// Cursor is the index into PicklistValues for the cycle-style
	// picklist editor + the toggle index for boolean.
	Cursor int

	// Selected is the multipicklist selection — values, not labels.
	Selected []string

	// ChosenID + ChosenName carry a reference editor's pick: the
	// Salesforce 18-char Id and the human-readable label respectively.
	ChosenID   string
	ChosenName string

	// Picker holds editor-specific extra state — currently used by
	// the reference editor for its hits + cursor + loading flag.
	// Only allocated when an editor actually needs richer state, so
	// the common text editor path stays a flat-struct copy.
	Picker *PickerState

	// Error is the last local validation message ("not a valid date").
	// Cleared on the next keystroke; PATCH-side errors land separately
	// on the recordEditSession's error map.
	Error string
}

// CommitMode is what a Commit call returns when it produces a value
// to send to Salesforce. None means "user is still editing, nothing
// to PATCH yet."
type CommitMode int

const (
	CommitNone  CommitMode = iota // editor not yet ready to commit
	CommitValue                   // PATCH this value
	CommitNull                    // PATCH null (clear the field)
)

// FieldEditor is the per-type contract. Implementations live next
// to the type they handle (field_editor_text.go, field_editor_picklist.go,
// …) and register themselves into fieldEditors at init time.
type FieldEditor interface {
	// CanEdit reports whether this field is editable. Wraps the
	// describe's Updateable flag plus any type-specific gates.
	CanEdit(f sf.Field) bool

	// Init prepares a fresh EditState seeded with the current value.
	// Called when the user enters edit mode on a field row.
	Init(f sf.Field, current any) EditState

	// RenderEditCell paints the in-line edit widget for the field
	// row's value cell. width is the cell width budget; focused
	// indicates whether this row currently owns the cursor.
	//
	// Editors may produce multi-line output (e.g. textarea expanding
	// vertically); the caller's row-flow accommodates Lipgloss height.
	RenderEditCell(s *EditState, width int, focused bool) string

	// HandleKey processes one keystroke while this editor owns
	// focus. Returns (consumed) so the dispatcher knows whether to
	// continue routing the key (e.g. esc / tab usually escape edit
	// mode and should not be consumed by the editor).
	HandleKey(s *EditState, msg tea.KeyMsg) (consumed bool, cmd tea.Cmd)

	// Commit produces the value to send to Salesforce. Returning
	// CommitNone keeps the field in edit mode; CommitValue / CommitNull
	// move it into the dirty map and exit edit mode. Editors that
	// validate locally (date / number) surface format errors here.
	Commit(s *EditState) (CommitMode, any, error)
}

// fieldEditors is the registry — keyed by sf.Field.Type (the wire
// string from describe: "string", "picklist", "reference", …).
// Populated by init() in each field_editor_*.go file.
var fieldEditors = map[string]FieldEditor{}

// registerFieldEditor adds an editor under one or more describe
// types. Late init order safe — every registration happens before
// the first /record render.
func registerFieldEditor(editor FieldEditor, types ...string) {
	for _, t := range types {
		fieldEditors[t] = editor
	}
}

// resolveFieldEditor returns the editor registered for the field's
// type, or nil when none exists (which means "read only by default"
// — defensive policy: better to refuse than to surface a half-broken
// widget for an unknown field type).
func resolveFieldEditor(f sf.Field) FieldEditor {
	if e, ok := fieldEditors[f.Type]; ok {
		return e
	}
	return nil
}
