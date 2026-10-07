package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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
