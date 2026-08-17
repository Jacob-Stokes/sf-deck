package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/project"
)

type modelLocalNavigation struct {
	projectsRes Resource[[]*project.Project]
	projectList ListView[*project.Project]

	setupList ListView[setupLink]
}
