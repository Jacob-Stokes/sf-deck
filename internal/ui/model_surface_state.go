package ui

type modelSurfaceState struct {
	fieldActionCur int

	objectActionCur int

	validationActionCur int

	recordTypeActionCur int

	triggerActionCur int

	bodyFocus bool

	recordDetailReturnTab Tab

	recordDrillStack []recordDrillFrame

	triggerDetailReturnTab Tab

	homeFocusedSectionLetter string

	homeDestCursor int
}

type recordDrillFrame struct {
	SObject string
	ID      string
}
