package ui

import "testing"

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
	// Heading at the top edge: skip bounces downward.
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
