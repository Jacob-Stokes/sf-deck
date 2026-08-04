package ui

// /flows list surface — see list_surface.go for the listSurface type.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var flowsListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.FlowsTableState },
	Cols:        flowListCols,
	SearchPtr:   func(d *orgData) *searchState { return d.FlowList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.FlowList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.FlowList.ResetCursor() },
	BulkTagTargets: func(d *orgData) (devproject.ItemKind, []string, bool) {
		items := d.FlowList.Filtered()
		refs := make([]string, 0, len(items))
		for _, f := range items {
			if f.DefinitionID != "" {
				refs = append(refs, f.DefinitionID)
			}
		}
		return devproject.KindFlow, refs, len(refs) > 0
	},
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(flowColumnSchema())
		cols := resolved.ListColumns()
		cols = m.applyFlagsColumnMode(cols)
		installListViewOrderRows(&d.FlowList, &d.FlowsTableState, cols,
			func(items []sf.Flow, row, col int) string {
				if col < 0 || col >= len(cols) {
					return ""
				}
				if col < len(cols) && cols[col].Name == "Marks" {
					return m.renderFlagsCell(marksForFlowList(items), row)
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.FlowList.Filtered()
		tagMap := m.bulkTagsForFlows(items)
		projMap := m.bulkProjectsForFlows(items)
		marks := marksForFlowList(items)
		left, right := m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindFlow, items[row].DefinitionID, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindFlow, items[row].DefinitionID, projMap)
			},
		)
		return listRenderModel{
			Title: "FLOWS · " + humanAge(d.Flows.FetchedAt()) +
				stateSuffix(d.Flows.Busy(), d.Flows.Err()),
			State:  &d.FlowsTableState,
			Search: d.FlowList.SearchPtr(),
			Err:    d.Flows.Err(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.FlowList.Cursor(),
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
			Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
				if row < 0 || row >= len(items) {
					return base
				}
				f := items[row]
				// Name col tracks active status; Status col gets the
				// per-state palette. Other cols keep the declared style.
				if col < 0 || col >= len(cols) {
					return base
				}
				switch cols[col].Name {
				case "Name":
					return base.Foreground(flowNameColor(f))
				case "Status":
					return base.Foreground(flowStatusColor(f.Status))
				}
				return base
			},
			Empty:       "  no flows",
			DataVersion: listVersionWithStore(d.FlowList.Version(), m),
		}, true
	},
}
