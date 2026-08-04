package ui

// /system — unified observability tab.
//
// Bundles previously-top-level Logs and Deploys plus the API usage
// surface that used to live in a Ctrl+A modal. Each surface is a
// subtab so they're cycle-able (tab / shift+tab) without burning a
// number key each. Future subtabs (Limits, Async Jobs, Login History,
// Setup Audit Trail) drop in by adding a Subtab const + a case here.

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderSystem prepends the shared subtab strip then dispatches to
// the right subtab renderer. Logs + Deploys reuse the existing
// renderers verbatim; API has its own.
//
// The strip costs one row at the top — we shave that off the inner
// height passed to the per-subtab renderer so list views still fit
// within the pane.
func (m Model) renderSystem(w, innerH int) string {
	subs := systemSubtabs()
	sel := m.systemSubtab()
	strip := renderSubtabStrip(subs, sel, w-4)
	bodyH := innerH - subtabReserve(strip)
	body := ""
	switch m.currentSubtab() {
	case SubtabSystemDeploys:
		body = m.renderDeploys(w, bodyH)
	case SubtabSystemAudit:
		body = m.renderSetupAudit(w, bodyH)
	case SubtabSystemInterviews:
		body = m.renderFlowInterviews(w, bodyH)
	case SubtabSystemAsyncJobs:
		body = m.renderAsyncJobs(w, bodyH)
	case SubtabSystemScheduled:
		body = m.renderScheduledJobs(w, bodyH)
	case SubtabSystemAPI:
		body = m.renderSystemAPI(w, bodyH)
	case SubtabSystemLogs:
		fallthrough
	default:
		body = m.renderApexLogs(w, bodyH)
	}
	if strip == "" {
		return body
	}
	return strip + "\n" + body
}

// renderSystemAPI renders the API call log inline — same data the
// Ctrl+A modal showed. Read-only; cleared on session restart since
// the underlying ring buffer is in-memory.
func (m Model) renderSystemAPI(w, innerH int) string {
	inner := w - 4
	var lines []string
	if Usage == nil {
		lines = append(lines, sectionTitle("API"))
		lines = append(lines, theme.Subtle.Render("  usage tracker is not active"))
		return strings.Join(lines, "\n")
	}
	today := Usage.Today()
	calls := Usage.Recent()
	header := fmt.Sprintf("API · today: %d  ·  recent: %d", today, len(calls))
	lines = append(lines, sectionTitle(header))

	if len(calls) == 0 {
		lines = append(lines, theme.Subtle.Render("  no API calls recorded yet this session"))
		return strings.Join(lines, "\n")
	}

	cols := []tableColumn{
		{Header: "AGE", Width: 12, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "ALIAS", Width: 22, Style: lipgloss.NewStyle().Foreground(theme.Cyan)},
		{Header: "CALL", Width: -1, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
	}
	lines = append(lines, renderTableHeader(cols, inner))

	now := time.Now()
	// Cap at 200 rows on screen — the buffer holds 500.
	limit := len(calls)
	if limit > 200 {
		limit = 200
	}
	rows := calls[:limit]

	// No selection model on this view — read-only.
	const sel = -1
	lines = append(lines, renderRows(
		len(rows), sel, innerH, len(lines), 1, inner,
		func(i int) string {
			c := rows[i]
			rowCols := make([]tableColumn, len(cols))
			copy(rowCols, cols)
			if !c.OK {
				rowCols[2].Style = lipgloss.NewStyle().Foreground(theme.Red)
			}
			return renderInteractiveTableRow(rowCols, []string{
				formatAgo(now, c.At),
				dashIfEmpty(c.Alias),
				formatCall(c),
			}, false, false, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}
