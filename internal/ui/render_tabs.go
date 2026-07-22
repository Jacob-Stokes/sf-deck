package ui

// Top-level tab bar rendering.
//
// The strip sits between renderHeader (logo + org pill + breadcrumb)
// and the body panes, replacing the old bottom-status-bar tab
// switcher. Each tab renders as a bordered pill with its nav number
// baked into the label ("1 home", "2 soql", …) so the 1-9 key
// contract is self-documenting without a separate legend row.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type renderedPill struct {
	text string
	id   string
	// sticky pills are never dropped by fitTabRowLayers on overflow.
	// Used for the "0 more…" pill (which is the user's escape hatch
	// to reach dropped tabs via the modal) and the active overflow
	// slot 9 (which represents wherever the user currently is). Drop
	// non-sticky middle pills first; the user can still reach them
	// via the More… modal that the sticky pill anchors.
	sticky bool
}

// renderTabBar draws the top tab strip at the given width. If the
// full pill set doesn't fit, trailing pills are dropped and a "…"
// marker is appended. The returned string is exactly `width` cols on
// every row so it aligns flush with the panes underneath.
//
// Right-aligned navigation pills are always rendered and never
// dropped; the numbered pills share the remaining space. The right
// strip (in display order, right-most last):
//   - "= ⚙ settings" — opens the settings modal
//   - "- Dev Projects" — opens the master DevProject list
//   - "_ <name>"      — only when the active org has a project loaded;
//     opens the loaded project's detail tab
//
// buildTabBarPills constructs the full tab-row pill list (numbered
// slots 1-8, spacer, sticky overflow slot 9 + slot 0) exactly as the
// renderer lays it out, and returns the active pill's index plus a
// parallel slice mapping each NUMBERED pill index to its Tab. Shared
// by renderTabBar and visiblePinnedTabs so the overflow picker's
// notion of "which tabs are hidden" is derived from the same pills the
// renderer actually fits — they can never drift.
//
// tabBySlot has one entry per numbered pill (in pill-index order); the
// spacer / overflow / more pills that follow have no tabBySlot entry.
func (m Model) buildTabBarPills() (pills []renderedPill, activeIdx int, tabBySlot []Tab) {
	views := TabsForNumbers()
	viewKeys := [][]string{
		Keys.Tab1, Keys.Tab2, Keys.Tab3, Keys.Tab4, Keys.Tab5,
		Keys.Tab6, Keys.Tab7, Keys.Tab8, Keys.Tab9,
	}
	activeStem := m.stemForTab(m.tab())
	activeIdx = -1
	// Slot 1-8: the user's pinned tabs.
	for i, v := range views {
		if i >= 8 || i >= len(viewKeys) {
			break
		}
		n := firstPretty(viewKeys[i])
		if n == "" {
			continue
		}
		label := n + " " + v.String()
		if s := m.searchStateForTab(v); s != nil && s.Applied() {
			label += "⌕"
		}
		active := v == activeStem
		if active {
			activeIdx = len(pills)
		}
		pills = append(pills, renderedPill{
			text: renderTabPill(label, active),
			id:   zoneTabID(v),
		})
		tabBySlot = append(tabBySlot, v)
	}
	// Visual gap between the pinned slots (1-8) and the overflow
	// cluster (slot 9 + slot 0). A no-id pill of fixed width so
	// the trailing pills don't shift when slot 9 appears.
	pills = append(pills, renderedPill{
		text: renderTabBarSpacer(2),
	})

	// Slot 9 — active overflow tab. Only rendered when set.
	// Marked sticky so it survives narrow windows: this is "where you
	// currently are" and dropping it would be disorienting.
	if m.overflowSet {
		ninePretty := firstPretty(Keys.Tab9)
		if ninePretty != "" {
			label := ninePretty + " " + m.overflowTab.String()
			active := activeStem == m.overflowTab
			if active {
				activeIdx = len(pills)
			}
			pills = append(pills, renderedPill{
				text:   renderOverflowPill(label, active),
				id:     zoneTabID(m.overflowTab),
				sticky: true,
			})
		}
	}

	// Slot 0 — More… modal trigger. Sticky: this is the user's
	// escape hatch to reach any pill that got dropped on overflow,
	// so it must remain reachable at every window size. Without
	// this, narrow windows silently hide both the dropped tabs AND
	// the way to find them.
	if morePretty := firstPretty(Keys.Tab0); morePretty != "" {
		pills = append(pills, renderedPill{
			text:   renderOverflowPill(morePretty+" more…", false),
			id:     zoneTabOverflow,
			sticky: true,
		})
	}
	return pills, activeIdx, tabBySlot
}

