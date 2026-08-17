package ui

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
	Field sf.Field

	Raw string

	Cursor int

	Selected []string

	ChosenID   string
	ChosenName string

	Picker *PickerState

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
	CanEdit(f sf.Field) bool

	Init(f sf.Field, current any) EditState

	RenderEditCell(s *EditState, width int, focused bool) string

	HandleKey(s *EditState, msg tea.KeyMsg) (consumed bool, cmd tea.Cmd)

	Commit(s *EditState) (CommitMode, any, error)
}

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
