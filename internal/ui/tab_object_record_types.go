package ui

// Record Types subtab of TabObjectDetail — the list of RecordType
// records defined on the currently-drilled sObject.
//
// Structurally identical to the Validation subtab: list in the main
// pane, compact summary of the selected row in the sidebar. Drill
// via Enter for full Metadata + action menu (TabRecordTypeDetail).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderObjectRecordTypes is the main-pane renderer for the Record
// Types subtab.
func (m Model) renderObjectRecordTypes(w, innerH int) string {
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

	r, ok := d.RecordTypes.Lists[sobj]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading record types…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching record types…")
	}
	rts := r.Value()

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("RECORD TYPES · %d · %s",
		len(rts),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))))
	lines = append(lines, "")

	if len(rts) == 0 {
		lines = append(lines, theme.Subtle.Render("  none defined on this object"))
		return strings.Join(lines, "\n")
	}

	sel := d.RecordTypes.Cursors[sobj]
	if sel < 0 || sel >= len(rts) {
		sel = 0
	}

	// Columns: active dot · developer name · label · description preview.
	devW := inner / 4
	if devW < 18 {
		devW = 18
	}
	labelW := inner / 4
	if labelW < 14 {
		labelW = 14
	}
	descW := inner - devW - labelW - 8

	lines = append(lines, renderRows(
		len(rts), sel, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderRecordTypeRow(rts[i], i == sel, m.focus == focusMain, devW, labelW, descW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

// renderRecordTypeRow is one row of the record-type list.
func renderRecordTypeRow(rt sf.RecordTypeRow, selected, mainFocused bool, devW, labelW, descW, inner int) string {
	dot := validationStatusDot(rt.Active) // green-active / muted-inactive; same visual as validation
	devStyle := lipgloss.NewStyle().Foreground(theme.Fg).Width(devW)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	descStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if selected {
		devStyle = devStyle.Bold(true)
	}
	dev := devStyle.Render(ansi.Truncate(rt.DeveloperName, devW-1, "…"))
	label := labelStyle.Render(ansi.Truncate(dashIfEmpty(rt.Name), labelW-1, "…"))
	desc := descStyle.Render(ansi.Truncate(dashIfEmpty(rt.Description), descW, "…"))
	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	return ansi.Truncate(prefix+dot+" "+dev+"  "+label+"  "+desc, inner, "…")
}
