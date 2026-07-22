package ui

// Bottom hint-line for list-table surfaces. One helper, every surface,
// so users see the same affordances on /objects, /flows, /records,
// SOQL — wherever a list-table is shown.
//
// Mode-sensitive: when horizontal-scroll overflow is active the hint
// surfaces the column-scroll keys instead of the surface defaults.
// Search state copy lives entirely on the top SearchBar — repeating
// it here was duplicated chrome two rows apart.
//
// Surfaces still get to add their own keys (e.g. "↵ open · r refresh"
// on /objects) by passing them as the trailing extras.

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
		// Default mode: list-specific affordances. Truly global keys
		// (drill, open, refresh, zen, …) live in the persistent
		// status bar; list-table-only keys live here so the footer
		// doesn't claim global space for things that only apply to
		// list surfaces.
		//
		// Order: per-row interaction (sort, paginate) first, then
		// column-mode toggles (tag/project/flag), then column resize,
		// then any surface extras, then the conditional info hint.
		base := []string{
			firstPretty(Keys.ColSort) + " sort",
			firstPretty(Keys.Paginate) + " page",
		}
		// The tag / project / flag column toggles only do anything on
		// surfaces whose rows are taggable/collectable (i.e. declare an
		// Identity). On operational surfaces like sessions, audit trail,
		// or flow interviews there's nothing to tag, so hide the hints
		// rather than advertise keys that no-op there.
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

// cursoredRowHint returns a hint that explains something about the
// specific row under the cursor, shown only when that row warrants it.
// Currently: on /flows, when the highlighted flow's newest version is
// a later draft than its active one (rendered "v3 (v4)" in the VERSION
// column), it explains the bracketed number. Empty when the cursored
// row needs no annotation.
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

// surfaceIsTaggable reports whether the current surface's rows can be
// tagged / collected — i.e. it declares an Identity resolver (on the
// subtab or its parent tab). Drives whether the tag/proj/flag column
// hints appear in the footer.
func (m Model) surfaceIsTaggable() bool {
	spec, sub := m.activeSpec()
	return (sub != nil && sub.Identity != nil) ||
		(spec != nil && spec.Identity != nil)
}

// columnResizeHint renders the compact column-resize gesture legend:
// grow / shrink keys plus the snap pair. Reads live bindings so a
// rebind updates the hint.
func (m Model) columnResizeHint() string {
	return firstPretty(Keys.ColShrink) + firstPretty(Keys.ColGrow) + " resize"
}

// infoHintForHiddenSidebar returns the "i info" hint ONLY when the
// current surface HAS an info panel that is currently NOT on screen —
// i.e. the tab/subtab registers a Sidebar but the user has toggled the
// sidebar off (\). When the panel is visible its info is already
// showing, and when the surface has no sidebar at all there's nothing
// to inspect, so in both those cases the hint is suppressed.
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
	// Sidebar exists but is hidden — offer the inspect shortcut.
	return firstPretty(Keys.InspectPanel) + " info"
}

// joinNonEmpty joins the non-empty parts with sep. Keeps the hint from
// growing stray "  ·  ·" gaps when an optional segment is blank.
func joinNonEmpty(parts []string, sep string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
