package ui

// Keybindings settings modal — user-editable list of every command
// in the keymap registry. Reachable via the command palette
// ("Edit keybindings…") or the global settings modal.
//
// UX shape:
//
//   ┌─ Keybindings ─────────────────────────────────────────────┐
//   │ /  filter                                                  │
//   │                                                            │
//   │   Process                                                  │
//   │ ▶ Quit                                          q · ctrl+c │
//   │   Focus orgs panel                                       0 │
//   │   Back / cancel                                        esc │
//   │                                                            │
//   │   Navigation                                               │
//   │   Move up                                          k · up  │
//   │   Move down                                        j · down│
//   │ ...                                                        │
//   │                                                            │
//   │ ↑↓ navigate · ↵ edit · esc close                           │
//   └────────────────────────────────────────────────────────────┘
//
// Editing a row opens a small input below it; the user types a
// space-separated list of keys, presses Enter to apply. Conflict
// warnings appear inline. Save-on-apply persists to
// ~/.sf-deck/keybindings.toml so changes survive restart.

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

// keybindingsModalState is the live state of the modal.
type keybindingsModalState struct {
	// Filter narrows the visible rows by Label substring.
	Filter string

	// Cursor is the highlighted row index in the (filtered) list.
	Cursor int

	// EditingID, when non-empty, is the command currently being
	// rebound. EditBuffer is the user's in-progress key list
	// (space-separated).
	EditingID  string
	EditBuffer string

	// Err is the non-fatal error from the last apply attempt.
	Err string

	// SuccessMsg flashes briefly after a successful save.
	SuccessMsg string

	// FooterHints is the active surface's footer hint set, captured
	// at open time — pinned as the first section of the Keys page so
	// "what does the footer show (including what got truncated)" is
	// answerable at a glance. Read-only rows; rebinding happens on
	// the command rows below.
	FooterHints []footerHint

	// Page selects between the Keys list (0) and the About page (1)
	// — the per-view help that `?` used to show in its own modal.
	// Tab toggles. About is captured at open time so the content
	// reflects the surface the user pressed ? on.
	Page  int
	About infoModalState
}

// openKeybindingsModal initialises and shows the modal on the Keys
// page. The per-view help rides along as the About page (Tab) so ?
// serves both "what can I press" and "what is this view".
func (m *Model) openKeybindingsModal() tea.Cmd {
	m.keybindingsModal = &keybindingsModalState{
		About:       helpForCurrentView(*m),
		FooterHints: footerShortcutsAll(*m),
	}
	return nil
}

// closeKeybindingsModal dismisses the modal.
func (m *Model) closeKeybindingsModal() {
	m.keybindingsModal = nil
}

// keybindingsRow is one row in the rendered list. Either a category
// header (Header non-empty, Cmd nil) or an actual command row.
type keybindingsRow struct {
	Header string
	Cmd    *keymap.Command
	// Hint is a read-only footer-shortcut row (the "Footer · this
	// view" section). Mutually exclusive with Cmd.
	Hint *footerHint
}

// visibleKeybindingRows produces the row slice for the current
// filter. Rows are grouped by Category, with category headers
// inserted between groups.
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

	// Group commands by category, preserving registry order within
	// each group.
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

// renderKeybindingsModal draws the modal; "" when not active.
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

	// Filter / search bar.
	filterPrefix := lipgloss.NewStyle().Foreground(theme.FgDim).Render("/ ")
	caretStyle := lipgloss.NewStyle().Foreground(theme.BorderHi)
	if km.EditingID == "" {
		// Filter typing is the default mode when not editing a row.
		lines = append(lines, filterPrefix+km.Filter+caretStyle.Render("│"))
	} else {
		lines = append(lines, filterPrefix+km.Filter)
	}
	lines = append(lines, "")

	// Body — windowed slice around cursor.
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
			// Footer-shortcut row: read-only, same label/key column
			// layout as command rows so the section scans uniformly.
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
		// Edit mode for this specific row.
		if km.EditingID == c.ID {
			editLine := fmt.Sprintf("  %-40s ", c.Label) +
				lipgloss.NewStyle().Foreground(theme.BorderHi).Render("[ ") +
				km.EditBuffer +
				caretStyle.Render("│") +
				lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" ]")
			lines = append(lines, editLine)
			continue
		}
		// Normal display.
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

// handleKeybindingsModalKey processes input while the modal is
// open. Returns (handled, cmd).
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

	// Tab flips between the Keys list and the About page from
	// either side; the About page consumes everything else except
	// esc (no filter, no edit there).
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
		// Open edit on the cursored row.
		rows := visibleKeybindingRows(km.Filter, km.FooterHints)
		if km.Cursor < 0 || km.Cursor >= len(rows) || rows[km.Cursor].Cmd == nil {
			// Footer-hint rows are read-only; the same action is
			// rebindable from its command row further down.
			return true, nil
		}
		c := rows[km.Cursor].Cmd
		km.EditingID = c.ID
		// Pre-fill with current bindings so the user edits in
		// place rather than starting from blank.
		km.EditBuffer = strings.Join(Keys.KeysByID(c.ID), " ")
		km.Err = ""
		return true, nil
	case tea.KeyUp:
		rows := visibleKeybindingRows(km.Filter, km.FooterHints)
		// Step backwards over headers.
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

// handleKeybindingsEdit handles the per-row edit input. Enter
// applies, Esc cancels, Backspace deletes a char, anything else
// appends.
func (m *Model) handleKeybindingsEdit(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	km := m.keybindingsModal
	switch msg.Code {
	case tea.KeyEsc:
		km.EditingID = ""
		km.EditBuffer = ""
		km.Err = ""
		return true, nil
	case tea.KeyEnter:
		// Apply the buffer. Empty buffer means "unbind" (set to nil).
		// Mutate ui.Keys directly — that's the global the
		// dispatcher reads, so changes take effect immediately.
		keys := strings.Fields(km.EditBuffer)
		if err := Keys.SetByID(km.EditingID, keys); err != nil {
			km.Err = err.Error()
			return true, nil
		}
		m.clearRenderCache()
		// Persist to disk. On error, surface but leave the change in
		// memory so the user can retry without retyping.
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

// keybindingsPageStrip renders the two-page header: Keybindings ·
// About. Active page bold in the accent colour; the About label
// borrows the captured help page's title when it has one so users
// see what's behind the tab ("About · Field detail · actions").
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
