package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderObjectValidation(w, innerH int) string {
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

	r, ok := d.ValidationRules.Lists[sobj]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading validation rules…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching validation rules…")
	}
	rules := r.Value()

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("VALIDATION RULES · %d · %s",
		len(rules),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))))
	lines = append(lines, "")

	if len(rules) == 0 {
		lines = append(lines, theme.Subtle.Render("  none defined on this object"))
		return strings.Join(lines, "\n")
	}

	sel := d.ValidationRules.Cursors[sobj]
	if sel < 0 || sel >= len(rules) {
		sel = 0
	}

	nameW := inner / 3
	if nameW < 20 {
		nameW = 20
	}
	descW := inner - nameW - 6

	lines = append(lines, renderRows(
		len(rules), sel, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderValidationRow(rules[i], i == sel, m.focus == focusMain, nameW, descW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

func renderValidationRow(r sf.ValidationRuleRow, selected, mainFocused bool, nameW, descW, inner int) string {
	dot := validationStatusDot(r.Active)
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Width(nameW)
	descStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	name := nameStyle.Render(ansi.Truncate(r.ValidationName, nameW-1, "…"))
	desc := descStyle.Render(ansi.Truncate(dashIfEmpty(r.Description), descW, "…"))
	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	return ansi.Truncate(prefix+dot+" "+name+"  "+desc, inner, "…")
}

func validationStatusDot(active bool) string {
	if active {
		return lipgloss.NewStyle().Foreground(theme.Green).Render("●")
	}
	return lipgloss.NewStyle().Foreground(theme.Muted).Render("●")
}