func (m Model) renderTabBar(width int) (string, []*lipgloss.Layer) {
	pills, activeIdx, _ := m.buildTabBarPills()
	rightPill, rightLayers := m.renderRightNavPills()
	return fitTabRowWithRightLayers(pills, activeIdx, rightPill, rightLayers, width)
}

// renderRightNavPills builds the right-aligned cluster of nav pills.
// Order (left-to-right within the cluster): loaded-project pill (if a
// project is loaded), Dev Projects pill, settings pill. The settings
// pill stays right-most since it's always-present and matches the
// historic visual landmark.
func (m Model) renderRightNavPills() (string, []*lipgloss.Layer) {
	var pills []renderedPill

	// Active-stem highlight: when the user IS on the corresponding tab
	// the pill renders with the same active style as a numbered pill.
	activeStem := m.stemForTab(m.tab())

	// `_ <project-name>` pill — only when a project is loaded AND
	// the user has at least one org (no scope without an org).
	if scope := m.activeScope(); scope.Loaded() {
		label := firstPretty(Keys.LoadOrgProject) + " " + scope.ProjectName
		// Active when on the dev-project detail tab AND the drilled-in
		// project matches the loaded one (most common case — opening
		// via the pill drills exactly that).
		active := false
		if m.tab() == TabDevProjectDetail {
			if d := m.activeOrgData(); d != nil && d.LoadedDevProjectID == m.devProjectCur {
				active = true
			}
		}
		pills = append(pills, renderedPill{
			text: renderTabPill(label, active),
			id:   zoneNavLoadedProject,
		})
	}

	// `- Dev Projects` pill — always present.
	devLabel := firstPretty(Keys.OpenDevProjects) + " Dev Projects"
	devActive := activeStem == TabDevProjects
	pills = append(pills, renderedPill{
		text: renderTabPill(devLabel, devActive),
		id:   zoneNavDevProjects,
	})

	// `# Tags` pill — always present. Opens the tag manager.
	tagsLabel := firstPretty(Keys.OpenTags) + " Tags"
	tagsActive := activeStem == TabTags
	pills = append(pills, renderedPill{
		text: renderTabPill(tagsLabel, tagsActive),
		id:   zoneNavTags,
	})

	// Settings pill used to live here but its key hint moved to the
	// global status bar — every other shortcut surfaces there too,
	// so the dedicated pill was duplicative chrome.

	return joinPillsHorizontalLayers(pills)
}

// renderLeftTabBar draws the "0 Orgs" pill above the left rail.
// Single pill (Bookmarks panel was removed when Dev Projects moved
// to the right-rail nav). Kept rather than removed because the
// keyboard affordance for `0` is non-obvious without a visible
// label, and the pill's active styling is the only on-screen
// indicator of which side has focus.
func (m Model) renderLeftTabBar(width int) (string, []*lipgloss.Layer) {
	utils := leftrailUtilities()
	if len(utils) == 0 {
		return "", nil
	}
	focused := m.focus == focusOrgs
	keys := []string{firstPretty(Keys.FocusOrgs)}
	pills := make([]renderedPill, 0, len(utils))
	activeIdx := -1
	for i, u := range utils {
		label := u.Label
		if i < len(keys) && keys[i] != "" {
			label = keys[i] + " " + u.Label
		}
		active := focused
		if active {
			activeIdx = i
		}
		id := zoneNavOrgs
		pills = append(pills, renderedPill{
			text: renderLeftTabPill(label, active, focused),
			id:   id,
		})
	}
	return fitTabRowLayers(pills, activeIdx, width)
}

// joinPillsHorizontal lays the pills out side-by-side with no gap so
// they read as one block. Each pill is multi-line (rounded border)
// and lipgloss handles the alignment. Returns "" for empty input so
// fitTabRowWithRight's "no right pill" path fires cleanly.
func joinPillsHorizontal(pills []string) string {
	if len(pills) == 0 {
		return ""
	}
	if len(pills) == 1 {
		return pills[0]
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, pills...)
}

func joinPillsHorizontalLayers(pills []renderedPill) (string, []*lipgloss.Layer) {
	if len(pills) == 0 {
		return "", nil
	}
	texts := make([]string, 0, len(pills))
	layers := make([]*lipgloss.Layer, 0, len(pills))
	x := 0
	for _, pill := range pills {
		texts = append(texts, pill.text)
		layers = append(layers, lipgloss.NewLayer(pill.text).X(x).Y(0).Z(1).ID(pill.id))
		x += lipgloss.Width(pill.text)
	}
	return joinPillsHorizontal(texts), layers
}

