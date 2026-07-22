package ui

// /components LWC + Aura list surfaces — declared via
// ListViewTableSpec[T]. LWC gets a green Exposed-cell tint;
// Aura has no equivalent flag.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var lwcTableSpec = ListViewTableSpec[sf.LWCBundle]{
	Schema:     lwcColumnSchema(),
	RowKindRef: func(b sf.LWCBundle) (devproject.ItemKind, string) { return devproject.KindLWC, b.ID },
	ListPtr:    func(d *orgData) *ListView[sf.LWCBundle] { return &d.LWCBundleList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.LWCBundlesTableState },
	ResErr:     func(d *orgData) error { return d.LWCBundles.Err() },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.LWCBundle) string {
		return "LWC · " + humanAge(d.LWCBundles.FetchedAt()) +
			stateSuffix(d.LWCBundles.Busy(), d.LWCBundles.Err())
	},
	Marks: marksForLWCList,
	Gutters: func(m Model, items []sf.LWCBundle) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
		tagMap := m.bulkTagsForBundles(devproject.KindLWC, items, nil)
		projMap := m.bulkProjectsForBundles(devproject.KindLWC, items, nil)
		return m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindLWC, items[row].ID, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindLWC, items[row].ID, projMap)
			},
		)
	},
	Recolor: func(items []sf.LWCBundle, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		if colName == "Exposed" && items[row].IsExposed {
			return base.Foreground(theme.Green)
		}
		return base
	},
	Empty: "  no LWCs in this org",
}

var lwcListSurface = listSurfaceFromSpec(lwcTableSpec)

var auraTableSpec = ListViewTableSpec[sf.AuraBundle]{
	Schema:     auraColumnSchema(),
	RowKindRef: func(b sf.AuraBundle) (devproject.ItemKind, string) { return devproject.KindAura, b.ID },
	ListPtr:    func(d *orgData) *ListView[sf.AuraBundle] { return &d.AuraBundleList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.AuraBundlesTableState },
	ResErr:     func(d *orgData) error { return d.AuraBundles.Err() },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.AuraBundle) string {
		return "AURA · " + humanAge(d.AuraBundles.FetchedAt()) +
			stateSuffix(d.AuraBundles.Busy(), d.AuraBundles.Err())
	},
	Marks: marksForAuraList,
	Gutters: func(m Model, items []sf.AuraBundle) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
		tagMap := m.bulkTagsForBundles(devproject.KindAura, nil, items)
		projMap := m.bulkProjectsForBundles(devproject.KindAura, nil, items)
		return m.listGutters(
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return m.resolveTagGutterCell(devproject.KindAura, items[row].ID, tagMap)
			},
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return rowProjectGutterFromMap(devproject.KindAura, items[row].ID, projMap)
			},
		)
	},
	Empty: "  no Aura components in this org",
}

var auraListSurface = listSurfaceFromSpec(auraTableSpec)
