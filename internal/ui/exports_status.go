package ui

// Status bar in-flight export indicator.
//
// While at least one export is in flight, the left side of the status
// bar shows a compact progress sentence the user can read across tab
// switches. Examples:
//
//   · downloading "Course Group Registers"… 14s
//   · post-processing "Course Group Registers"… 32s
//   · 2 exports in progress…
//
// The animated ellipsis cycles through ".", "..", "..." so the user
// has visual confirmation that the TUI hasn't hung. The exportActivity
// frame counter (incremented by exportActivityTickMsg every 500ms)
// drives the animation.

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderExportActivity returns the left-side activity badge for the
// status bar, or "" when nothing is running. Designed to fit in
// roughly half the status bar; longer report names get truncated.
func (m Model) renderExportActivity() string {
	if m.exports == nil {
		return ""
	}
	inflight, _ := m.exports.snapshot()
	if len(inflight) == 0 {
		return ""
	}
	dots := exportActivityDots(m.exportActivityFrame)
	if len(inflight) == 1 {
		j := inflight[0]
		elapsed := exportElapsed(j.StartedAt)
		name := j.Name
		// Cap report name so the badge doesn't dominate the bar; we
		// also clamp the entire badge to ~half the bar width below.
		if len(name) > 32 {
			name = ansi.Truncate(name, 31, "…")
		}
		body := fmt.Sprintf("%s \"%s\"%s %s", string(j.Phase), name, dots, elapsed)
		return decorateActivity(body, m.width/2)
	}
	body := fmt.Sprintf("%d exports in progress%s", len(inflight), dots)
	return decorateActivity(body, m.width/2)
}

// decorateActivity wraps the activity text in a yellow dot + theme
// styling, then truncates to budget. The leading "· " mirrors how the
// rest of the bar uses dots as soft separators.
func decorateActivity(body string, budget int) string {
	dot := lipgloss.NewStyle().Foreground(theme.Yellow).Background(theme.Panel).Render("· ")
	text := lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Panel).Render(body)
	out := dot + text
	// Truncate naively if the body is wider than allowed. ansi.Truncate
	// could be used for stricter handling; this is good enough for the
	// short strings the badge produces.
	if budget > 0 && lipgloss.Width(out) > budget {
		// Strip styling for the fallback measurement; the recompose is
		// "· " + truncated text.
		max := budget - 2
		if max < 1 {
			max = 1
		}
		if len(body) > max {
			body = ansi.Truncate(body, max, "…")
		}
		text = lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Panel).Render(body)
		out = dot + text
	}
	return out
}

// exportActivityDots cycles 0→".", 1→"..", 2→"...", 3→"" so the
// trailing ellipsis appears to chase itself.
func exportActivityDots(frame int) string {
	switch frame % 4 {
	case 0:
		return "."
	case 1:
		return ".."
	case 2:
		return "..."
	}
	return ""
}

// exportElapsed renders how long an export has been running. Sub-
// second is "0s" so the cell width stays stable; minutes use "m:ss"
// shape so a 2-minute export reads "2m05s" not the awkward "125s".
func exportElapsed(start time.Time) string {
	d := time.Since(start)
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) - mins*60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

// exportPhaseLabel returns the user-facing label for a phase. Used by
// the modal + /home subtab; status bar uses j.Phase directly.
func exportPhaseLabel(p exportPhase) string {
	switch p {
	case exportPhaseQueued:
		return "queued"
	case exportPhaseDownloading:
		return "downloading"
	case exportPhasePostProcess:
		return "post-processing"
	case exportPhaseConverting:
		return "converting"
	case exportPhaseWriting:
		return "writing"
	case exportPhaseRetrieving:
		return "retrieving from org"
	case exportPhaseDone:
		return "done"
	case exportPhaseFailed:
		return "failed"
	}
	return string(p)
}

// formatExportDuration renders started→finished as a human duration.
// Used in history rows ("2m04s") and tooltip-style hints.
func formatExportDuration(j *exportJob) string {
	end := j.FinishedAt
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(j.StartedAt)
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) - mins*60
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

// exportSize renders bytes as a short human string ("1.4 MB").
func exportSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	const (
		k = 1024
		m = 1024 * k
		g = 1024 * m
	)
	switch {
	case n >= g:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(g))
	case n >= m:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(m))
	case n >= k:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(k))
	}
	return fmt.Sprintf("%d B", n)
}
