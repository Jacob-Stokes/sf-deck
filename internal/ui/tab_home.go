package ui

// /home — landing surface.
//
// Layout: pinned ORG card up top, subtab strip, per-subtab list-table
// body. Each subtab now uses the shared list-table primitive
// (uilayout.LayoutListTable + RenderListTableHeader/Row) so search,
// sort, column-mode, and horizontal scroll all behave the same as
// /objects, /flows, /apex, etc.
//
// The Recent + Logs + API + Audit subtabs delegate to other
// renderers (renderRecent, renderApexLogs, renderSystemAPI,
// joinPlaceholder) — same lookup path as the rest of the registry.

import (
	_ "embed"
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// homeLandingLogo is the ASCII banner shown on /home → Landing,
// figlet-rendered "SF-DECK" in the ANSI Shadow font. Embedded as a
// file so trailing whitespace on rows 3-4 (which are otherwise
// uneven) survives — editor whitespace-stripping mangled the
// inline string-literal version, sliding rows by 1-2 cells.
//
// Generated with: figlet -f "ANSI Shadow" SF-DECK
//
// Visual width 52, height 6 (the trailing blank row figlet emits is
// stripped at render time).
//
//go:embed home_logo.txt
var homeLandingLogo string

// renderHome dispatches by subtab. ORG card is always pinned; the
// remaining vertical budget is owned by the subtab body.
func (m Model) renderHome(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		// No org connected yet — show the onboarding panel.
		// renderHomeOnboarding picks the right copy based on whether
		// `sf` is missing (install path) vs. just no orgs configured
		// (login path).
		return m.renderHomeOnboarding(inner, innerH)
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org auth error: run `sf org login web --alias " + o.Alias + "`")
	}
	d := m.data[o.Username]

	var lines []string
	// ORG card moved to the right sidebar (sidebarHome) so the main
	// pane is dedicated to the active subtab's content. The sidebar
	// version pairs with the rotating cloud banner; both pull from
	// d.OrgInfo + d.Home.
	_ = o // keep ref to discourage future mistakes that re-pin a card here
	subs := homeSubtabs()
	sel := m.homeSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}
	strip := renderSubtabStrip(subs, sel, inner)
	if strip != "" {
		lines = append(lines, strip)
	}

	// Hand the subtab the remaining vertical budget.
	budget := innerH - len(lines) - 1
	if budget < 5 {
		budget = 5
	}

	switch subs[sel].ID {
	case SubtabHomeLanding:
		lines = append(lines, m.renderHomeLanding(inner, budget)...)
	case SubtabHomeRecent:
		lines = append(lines, m.renderRecent(inner+4, budget))
	case SubtabHomeNotifications:
		lines = append(lines, m.renderHomeNotifications(d, inner, budget)...)
	case SubtabHomeLimits:
		// Routes through the standard list-table renderer via the
		// homeLimitsListSurface BuildRenderModel — same path /apex
		// and friends use, so scroll / search / sort / column-mode
		// all behave consistently.
		lines = append(lines, renderListSurface(m, &homeLimitsListSurface, inner+4, budget, d))
	case SubtabHomeLicenses:
		// Same shared-renderer path as Limits. UserLicense +
		// PermSetLicense are merged into one list via homeLicenseRow
		// in SyncHomeLists.
		lines = append(lines, renderListSurface(m, &homeLicensesListSurface, inner+4, budget, d))
	case SubtabHomeDownloads:
		lines = append(lines, m.renderHomeDownloads(inner, budget)...)
	}

	return strings.Join(lines, "\n")
}

// Legacy renderHomeOrgCard removed — the ORG identity block now
// lives in sidebarHome (sidebar.go) alongside the rotating cloud
// banner. Contributors who want to update the ORG card should
// edit sidebarHome.

