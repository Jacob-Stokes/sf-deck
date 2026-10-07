package ui

// Bundle-detail list state.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// modelBundleDetailState owns the list view + table state for the
// /bundle drilldown.
//
// Two view modes share the same tab surface:
//
//   - bundleViewComponents (default): the manifest preview rows
//     (To retrieve / To deploy / …). The Components table.
//   - bundleViewFiles: a cd-style file browser of the bundle's
//     on-disk directory. Useful for the non-force-app stuff
//     (README, sfdx-project.json, sub-dirs the user added) that
//     never appears in the manifest.
//
// `[` / `]` and Shift+Left / Right cycle the mode. The active
// view's ListView + LIST_TABLE_STATE both live here so the
// renderer can read whichever pair matches the current mode.
type modelBundleDetailState struct {
	bundleDetailList ListView[bundleDetailRow]

	bundleDetailTable uilayout.ListTableState

	bundleDetailView bundleDetailView

	bundleFilesList ListView[bundleFileRow]

	bundleFilesTable uilayout.ListTableState

	bundleFilesCwd string

	bundleFilesLoadedFor string
}

type bundleDetailView int

const (
	bundleViewComponents bundleDetailView = iota
	bundleViewFiles
)

func (v bundleDetailView) String() string {
	switch v {
	case bundleViewComponents:
		return "components"
	case bundleViewFiles:
		return "files"
	}
	return "unknown"
}
