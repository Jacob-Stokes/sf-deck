package ui

// /dev-projects detail · Items list surface. Replaces the legacy
// hand-rolled hierarchical-nest renderer with a proper sortable
// list table that handles all 19 ItemKinds uniformly.

import (
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var devProjectItemsListSurface = listSurface{
	State: func(d *orgData) *uilayout.ListTableState {
		if d == nil {
			return nil
		}
		return &d.DevProjectItemsTable
	},
	Cols: devProjectItemListCols,
	SearchPtr: func(d *orgData) *searchState {
		if d == nil {
			return nil
		}
		return d.DevProjectItems.SearchPtr()
	},
	MoveCursor: func(d *orgData, n int) {
		if d == nil {
			return
		}
		d.DevProjectItems.MoveBy(n)
	},
	ResetCursor: func(d *orgData) {
		if d == nil {
			return
		}
		d.DevProjectItems.ResetCursor()
	},
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(devProjectItemColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.DevProjectItems, &d.DevProjectItemsTable, cols,
			func(items []devproject.Item, row, col int) string {
				if col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.DevProjectItems.Filtered()
		title := fmt.Sprintf("ITEMS · %d", len(items))
		return listRenderModel{
			Title:  title,
			State:  &d.DevProjectItemsTable,
			Search: d.DevProjectItems.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.DevProjectItems.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				if col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedCellByID(resolved, items[row], cols[col].Name)
			},
			Empty:       "  no items in this view",
			DataVersion: listVersionWithStore(d.DevProjectItems.Version(), m),
		}, true
	},
}

// devProjectItemListCols is the published column list — uses the
// schema's DefaultColumns under the hood so the surface keeps a
// single source of truth.
func devProjectItemListCols() []uilayout.ListColumn {
	return mustResolveColumns(devProjectItemColumnSchema()).ListColumns()
}