// renderTabPill draws one tab cell. All pills use a full rounded
// border; the active tab pops via BorderHi + bold bright text, while
// inactive tabs use muted border + muted foreground.
func renderTabPill(label string, active bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Foreground(theme.Muted)
	if active {
		style = style.
			BorderForeground(theme.BorderHi).
			Foreground(theme.Fg).
			Bold(true)
	}
	return style.Render(label)
}

// renderOverflowPill is the styling variant used for slot 0 (More…)
// and slot 9 (active overflow tab). Visually distinct from the
// pinned slots so the user recognises them as "extras" without
// reading the labels — italic + a softer dim border.
func renderOverflowPill(label string, active bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.FgDim).
		Padding(0, 1).
		Foreground(theme.FgDim).
		Italic(true)
	if active {
		style = style.
			BorderForeground(theme.BorderHi).
			Foreground(theme.Fg).
			Bold(true).
			Italic(false)
	}
	return style.Render(label)
}

// visiblePinnedTabs returns the set of pinned tabs (slots 1-8) the
// strip would actually show at the current Model width. Used by the
// More… modal to filter out pinned-and-visible tabs so the modal
// only ever lists tabs the user can't reach from the strip right
// now: non-pinned tabs (their permanent home) plus pinned tabs the
// fit logic dropped on a narrow window.
//
// Re-uses the same pill-construction shape as renderTabBar so the
// drop decisions stay in sync. Width-only — doesn't render anything,
// just answers "which pinned slots survive."
func (m Model) visiblePinnedTabs() map[Tab]bool {
	// MUST match the width the renderer actually gives the main tab bar
	// (mainTabBarWidth), not the raw terminal width. Using m.width here
	// budgeted 2 extra columns (the edge gutters) — more with the left
	// rail open — so at boundary widths a pinned tab could be dropped by
	// the renderer yet counted visible here, vanishing from BOTH the
	// strip and the More… modal (field bug: /system unreachable).
	width := m.mainTabBarWidth()
	if width <= 0 {
		// No width yet (very first frame); assume everything fits.
		out := map[Tab]bool{}
		for _, v := range TabsForNumbers() {
			out[v] = true
		}
		return out
	}
	// Build the EXACT pill list the renderer fits, and apply the SAME
	// right-cluster budget rule as fitTabRowWithRightLayers — so a tab
	// this reports as hidden is exactly a tab renderTabBar dropped, and
	// the 0 More… picker offers it.
	pills, activeIdx, tabBySlot := m.buildTabBarPills()
	rightPill, _ := m.renderRightNavPills()
	budget := tabRowLeftBudget(width, lipgloss.Width(rightPill))
	keep := tabRowKeep(pills, activeIdx, budget)
	out := map[Tab]bool{}
	for i := 0; i < len(tabBySlot); i++ {
		if keep[i] {
			out[tabBySlot[i]] = true
		}
	}
	return out
}

// tabRowLeftBudget returns the width available to the tab pills once
// the right nav cluster is accounted for — matching exactly how
// fitTabRowWithRightLayers splits the row. When the right cluster alone
// would fill (or overflow) the row, the renderer drops it and fits the
// pills against the FULL width, so we do the same here.
func tabRowLeftBudget(width, rightW int) int {
	if rightW >= width {
		return width
	}
	return width - rightW
}

// renderTabBarSpacer draws the gap between slot 8 and the trailing
// overflow cluster (slots 9 + 0). Renders as a fixed-width pad of
// blanks so the trailing pills don't visually shift when the slot 9
// pill appears or disappears.
func renderTabBarSpacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render(" ")
}

