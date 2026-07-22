package ui

// /reports subtab surfaces: Dashboards + Report Types. Both pure
// spec-derived list surfaces — chips, sort, columns, widths all come
// from the shared engines.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var dashboardsTableSpec = ListViewTableSpec[sf.DashboardRow]{
	Schema:   dashboardColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.DashboardRow] { return &d.DashboardList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.DashboardsTableState },
	Title: func(m Model, d *orgData, items []sf.DashboardRow) string {
		return standardListTitle("DASHBOARDS", d.DashboardList.Len(), &d.Dashboards)
	},
	ResErr: func(d *orgData) error { return d.Dashboards.Err() },
	Empty:  "  no dashboards in this org",
}

var dashboardsListSurface = listSurfaceFromSpec(dashboardsTableSpec)

var reportTypesTableSpec = ListViewTableSpec[sf.ReportTypeRow]{
	Schema:   reportTypeColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.ReportTypeRow] { return &d.ReportTypeList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.ReportTypesTableState },
	Title: func(m Model, d *orgData, items []sf.ReportTypeRow) string {
		return standardListTitle("REPORT TYPES", d.ReportTypeList.Len(), &d.ReportTypes)
	},
	ResErr: func(d *orgData) error { return d.ReportTypes.Err() },
	Empty:  "  no report types visible",
}

var reportTypesListSurface = listSurfaceFromSpec(reportTypesTableSpec)
