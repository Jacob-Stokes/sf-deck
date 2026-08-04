package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestVisiblePinnedTabsMatchesRenderer is the regression guard for the
// bug where numbered tabs that don't fit the strip failed to appear in
// the 0 More… overflow picker. visiblePinnedTabs (which the picker uses
// to decide "what's hidden") and the actual renderer must agree on
// which numbered tabs survive at every width — they now share
// buildTabBarPills + tabRowKeep + tabRowLeftBudget, so this asserts the
// two never drift.
//
// For each width we compute the renderer's keep AT THE WIDTH THE FRAME
// ACTUALLY RENDERS THE BAR (mainTabBarWidth — inner width minus edge
// gutters minus the open rail), and compare to visiblePinnedTabs().
// The original version of this test computed the expected keep at the
// raw terminal width — the exact mistake visiblePinnedTabs itself made
// — so both agreed with each other while disagreeing with the real
// strip: at boundary widths a pinned tab (/system, in the field report)
// was dropped from the strip yet excluded from the More… modal,
// becoming unreachable. Rail-open is covered too: the rail eats
// 24-34 columns of the bar, the largest source of drift.
func TestVisiblePinnedTabsMatchesRenderer(t *testing.T) {
	// Widths from "everything fits" down to "almost nothing fits";
	// step 1 so fit-boundary widths (the bug's habitat) are all hit.
	for _, leftOpen := range []bool{false, true} {
		for width := 40; width <= 220; width++ {
			m := Model{}
			m.width = width
			m.leftOpen = leftOpen

			pills, activeIdx, tabBySlot := m.buildTabBarPills()
			rightPill, _ := m.renderRightNavPills()
			barW := m.mainTabBarWidth()
			budget := tabRowLeftBudget(barW, lipgloss.Width(rightPill))
			keep := tabRowKeep(pills, activeIdx, budget)

			rendererVisible := map[Tab]bool{}
			for i := range tabBySlot {
				if keep[i] {
					rendererVisible[tabBySlot[i]] = true
				}
			}

			got := m.visiblePinnedTabs()
			for _, tab := range tabBySlot {
				if got[tab] != rendererVisible[tab] {
					t.Errorf("width=%d leftOpen=%v tab=%v: visiblePinnedTabs=%v, renderer keeps=%v — a dropped tab must be reachable via 0 More…",
						width, leftOpen, tab, got[tab], rendererVisible[tab])
				}
			}
		}
	}
}

// TestNarrowStripDropsTabsIntoOverflow checks the end-to-end intent: at
// a width too small for all numbered tabs, at least one numbered tab is
// hidden AND the More… sticky pill still renders (so the picker is
// reachable). A regression here means either nothing drops (fit math
// too generous) or the escape hatch itself got dropped.
func TestNarrowStripDropsTabsIntoOverflow(t *testing.T) {
	const width = 60 // narrow but realistic (below any real terminal is degenerate)
	m := Model{}
	m.width = width

	visible := m.visiblePinnedTabs()
	_, _, tabBySlot := m.buildTabBarPills()

	hidden := 0
	for _, tab := range tabBySlot {
		if !visible[tab] {
			hidden++
		}
	}
	if hidden == 0 {
		t.Fatalf("width=%d: expected some numbered tabs to be hidden, got all %d visible", width, len(tabBySlot))
	}

	// The More… escape hatch must survive (sticky pill) so the hidden
	// tabs stay reachable.
	strip, _ := m.renderTabBar(m.width)
	more := firstPretty(Keys.Tab0) + " more…"
	if !strings.Contains(strip, more) {
		t.Errorf("width=%d: strip must keep the %q escape hatch so dropped tabs are reachable", width, more)
	}
}
