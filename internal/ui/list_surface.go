package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

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

	SearchPtr func(*orgData) *searchState

	MoveCursor func(*orgData, int)

	ResetCursor func(*orgData)

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

	BulkTagTargets func(d *orgData) (devproject.ItemKind, []string, bool)
}
