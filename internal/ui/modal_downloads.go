package ui

// Downloads modal — Ctrl+D global overlay listing in-flight + recent
// exports. Designed as a "where did my report go" answer panel; works
// from any tab so the user doesn't have to navigate to /reports first.
//
// Two sections: in-flight (top) and history (below). In-flight rows
// auto-update via the same exportActivityTickMsg that drives the
// status bar — open the modal during a long export and you can
// watch the phase change live.
//
// Actions per row:
//   Enter / o   open the file in the default app
//   r           reveal in Finder (macOS open -R)
//   d           remove from history (in-flight rows can't be removed)
//   y           yank the path to clipboard
//   esc / ctrl+d close

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// downloadsModalState is the live state of the downloads overlay.
// Stored on the Model under m.downloadsModal (nil when hidden).
type downloadsModalState struct {
	Cursor int
	// rows is recomputed on every render so the modal mirrors registry
	// changes. The cursor index addresses this slice; when it shrinks
	// (e.g. user deletes a row), we clamp on next render.
}

// openDownloadsModal stashes a fresh state so the renderer kicks in.
// Pointer receiver — modal lifetime lives on the Model.
func (m *Model) openDownloadsModal() {
	if m.downloadsModal != nil {
		// Toggle behavior: Ctrl+D again closes the modal.
		m.downloadsModal = nil
		return
	}
	m.downloadsModal = &downloadsModalState{}
}

// downloadsModalRows is the merged view of in-flight + history. Sorted
// in-flight-first (so users see "your export is still running" at the
// top) then history newest-first.
func (m Model) downloadsModalRows() []*exportJob {
	if m.exports == nil {
		return nil
	}
	inflight, history := m.exports.snapshot()
	rows := make([]*exportJob, 0, len(inflight)+len(history))
	rows = append(rows, inflight...)
	rows = append(rows, history...)
	return rows
}

// renderDownloadsModal draws the modal. Returns "" when hidden.
//
// Layout is fixed-size: 90-col width, 16-row viewport over the
// underlying rows slice (which can be up to ~200). The cursor
// scrolls the viewport so the active row stays roughly 1/3 of
// the way down — same pattern the choice modal + global search
// use — keeping context visible above and below.
func (m Model) renderDownloadsModal() string {
	if m.downloadsModal == nil {
		return ""
	}
	w := modalWidth(m.width, 90, 90)
	inner := w - 4
	visibleRows := m.settings.LayoutDownloadsModalRows()
	rows := m.downloadsModalRows()
	cursor := m.downloadsModal.Cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) && len(rows) > 0 {
		cursor = len(rows) - 1
	}

	var lines []string
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render("Downloads"))
	subhead := "in-flight first, then most recent · last 200 entries persist across sessions"
	if len(rows) > visibleRows {
		subhead = fmt.Sprintf("showing %d of %d · ↑/↓ to scroll the rest",
			visibleRows, len(rows))
	}
	lines = append(lines, theme.Subtle.Render(subhead))
	lines = append(lines, "")

	if len(rows) == 0 {
		// Empty state still pads to the fixed body height so the modal
		// doesn't visibly shrink between "nothing yet" and "first row".
		lines = append(lines, theme.Subtle.Render(
			"  no exports yet — press "+firstPretty(Keys.ReportExport)+" on a report to export."))
		for i := 1; i < visibleRows; i++ {
			lines = append(lines, "")
		}
	} else {
		start, end := downloadsViewportWindow(cursor, len(rows), visibleRows)
		// Section labels render inline only when the visible window
		// crosses the inflight/history boundary. Putting them above
		// each row run keeps the header attached to its section even
		// while scrolling.
		var sectionInflight, sectionHistory bool
		bodyLines := 0
		for i := start; i < end; i++ {
			j := rows[i]
			isInflight := j.Phase != exportPhaseDone && j.Phase != exportPhaseFailed
			if isInflight && !sectionInflight {
				lines = append(lines,
					lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Render("  IN FLIGHT"))
				sectionInflight = true
				bodyLines++
			}
			if !isInflight && !sectionHistory {
				if sectionInflight {
					lines = append(lines, "")
					bodyLines++
				}
				lines = append(lines,
					lipgloss.NewStyle().Foreground(theme.Muted).Bold(true).Render("  RECENT"))
				sectionHistory = true
				bodyLines++
			}
			lines = append(lines, renderDownloadRow(j, i == cursor, inner))
			bodyLines++
		}
		// Pad the body up to the fixed visible-row count so the modal
		// doesn't grow/shrink with the row count. Section labels +
		// the spacer-line consume some budget; pad whatever remains.
		for bodyLines < visibleRows {
			lines = append(lines, "")
			bodyLines++
		}
	}

	lines = append(lines, "")
	hint := theme.Subtle.Render(
		"↑/↓ move · ↵/o open · r reveal · y yank path · d remove · esc close")
	lines = append(lines, hint)

	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Width(inner).
		Render(body)
	return box
}

