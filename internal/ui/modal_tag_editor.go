package ui

// Tag editor modal — create-or-edit a single tag. Used by the tag
// manager's `n` (new) and `↵` (edit) gestures.
//
// Three fields: name, color, icon. Tab cycles between them; Enter
// saves; Esc cancels. The color slot is a 7-entry palette picker
// (matches tagPalette) cycled with ←/→. The icon slot is a single-
// rune capture — first printable character lands as the icon (or
// backspace clears).
//
// Reuses the modalBox primitive + flash-on-success pattern shared by
// editModal / choiceModal.

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// tagEditorState holds the live state of the editor.
type tagEditorState struct {
	// Original is the tag being edited (zero ID = create new).
	Original devproject.Tag

	// Working values (mutated as the user types / cycles).
	Name  string
	Color string
	Icon  string

	// Field is the focused slot: 0=name, 1=color, 2=icon.
	Field int

	// Err is the last commit error so the user can fix and retry.
	Err error
}

// openTagEditor opens the editor for an existing tag (Original.ID > 0)
// or a new one (Original.ID == 0). Pre-populates from Original.
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

// renderTagEditor draws the editor overlay. Returns "" when not open.
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

	// Live-preview pill of the in-progress edit.
	preview := renderTagPill(devproject.Tag{
		Name:  fallbackName(te.Name),
		Color: te.Color,
		Icon:  te.Icon,
	})

	// Three rows, each with a bullet on the focused slot.
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

// tagEditorRow formats one labelled field row with a focus marker.
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

// colorPalettePicker returns "blue cyan green …" with the active color
// highlighted. The full palette renders so the user sees every option
// without cycling blindly. When focused, a trailing "←/→" cue makes the
// change affordance obvious right on the row (the footer hint alone was
// easy to miss).
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

// updateTagEditor handles key presses.
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
		// Move to the next field. Arrows are the primary way (intuitive);
		// tab still works for keyboard-flow habit.
		te.Field = (te.Field + 1) % 3
		return m, nil
	case "up", "shift+tab":
		te.Field--
		if te.Field < 0 {
			te.Field = 2
		}
		return m, nil
	case "left", "right":
		// On the colour line, ←/→ cycle the palette. On other lines they
		// do nothing (name/icon are text — arrows don't move a caret here).
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

	// Printable input. Color slot consumes nothing here (use ←/→).
	if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
		switch te.Field {
		case 0:
			te.Name += key
		case 2:
			// Icon = single rune; replace whatever was there.
			te.Icon = key
		}
	}
	// Unicode runes (emoji etc.) come through with longer key strings.
	// We only accept them for the icon slot.
	if te.Field == 2 && len(key) > 1 && !strings.HasPrefix(key, "ctrl+") &&
		!strings.HasPrefix(key, "alt+") && !strings.HasPrefix(key, "shift+") {
		te.Icon = key
	}
	return m, nil
}

// commitTagEditor saves the tag (create or update) and closes the
// modal on success. Failures stay on screen with the error visible.
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

// cycleColor advances (or reverses) through tagPalette, wrapping at
// the ends. Unknown active color → first palette entry.
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
