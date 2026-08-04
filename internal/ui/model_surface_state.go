package ui

// Cross-surface cursors + drill-return state.
//
// Extracted from model.go. modelSurfaceState is embedded into Model so
// existing field access (m.fieldActionCur, m.bodyFocus, …) keeps
// working unchanged.

// modelSurfaceState owns cross-surface cursors and drill-return state.
type modelSurfaceState struct {
	// fieldActionCur is the cursor on TabFieldDetail's action menu
	// (right sidebar). Reset when switching fields.
	fieldActionCur int

	// objectActionCur is the row cursor on the Details-subtab MAIN
	// pane. It indexes into objectDetailRows (the navigable content
	// rows — identity / capabilities / features / fields). The right
	// sidebar is now info-only; it merely highlights whichever action
	// the cursored row maps to. Enter / ctrl+e on an actionable row
	// fires that row's edit or toggle modal. Reset when switching
	// objects. (Name kept for historical continuity with the other
	// *ActionCur cursors even though it now means "detail row".)
	objectActionCur int

	// validationActionCur is the cursor on the Validation-subtab
	// action menu (right sidebar). Reset when switching rules.
	validationActionCur int

	// recordTypeActionCur is the cursor on the Record Types drill
	// action menu (right sidebar). Reset when switching record types.
	recordTypeActionCur int

	// triggerActionCur is the cursor on the Trigger drill action
	// menu (right sidebar). Reset when switching triggers.
	triggerActionCur int

	// bodyFocus toggles whether j / k on a code-detail tab steers the
	// scrollable code body or the right-sidebar action menu. Default
	// true: most users open these tabs to read the body, action firing
	// happens after a glance through. Tab key flips the focus.
	bodyFocus bool

	// recordDetailReturnTab is the tab to pop back to when the user
	// hits Esc on TabRecordDetail. Set when the drill is opened so
	// records reached from /soql / /reports / /recent each return to
	// their own surface. Default fallback is TabRecords.
	recordDetailReturnTab Tab

	// recordDrillStack tracks the record→record drill chain so Esc
	// can pop back through parents one level at a time. Each frame
	// captures the (sObject, id) that was active BEFORE pushing into
	// a new related record; popping restores that record's detail
	// and leaves recordDetailReturnTab as it was originally set.
	// Empty when the user is on the top-level drill (Esc → returnTab).
	recordDrillStack []recordDrillFrame

	// triggerDetailReturnTab is the tab to pop back to from
	// TabTriggerDetail. Triggers can be reached from Object Detail,
	// the Apex trigger list, and project views, so this also drives
	// the active top-level tab family while drilled in.
	triggerDetailReturnTab Tab

	// homeFocusedSectionLetter is the section currently in focus on
	// /home Landing's Lightning Destinations grid. Empty = no
	// section focused (top-level navigation; item letters are inert
	// and only the section letters fire). Set by typing a section
	// letter; cleared by Esc.
	homeFocusedSectionLetter string

	// homeDestCursor is the flat-index cursor across the destinations
	// grid for j/k navigation. -1 means "no cursor yet, use the first
	// row of the focused section (or row 0) on first move."
	homeDestCursor int
}

// recordDrillFrame is one level of the record→record drill chain.
// Captured at push-time so pop can restore the previous record's
// detail context verbatim.
type recordDrillFrame struct {
	SObject string
	ID      string
}
