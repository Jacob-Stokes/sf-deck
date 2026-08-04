package ui

// Export save dialog — the second-step modal that runs after the
// user has picked format/scope on a report (or other) export. It has
// one required field and one optional field:
//
//   1. Save path — single-line text editor pre-populated with the
//      configured default. User can rename or change directory.
//   2. Open after save — an optional checkbox, on by default. When set,
//      sf-deck calls openPath() on the resulting file as soon as
//      the export completes successfully.
//
// Why a dedicated modal type and not a reuse of editModalState: the
// overwrite check must be race-safe and shared by every file export,
// while some callers also need the checkbox. A purpose-built form
// keeps those guarantees in one place.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// exportSaveState is the live state for the save-export modal. The
// modal owns its own focus cursor (0 = path field, 1 = open
// checkbox) so tab/shift+tab cycles cleanly without leaking into the
// surrounding key dispatch. Enter confirms from either focus.
type exportSaveState struct {
	Title string

	// Path is the editable save path. Modified in place as the user
	// types; submitted via Confirm.
	Path string
	// pathCursor is the character cursor within Path. Always in
	// [0, len(Path)].
	pathCursor int

	// OpenAfter is the auto-open toggle. True by default.
	OpenAfter bool
	// ShowOpenAfter controls whether the open-after-save checkbox is part of
	// the form. Some exports (for example DevProject reference lists) only
	// need the shared path validation and overwrite confirmation.
	ShowOpenAfter bool

	// focus tracks which field has the cursor: 0 = path, 1 = open
	// toggle (when ShowOpenAfter is true). Confirm is bound to enter on
	// any focus.
	focus int

	// Confirm fires on enter (any focus). Receives the final
	// (path, openAfter) values. The closure is responsible for
	// dispatching whatever message kicks off the actual export.
	Confirm func(path string, openAfter bool, overwrite bool) tea.Cmd

	// overwritePath records the path for which the user has seen the
	// overwrite warning. Enter must be pressed again without changing the
	// path before Confirm receives overwrite=true.
	overwritePath string

	// Err is the last validation error (empty path, etc.). Cleared
	// on the next Confirm attempt.
	Err string
}

// openExportSaveModal mounts the modal and returns nil (no async
// work needed at open time).
func (m *Model) openExportSaveModal(state exportSaveState) tea.Cmd {
	state.pathCursor = len([]rune(state.Path)) // rune index, not bytes
	m.exportSave = &state
	return nil
}

// renderExportSaveModal is called from render.go's overlay layer
// when m.exportSave is non-nil.
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

	// Path field
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
		// Checkbox field
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

// handleExportSaveKey routes keystrokes to the export-save modal
// when it has focus. Always returns ok=true (the modal swallows
// unhandled keys so they don't leak to the surface below).
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
	// Printable input → insert at the cursor when the path field has
	// focus. KeyPressMsg.Text carries the typed rune(s) (empty for
	// control/named keys); this covers multi-byte input (é, ü, CJK,
	// emoji in a folder name) that the old len(key)==1 ASCII check
	// silently dropped.
	if es.focus == 0 {
		if press, ok := msg.(tea.KeyPressMsg); ok && press.Text != "" {
			es.insertAtCursor(press.Text)
			return m, nil
		}
	}
	// Swallow anything else so it doesn't leak to the surface below.
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

// backspaceAtCursor removes the rune immediately before the cursor.
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