// downloadsViewportWindow returns [start, end) for a viewport of
// `visible` rows around `cursor`, clamped to [0, n). Cursor sits
// roughly 1/3 down the window so movement feels like vim's `j`
// rather than landing on the top edge. Same shape as the
// choice-modal viewport helper; copied locally to avoid coupling
// the two modals.
func downloadsViewportWindow(cursor, n, visible int) (int, int) {
	if visible <= 0 || n <= 0 {
		return 0, 0
	}
	if n <= visible {
		return 0, n
	}
	start := cursor - visible/3
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > n {
		end = n
		start = end - visible
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

// renderDownloadRow formats one job row with kind badge + name +
// phase/elapsed/size on the right. Active row gets a left bar +
// bold treatment.
func renderDownloadRow(j *exportJob, active bool, inner int) string {
	var status, statusColor string
	switch j.Phase {
	case exportPhaseDone:
		status = exportSize(j.SizeBytes) + "  " + formatExportDuration(j)
		statusColor = "muted"
	case exportPhaseFailed:
		status = "FAILED"
		statusColor = "red"
	default:
		status = exportPhaseLabel(j.Phase) + "…"
		statusColor = "yellow"
	}
	kindBadge := "report"
	switch j.Kind {
	case exportKindProject:
		kindBadge = "project"
	case exportKindManifest:
		kindBadge = "manifest"
	}

	// Left: kind badge + name. Right: status. Layout calculated to
	// fit `inner` minus a 2-col left bar/spacing budget.
	leftBudget := inner - lipgloss.Width(status) - 6
	if leftBudget < 20 {
		leftBudget = 20
	}
	name := j.Name
	leftRaw := fmt.Sprintf("[%s] %s", kindBadge, name)
	if len(leftRaw) > leftBudget {
		leftRaw = ansi.Truncate(leftRaw, leftBudget, "…")
	}

	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	statusStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	switch statusColor {
	case "yellow":
		statusStyle = lipgloss.NewStyle().Foreground(theme.Yellow)
	case "red":
		statusStyle = lipgloss.NewStyle().Foreground(theme.Red)
	}

	if active {
		nameStyle = nameStyle.Bold(true)
	}
	pad := inner - lipgloss.Width(leftRaw) - lipgloss.Width(status) - 4
	if pad < 1 {
		pad = 1
	}
	row := nameStyle.Render(leftRaw) + strings.Repeat(" ", pad) + statusStyle.Render(status)

	if active {
		bar := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌")
		return "  " + bar + " " + row
	}
	return "    " + row
}

// handleDownloadsModalKey routes key events while the modal is up.
// Returns the new model + command. Caller checks m.downloadsModal !=
// nil to decide whether to dispatch here.
func (m *Model) handleDownloadsModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.downloadsModal == nil {
		return *m, nil
	}
	rows := m.downloadsModalRows()
	st := m.downloadsModal
	if st.Cursor < 0 {
		st.Cursor = 0
	}
	if st.Cursor >= len(rows) && len(rows) > 0 {
		st.Cursor = len(rows) - 1
	}
	key := msg.String()
	switch {
	case key == "esc", key == "ctrl+j":
		m.downloadsModal = nil
		return *m, nil
	case key == "up", key == "k":
		if st.Cursor > 0 {
			st.Cursor--
		}
		return *m, nil
	case key == "down", key == "j":
		if st.Cursor < len(rows)-1 {
			st.Cursor++
		}
		return *m, nil
	case key == "enter", matches(key, Keys.DownloadOpen):
		if len(rows) == 0 {
			return *m, nil
		}
		j := rows[st.Cursor]
		if j.Path == "" || j.Phase == exportPhaseFailed {
			m.flash("nothing to open — file not saved")
			return *m, nil
		}
		if err := openPath(j.Path); err != nil {
			m.flash("open failed: " + err.Error())
		}
		return *m, nil
	case matches(key, Keys.DownloadReveal):
		if len(rows) == 0 {
			return *m, nil
		}
		j := rows[st.Cursor]
		if j.Path == "" || j.Phase == exportPhaseFailed {
			return *m, nil
		}
		if err := revealInFinder(j.Path); err != nil {
			m.flash("reveal failed: " + err.Error())
		}
		return *m, nil
	case matches(key, Keys.DownloadYankPath):
		if len(rows) == 0 {
			return *m, nil
		}
		j := rows[st.Cursor]
		if j.Path == "" {
			return *m, nil
		}
		if err := writeClipboard(j.Path); err != nil {
			m.flash("clipboard unavailable (" + err.Error() + ")")
			return *m, nil
		}
		m.flash("yanked path → " + j.Path)
		return *m, nil
	case matches(key, Keys.DownloadRemove):
		if len(rows) == 0 {
			return *m, nil
		}
		j := rows[st.Cursor]
		// Inflight jobs can't be deleted — they're still running. Done
		// + Failed both sit in history and can be removed.
		if j.Phase != exportPhaseDone && j.Phase != exportPhaseFailed {
			m.flash("can't remove an in-flight export — wait for it to finish")
			return *m, nil
		}
		m.exports.removeFromHistory(j.ID)
		// Cursor stays on the same index so the next row slides up; clamp
		// to the new size on next render.
		return *m, nil
	}
	return *m, nil
}

// revealInFinder opens the parent folder with the file selected (macOS
// `open -R`). On Linux/Windows we fall back to opening the parent
// folder — there's no universal "reveal" gesture across DEs.
func revealInFinder(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Run()
	case "windows":
		return exec.Command("explorer", "/select,", path).Run()
	default:
		// Best-effort: open the containing directory. The file itself
		// won't be selected but the user lands in the right place.
		dir := path
		if i := strings.LastIndex(dir, "/"); i >= 0 {
			dir = dir[:i]
		}
		return exec.Command("xdg-open", dir).Run()
	}
}
