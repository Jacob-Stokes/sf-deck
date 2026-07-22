package ui

// /home → Downloads subtab.
//
// Compact view of recently-saved exports + anything in flight. Pairs
// with the Ctrl+D modal — same data source (m.exports) — but here it
// gets full main-pane real estate so it can show more rows + extra
// columns (path, size, age) without the modal's space pressure.
//
// Open from any tab via Ctrl+D for a quick peek; navigate to /home →
// Downloads when you want the list to stick around while you're doing
// something else.

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// homeDownloadsRows returns the cursor-addressable rows for the
// /home Downloads subtab — inflight + history concatenated. Same
// shape used by the renderer so the cursor index always points to
// the right job.
func (m Model) homeDownloadsRows() []*exportJob {
	if m.exports == nil {
		return nil
	}
	inflight, history := m.exports.snapshot()
	out := make([]*exportJob, 0, len(inflight)+len(history))
	out = append(out, inflight...)
	out = append(out, history...)
	return out
}

// renderHomeDownloads renders the Downloads subtab body. Returns the
// pre-joined line slice to match the rest of the subtab renderers.
func (m Model) renderHomeDownloads(inner, budget int) []string {
	if m.exports == nil {
		return []string{dimLine("  exports tracker not active", inner)}
	}
	inflight, history := m.exports.snapshot()
	rows := m.homeDownloadsRows()
	cursor := m.homeDownloadsCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) && len(rows) > 0 {
		cursor = len(rows) - 1
	}

	var lines []string
	title := fmt.Sprintf("DOWNLOADS · %d in flight · %d in history",
		len(inflight), len(history))
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(title))
	lines = append(lines, "")

	// On Downloads, r is rebound to "reveal in Finder" — the global
	// r/refresh hint in the footer would be misleading here, but the
	// surface-local hint is the place to be honest. Surface the
	// global-refresh variant (^r) which still works as a hard reload
	// of the active org's data.
	hint := "  ↵/o open · " +
		firstPretty(Keys.DownloadReveal) + " reveal · " +
		firstPretty(Keys.DownloadYankPath) + " yank · " +
		firstPretty(Keys.DownloadRemove) + " remove · " +
		firstPretty(Keys.GlobalRefresh) + " refresh"
	if len(rows) == 0 {
		lines = append(lines,
			theme.Subtle.Render("  no exports yet — press "+firstPretty(Keys.ReportExport)+" on a /reports row to start one."))
		return append(lines, "", theme.Subtle.Render(hint))
	}

	rowIdx := 0
	if len(inflight) > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Render("  IN FLIGHT"))
		for _, j := range inflight {
			lines = append(lines, formatHomeDownloadRow(j, rowIdx == cursor, inner))
			rowIdx++
		}
		lines = append(lines, "")
	}

	if len(history) > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Muted).Bold(true).Render("  RECENT"))
		// Cap to the budget so we don't run past the available height.
		// Reserve 6 lines for header + section labels + footer.
		max := budget - 6 - len(inflight)
		if max < 5 {
			max = 5
		}
		shown := history
		truncated := 0
		if len(shown) > max {
			truncated = len(shown) - max
			shown = shown[:max]
		}
		for _, j := range shown {
			lines = append(lines, formatHomeDownloadRow(j, rowIdx == cursor, inner))
			rowIdx++
		}
		if truncated > 0 {
			lines = append(lines,
				theme.Subtle.Render(fmt.Sprintf("  …and %d older (open with %s)",
					truncated, firstPretty(Keys.OpenDownloads))))
		}
	}

	lines = append(lines, "")
	lines = append(lines, theme.Subtle.Render(hint))
	return lines
}

