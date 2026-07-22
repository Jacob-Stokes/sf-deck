package ui

// list_surface_spec.go — generic builder that collapses the
// repeated BuildRenderModel boilerplate every static list surface
// was writing by hand.
//
// Before: each surface declared a 60-90 line listSurface literal
// that resolved the schema, installed the sort hook, filtered the
// list, built marks/gutters, and assembled a listRenderModel. The
// variation between surfaces was 5-10 lines (title format, marks
// fn, gutter wiring); the rest was identical scaffolding.
//
// After: surfaces declare a typed ListViewTableSpec[T] with just
// the variation. The spec's Build() method returns a
// listRenderModel built from the spec's pieces — sort hook,
// filtered items, marks, gutters, recolor, empty hint, version.
//
// Reports stays separate (dynamic columns from server response).
// Chip-driven surfaces (objects, flows, apex by chip, perms by
// chip) still need bespoke BuildRenderModel closures because the
// chip selection forks the resource lookup; this builder targets
// the "one ListView[T] + Resource[T] + schema" cases.

import (
	"charm.land/lipgloss/v2"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// ListViewTableSpec[T] is the declarative shape that every "static
// list surface" boils down to. Build() returns a listRenderModel
// suitable for a listSurface.BuildRenderModel closure.
//
// Required:
//   - Schema: the tablemodel.Schema[T] that resolves columns + cells
//   - ListPtr: returns the per-orgData ListView[T] this surface owns
//   - StatePtr: returns the per-orgData ListTableState pointer for
//     column-mode + sort persistence
//   - Title: returns the title string (usually composed from row
//     counts + a "fetched X ago · busy/err" suffix)
//   - Empty: the message shown when N == 0
//
// Optional:
//   - Marks: per-row mark badges (drives the Marks gutter column)
//   - Gutters: (left, right) gutter functions; nil = no decoration.
//     Most surfaces use the tag+project gutters via m.listGutters.
//   - Recolor: per-cell style override (e.g. tint Valid red when
//     IsValid is false)
//   - FlagsAware: when true, m.applyFlagsColumnMode is called on
//     the resolved column list before render — surfaces that want
//     the user's column-visibility settings respected
//   - FooterExtras: extra footer hint text appended to the standard
//     "↵ open · r refresh" footer
type ListViewTableSpec[T any] struct {
	Schema   tablemodel.Schema[T]
	ListPtr  func(d *orgData) *ListView[T]
	StatePtr func(d *orgData) *uilayout.ListTableState
	Title    func(m Model, d *orgData, items []T) string
	Empty    string
	// EmptyFn is the dynamic alternative to Empty — used when the
	// empty-state message depends on org state (e.g. licenses query
	// failed with an error vs. just no rows). EmptyFn wins when set.
	EmptyFn func(m Model, d *orgData) string

	// ResErr returns the backing resource's last fetch error, if any.
	// When set and the list is empty, the empty-state renders the error
	// (with a hint for common causes) instead of the generic "no rows" —
	// so an org that can't serve this surface (no API access, missing
	// FLS, a Tooling failure) explains itself. Wire it to the resource's
	// .Err() accessor. Optional; nil keeps the plain empty message.
	ResErr func(d *orgData) error

	Marks   func(items []T) []uilayout.RowMark
	Gutters func(m Model, items []T) (left []uilayout.GutterSpec, right []uilayout.GutterSpec)
	Recolor func(items []T, row, col int, colName string, base lipgloss.Style) lipgloss.Style

	FlagsAware   bool
	FooterExtras string

	// ColStyles is an optional per-column-name style override map
	// applied to the resolved column list before render. Used by
	// surfaces (home Limits/Licenses) that paint columns in fixed
	// theme palettes regardless of row content. Empty = no override.
	ColStyles map[string]lipgloss.Style

	// RowKindRef, when non-nil, maps one item to its taggable
	// (kind, ref) identity — the per-row analogue of the surface's
	// cursored-row Identity resolver. Enables bulk tagging (T):
	// the picker targets every row of the current filtered view.
	RowKindRef func(item T) (devproject.ItemKind, string)
}

// Build evaluates the spec against the given Model + orgData and
// returns the populated listRenderModel + ok flag. Callers wire
// this into a listSurface's BuildRenderModel:
//
//	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
//	    return apexClassesTableSpec.Build(m, d)
//	}
func (s ListViewTableSpec[T]) Build(m Model, d *orgData) (listRenderModel, bool) {
	if d == nil || s.ListPtr == nil || s.StatePtr == nil {
		return listRenderModel{}, false
	}
	lv := s.ListPtr(d)
	state := s.StatePtr(d)
	if lv == nil || state == nil {
		return listRenderModel{}, false
	}

	resolved := mustResolveColumns(s.Schema)
	cols := resolved.ListColumns()
	if s.FlagsAware {
		cols = m.applyFlagsColumnMode(cols)
	}
	if len(s.ColStyles) > 0 {
		cols = withColStyles(cols, s.ColStyles)
	}

	// Pre-compute marks once so the sort-install callback + the
	// final render closure see the same slice (marks track row
	// identity; recomputing inside the cell closure would race
	// the install-order pass when N is large).
	var marks []uilayout.RowMark
	if s.Marks != nil {
		marks = s.Marks(lv.Items())
	}

	installListViewOrderRows(lv, state, cols,
		func(items []T, row, col int) string {
			if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
				return ""
			}
			if cols[col].Name == "Marks" {
				return m.renderFlagsCell(s.marksFor(items), row)
			}
			// Sort on the column's SortKey (raw value), not the
			// rendered label — e.g. Apex SIZE renders "1.5K" but
			// must order by the underlying char count.
			return resolvedSortCellByID(resolved, items[row], cols[col].Name)
		})

	items := lv.Filtered()
	// Recompute marks against the filtered slice — the sort
	// callback above saw the full list; the render closure now
	// needs the filtered subset's marks. Cheap: marks slices are
	// small and lazy.
	if s.Marks != nil {
		marks = s.Marks(items)
	}

	var left, right []uilayout.GutterSpec
	if s.Gutters != nil {
		left, right = s.Gutters(m, items)
	}

	empty := s.Empty
	if s.EmptyFn != nil {
		empty = s.EmptyFn(m, d)
	}
	var resErr error
	if s.ResErr != nil {
		resErr = s.ResErr(d)
	}

	model := listRenderModel{
		Title:  s.Title(m, d, items),
		State:  state,
		Search: lv.SearchPtr(),
		Cols:   cols,
		N:      len(items),
		Cursor: lv.Cursor(),
		Cell: func(row, col int) string {
			if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
				return ""
			}
			if cols[col].Name == "Marks" {
				return m.renderFlagsCell(marks, row)
			}
			return resolvedCellByID(resolved, items[row], cols[col].Name)
		},
		Marks:        marks,
		Gutters:      left,
		RightGutters: right,
		Empty:        empty,
		Err:          resErr,
		FooterExtras: s.FooterExtras,
		DataVersion:  listVersionWithStore(lv.Version(), m),
	}

	if s.Recolor != nil {
		model.Recolor = func(row, col int, base lipgloss.Style) lipgloss.Style {
			if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
				return base
			}
			return s.Recolor(items, row, col, cols[col].Name, base)
		}
	}

	return model, true
}

