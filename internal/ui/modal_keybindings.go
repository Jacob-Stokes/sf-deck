package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/keymap"
)

type keybindingsModalState struct {
	Filter string

	Cursor int

	EditingID  string
	EditBuffer string

	Err string

	SuccessMsg string

	FooterHints []footerHint

	Page  int
	About infoModalState
}

func (m *Model) openKeybindingsModal() tea.Cmd {
	m.keybindingsModal = &keybindingsModalState{
		About:       helpForCurrentView(*m),
		FooterHints: footerShortcutsAll(*m),
	}
	return nil
}

func (m *Model) closeKeybindingsModal() {
	m.keybindingsModal = nil
}

type keybindingsRow struct {
	Header string
	Cmd    *keymap.Command
	Hint   *footerHint
}

func visibleKeybindingRows(filter string, footer []footerHint) []keybindingsRow {
	q := strings.ToLower(strings.TrimSpace(filter))

	out := []keybindingsRow{}
	// "Footer · this view" leads: the exact hint set the status bar
	// shows for the active surface — including everything a narrow
	// terminal truncated. Same builder as the footer itself, so the
	// two can't drift.
	var hintRows []keybindingsRow
	for i := range footer {
		h := footer[i]
		if q != "" && !strings.Contains(strings.ToLower(h.d), q) &&
			!strings.Contains(strings.ToLower(h.k), q) {
			continue
		}
		hintRows = append(hintRows, keybindingsRow{Hint: &h})
	}
	if len(hintRows) > 0 {
		out = append(out, keybindingsRow{Header: "Footer · this view"})
		out = append(out, hintRows...)
	}

	type catGroup struct {
		name string
		cmds []keymap.Command
	}
	groups := []catGroup{}
	groupIdx := map[string]int{}
	for _, c := range keymap.Commands {
		if q != "" && !strings.Contains(strings.ToLower(c.Label), q) &&
			!strings.Contains(strings.ToLower(c.ID), q) {
			continue
		}
		idx, ok := groupIdx[c.Category]
		if !ok {
			groupIdx[c.Category] = len(groups)
			groups = append(groups, catGroup{name: c.Category, cmds: []keymap.Command{c}})
			continue
		}
		groups[idx].cmds = append(groups[idx].cmds, c)
	}
	// Stable category order — alphabetical, with "Process" first
	// because it's the most fundamental category.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].name == "Process" {
			return true
		}
		if groups[j].name == "Process" {
			return false
		}
		return groups[i].name < groups[j].name
	})
	for _, g := range groups {
		out = append(out, keybindingsRow{Header: g.name})
		for i := range g.cmds {
			cmd := g.cmds[i]
			out = append(out, keybindingsRow{Cmd: &cmd})
		}
	}
	return out
}

