package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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

func decorateActivity(body string, budget int) string {
	dot := lipgloss.NewStyle().Foreground(theme.Yellow).Background(theme.Panel).Render("· ")
	text := lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Panel).Render(body)
	out := dot + text
	if budget > 0 && lipgloss.Width(out) > budget {
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
