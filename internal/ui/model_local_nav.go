package ui

// Local filesystem + static setup-list state.
//
// Extracted from model.go. modelLocalNavigation is embedded into Model
// so existing field access (m.projectsRes, m.setupList, …) keeps
// working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/project"
)

// modelLocalNavigation owns local filesystem and static setup-list state.
type modelLocalNavigation struct {
	// Projects (local FS).
	projectsRes Resource[[]*project.Project]
	projectList ListView[*project.Project]

	// Setup shortcuts.
	setupList ListView[setupLink]
}
