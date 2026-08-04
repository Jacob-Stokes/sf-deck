package ui

// /apex Classes + Triggers list surfaces. Both follow the static-
// list-with-schema shape — declared via ListViewTableSpec[T] so
// the per-surface code is just the schema, list/state accessors,
// title, marks, gutters, and the Valid-red recolor rule.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var apexClassesTableSpec = ListViewTableSpec[sf.ApexClassRow]{
	Schema:     apexClassColumnSchema(),
	RowKindRef: func(a sf.ApexClassRow) (devproject.ItemKind, string) { return devproject.KindApexClass, a.ID },
	ListPtr:    func(d *orgData) *ListView[sf.ApexClassRow] { return &d.ApexClassList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.ApexClassesTableState },
	ResErr:     func(d *orgData) error { return d.ApexClasses.Err() },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.ApexClassRow) string {
		return "APEX CLASSES · " + humanAge(d.ApexClasses.FetchedAt()) +
			stateSuffix(d.ApexClasses.Busy(), d.ApexClasses.Err())
	},
	Marks: marksForApexClassList,
	Gutters: func(m Model, items []sf.ApexClassRow) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
		tagMap := m.bulkTagsForApexClasses(items)
		projMap := m.bulkProjectsForApexClasses(items)
		return m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindApexClass, items[row].ID, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindApexClass, items[row].ID, projMap)
			},
		)
	},
	Recolor: func(items []sf.ApexClassRow, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		// Tint Valid red when the class is invalid — same visual
		// cue the bespoke renderer used.
		if colName == "Valid" && !items[row].IsValid {
			return base.Foreground(theme.Red)
		}
		return base
	},
	Empty: "  no apex classes in this org",
}

var apexClassesListSurface = listSurfaceFromSpec(apexClassesTableSpec)

var apexTriggersTableSpec = ListViewTableSpec[sf.TriggerRow]{
	Schema:     apexTriggerColumnSchema(),
	RowKindRef: func(t sf.TriggerRow) (devproject.ItemKind, string) { return devproject.KindApexTrigger, t.ID },
	ListPtr:    func(d *orgData) *ListView[sf.TriggerRow] { return &d.ApexTriggerList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.ApexTriggersTableState },
	ResErr:     func(d *orgData) error { return d.ApexTriggersFlat.Err() },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.TriggerRow) string {
		return "TRIGGERS · " + humanAge(d.ApexTriggersFlat.FetchedAt()) +
			stateSuffix(d.ApexTriggersFlat.Busy(), d.ApexTriggersFlat.Err())
	},
	Marks: marksForApexTriggerList,
	Gutters: func(m Model, items []sf.TriggerRow) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
		tagMap := m.bulkTagsForApexTriggers(items)
		projMap := m.bulkProjectsForApexTriggers(items)
		return m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindApexTrigger, items[row].ID, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindApexTrigger, items[row].ID, projMap)
			},
		)
	},
	Recolor: func(items []sf.TriggerRow, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		if colName == "Valid" && !items[row].Valid {
			return base.Foreground(theme.Red)
		}
		return base
	},
	Empty: "  no triggers in this org",
}

var apexTriggersListSurface = listSurfaceFromSpec(apexTriggersTableSpec)
