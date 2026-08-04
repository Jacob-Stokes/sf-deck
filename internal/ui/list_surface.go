package ui

// List surface registry — third migration step on the path to a
// declarative TabSpec. Companion to chipSurface (chip_surface.go)
// and openSurface (open_surface.go).
//
// listSurface answers the parallel "where is this list's state?"
// question that previously had THREE switches:
//
//   1. tab_registry.go (TabSpec hooks: MoveCursor / SearchPtr /
//      ResetCursor) — for cursor + filter dispatch.
//   2. render_panes.go (searchStateForTab switch) — for the
//      "is this surface filtered? show the yellow border / filter
//      pill" pass.
//   3. listtable_keys.go (activeListTable switch) — for c (column
//      mode) / z (zen) / s (sort) target resolution.
//
// One listSurface entry now holds:
//   - State    : *uilayout.ListTableState   (column-mode, sort, scroll)
//   - Cols     : []uilayout.ListColumn      (canonical column spec)
//   - SearchPtr: *searchState               (filter buffer)
//   - MoveCursor / ResetCursor              (cursor delta + reset hooks)
//
// The three legacy switches walk the registry and only fall back to
// bespoke arms for surfaces with idiosyncratic state shapes
// (TabSOQL, TabReportDetail, TabObjectDetail's per-subtab cursor
// multiplexing, TabRecords picker-vs-list mode, TabPermParentDetail).
//
// Each entry below is a named package-level var so TabSpec entries
// (in tab_registry.go) point at it directly via Subtabs[i].List or
// TabSpec.List.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// listSurface bundles the per-surface list plumbing.
type listSurface struct {
	// State returns a pointer to the surface's persistent
	// list-table state (column widths, sort, horizontal scroll).
	// nil-safe — surfaces without table-mode state can omit it
	// (column-mode keys then no-op on that surface).
	State func(*orgData) *uilayout.ListTableState

	// Cols returns the canonical column spec for the surface.
	// Called fresh each time activeListTable looks up the surface
	// because column-mode resolution + sort use the same defs.
	Cols func() []uilayout.ListColumn

	// SearchPtr returns the surface's per-list-view search state.
	// Drives the / sticky-filter buffer and the yellow filtered-
	// pane indicator.
	SearchPtr func(*orgData) *searchState

	// MoveCursor applies a row-cursor delta to the surface's
	// underlying ListView. Mirrors the existing TabSpec.MoveCursor
	// shape so we can move it off TabSpec onto the surface.
	MoveCursor func(*orgData, int)

	// ResetCursor clears the surface's row cursor — called when
	// the surface filter changes (chip switch) so the user lands
	// on row 0 of the new filtered view.
	ResetCursor func(*orgData)

	// MeasureCell, when non-nil, returns the rendered width of the
	// cell at (col, row) for the surface's current data. Used by
	// snap-to-content (}) to fit a column to its widest
	// visible value. Implementations should NOT include selection
	// highlights / search highlights — just the raw cell text width.
	//
	// Surfaces without per-cell access can leave this nil; snap-to-
	// content then falls back to the column's static header width.
	//
	// When BuildRenderModel is set, MeasureCell is derived
	// automatically (the shared snap path uses the model's Cell).
	// Surfaces using the shared renderer don't need to declare both.
	MeasureCell func(d *orgData, col int) int

	// BuildRenderModel, when non-nil, opts the surface into the
	// shared list-table renderer (renderListModel). Returns a
	// per-frame listRenderModel describing what the table should
	// look like this frame. Tab renderers keep their own
	// orchestrating logic above the table; the model only describes
	// the table block itself.
	//
	// The bool return is "this surface has data ready" — false
	// means the orchestrating tab should render its own
	// busy/error/loading state and skip calling renderListModel.
	//
	// Surfaces that want bespoke rendering leave this nil. The
	// migration path is incremental: add BuildRenderModel for
	// surfaces that fit the shared shape; leave bespoke renderers
	// alone for surfaces that don't.
	BuildRenderModel func(m Model, d *orgData) (listRenderModel, bool)

	// BulkTagTargets, when non-nil, returns the (kind, refs, labels)
	// of every row in the surface's CURRENT filtered view — the
	// "tag everything visible" targets. Derived automatically from
	// ListViewTableSpec.RowKindRef; surfaces without it don't
	// support bulk tagging (the T keybind flashes a hint).
	BulkTagTargets func(d *orgData) (devproject.ItemKind, []string, bool)
}