// formatHomeDownloadRow renders one job as a single-line row with the
// status / size / when columns aligned to the right edge. `active`
// adds the highlight bar + bold treatment matching other lists.
func formatHomeDownloadRow(j *exportJob, active bool, inner int) string {
	kind := string(j.Kind)
	var rightStatus string
	var rightStyle lipgloss.Style
	switch j.Phase {
	case exportPhaseDone:
		rightStatus = exportSize(j.SizeBytes) + "  " + prettyAgo(j.FinishedAt)
		rightStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	case exportPhaseFailed:
		rightStatus = "FAILED  " + prettyAgo(j.FinishedAt)
		rightStyle = lipgloss.NewStyle().Foreground(theme.Red)
	default:
		rightStatus = exportPhaseLabel(j.Phase) + "…  " + exportElapsed(j.StartedAt)
		rightStyle = lipgloss.NewStyle().Foreground(theme.Yellow)
	}

	leftPrefix := "    "
	if active {
		leftPrefix = "  " +
			lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
	}
	leftRaw := fmt.Sprintf("[%s] %s", kind, j.Name)
	leftBudget := inner - lipgloss.Width(rightStatus) - 6 - lipgloss.Width(leftPrefix)
	if leftBudget < 20 {
		leftBudget = 20
	}
	if len(leftRaw) > leftBudget {
		leftRaw = ansi.Truncate(leftRaw, leftBudget, "…")
	}
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if active {
		nameStyle = nameStyle.Bold(true)
	}
	pad := inner - lipgloss.Width(leftPrefix) - lipgloss.Width(leftRaw) - lipgloss.Width(rightStatus) - 2
	if pad < 1 {
		pad = 1
	}
	return leftPrefix + nameStyle.Render(leftRaw) +
		strings.Repeat(" ", pad) + rightStyle.Render(rightStatus)
}

// homeDownloadsMoveCursor is the SubtabSpec.MoveCursor closure for
// the /home Downloads subtab. Clamps to the row count so
// over-scrolling lands on the last/first row.
func homeDownloadsMoveCursor(m *Model, delta int) {
	rows := m.homeDownloadsRows()
	n := len(rows)
	if n == 0 {
		m.homeDownloadsCursor = 0
		return
	}
	c := m.homeDownloadsCursor + delta
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	m.homeDownloadsCursor = c
}

// homeDownloadsActivate is Enter on /home Downloads — open the
// cursored file with the OS default app. Mirrors the modal's
// Enter/o behavior so muscle memory carries over.
func homeDownloadsActivate(m *Model) tea.Cmd {
	rows := m.homeDownloadsRows()
	if len(rows) == 0 {
		return nil
	}
	if m.homeDownloadsCursor < 0 || m.homeDownloadsCursor >= len(rows) {
		return nil
	}
	j := rows[m.homeDownloadsCursor]
	if j.Path == "" || j.Phase == exportPhaseFailed {
		m.flash("nothing to open — file not saved")
		return nil
	}
	if err := openPath(j.Path); err != nil {
		m.flash("open failed: " + err.Error())
	}
	return nil
}

// onHomeDownloadsKey routes the per-subtab keys (r/y/d) for the
// Downloads list. Returns true when the key was consumed; the
// global handler should skip further dispatch in that case. Open
// (Enter / o) is wired via Activate; movement via MoveCursor.
func (m *Model) onHomeDownloadsKey(key string) bool {
	if m.tab() != TabHome || m.homeSubtab() < 0 {
		return false
	}
	subs := homeSubtabs()
	if m.homeSubtab() >= len(subs) || subs[m.homeSubtab()].ID != SubtabHomeDownloads {
		return false
	}
	rows := m.homeDownloadsRows()
	if len(rows) == 0 {
		return false
	}
	if m.homeDownloadsCursor < 0 || m.homeDownloadsCursor >= len(rows) {
		return false
	}
	j := rows[m.homeDownloadsCursor]
	switch {
	case matches(key, Keys.DownloadOpen):
		if j.Path == "" || j.Phase == exportPhaseFailed {
			m.flash("nothing to open — file not saved")
			return true
		}
		if err := openPath(j.Path); err != nil {
			m.flash("open failed: " + err.Error())
		}
		return true
	case matches(key, Keys.DownloadReveal):
		if j.Path == "" || j.Phase == exportPhaseFailed {
			return true
		}
		if err := revealInFinder(j.Path); err != nil {
			m.flash("reveal failed: " + err.Error())
		}
		return true
	case matches(key, Keys.DownloadYankPath):
		if j.Path == "" {
			return true
		}
		_ = writeClipboard(j.Path)
		m.flash("yanked path → " + j.Path)
		return true
	case matches(key, Keys.DownloadRemove):
		if j.Phase != exportPhaseDone && j.Phase != exportPhaseFailed {
			m.flash("can't remove an in-flight export — wait for it to finish")
			return true
		}
		m.exports.removeFromHistory(j.ID)
		// Cursor stays where it is; clamp on next render.
		return true
	}
	return false
}

// prettyAgo returns "5m ago" / "2h ago" / "yesterday" / "Apr 28"
// style relative-time strings. Tighter than prettyDate for the
// downloads list where we want short rather than absolute times.
func prettyAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}
