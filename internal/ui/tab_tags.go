package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderTags(w, innerH int) string {
	inner := w - 4
	if m.devProjects == nil {
		return theme.Subtle.Render("  tags require the dev-project store · cannot continue")
	}
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil {
		return redLine("  " + err.Error())
	}

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("TAGS · %d", len(tags))))
	lines = append(lines, dimLine(
		"  ↵ drill  ·  "+firstPretty(Keys.EditProject)+" edit  ·  "+
			firstPretty(Keys.DeleteProject)+" delete  ·  "+
			firstPretty(Keys.NewProject)+" new  ·  esc back", inner))
	lines = append(lines, "")

	if len(tags) == 0 {
		lines = append(lines, theme.Subtle.Render(
			"  no tags yet — press "+firstPretty(Keys.NewProject)+" to create one, or "+
				firstPretty(Keys.Tag)+" on any item to start tagging"))
		return strings.Join(lines, "\n")
	}

	pillW := 0
	for _, t := range tags {
		if w := lipgloss.Width(renderTagPill(t.Tag)); w > pillW {
			pillW = w
		}
	}
	if pillW < 8 {
		pillW = 8
	}

	cols := []tableColumn{
		{Header: "", Width: pillW, Style: lipgloss.NewStyle()},
		{Header: "NAME", Width: 22, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Header: "ICON", Width: 6, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Header: "COLOR", Width: 10, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "USAGE", Width: 8, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "CREATED", Width: -1, Style: lipgloss.NewStyle().Foreground(theme.FgDim)},
	}
	lines = append(lines, renderTableHeader(cols, inner))

	sel := m.tagsCursor
	if sel >= len(tags) {
		sel = 0
	}
	lines = append(lines, renderRows(
		len(tags), sel, innerH, len(lines), 1, inner,
		func(i int) string {
			t := tags[i]
			pill := renderTagPill(t.Tag)
			cells := []string{
				pill,
				t.Name,
				t.Icon,
				dashIfEmpty(t.Color),
				fmt.Sprintf("%d", t.Count),
				humanAge(t.CreatedAt),
			}
			return renderInteractiveTableRow(cols, cells, i == sel, m.focus == focusMain, inner)
		},
	)...)

	return strings.Join(lines, "\n")
}

func (m Model) sidebarTags(inner int) string {
	if m.devProjects == nil {
		return sideEmpty("store unavailable")
	}
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil || len(tags) == 0 {
		return sideEmpty("no tags")
	}
	idx := m.tagsCursor
	if idx >= len(tags) {
		idx = 0
	}
	t := tags[idx]

	bindings, err := m.devProjects.ItemsWithTag(t.ID, "")
	if err != nil {
		return sideEmpty(err.Error())
	}
	byKind := map[devproject.ItemKind]int{}
	byOrg := map[string]int{}
	for _, b := range bindings {
		byKind[b.ItemKind]++
		if b.OrgUser != "" {
			byOrg[b.OrgUser]++
		}
	}

	rows := []kv{
		{"name", t.Name},
		{"color", dashIfEmpty(t.Color)},
		{"icon", dashIfEmpty(t.Icon)},
		{"items", fmt.Sprintf("%d", t.Count)},
		{"orgs", fmt.Sprintf("%d", len(byOrg))},
		{"created", humanTimeAgo(t.CreatedAt)},
	}

	var extra []string
	if len(byKind) > 0 {
		extra = append(extra, "", sideSection("by kind"))
		for k, n := range byKind {
			extra = append(extra, sideKV(string(k), fmt.Sprintf("%d", n), inner))
		}
	}
	extra = append(extra, "", sideDim("  ↵ drill  ·  "+firstPretty(Keys.EditProject)+" edit  ·  "+
		firstPretty(Keys.DeleteProject)+" delete  ·  "+firstPretty(Keys.NewProject)+" new", inner))

	title := t.Name
	if t.Icon != "" {
		title = t.Icon + " " + title
	}
	return renderKVPanel(inner, title, rows, extra...)
}

func (m *Model) moveTagsCursor(delta int) {
	if m.devProjects == nil {
		return
	}
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil || len(tags) == 0 {
		return
	}
	m.tagsCursor += delta
	if m.tagsCursor < 0 {
		m.tagsCursor = 0
	}
	if m.tagsCursor >= len(tags) {
		m.tagsCursor = len(tags) - 1
	}
}

func (m *Model) triggerTagEdit() tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil || len(tags) == 0 {
		return nil
	}
	idx := m.tagsCursor
	if idx >= len(tags) {
		return nil
	}
	return m.openTagEditor(tags[idx].Tag)
}

func (m *Model) triggerTagDelete() tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	tags, err := m.devProjects.ListTagsWithUsage()
	if err != nil || len(tags) == 0 {
		return nil
	}
	idx := m.tagsCursor
	if idx >= len(tags) {
		return nil
	}
	t := tags[idx]
	hint := fmt.Sprintf("Delete %q? Cascades to %d binding(s) — cannot be undone.",
		t.Name, t.Count)
	state := choiceModalState{
		Title: "Delete tag",
		Hint:  hint,
		Options: []choiceOption{
			{Label: "Cancel", Value: "cancel", Cancel: true},
			{Label: "Delete", Hint: t.Name, Value: "ok"},
		},
		Cursor: 0,
		Save: func(val any) error {
			if val != "ok" {
				return nil
			}
			return m.devProjects.DeleteTag(t.ID)
		},
		SuccessMsg: "tag deleted",
		OnSuccess: func() tea.Cmd {
			if m.tagsCursor > 0 {
				m.tagsCursor--
			}
			return nil
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) triggerTagNew() tea.Cmd {
	return m.openTagEditor(devproject.Tag{Color: "blue"})
}
