package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

type modelDevProjectState struct {
	devProjectList        ListView[devproject.DevProject]
	devProjectCur         string // ID of the drilled-in dev project (TabDevProjectDetail)
	devProjectShowAllOrgs bool   // when true, detail view shows items from every org

	devProjectKindChip       devproject.ItemKind
	devProjectKindChipCursor int
}