// marksFor returns the cached marks slice — internal helper that
// recomputes if the spec didn't pre-cache (defensive; the build
// path always pre-caches but the sort-install callback may fire
// before that).
func (s ListViewTableSpec[T]) marksFor(items []T) []uilayout.RowMark {
	if s.Marks == nil {
		return nil
	}
	return s.Marks(items)
}

// listSurfaceFromSpec wires a ListViewTableSpec[T] into a complete
// listSurface, filling in the Cols/State/SearchPtr/MoveCursor/
// ResetCursor closures from the spec's accessors. Saves another
// ~6 lines of boilerplate per surface.
func listSurfaceFromSpec[T any](spec ListViewTableSpec[T]) listSurface {
	return listSurface{
		State: func(d *orgData) *uilayout.ListTableState {
			if d == nil {
				return nil
			}
			return spec.StatePtr(d)
		},
		Cols: func() []uilayout.ListColumn {
			return schemaListColumns(spec.Schema)
		},
		SearchPtr: func(d *orgData) *searchState {
			if d == nil {
				return nil
			}
			return spec.ListPtr(d).SearchPtr()
		},
		MoveCursor: func(d *orgData, n int) {
			if d == nil {
				return
			}
			spec.ListPtr(d).MoveBy(n)
		},
		ResetCursor: func(d *orgData) {
			if d == nil {
				return
			}
			spec.ListPtr(d).ResetCursor()
		},
		BuildRenderModel: spec.Build,
		BulkTagTargets: func(d *orgData) (devproject.ItemKind, []string, bool) {
			if spec.RowKindRef == nil || d == nil {
				return "", nil, false
			}
			items := spec.ListPtr(d).Filtered()
			if len(items) == 0 {
				return "", nil, false
			}
			kind, _ := spec.RowKindRef(items[0])
			refs := make([]string, 0, len(items))
			for _, it := range items {
				_, ref := spec.RowKindRef(it)
				if ref != "" {
					refs = append(refs, ref)
				}
			}
			return kind, refs, len(refs) > 0
		},
	}
}
