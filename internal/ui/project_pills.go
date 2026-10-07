package ui

import (
	"crypto/sha1"
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// ProjectGutterWidth is the screen width reserved for the project-
// membership gutter on every list-table render. Wide enough for the
// header label "PROJECTS" (8 cells) and the longest natural body
// content "9 projects" (10 cells). Single-project rows truncate
// long names with an ellipsis to fit.
const ProjectGutterWidth = 12

func (m Model) projectGutterWidth() int {
	if m.settings != nil && !m.settings.ProjectColumnVisible() {
		return 0
	}
	return ProjectGutterWidth
}

func projectColorFor(projectID string) color.Color {
	if projectID == "" {
		return theme.Border
	}
	h := sha1.Sum([]byte(projectID))
	return tagColorFor(tagPalette[int(h[0])%len(tagPalette)])
}

func rowProjectGutterFromMap(kind devproject.ItemKind, ref string, projects map[string][]devproject.DevProject) string {
	if ref == "" || len(projects) == 0 {
		return ""
	}
	bound, ok := projects[string(kind)+":"+ref]
	if !ok || len(bound) == 0 {
		return ""
	}
	if len(bound) == 1 {
		p := bound[0]
		label := ansi.Truncate(p.Name, ProjectGutterWidth-2, "…")
		return lipgloss.NewStyle().
			Background(projectColorFor(p.ID)).
			Foreground(theme.Bg).
			Bold(true).
			Padding(0, 1).
			Render(label)
	}
	label := fmt.Sprintf("%d projects", len(bound))
	return lipgloss.NewStyle().
		Background(theme.Border).
		Foreground(theme.Fg).
		Bold(true).
		Padding(0, 1).
		Render(label)
}

func renderProjectPills(projects []devproject.DevProject) string {
	if len(projects) == 0 {
		return ""
	}
	pills := make([]string, 0, len(projects))
	for _, p := range projects {
		pills = append(pills, lipgloss.NewStyle().
			Background(projectColorFor(p.ID)).
			Foreground(theme.Bg).
			Bold(true).
			Padding(0, 1).
			Render(p.Name))
	}
	return strings.Join(pills, " ")
}

func (m Model) sidebarProjectSection(kind devproject.ItemKind, ref, orgUser string, inner int) string {
	if m.devProjects == nil || ref == "" {
		return ""
	}
	projects, err := m.devProjects.ProjectsForItem(kind, ref, orgUser)
	if err != nil || len(projects) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sideSection("projects"))
	b.WriteString("\n  ")
	b.WriteString(renderProjectPills(projects))
	return b.String()
}

// sidebarTagsProjectsSection renders the TAGS and PROJECTS sidebar
// blocks together. In stacked mode (sidebar below the main pane,
// where horizontal room is plentiful) and when BOTH blocks have
// content that fits, they sit side by side as two columns; otherwise
// they fall back to the vertical stack (tags block, then projects
// block) — the same output the two callers produced individually.
//
// Returns "" when neither block has content, so callers can guard on
// the empty string exactly as before.
func (m Model) sidebarTagsProjectsSection(kind devproject.ItemKind, ref, orgUser string, inner int) string {
	// Stacked mode surfaces tags + projects in the panel HEADER instead
	// (kvPanelTagged) — appended to the title / right-aligned — because a
	// narrow stacked column truncated the body pills and forced the user
	// into the inspect (i) modal to read them. So suppress the body
	// section here in stacked mode. The inspect modal (sidebarForModal)
	// renders roomy and keeps the full body section.
	if m.compactSidebarPills() {
		return ""
	}

	tags := m.sidebarTagSection(kind, ref, orgUser, inner)
	projects := m.sidebarProjectSection(kind, ref, orgUser, inner)

	if m.compactSidebarPills() && tags != "" && projects != "" {
		if col := joinSidebarColumns(tags, projects, inner); col != "" {
			return col
		}
	}
	return tags + projects
}

func (m Model) sidebarTagsProjectsExtra(kind devproject.ItemKind, ref, orgUser string, inner int) []string {
	if section := m.sidebarTagsProjectsSection(kind, ref, orgUser, inner); section != "" {
		return []string{section}
	}
	return nil
}

// joinSidebarColumns lays two sidebar section blocks side by side as
// two equal columns separated by a gutter. Each block is a leading
// "\n" + header line + "\n  " + pills (see sidebarTagSection /
// sidebarProjectSection). Returns "" when the combined width would
// overflow inner, so the caller can fall back to vertical stacking.
func joinSidebarColumns(left, right string, inner int) string {
	const gutter = 4
	l := strings.TrimPrefix(left, "\n")
	r := strings.TrimPrefix(right, "\n")

	colW := (inner - gutter) / 2
	if colW < 12 {
		return "" // too narrow to split — stack vertically instead
	}
	if lipgloss.Width(l) > colW || lipgloss.Width(r) > colW {
		return "" // a pill row wouldn't fit its column — stack instead
	}

	leftCol := lipgloss.NewStyle().Width(colW).Render(l)
	rightCol := lipgloss.NewStyle().Width(colW).Render(r)
	joined := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftCol,
		strings.Repeat(" ", gutter),
		rightCol,
	)
	return "\n" + joined
}