func (m Model) renderKeybindingsModal() string {
	km := m.keybindingsModal
	if km == nil {
		return ""
	}

	w := modalWidth(m.width, 70, 110)
	inner := w - 4

	rows := visibleKeybindingRows(km.Filter, km.FooterHints)

	// Clamp cursor to a real row (skip headers when landing).
	if km.Cursor < 0 {
		km.Cursor = 0
	}
	if km.Cursor >= len(rows) {
		km.Cursor = len(rows) - 1
	}
	for km.Cursor >= 0 && km.Cursor < len(rows) && rows[km.Cursor].Header != "" {
		km.Cursor++ // skip headers
	}
	if km.Cursor >= len(rows) {
		km.Cursor = 0
	}

	var lines []string
	lines = append(lines, keybindingsPageStrip(km, inner))
	lines = append(lines, strings.Repeat("─", inner))

	if km.Page == 1 {
		return modalBox(strings.Join(
			append(lines, renderKeybindingsAbout(km, m.height, inner)...), "\n"), w)
	}

	filterPrefix := lipgloss.NewStyle().Foreground(theme.FgDim).Render("/ ")
	caretStyle := lipgloss.NewStyle().Foreground(theme.BorderHi)
	if km.EditingID == "" {
		lines = append(lines, filterPrefix+km.Filter+caretStyle.Render("│"))
	} else {
		lines = append(lines, filterPrefix+km.Filter)
	}
	lines = append(lines, "")

	maxRows := 18
	bodyStart := 0
	if km.Cursor >= maxRows {
		bodyStart = km.Cursor - maxRows + 1
	}
	bodyEnd := bodyStart + maxRows
	if bodyEnd > len(rows) {
		bodyEnd = len(rows)
	}

	for i := bodyStart; i < bodyEnd; i++ {
		row := rows[i]
		if row.Header != "" {
			h := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).
				Render("  " + row.Header)
			lines = append(lines, h)
			continue
		}
		if row.Hint != nil {
			labelW := inner - 30
			if labelW < 20 {
				labelW = 20
			}
			line := fmt.Sprintf("  %-*s  %s", labelW, row.Hint.d, row.Hint.k)
			if i == km.Cursor {
				line = lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Blue).Render(line)
			}
			lines = append(lines, line)
			continue
		}
		c := row.Cmd
		if km.EditingID == c.ID {
			editLine := fmt.Sprintf("  %-40s ", c.Label) +
				lipgloss.NewStyle().Foreground(theme.BorderHi).Render("[ ") +
				km.EditBuffer +
				caretStyle.Render("│") +
				lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" ]")
			lines = append(lines, editLine)
			continue
		}
		keys := Keys.KeysByID(c.ID)
		keyDisplay := strings.Join(keys, " · ")
		if keyDisplay == "" {
			keyDisplay = lipgloss.NewStyle().Foreground(theme.FgDim).
				Italic(true).Render("(unbound)")
		}
		labelW := inner - 30
		if labelW < 20 {
			labelW = 20
		}
		label := c.Label
		if len(label) > labelW {
			label = ansi.Truncate(label, labelW, "…")
		}
		line := fmt.Sprintf("  %-*s  %s", labelW, label, keyDisplay)
		if i == km.Cursor {
			line = lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Blue).Render(line)
		}
		lines = append(lines, line)
	}

	if km.Err != "" {
		lines = append(lines, "")
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Red).Render("  "+km.Err))
	}
	if km.SuccessMsg != "" {
		lines = append(lines, "")
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.Green).Render("  "+km.SuccessMsg))
	}

	lines = append(lines, "")
	footer := lipgloss.NewStyle().Foreground(theme.FgDim).
		Render("↑↓ navigate · ↵ edit · esc close · saved to ~/.sf-deck/keybindings.toml")
	if km.EditingID != "" {
		footer = lipgloss.NewStyle().Foreground(theme.FgDim).
			Render("type new keys (space-sep), ↵ apply · esc cancel")
	}
	lines = append(lines, footer)

	return modalBox(strings.Join(lines, "\n"), w)
}

func (m *Model) handleKeybindingsModalKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keybindingsModal == nil {
		return false, nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return true, nil
	}
	km := m.keybindingsModal

	if km.EditingID != "" {
		return m.handleKeybindingsEdit(press)
	}

	if press.Code == tea.KeyTab {
		km.Page = 1 - km.Page
		km.Err = ""
		return true, nil
	}
	if km.Page == 1 {
		if press.Code == tea.KeyEsc {
			m.closeKeybindingsModal()
		}
		return true, nil
	}

	switch press.Code {
	case tea.KeyEsc:
		m.closeKeybindingsModal()
		return true, nil
	case tea.KeyEnter:
		rows := visibleKeybindingRows(km.Filter, km.FooterHints)
		if km.Cursor < 0 || km.Cursor >= len(rows) || rows[km.Cursor].Cmd == nil {
			return true, nil
		}
		c := rows[km.Cursor].Cmd
		km.EditingID = c.ID
		km.EditBuffer = strings.Join(Keys.KeysByID(c.ID), " ")
		km.Err = ""
		return true, nil
	case tea.KeyUp:
		rows := visibleKeybindingRows(km.Filter, km.FooterHints)
		for km.Cursor > 0 {
			km.Cursor--
			if km.Cursor < len(rows) && rows[km.Cursor].Header == "" {
				return true, nil
			}
		}
		return true, nil
	case tea.KeyDown:
		rows := visibleKeybindingRows(km.Filter, km.FooterHints)
		for km.Cursor < len(rows)-1 {
			km.Cursor++
			if km.Cursor < len(rows) && rows[km.Cursor].Header == "" {
				return true, nil
			}
		}
		return true, nil
	case tea.KeyBackspace:
		if len(km.Filter) > 0 {
			km.Filter = km.Filter[:len(km.Filter)-1]
			km.Cursor = 0
		}
		return true, nil
	}

	r := keypressRune(press)
	if r != 0 {
		km.Filter += string(r)
		km.Cursor = 0
	}
	return true, nil
}

