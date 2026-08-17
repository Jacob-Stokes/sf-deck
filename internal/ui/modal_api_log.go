package ui

// API call log modal — opens with ctrl+a. Shows the most recent calls
// the TUI has fired in this session, newest at the top. Diagnostic
// aid for "why is my API counter going up" questions; the ring buffer
// lives in-memory on the usage tracker (NOT persisted).

import (
	"fmt"
	"strings"
	"time"
)

func (m *Model) openAPILogModal() {
	state := infoModalState{
		Title: apiLogTitle(),
	}
	if Usage == nil {
		state.Rows = []infoRow{
			{Body: "usage tracker is not active — nothing to show."},
		}
		m.showInfoModal(state)
		return
	}
	calls := Usage.Recent()
	if len(calls) == 0 {
		state.Rows = []infoRow{
			{Body: "no API calls recorded yet in this session."},
		}
		m.showInfoModal(state)
		return
	}
	// Cap at 80 rows so the modal doesn't overflow common terminal
	// heights; the full ring buffer is 500.
	const maxRows = 80
	shown := calls
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}

	state.Rows = make([]infoRow, 0, len(shown)+2)
	state.Rows = append(state.Rows, infoRow{
		Body: fmt.Sprintf("most recent %d of %d tracked calls · newest first",
			len(shown), len(calls)),
	})
	state.Rows = append(state.Rows, infoRow{})

	now := time.Now()
	for _, c := range shown {
		state.Rows = append(state.Rows, infoRow{
			Label: formatAgo(now, c.At),
			Body:  formatCall(c),
		})
	}
	m.showInfoModal(state)
}

func apiLogTitle() string {
	if Usage == nil {
		return "API Call Log"
	}
	return fmt.Sprintf("API Call Log  ·  today: %d", Usage.Today())
}

func formatAgo(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 2*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return t.Format("15:04:05")
}

func formatCall(c UsageCall) string {
	var b strings.Builder
	if !c.OK {
		b.WriteString("ERR ")
	}
	if c.Caller != "" {
		b.WriteString("[")
		b.WriteString(c.Caller)
		b.WriteString("] ")
	}
	if c.Alias != "" {
		b.WriteString(c.Alias)
		b.WriteString(" · ")
	}
	if len(c.Args) > 0 {
		b.WriteString(strings.Join(c.Args, " "))
	} else {
		b.WriteString(c.Command)
	}
	if c.Err != "" {
		b.WriteString(" — ")
		b.WriteString(c.Err)
	}
	return b.String()
}
