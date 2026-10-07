package ui

// Bottom hint-line for list-table surfaces. One helper, every surface,
// so users see the same affordances on /objects, /flows, /records,
// SOQL — wherever a list-table is shown.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// listTableHint composes the standard hint for a list-table surface.
//
//	state    — the surface's ListTableState (nil → no list-table modes)
//	res      — most recent layout resolution (for overflow detection)
//	totalCols — len(spec.Cols), used for "X / Y →" indicator
//	search   — the active surface's search-state pointer (or nil)
//	extras   — surface-specific keys to append (e.g. "↵ open · r refresh")
//
// Returns just the body text — caller wraps with dimLine + width.
//
// Column-resize gestures (grow/shrink/snap) and the "i info" hint live
// HERE (the main-panel footer), not the global status bar — they only
// apply to list surfaces. `i` is shown only when the sidebar is hidden
// (m.infoHintForHiddenSidebar), since otherwise the info is already on
// screen in the panel.
func (m Model) listTableHint(
	state *uilayout.ListTableState,
	res uilayout.ResolvedWidths,
	totalCols int,
	search *searchState,
	extras string,
) string {
	// search-state copy lives entirely on the top SearchBar now
	// (one line per state: idle / active / committed). Repeating it
	// at the bottom of the same panel was just visual noise —
	// every search hint already shows up two rows above this one.
	// Bottom hint stays focused on table-mode keys (column toggles,
	// overflow scroll, ...) and surface-specific extras.
	_ = search
	switch {
	case res.Overflow:
		parts := []string{
			"  ← → scroll cols (" + uilayout.HScrollIndicator(res, totalCols) + ")",
			firstPretty(Keys.ColSort) + " sort",
			m.columnResizeHint(),
			firstPretty(Keys.ZenMode) + " zen",
			"esc back",
		}
		// Row-specific annotation (e.g. the flow "v3 (v4)" draft hint)
		// still applies while columns overflow — the VERSION column may
		// well be one of the visible ones.
		if h := m.cursoredRowHint(); h != "" {
			parts = append(parts, h)
		}
		return joinNonEmpty(parts, " · ")
	default:
		base := []string{
			firstPretty(Keys.ColSort) + " sort",
			firstPretty(Keys.Paginate) + " page",
		}
		if m.surfaceIsTaggable() {
			base = append(base,
				firstPretty(Keys.TagColumn)+" tag col",
				firstPretty(Keys.ProjectColumn)+" proj col",
				firstPretty(Keys.FlagColumn)+" flag col",
			)
		}
		base = append(base, m.columnResizeHint())
		parts := []string{"  " + joinNonEmpty(base, " · ")}
		if extras != "" {
			parts = append(parts, extras)
		}
		if h := m.cursoredRowHint(); h != "" {
			parts = append(parts, h)
		}
		if h := m.infoHintForHiddenSidebar(); h != "" {
			parts = append(parts, h)
		}
		return joinNonEmpty(parts, " · ")
	}
}

func (m Model) cursoredRowHint() string {
	if m.tab() == TabFlows {
		if d := m.activeOrgData(); d != nil {
			if f, ok := d.FlowList.Selected(); ok && flowVersionMismatch(f) {
				return "(v" + itoa(f.LatestVersionNum) + ") = newer " + flowLatestStatusWord(f) + " version"
			}
		}
	}
	return ""
}

func (m Model) surfaceIsTaggable() bool {
	spec, sub := m.activeSpec()
	return (sub != nil && sub.Identity != nil) ||
		(spec != nil && spec.Identity != nil)
}

func (m Model) columnResizeHint() string {
	return firstPretty(Keys.ColShrink) + firstPretty(Keys.ColGrow) + " resize"
}

func (m Model) infoHintForHiddenSidebar() string {
	spec, sub := m.activeSpec()
	hasSidebar := (sub != nil && sub.Sidebar != nil) ||
		(spec != nil && spec.Sidebar != nil)
	if !hasSidebar {
		return "" // nothing to inspect
	}
	if m.sidebarOpen {
		return "" // panel already on screen (beside or stacked)
	}
	return firstPretty(Keys.InspectPanel) + " info"
}

func joinNonEmpty(parts []string, sep string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