func (m *Model) handleKeybindingsEdit(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	km := m.keybindingsModal
	switch msg.Code {
	case tea.KeyEsc:
		km.EditingID = ""
		km.EditBuffer = ""
		km.Err = ""
		return true, nil
	case tea.KeyEnter:
		keys := strings.Fields(km.EditBuffer)
		if err := Keys.SetByID(km.EditingID, keys); err != nil {
			km.Err = err.Error()
			return true, nil
		}
		m.clearRenderCache()
		if err := Keys.SaveTOML(); err != nil {
			km.Err = "applied in memory; disk write failed: " + err.Error()
			return true, nil
		}
		km.SuccessMsg = "saved"
		km.EditingID = ""
		km.EditBuffer = ""
		return true, nil
	case tea.KeyBackspace:
		if len(km.EditBuffer) > 0 {
			km.EditBuffer = km.EditBuffer[:len(km.EditBuffer)-1]
		}
		return true, nil
	}
	r := keypressRune(msg)
	if r != 0 {
		km.EditBuffer += string(r)
	}
	return true, nil
}

func keybindingsPageStrip(km *keybindingsModalState, inner int) string {
	active := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	idle := lipgloss.NewStyle().Foreground(theme.FgDim)
	sep := lipgloss.NewStyle().Foreground(theme.Muted).Render("  ·  ")
	aboutLabel := "About"
	if km.About.Title != "" && km.About.Title != "Help" {
		aboutLabel = "About · " + km.About.Title
	}
	keysLbl, aboutLbl := idle.Render("Keybindings"), idle.Render(aboutLabel)
	if km.Page == 0 {
		keysLbl = active.Render("Keybindings")
	} else {
		aboutLbl = active.Render(aboutLabel)
	}
	hint := lipgloss.NewStyle().Foreground(theme.FgDim).Render("  (tab switches)")
	return ansi.Truncate(keysLbl+sep+aboutLbl+hint, inner, "…")
}

// renderKeybindingsAbout renders the About page body — the same
// Label/Body row shape the old standalone info modal used. Height-
// clamped so long pages can't push the modal off-screen.
func renderKeybindingsAbout(km *keybindingsModalState, termH, inner int) []string {
	labelStyle := lipgloss.NewStyle().Foreground(theme.Muted).Bold(true)
	var lines []string
	for _, r := range km.About.Rows {
		switch {
		case r.Label == "" && r.Body == "":
			lines = append(lines, "")
		case r.Label == "":
			lines = append(lines, ansi.Truncate(r.Body, inner, "…"))
		default:
			lines = append(lines, ansi.Truncate(
				labelStyle.Render(r.Label)+"  "+r.Body, inner, "…"))
		}
	}
	maxBody := termH - 8
	if maxBody < 5 {
		maxBody = 5
	}
	if len(lines) > maxBody {
		lines = append(lines[:maxBody-1],
			lipgloss.NewStyle().Foreground(theme.FgDim).Render("… (truncated)"))
	}
	lines = append(lines, "",
		lipgloss.NewStyle().Foreground(theme.FgDim).
			Render("tab keybindings · esc close"))
	return lines
}
