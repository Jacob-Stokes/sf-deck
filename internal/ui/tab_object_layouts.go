package ui

// /objects drill · Layouts subtab — the page layouts defined on the
// drilled sObject. Names only (the layout editor is a Setup iframe,
// so there's no in-TUI drill); o on the subtab opens Object
// Manager's Page Layouts list for this object.

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderObjectLayouts(w, innerH int) string {
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

	r, ok2 := d.PageLayouts.Lists[sobj]
	if !ok2 || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading layouts…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching layouts…")
	}
	rows := r.Value()

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("PAGE LAYOUTS · %d · %s",
		len(rows),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))))
	lines = append(lines, "")

	if len(rows) == 0 {
		lines = append(lines, theme.Subtle.Render("  no layouts on this object"))
		return strings.Join(lines, "\n")
	}

	sel := d.PageLayouts.Cursors[sobj]
	if sel < 0 || sel >= len(rows) {
		sel = 0
	}
	for i, row := range rows {
		prefix := "  "
		style := theme.Subtle
		if i == sel && m.focus == focusMain {
			prefix = "▌ "
			style = style.Foreground(theme.Fg).Bold(true)
		}
		lines = append(lines, prefix+style.Render(row.Name))
	}
	lines = append(lines, "",
		dimLine("  layouts edit in Setup (Object Manager) only", inner))
	return strings.Join(lines, "\n")
}
