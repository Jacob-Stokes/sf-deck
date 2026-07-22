package ui

// System-permissions subtab of TabPermParentDetail.
//
// Displays the ~200 boolean Permissions* fields on a PermissionSet
// as a searchable list. Toggle via Space (Phase H).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderPermParentSystem is the main-pane renderer for the System subtab.
func (m Model) renderPermParentSystem(w, inner, innerH int, o sf.Org) string {
	d := m.ensureOrgDataRef(o.Username)

	if d.PermParentPermSetID == "" {
		return psgNoDirectPermsNote(inner, "System")
	}

	res := d.EnsureSystemPerms(targetArg(o), d.PermParentPermSetID)
	if res.FetchedAt().IsZero() {
		var lines []string
		lines = append(lines, sectionTitle("SYSTEM PERMISSIONS"))
		if res.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading system permissions…"))
		} else if err := res.Err(); err != nil {
			lines = append(lines, redLine("  "+err.Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching…"))
		}
		return strings.Join(lines, "\n")
	}

	allPerms := res.Value()
	search := d.SysPermSearch[d.PermParentPermSetID]
	q := ""
	if search != nil {
		q = strings.ToLower(search.Buffer())
	}
	var perms []sf.SystemPermission
	if q == "" {
		perms = allPerms
	} else {
		for _, p := range allPerms {
			if strings.Contains(strings.ToLower(p.Name), q) ||
				strings.Contains(strings.ToLower(p.Label), q) {
				perms = append(perms, p)
			}
		}
	}
	cur := d.Cursors.Get(cursorKindSystemPerms, len(perms), d.PermParentPermSetID)

	onCount := 0
	for _, p := range allPerms {
		if p.Value {
			onCount++
		}
	}

	age := humanAge(res.FetchedAt()) + stateSuffix(res.Busy(), res.Err())
	var headerText string
	if q == "" {
		headerText = fmt.Sprintf("SYSTEM PERMISSIONS · %d / %d on · %s", onCount, len(allPerms), age)
	} else {
		headerText = fmt.Sprintf("SYSTEM PERMISSIONS · %d / %d shown · %d on · %s", len(perms), len(allPerms), onCount, age)
	}

	var lines []string
	lines = append(lines, headerWithSearchPill(headerText, derefSearch(search)))
	lines = append(lines, searchBar(derefSearch(search), inner))
	if len(allPerms) == 0 {
		lines = append(lines, theme.Subtle.Render("  no system permissions found"))
		return strings.Join(lines, "\n")
	}
	if len(perms) == 0 {
		lines = append(lines, theme.Subtle.Render("  no matches"))
		return strings.Join(lines, "\n")
	}

	// Column widths.
	nameW := inner * 35 / 100
	if nameW < 20 {
		nameW = 20
	}
	labelW := inner - nameW - 8
	if labelW < 16 {
		labelW = 16
	}

	lines = append(lines, renderSysPermHeader(nameW, labelW, inner))
	lines = append(lines, renderRows(
		len(perms), cur, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderSysPermRow(perms[i], i == cur, m.focus == focusMain, nameW, labelW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

func renderSysPermHeader(nameW, labelW, inner int) string {
	nameStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(nameW)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	line := "  " + nameStyle.Render("PERMISSION") + sep + labelStyle.Render("LABEL") + sep + "ON"
	return ansi.Truncate(line, inner, "…")
}

func renderSysPermRow(p sf.SystemPermission, selected, mainFocused bool, nameW, labelW, inner int) string {
	fg := theme.Fg
	nameStyle := lipgloss.NewStyle().Foreground(fg).Width(nameW)
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(labelW)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}

	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	name := nameStyle.Render(ansi.Truncate(p.Name, nameW-1, "…"))
	label := labelStyle.Render(ansi.Truncate(p.Label, labelW-1, "…"))
	onCell := sysPermCell(p.Value)
	return ansi.Truncate(prefix+name+sep+label+sep+onCell, inner, "…")
}

func sysPermCell(on bool) string {
	if on {
		return lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("●")
	}
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render("○")
}
