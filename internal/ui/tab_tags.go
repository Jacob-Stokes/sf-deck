package ui

// /tags — the tag manager.
//
// Columns: PILL · NAME · ICON · COLOR · USAGE · CREATED.
// Per-row actions:
//   ↵ → edit (rename / change color / change icon)
//   d → delete (cascades all bindings) — confirmation modal
//   r → refresh usage counts (re-query)
//
// Tag bindings live in the same SQLite store as Dev Projects; the
// store API (ListTagsWithUsage, UpdateTag, DeleteTag) provides every
// operation this tab needs. No async — all SQLite is fast enough to
// run on the render goroutine.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderTags is the main-pane renderer for TabTags.
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

	// Pill column sizes to the widest rendered pill so longer tag
	// labels aren't truncated mid-name. lipgloss.Width measures the
	// pill's actual on-screen width (counting ANSI escapes correctly)
	// rather than relying on len(t.Name) + padding heuristics.
	pillW := 0
	for _, t := range tags {
		if w := lipgloss.Width(renderTagPill(t.Tag)); w > pillW {
			pillW = w
		}
	}
	if pillW < 8 {
		// Floor so the column doesn't collapse when every tag has a
		// very short name.
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

// sidebarTags shows usage breakdown for the cursored tag — what kinds
// of items are tagged with this, and how many of each.
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
	// Count by kind.
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

// moveTagsCursor handles ↑/↓/jump on the tags list.
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

// triggerTagEdit opens the tag-edit modal for the cursored tag. Used
// by Enter on TabTags. Falls through if no tags exist.
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

// triggerTagDelete confirms + deletes the cursored tag. Cascade-
// deletes all bindings (FK ON DELETE CASCADE). Includes a confirm
// modal so accidental presses don't nuke data.
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
			// Move cursor up one if it now points past the end.
			if m.tagsCursor > 0 {
				m.tagsCursor--
			}
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// triggerTagNew opens the tag-edit modal pre-populated for a new
// tag. Save creates the tag and refreshes the cursor onto it.
func (m *Model) triggerTagNew() tea.Cmd {
	return m.openTagEditor(devproject.Tag{Color: "blue"})
}
