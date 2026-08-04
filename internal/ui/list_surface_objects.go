package ui

// /objects list surface — see list_surface.go for the listSurface type.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var objectsListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.ObjectsTableState },
	Cols:        sobjectListCols,
	SearchPtr:   func(d *orgData) *searchState { return d.SObjectList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.SObjectList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.SObjectList.ResetCursor() },
	BulkTagTargets: func(d *orgData) (devproject.ItemKind, []string, bool) {
		items := d.SObjectList.Filtered()
		refs := make([]string, 0, len(items))
		for _, o := range items {
			if o.Name != "" {
				refs = append(refs, o.Name)
			}
		}
		return devproject.KindSObject, refs, len(refs) > 0
	},
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(sobjectColumnSchema())
		cols := m.applyFlagsColumnMode(resolved.ListColumns())
		installListViewOrderRows(&d.SObjectList, &d.ObjectsTableState, cols,
			func(items []sf.SObject, row, col int) string {
				if col < 0 || col >= len(cols) {
					return ""
				}
				if col < len(cols) && cols[col].Name == "Marks" {
					return m.renderFlagsCell(marksForSObjectList(items), row)
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.SObjectList.Filtered()
		tagMap := m.bulkTagsForObjects(items)
		projMap := m.bulkProjectsForObjects(items)
		marks := marksForSObjectList(items)
		left, right := m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindSObject, items[row].Name, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindSObject, items[row].Name, projMap)
			},
		)
		return listRenderModel{
			Title: "SOBJECTS · " + humanAge(d.SObjects.FetchedAt()) +
				stateSuffix(d.SObjects.Busy(), d.SObjects.Err()),
			State:  &d.ObjectsTableState,
			Search: d.SObjectList.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.SObjectList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				if col < 0 || col >= len(cols) {
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
			Empty:        "  no matches",
			DataVersion:  listVersionWithStore(d.SObjectList.Version(), m),
		}, true
	},
}