// renderHomeNotifications — bell-stream rows as a list-table.
// renderHomeLanding draws the SF-DECK logo at the top, then a
// "LIGHTNING DESTINATIONS" grid below — categorised admin/data/
// users/code links that open in the user's browser. The sidebar
// carries the org identity + limits + deploys; the main pane is
// the launchpad.
//
// budget is the available content height. The logo is fixed-height;
// the destinations grid takes whatever's left and may overflow
// (no scrolling yet — the catalog is bounded at ~38 entries which
// fits comfortably in two columns on standard 24-row terminals).
func (m Model) renderHomeLanding(inner, budget int) []string {
	// Split the embedded banner. figlet adds a trailing blank line +
	// possibly a final empty after the \n; trim those so the logo
	// block is exactly the printable rows.
	logoLines := strings.Split(strings.TrimRight(homeLandingLogo, "\n"), "\n")
	for len(logoLines) > 0 && strings.TrimSpace(logoLines[len(logoLines)-1]) == "" {
		logoLines = logoLines[:len(logoLines)-1]
	}

	out := make([]string, 0, 64)
	// Top breathing room — two blank lines before the logo.
	out = append(out, "", "")
	for _, ln := range logoLines {
		out = append(out, centerLine(ln, inner, theme.BorderHi, false))
	}
	// Tagline.
	out = append(out, "")
	out = append(out, centerLine("a salesforce TUI", inner, theme.BorderHi, false))
	// Bottom breathing room — three blanks before the destinations grid.
	out = append(out, "", "", "")
	// Destinations grid. destRow is the cursored entry's absolute row
	// within the grid slice; offset it by the lines already in `out` to
	// get its row in the combined block.
	gridStart := len(out)
	grid, destRow := m.renderHomeDestinations(inner)
	out = append(out, grid...)

	cursorAbs := -1
	if destRow >= 0 {
		cursorAbs = gridStart + destRow
	}
	// Top-anchored scroll: keep the logo visible on first paint (cursor
	// starts on the first destination, which is below the fold of a tall
	// logo — a centred 1/3-bias scroll would hide the logo immediately).
	// Only scroll down once the cursor would fall past the bottom. "↑/↓ N
	// more" markers show when the view is windowed.
	return splitScrolled(scrollLinesKeepTop(out, cursorAbs, budget))
}

// splitScrolled re-splits a scrollLinesToCursor result (a joined
// string) back into the []string the landing renderer returns.
func splitScrolled(s string) []string {
	return strings.Split(s, "\n")
}

// centerLine pads `s` left/right with spaces so it visually sits in the
// middle of `width` cells, then wraps it in a foreground style. ANSI
// codes inside `s` are ignored for width math via ansi.StringWidth so
// already-styled banner lines still center correctly.
func centerLine(s string, width int, fg color.Color, dim bool) string {
	w := ansi.StringWidth(s)
	pad := (width - w) / 2
	if pad < 0 {
		pad = 0
	}
	style := lipgloss.NewStyle().Foreground(fg)
	if dim {
		style = style.Foreground(theme.FgDim)
	}
	return strings.Repeat(" ", pad) + style.Render(s)
}

func (m Model) renderHomeNotifications(d *orgData, inner, budget int) []string {
	if d == nil {
		return []string{dimLine("  loading…", inner)}
	}
	if d.Notifications.FetchedAt().IsZero() {
		if d.Notifications.Busy() {
			return []string{dimLine("  loading notifications…", inner)}
		}
		if err := d.Notifications.Err(); err != nil {
			return []string{redLine("  notifications: " + err.Error())}
		}
		return []string{dimLine("  press "+firstPretty(Keys.Refresh)+" to load notifications", inner)}
	}
	if err := d.Notifications.Err(); err != nil {
		return []string{redLine("  notifications: " + err.Error())}
	}
	resolved := mustResolveColumns(homeNotifColumnSchema())
	cols := withColStyles(resolved.ListColumns(), map[string]lipgloss.Style{
		"When":  lipgloss.NewStyle().Foreground(theme.FgDim),
		"State": lipgloss.NewStyle().Foreground(theme.Muted),
		"Type":  lipgloss.NewStyle().Foreground(theme.Muted),
		"Title": lipgloss.NewStyle().Foreground(theme.Fg),
		"Body":  lipgloss.NewStyle().Foreground(theme.FgDim),
	})
	installListViewOrderRows(&d.HomeNotifList, &d.HomeNotifTableState, cols,
		func(items []sf.Notification, row, col int) string {
			return resolvedSortCellForListColumn(resolved, items, cols, row, col)
		})
	items := d.HomeNotifList.Filtered()
	val := d.Notifications.Value()
	if d.HomeNotifList.Len() == 0 {
		return []string{
			dimLine("  no notifications", inner),
			dimLine("  the API mirrors the Lightning bell — old items age out", inner),
		}
	}
	spec := uilayout.ListTableSpec{
		Cols: cols,
		N:    len(items),
		Cell: func(row, col int) string {
			if row < 0 || row >= len(items) {
				return ""
			}
			return resolvedCellForListColumn(resolved, items, cols, row, col)
		},
	}
	title := fmt.Sprintf("NOTIFICATIONS · %d unread · %d total",
		val.UnreadCount, d.HomeNotifList.Len())
	return m.renderHomeListTable(spec, &d.HomeNotifTableState, d.HomeNotifList.Cursor(),
		title, d.HomeNotifList.SearchPtr(), inner, budget,
		func(row, col int, base lipgloss.Style) lipgloss.Style {
			if cols[col].Name == "State" && !items[row].Read {
				return base.Foreground(theme.Yellow)
			}
			return base
		})
}

