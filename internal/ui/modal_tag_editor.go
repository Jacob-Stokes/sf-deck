package ui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type tagEditorState struct {
	Original devproject.Tag

	Name  string
	Color string
	Icon  string

	Field int

	Err error
}

func (m *Model) openTagEditor(t devproject.Tag) tea.Cmd {
	color := t.Color
	if color == "" {
		color = "blue"
	}
	m.tagEditor = &tagEditorState{
		Original: t,
		Name:     t.Name,
		Color:    color,
		Icon:     t.Icon,
		Field:    0,
	}
	return nil
}

func (m Model) renderTagEditor() string {
	te := m.tagEditor
	if te == nil {
		return ""
	}
	title := "New tag"
	if te.Original.ID > 0 {
		title = "Edit tag · " + te.Original.Name
	}
	header := lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).Render(title)

	preview := renderTagPill(devproject.Tag{
		Name:  fallbackName(te.Name),
		Color: te.Color,
		Icon:  te.Icon,
	})

	nameRow := tagEditorRow("name", fallbackName(te.Name), te.Field == 0)
	colorRow := tagEditorRow("color", colorPalettePicker(te.Color, te.Field == 1), te.Field == 1)
	iconRow := tagEditorRow("icon", iconValueOrPlaceholder(te.Icon), te.Field == 2)

	hint := lipgloss.NewStyle().Foreground(theme.Muted).Render(
		"↑/↓ move · ←/→ change color · type to set name/icon · ↵ save · esc cancel")

	rows := []string{
		header,
		"",
		"  preview: " + preview,
		"",
		nameRow,
		colorRow,
		iconRow,
		"",
		hint,
	}
	if te.Err != nil {
		rows = append(rows, lipgloss.NewStyle().Foreground(theme.Red).Render(
			"error: "+te.Err.Error()))
	}
	width := modalWidth(m.width, 50, 70)
	return modalBox(strings.Join(rows, "\n"), width)
}

func fallbackName(s string) string {
	if s == "" {
		return "(unnamed)"
	}
	return s
}

func iconValueOrPlaceholder(s string) string {
	if s == "" {
		return lipgloss.NewStyle().Foreground(theme.FgDim).Render("(none — type any character)")
	}
	return s
}

func tagEditorRow(label, value string, focused bool) string {
	mark := "  "
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	valueStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if focused {
		mark = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▶ ")
		labelStyle = labelStyle.Bold(true)
	}
	return mark + labelStyle.Render(label) + "  " + valueStyle.Render(value)
}

func colorPalettePicker(active string, focused bool) string {
	parts := make([]string, 0, len(tagPalette))
	for _, c := range tagPalette {
		style := lipgloss.NewStyle().Foreground(tagColorFor(c))
		label := c
		if c == active {
			style = style.Bold(true).Underline(true)
			label = "[" + c + "]"
		}
		parts = append(parts, style.Render(label))
	}
	out := strings.Join(parts, " ")
	if focused {
		out += "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render("←/→ change")
	}
	return out
}

func (m Model) updateTagEditor(msg tea.KeyMsg) (Model, tea.Cmd) {
	te := m.tagEditor
	if te == nil {
		return m, nil
	}
	key := msg.String()

	switch key {
	case "esc":
		m.tagEditor = nil
		return m, nil
	case "down", "tab":
		te.Field = (te.Field + 1) % 3
		return m, nil
	case "up", "shift+tab":
		te.Field--
		if te.Field < 0 {
			te.Field = 2
		}
		return m, nil
	case "left", "right":
		if te.Field == 1 {
			te.Color = cycleColor(te.Color, key == "right")
		}
		return m, nil
	case "backspace":
		switch te.Field {
		case 0:
			if len(te.Name) > 0 {
				te.Name = te.Name[:len(te.Name)-1]
			}
		case 2:
			te.Icon = ""
		}
		return m, nil
	case "enter":
		return m, m.commitTagEditor()
	}

	if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
		switch te.Field {
		case 0:
			te.Name += key
		case 2:
			te.Icon = key
		}
	}
	if te.Field == 2 && len(key) > 1 && !strings.HasPrefix(key, "ctrl+") &&
		!strings.HasPrefix(key, "alt+") && !strings.HasPrefix(key, "shift+") {
		te.Icon = key
	}
	return m, nil
}

func (m *Model) commitTagEditor() tea.Cmd {
	te := m.tagEditor
	if te == nil {
		return nil
	}
	name := strings.TrimSpace(te.Name)
	if name == "" {
		te.Err = errors.New("name required")
		return nil
	}
	if m.devProjects == nil {
		te.Err = errors.New("store unavailable")
		return nil
	}
	if te.Original.ID == 0 {
		_, err := m.devProjects.CreateTag(name, te.Color, te.Icon)
		if err != nil {
			te.Err = err
			return nil
		}
		m.flash("created tag · " + name)
	} else {
		if err := m.devProjects.UpdateTag(te.Original.ID, name, te.Color, te.Icon); err != nil {
			te.Err = err
			return nil
		}
		m.flash("updated tag · " + name)
	}
	m.tagEditor = nil
	return nil
}

func cycleColor(active string, forward bool) string {
	if len(tagPalette) == 0 {
		return ""
	}
	idx := -1
	for i, c := range tagPalette {
		if c == active {
			idx = i
			break
		}
	}
	if idx == -1 {
		return tagPalette[0]
	}
	if forward {
		idx = (idx + 1) % len(tagPalette)
	} else {
		idx = (idx - 1 + len(tagPalette)) % len(tagPalette)
	}
	return tagPalette[idx]
}
