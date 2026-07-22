package ui

// Triggers subtab of TabObjectDetail — the list of ApexTrigger
// records defined on the currently-drilled sObject.
//
// Structurally identical to Validation / Record Types: list in the
// main pane, compact summary of the selected row in the sidebar.
// Drill via Enter for Body + action menu (TabTriggerDetail).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderObjectTriggers is the main-pane renderer for the Triggers
// subtab.
func (m Model) renderObjectTriggers(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	sobj := d.DescribeCur
	if sobj == "" {
		return theme.Subtle.Render("  press enter on an object in /objects first")
	}

	r, ok := d.Triggers.Lists[sobj]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading triggers…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching triggers…")
	}
	trigs := r.Value()

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("TRIGGERS · %d · %s",
		len(trigs),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))))
	lines = append(lines, "")

	if len(trigs) == 0 {
		lines = append(lines, theme.Subtle.Render("  none defined on this object"))
		return strings.Join(lines, "\n")
	}

	sel := d.Triggers.Cursors[sobj]
	if sel < 0 || sel >= len(trigs) {
		sel = 0
	}

	// Columns: status dot · name · events · length. Status drives the
	// dot color; the invalid bit taints an otherwise-active trigger.
	nameW := inner / 3
	if nameW < 20 {
		nameW = 20
	}
	lenW := 8
	eventsW := inner - nameW - lenW - 8

	lines = append(lines, renderRows(
		len(trigs), sel, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderTriggerRow(trigs[i], i == sel, m.focus == focusMain, nameW, eventsW, lenW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

// renderTriggerRow is one row of the trigger list.
func renderTriggerRow(t sf.TriggerRow, selected, mainFocused bool, nameW, eventsW, lenW, inner int) string {
	dot := triggerStatusDot(t.Status, t.Valid)
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Width(nameW)
	eventsStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(eventsW)
	lenStyle := lipgloss.NewStyle().Foreground(theme.FgDim).Width(lenW).Align(lipgloss.Right)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	name := nameStyle.Render(ansi.Truncate(t.Name, nameW-1, "…"))
	events := eventsStyle.Render(ansi.Truncate(dashIfEmpty(t.Events), eventsW-1, "…"))
	length := lenStyle.Render(fmt.Sprintf("%d", t.Len))
	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	return ansi.Truncate(prefix+dot+" "+name+"  "+events+" "+length, inner, "…")
}

// triggerStatusDot picks a dot colour from Status + IsValid. Active +
// valid = green; active + invalid = yellow (compilation error);
// inactive = muted.
func triggerStatusDot(status string, valid bool) string {
	color := theme.Muted
	if status == "Active" {
		if valid {
			color = theme.Green
		} else {
			color = theme.Yellow
		}
	}
	return lipgloss.NewStyle().Foreground(color).Render("●")
}
