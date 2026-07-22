package ui

// Bundle-detail list state.
//
// TabBundleDetail used to be a static text dump — no cursor, no
// scroll, no further drilldown. This state struct gives it a
// proper sortable list-table the same way /flows or /apex have:
// rows are the components in the bundle (parsed from the preview
// data), sortable by action / kind / member, with cursor +
// horizontal-scroll persistence.
//
// Lives on Model (not orgData) because bundles aren't org-scoped
// — the user can drill into a bundle from any active org. Mirrors
// the devProjectList shape one level up.

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
	// bundleDetailList backs the COMPONENTS view's table body.
	// Populated by applyBundlePreviewLoaded when the manifest
	// preview shell-out returns; reset on bundle switch.
	bundleDetailList ListView[bundleDetailRow]

	// bundleDetailTable persists column widths, sort, horizontal
	// scroll for the components view across renders + sessions.
	bundleDetailTable uilayout.ListTableState

	// bundleDetailView is the active view mode (components vs.
	// files). Defaults to bundleViewComponents on a fresh drill.
	bundleDetailView bundleDetailView

	// bundleFilesList backs the FILES view's table body. Re-read
	// from disk on every cwd change (entered a folder, popped to
	// parent, switched bundles) — file metadata is cheap to fetch
	// vs. the complexity of a watcher.
	bundleFilesList ListView[bundleFileRow]

	// bundleFilesTable mirrors bundleDetailTable for the files
	// view — separate so column-mode / sort prefs don't bleed
	// between the two surfaces.
	bundleFilesTable uilayout.ListTableState

	// bundleFilesCwd is the current working directory RELATIVE to
	// the bundle root. Empty = bundle root. Joined with b.Path
	// before any ReadDir. Switching bundles resets this to "".
	bundleFilesCwd string

	// bundleFilesLoadedFor is the (bundleID + cwd) the current
	// bundleFilesList reflects — used by the renderer to detect
	// "do I need to re-read the directory?" without burning a
	// disk hit per frame.
	bundleFilesLoadedFor string
}

// bundleDetailView is the active row-body view on TabBundleDetail.
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
