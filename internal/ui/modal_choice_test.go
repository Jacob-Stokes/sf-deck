package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestChoiceModalCursorSkipsHeadings pins the chip-manager separator
// behaviour: section headings render but the cursor steps over them in
// both directions, bounces at edges, and Enter can never fire on one.
func TestChoiceModalCursorSkipsHeadings(t *testing.T) {
	cm := &choiceModalState{
		Options: []choiceOption{
			{Label: "+ New view…", Value: "new"},
			{Label: "── built-ins ──", Value: "_sep_built-ins", Heading: true},
			{Label: "All", Value: "all"},
		},
	}
	// Down from row 0 must land on "All" (index 2), skipping the heading.
	cm.visibleCursor = 1
	choiceModalSyncCursor(cm)
	choiceModalSkipHeading(cm, 1)
	if cm.Cursor != 2 {
		t.Fatalf("down over heading: cursor = %d, want 2", cm.Cursor)
	}
	// Up from "All" must land back on row 0.
	cm.visibleCursor = 1
	choiceModalSyncCursor(cm)
	choiceModalSkipHeading(cm, -1)
	if cm.Cursor != 0 {
		t.Fatalf("up over heading: cursor = %d, want 0", cm.Cursor)
	}
	cm2 := &choiceModalState{
		Options: []choiceOption{
			{Label: "── pinned ──", Heading: true},
			{Label: "All", Value: "all"},
		},
	}
	cm2.visibleCursor = 0
	choiceModalSyncCursor(cm2)
	choiceModalSkipHeading(cm2, -1)
	if cm2.Cursor != 1 {
		t.Fatalf("bounce at top: cursor = %d, want 1", cm2.Cursor)
	}
}

func TestChoiceModalCtrlCQuitsWhileBusy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		loading bool
		saving  bool
	}{
		{name: "loading", loading: true},
		{name: "saving", saving: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{modelTransient: modelTransient{
				choiceModal: &choiceModalState{
					Loading: tc.loading,
					Saving:  tc.saving,
				},
			}}

			_, cmd := m.handleChoiceModalKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			if cmd == nil {
				t.Fatal("Ctrl+C returned no command")
			}
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Fatalf("Ctrl+C command returned %T, want tea.QuitMsg", msg)
			}
		})
	}
}