// renderHomeListTable is the home-subtab adapter that builds a
// listRenderModel from the spec + recolor + title + state + search
// the home renderers already pass in, and delegates to the shared
// renderListModel. Lets the eight home subtabs keep their
// imperative shape (build cols + spec inline, branch on busy/empty
// states, then hand off here) while still flowing through the same
// renderer the other tabs use.
func (m Model) renderHomeListTable(
	spec uilayout.ListTableSpec,
	state *uilayout.ListTableState,
	sel int,
	title string,
	search *searchState,
	inner, budget int,
	recolor func(row, col int, base lipgloss.Style) lipgloss.Style,
) []string {
	model := listRenderModel{
		Title:   title,
		State:   state,
		Search:  search,
		Cols:    spec.Cols,
		N:       spec.N,
		Cursor:  sel,
		Cell:    spec.Cell,
		Marks:   spec.Marks,
		Gutters: spec.Gutters,
		Recolor: recolor,
	}
	return renderListModel(m, model, m.focus, inner, budget)
}

// notifTypeLabel collapses Salesforce's wire types into shorter
// labels for the TYPE column.
func notifTypeLabel(t string) string {
	switch t {
	case "task_mention", "mention":
		return "@mention"
	case "approval_request":
		return "approval"
	case "task_assigned":
		return "task"
	case "share":
		return "share"
	case "thanks":
		return "thanks"
	case "custom_notification":
		return "custom"
	case "feed":
		return "feed"
	}
	return t
}

// asciiBar renders a fixed-width text progress bar with severity-
// colored fill. Threshold colors match the % column tints used on
// /home Limits + Licenses so the visuals stay coherent across the
// "are we close to a ceiling?" cells.
//
//	0–39%  → cyan (healthy)
//	40–69% → cyan (still healthy, just more visible)
//	70–89% → yellow (heads up)
//	≥90%   → red   (problem)
//
// The unfilled portion stays muted so the contrast is always between
// "this much" and "this much potential" — never between two equally
// loud colors.
//
// Returns ANSI-styled output; lipgloss preserves embedded codes when
// the cell style wraps the result, so callers can stick this in a
// column whose Style sets a width without re-tinting the bar.
func asciiBar(used, total, width int) string {
	if width < 4 {
		width = 4
	}
	if total <= 0 {
		return strings.Repeat(" ", width)
	}
	inner := width - 2
	filled := used * inner / total
	if filled < 0 {
		filled = 0
	}
	if filled > inner {
		filled = inner
	}
	pct := used * 100 / total
	fillColor := theme.Cyan
	switch {
	case pct >= 90:
		fillColor = theme.Red
	case pct >= 70:
		fillColor = theme.Yellow
	}
	filledStyle := lipgloss.NewStyle().Foreground(fillColor)
	emptyStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	return "[" +
		filledStyle.Render(strings.Repeat("=", filled)) +
		emptyStyle.Render(strings.Repeat("·", inner-filled)) +
		"]"
}

// fmtThousands formats an int with comma separators.
func fmtThousands(n int) string {
	if n < 0 {
		return "-" + fmtThousands(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	rest := fmtThousands(n / 1000)
	return fmt.Sprintf("%s,%03d", rest, n%1000)
}