// tabRowKeep is the SINGLE source of truth for which tab pills survive
// a given width. Both the real renderer (fitTabRowLayers) and the
// overflow-modal's visiblePinnedTabs call it, so "which tabs render"
// and "which tabs the 0 More… picker offers" can never disagree — a
// dropped numbered tab is always reachable via overflow.
//
// Three priority tiers:
//  1. Sticky pills ("0 more…", active overflow slot 9) — always kept;
//     the user's anchor + overflow escape hatch.
//  2. The active pill — kept so the user can see where they are.
//  3. Non-sticky, non-active pills — kept greedily from the left until
//     the row's budget runs out.
func tabRowKeep(pills []renderedPill, activeIdx, width int) []bool {
	keep := make([]bool, len(pills))
	if width <= 0 || len(pills) == 0 {
		return keep
	}
	widths := make([]int, len(pills))
	for i, p := range pills {
		widths[i] = lipgloss.Width(p.text)
	}
	used := 0
	for i, p := range pills {
		if p.sticky {
			keep[i] = true
			used += widths[i]
		}
	}
	if activeIdx >= 0 && activeIdx < len(pills) && !keep[activeIdx] {
		keep[activeIdx] = true
		used += widths[activeIdx]
	}
	for i, p := range pills {
		if keep[i] || p.sticky {
			continue
		}
		if used+widths[i] > width {
			continue
		}
		keep[i] = true
		used += widths[i]
	}
	return keep
}

func fitTabRowLayers(pills []renderedPill, activeIdx, width int) (string, []*lipgloss.Layer) {
	if width <= 0 || len(pills) == 0 {
		return "", nil
	}
	// Final assembly preserves the original pill order so the visible
	// row reads left-to-right as authored: pinned slots first, then
	// the overflow cluster on the right.
	keep := tabRowKeep(pills, activeIdx, width)
	// Collect in original order.
	kept := pills[:0:0]
	for i, p := range pills {
		if keep[i] {
			kept = append(kept, p)
		}
	}

	keptTexts := make([]string, 0, len(kept))
	layers := make([]*lipgloss.Layer, 0, len(kept))
	x := 0
	for _, pill := range kept {
		keptTexts = append(keptTexts, pill.text)
		if pill.id != "" {
			layers = append(layers, lipgloss.NewLayer(pill.text).X(x).Y(0).Z(1).ID(pill.id))
		}
		x += lipgloss.Width(pill.text)
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, keptTexts...)
	// Pad each line to exactly `width`.
	lines := strings.Split(joined, "\n")
	for i, ln := range lines {
		w := lipgloss.Width(ln)
		if w < width {
			lines[i] = ln + strings.Repeat(" ", width-w)
		} else if w > width {
			lines[i] = ansi.Truncate(ln, width, "")
		}
	}
	return strings.Join(lines, "\n"), layers
}

func fitTabRowWithRightLayers(
	pills []renderedPill,
	activeIdx int,
	rightPill string,
	rightLayers []*lipgloss.Layer,
	width int,
) (string, []*lipgloss.Layer) {
	if width <= 0 {
		return "", nil
	}
	rightW := lipgloss.Width(rightPill)
	if rightW >= width {
		// Degenerate: right pill alone fills the row. Drop it.
		return fitTabRowLayers(pills, activeIdx, width)
	}
	leftWidth := width - rightW
	leftBlock, layers := fitTabRowLayers(pills, activeIdx, leftWidth)
	if leftBlock == "" {
		// No numbered pills fit; pad blanks on the left.
		leftBlock = strings.Repeat(" ", leftWidth)
	}
	// Join line-by-line so multi-row borders line up.
	leftLines := strings.Split(leftBlock, "\n")
	rightLines := strings.Split(rightPill, "\n")
	// Pad the shorter side vertically with blank lines of the right
	// visual width so JoinHorizontal lays out cleanly.
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, strings.Repeat(" ", rightW))
	}
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, strings.Repeat(" ", leftWidth))
	}
	out := make([]string, len(leftLines))
	for i := range leftLines {
		lw := lipgloss.Width(leftLines[i])
		// Pad left line to exactly leftWidth then append the right pill line.
		if lw < leftWidth {
			leftLines[i] += strings.Repeat(" ", leftWidth-lw)
		} else if lw > leftWidth {
			leftLines[i] = ansi.Truncate(leftLines[i], leftWidth, "")
		}
		out[i] = leftLines[i] + rightLines[i]
	}
	for _, layer := range rightLayers {
		layer.X(layer.GetX() + leftWidth)
		layers = append(layers, layer)
	}
	return strings.Join(out, "\n"), layers
}

// renderLeftTabPill mirrors renderTabPill but in the left rail's
// Magenta accent family, so the two tab bars don't read as a single
// strip. Active pill swaps to BorderHi only when the rail is focused,
// staying calm otherwise so focus is obvious.
func renderLeftTabPill(label string, active, focused bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Foreground(theme.Muted)
	if active {
		accent := theme.Magenta
		if focused {
			accent = theme.BorderHi
		}
		style = style.
			BorderForeground(accent).
			Foreground(theme.Fg).
			Bold(true)
	}
	return style.Render(label)
}
