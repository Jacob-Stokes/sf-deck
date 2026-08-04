package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderProjects lists local SFDX projects discovered under the user's
// default search roots. Bespoke row layout (two-line per project) so
// it doesn't go through renderList.
func (m Model) renderProjects(w, innerH int) string {
	inner := w - 4
	var lines []string
	lines = append(lines, headerWithSearchPill(
		fmt.Sprintf("LOCAL SFDX PROJECTS · %d", m.projectList.Len()),
		m.projectList.Search))
	lines = append(lines, searchBar(m.projectList.Search, inner))
	if m.projectsRes.FetchedAt().IsZero() && m.projectsRes.Busy() {
		lines = append(lines, theme.Subtle.Render("  discovering…"))
		return strings.Join(lines, "\n")
	}
	if err := m.projectsRes.Err(); err != nil {
		lines = append(lines, redLine("  "+err.Error()))
		return strings.Join(lines, "\n")
	}
	projs := m.projectList.Filtered()
	if len(projs) == 0 {
		if m.projectList.Len() == 0 {
			lines = append(lines,
				theme.Subtle.Render("  none found in ~, ~/code, ~/work, ~/dev, ~/projects, ~/src"),
				theme.Subtle.Render("  (tip: run `sf project generate` to create one)"))
		} else {
			lines = append(lines, theme.Subtle.Render("  no matches"))
		}
		return strings.Join(lines, "\n")
	}
	sel := m.projectList.Cursor()
	if sel >= len(projs) {
		sel = 0
	}
	// Each project renders as 2-3 lines; renderRow returns the full
	// multi-line block joined with "\n" so the viewport still budgets
	// by *projects* rather than raw lines. Pass rowLines=3 so the
	// windowing math understands each "row" actually consumes 3 lines.
	lines = append(lines, renderRowsN(
		len(projs), sel, innerH, len(lines), 0, inner, 3,
		func(i int) string {
			p := projs[i]
			selected := i == sel && m.focus == focusMain
			style := lipgloss.NewStyle().Foreground(theme.Fg)
			prefix := "  "
			if selected {
				style = style.Bold(true)
				prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
			}
			line := prefix + style.Render(p.Name)
			if p.Namespace != "" {
				line += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("ns="+p.Namespace)
			}
			sub := []string{ansi.Truncate(line, inner, "…")}
			sub = append(sub, ansi.Truncate(
				lipgloss.NewStyle().Foreground(theme.FgDim).Render("    "+p.Path), inner, "…"))
			if p.SourceAPIVersion != "" {
				meta := fmt.Sprintf("    api v%s · %d package dir(s)", p.SourceAPIVersion, len(p.PackageDirs))
				sub = append(sub,
					lipgloss.NewStyle().Foreground(theme.Muted).Render(ansi.Truncate(meta, inner, "…")))
			}
			return strings.Join(sub, "\n")
		},
	)...)
	return strings.Join(lines, "\n")
}

// --- setup shortcuts ---------------------------------------------------

type setupLink struct {
	Name string
	Path string
}

// setupLink implements sf.Openable with a single target — the literal
// path it was built with.
func (l setupLink) Targets() []sf.OpenTarget {
	return []sf.OpenTarget{{ID: "open", Label: l.Name, Path: l.Path}}
}

var setupLinks = []setupLink{
	{"Flows", "/lightning/setup/Flows/home"},
	{"Apex Classes", "/lightning/setup/ApexClasses/home"},
	{"Apex Triggers", "/lightning/setup/ApexTriggers/home"},
	{"Profiles", "/lightning/setup/Profiles/home"},
	{"Permission Sets", "/lightning/setup/PermSets/home"},
	{"Named Credentials", "/lightning/setup/NamedCredential/home"},
	{"Connected Apps", "/lightning/setup/ConnectedApplication/home"},
	{"Users", "/lightning/setup/ManageUsers/home"},
	{"Object Manager", "/lightning/setup/ObjectManager/home"},
	{"Validation Rules", "/lightning/setup/ValidationRules/home"},
	{"Custom Metadata", "/lightning/setup/CustomMetadata/home"},
	{"Deploy Status", "/lightning/setup/DeployStatus/home"},
}

func (m Model) renderSetup(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	var lines []string
	lines = append(lines, headerWithSearchPill("SETUP SHORTCUTS", m.setupList.Search))
	if !ok {
		lines = append(lines, noOrgPlaceholder())
		return strings.Join(lines, "\n")
	}
	lines = append(lines, dimLine(
		"  "+firstPretty(Keys.OpenDefault)+" → open · "+
			firstPretty(Keys.OpenMenu)+" → pick  ("+o.InstanceURL+")", inner))
	lines = append(lines, searchBar(m.setupList.Search, inner))

	filtered := m.setupList.Filtered()
	if len(filtered) == 0 {
		lines = append(lines, theme.Subtle.Render("  no matches"))
		return strings.Join(lines, "\n")
	}
	sel := m.setupList.Cursor()
	if sel >= len(filtered) {
		sel = 0
	}
	lines = append(lines, renderRows(
		len(filtered), sel, innerH, len(lines), 0, inner,
		func(i int) string {
			l := filtered[i]
			selected := i == sel && m.focus == focusMain
			style := lipgloss.NewStyle().Foreground(theme.Fg)
			prefix := "  "
			if selected {
				style = style.Bold(true)
				prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
			}
			row := prefix + style.Render(l.Name) +
				"  " + lipgloss.NewStyle().Foreground(theme.Muted).Render(l.Path)
			return ansi.Truncate(row, inner, "…")
		},
	)...)
	return strings.Join(lines, "\n")
}
