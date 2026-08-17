package ui

// Export save dialog — the second-step modal that runs after the
// user has picked format/scope on a report (or other) export. It has
// one required field and one optional field:

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type exportSaveState struct {
	Title string

	Path       string
	pathCursor int

	OpenAfter     bool
	ShowOpenAfter bool

	focus int

	Confirm func(path string, openAfter bool, overwrite bool) tea.Cmd

	// overwritePath records the path for which the user has seen the
	// overwrite warning. Enter must be pressed again without changing the
	// path before Confirm receives overwrite=true.
	overwritePath string

	Err string
}

func (m *Model) openExportSaveModal(state exportSaveState) tea.Cmd {
	state.pathCursor = len([]rune(state.Path)) // rune index, not bytes
	m.exportSave = &state
	return nil
}

func (m Model) renderExportSaveModal() string {
	es := m.exportSave
	if es == nil {
		return ""
	}
	inner := modalWidth(m.width, 56, 72) - 4
	if inner < 30 {
		inner = 30
	}
	var lines []string
	titleStyle := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	lines = append(lines, titleStyle.Render(es.Title))
	lines = append(lines, "")

	pathLabel := "  Save to"
	if es.focus == 0 {
		pathLabel = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " Save to"
	}
	lines = append(lines, pathLabel)
	pathStyle := lipgloss.NewStyle().Foreground(theme.Fg).Background(theme.Panel)
	if es.focus == 0 {
		pathStyle = pathStyle.Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderHi)
	} else {
		pathStyle = pathStyle.Border(lipgloss.RoundedBorder()).BorderForeground(theme.Border)
	}
	lines = append(lines, pathStyle.Width(inner-4).Render(" "+es.Path+" "))
	lines = append(lines, "")

	if es.ShowOpenAfter {
		box := "[ ]"
		if es.OpenAfter {
			box = "[x]"
		}
		checkLabel := "  " + box + " Open after save"
		if es.focus == 1 {
			checkLabel = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") +
				" " + box + " " +
				lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render("Open after save")
		} else {
			checkLabel = lipgloss.NewStyle().Foreground(theme.Muted).Render(checkLabel)
		}
		lines = append(lines, checkLabel)
		lines = append(lines, "")
	}

	if es.Err != "" {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Red).Render("  "+es.Err))
		lines = append(lines, "")
	}

	hint := "  ↵ save  ·  esc cancel"
	if es.ShowOpenAfter {
		hint = "  ↵ save  ·  tab move  ·  space toggle  ·  esc cancel"
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render(hint))

	body := strings.Join(lines, "\n")
	box2 := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderHi).
		Background(theme.Bg).
		Padding(1, 2).
		Render(body)
	return box2
}

func (m Model) handleExportSaveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	es := m.exportSave
	if es == nil {
		return m, nil
	}
	key := msg.String()
	n := len([]rune(es.Path)) // rune length; pathCursor is a rune index
	switch key {
	case "esc":
		m.exportSave = nil
		return m, nil
	case "tab", "down":
		if es.ShowOpenAfter {
			es.focus = (es.focus + 1) % 2
		}
		return m, nil
	case "shift+tab", "up":
		if es.ShowOpenAfter {
			es.focus = (es.focus - 1 + 2) % 2
		}
		return m, nil
	case "enter":
		path := strings.TrimSpace(es.Path)
		if path == "" {
			es.Err = "path required"
			return m, nil
		}
		overwrite := false
		info, err := os.Lstat(expandTilde(path))
		if err == nil {
			if info.IsDir() {
				es.Err = "path is a directory"
				return m, nil
			}
			if es.overwritePath != path {
				es.overwritePath = path
				es.Err = "file exists — press Enter again to overwrite"
				return m, nil
			}
			overwrite = true
		} else if !errors.Is(err, os.ErrNotExist) {
			es.Err = "cannot check path: " + err.Error()
			return m, nil
		}
		openAfter := es.OpenAfter
		confirm := es.Confirm
		m.exportSave = nil
		if confirm != nil {
			return m, confirm(path, openAfter, overwrite)
		}
		return m, nil
	case "space":
		if es.ShowOpenAfter && es.focus == 1 {
			es.OpenAfter = !es.OpenAfter
			return m, nil
		}
		es.insertAtCursor(" ")
		return m, nil
	case "backspace":
		if es.focus == 0 {
			es.backspaceAtCursor()
		}
		return m, nil
	case "left":
		if es.focus == 0 && es.pathCursor > 0 {
			es.pathCursor--
		}
		return m, nil
	case "right":
		if es.focus == 0 && es.pathCursor < n {
			es.pathCursor++
		}
		return m, nil
	case "home", "ctrl+a":
		if es.focus == 0 {
			es.pathCursor = 0
		}
		return m, nil
	case "end", "ctrl+e":
		if es.focus == 0 {
			es.pathCursor = n
		}
		return m, nil
	case "ctrl+u":
		if es.focus == 0 {
			r := []rune(es.Path)
			if es.pathCursor <= len(r) {
				es.Path = string(r[es.pathCursor:])
			}
			es.pathCursor = 0
		}
		return m, nil
	}
	if es.focus == 0 {
		if press, ok := msg.(tea.KeyPressMsg); ok && press.Text != "" {
			es.insertAtCursor(press.Text)
			return m, nil
		}
	}
	return m, nil
}

// insertAtCursor inserts s at the rune cursor. Operates on []rune so
// multi-byte characters and cursor positions never split a UTF-8
// sequence.
func (es *exportSaveState) insertAtCursor(s string) {
	r := []rune(es.Path)
	if es.pathCursor < 0 {
		es.pathCursor = 0
	}
	if es.pathCursor > len(r) {
		es.pathCursor = len(r)
	}
	ins := []rune(s)
	out := make([]rune, 0, len(r)+len(ins))
	out = append(out, r[:es.pathCursor]...)
	out = append(out, ins...)
	out = append(out, r[es.pathCursor:]...)
	es.Path = string(out)
	es.pathCursor += len(ins)
}

func (es *exportSaveState) backspaceAtCursor() {
	if es.pathCursor <= 0 {
		return
	}
	r := []rune(es.Path)
	if es.pathCursor > len(r) {
		es.pathCursor = len(r)
	}
	es.Path = string(r[:es.pathCursor-1]) + string(r[es.pathCursor:])
	es.pathCursor--
}

// debug helper: format for log lines
func (es exportSaveState) String() string {
	return fmt.Sprintf("exportSave{path=%q open=%v focus=%d}", es.Path, es.OpenAfter, es.focus)
}
