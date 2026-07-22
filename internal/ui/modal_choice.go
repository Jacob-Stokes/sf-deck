package ui

// Choice modal — a "pick one of these options" overlay used when
// editing a field's boolean/enum Metadata property. Sibling to the
// text editModal; shares the same modalBox primitive, Save/OnSuccess
// callback shape, and saving/loading lifecycle.
//
// Use when: the set of valid values is small + discrete (required vs
// nullable, cascade vs restricted-delete, picklist-kind vs text,
// etc.). For free-form prose → editModal.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// choiceModalVisibleIndices returns the indices into cm.Options that
// should be visible given the current search state. When no search is
// active or the modal isn't searchable, returns every index in order.
func choiceModalVisibleIndices(cm *choiceModalState) []int {
	out := make([]int, 0, len(cm.Options))
	q := strings.ToLower(cm.SearchQuery)
	if !cm.Searchable || q == "" {
		for i := range cm.Options {
			out = append(out, i)
		}
		return out
	}
	for i, opt := range cm.Options {
		if strings.Contains(strings.ToLower(opt.Label), q) ||
			strings.Contains(strings.ToLower(opt.Hint), q) {
			out = append(out, i)
		}
	}
	return out
}

// choiceModalSyncCursor maps cm.visibleCursor → cm.Cursor (the index
// into cm.Options) so submit handlers find the correct row.
func choiceModalSyncCursor(cm *choiceModalState) {
	visible := choiceModalVisibleIndices(cm)
	if len(visible) == 0 {
		return
	}
	if cm.visibleCursor < 0 {
		cm.visibleCursor = 0
	}
	if cm.visibleCursor >= len(visible) {
		cm.visibleCursor = len(visible) - 1
	}
	cm.Cursor = visible[cm.visibleCursor]
}

// choiceModalSkipHeading nudges the cursor off Heading rows in the
// given direction (+1 down, -1 up), bouncing at the list edges. Call
// after any cursor move/reset; no-op when the row is selectable. If
// every visible row is a heading (degenerate), the cursor stays put.
func choiceModalSkipHeading(cm *choiceModalState, dir int) {
	visible := choiceModalVisibleIndices(cm)
	if len(visible) == 0 || dir == 0 {
		return
	}
	bounced := false
	for i := 0; i < len(visible); i++ {
		if !cm.Options[visible[cm.visibleCursor]].Heading {
			cm.Cursor = visible[cm.visibleCursor]
			return
		}
		next := cm.visibleCursor + dir
		if next < 0 || next >= len(visible) {
			if bounced {
				return
			}
			bounced = true
			dir = -dir
			continue
		}
		cm.visibleCursor = next
	}
	cm.Cursor = visible[cm.visibleCursor]
}

// choiceModalWindow returns [start, end) for an option viewport of
// `visible` rows around `cursor`, clamped to [0, n). Cursor sits ~1/3
// down the visible window so scrolling feels like vim's `j` rather
// than landing on the top edge after each move.
func choiceModalWindow(cursor, n, visible int) (int, int) {
	if visible <= 0 || n <= 0 {
		return 0, 0
	}
	if n <= visible {
		return 0, n
	}
	start := cursor - visible/3
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > n {
		end = n
		start = end - visible
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

// choiceOption is one selectable value in the choice modal.
type choiceOption struct {
	// Label is the human-readable name shown in the list.
	Label string
	// Hint is an optional one-line explainer shown under the label
	// when this option is selected.
	Hint string
	// Value is the raw value committed via Save(value). Kept
	// separate from Label so we can show "Nullable" but commit
	// `true`, etc.
	Value any
	// Cancel marks this option as a dismiss-without-saving shortcut.
	// Useful for destructive-confirmation modals where Cancel is
	// the safe default and shouldn't fire Save at all.
	Cancel bool
	// Heading marks a non-selectable section label (e.g. the
	// "── built-ins ──" separators in the chip manager). Headings
	// render in the list but the cursor skips over them and Enter
	// never fires on them.
	Heading bool
}

// choiceModalState is the live state of a choice modal.
type choiceModalState struct {
	// Title / Hint / Loading / Saving / Err mirror editModalState
	// so the render + key handlers feel familiar.
	Title string
	Hint  string

	// Options is the full list; Cursor is the current highlight. The
	// modal doesn't enforce a "default option" — callers pre-position
	// the cursor to reflect the current value.
	Options []choiceOption
	Cursor  int

	// SuccessMsg is the flash-banner string shown after a successful
	// commit. Optional.
	SuccessMsg string

	// LoadCurrent, Save, OnSuccess — same roles as on editModalState.
	// Save receives the raw choiceOption.Value of the selected row
	// so the caller doesn't need to re-lookup after cursor moves.
	// OnSuccess receives the same val when set, so multi-step modal
	// flows can carry the picked value through to the next step
	// without stashing it in a package-level global. Older callers
	// using OnSuccess func() tea.Cmd should migrate; the modal
	// invokes OnSuccessTyped when set, otherwise falls back to the
	// no-arg variant.
	LoadCurrent    func() (any, error)
	Save           func(val any) error
	OnSuccess      func() tea.Cmd
	OnSuccessTyped func(val any) tea.Cmd

	// OnCancel fires when the user dismisses the modal with esc. When
	// set, it runs INSTEAD of a bare close — used by drill-down menus
	// (Settings → submenu → leaf) so esc pops one level up (reopening
	// the parent menu) rather than closing the whole stack. Nil = plain
	// close, the default.
	OnCancel func() tea.Cmd

	// Searchable enables `/` to start a substring filter over Label
	// + Hint. Used by long pickers (list-view import, theme picker
	// could swap to this). Off by default to keep small confirm-style
	// modals (cancel/delete) free of unexpected typing behaviour.
	Searchable bool

	// AltKeys lists extra single-rune keys (e.g. "e") that act on the
	// selected option as a secondary channel beside Enter. Pressing
	// one closes the modal and fires OnAltTyped(key, value). Ignored
	// while the search input is active, and on Heading/Cancel rows.
	AltKeys    string
	OnAltTyped func(key string, val any) tea.Cmd

	// Wide opts the modal into a larger size (~80% of terminal,
	// clamped to 80..140) for browse-shaped modals like the chip
	// manager that need room for tabular row content. Default is
	// the compact 48..70 confirm size.
	Wide bool

	// Internal live state.
	Loading       bool
	Saving        bool
	Err           string
	SearchActive  bool   // true while the user is typing a filter
	SearchQuery   string // committed filter buffer
	visibleCursor int    // cursor position in the filtered slice
}

// renderChoiceModal renders the choice modal, or "".
func (m Model) renderChoiceModal() string {
	if m.choiceModal == nil {
		return ""
	}
	cm := m.choiceModal
	minW, maxW := 48, 70
	if cm.Wide {
		minW, maxW = 80, 140
	}
	w := modalWidth(m.width, minW, maxW)
	inner := w - 4

	var lines []string
	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(cm.Title),
		strings.Repeat("─", inner),
	)
	if cm.Hint != "" {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Render(cm.Hint),
			"",
		)
	}

	// Search bar — only shown when the modal opted in.
	if cm.Searchable {
		switch {
		case cm.SearchActive:
			caret := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("│")
			lines = append(lines,
				lipgloss.NewStyle().Foreground(theme.FgDim).Render("/")+cm.SearchQuery+caret)
		case cm.SearchQuery != "":
			lines = append(lines,
				lipgloss.NewStyle().Foreground(theme.FgDim).Render("/")+cm.SearchQuery)
		default:
			lines = append(lines,
				lipgloss.NewStyle().Foreground(theme.FgDim).Render("/  type to filter"))
		}
		lines = append(lines, "")
	}

	if cm.Loading {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).
				Render("loading current value…"))
	} else {
		// Filter the option list when a search is active.
		visible := choiceModalVisibleIndices(cm)
		if len(visible) == 0 {
			lines = append(lines,
				lipgloss.NewStyle().Foreground(theme.FgDim).Italic(true).Render("  no matches"))
		} else {
			// Viewport around the cursor. Wide modals (chip manager
			// today) get more rows because they're browse-shaped —
			// users want to scan the full list, not a peek window.
			visibleRows := 16
			if cm.Wide {
				visibleRows = 24
			}
			cursorPos := cm.visibleCursor
			if cursorPos < 0 || cursorPos >= len(visible) {
				cursorPos = 0
			}
			start, end := choiceModalWindow(cursorPos, len(visible), visibleRows)
			if start > 0 {
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).
					Render("    ↑ more above"))
			}
			for vi := start; vi < end; vi++ {
				i := visible[vi]
				opt := cm.Options[i]
				prefix := "  "
				labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
				hintStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
				if opt.Cancel {
					labelStyle = lipgloss.NewStyle().Foreground(theme.FgDim)
				}
				if vi == cursorPos {
					prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
					labelStyle = labelStyle.Bold(true)
				}
				lines = append(lines, prefix+labelStyle.Render(opt.Label))
				if opt.Hint != "" && vi == cursorPos {
					lines = append(lines, hintStyle.Render("    "+opt.Hint))
				}
			}
			if end < len(visible) {
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).
					Render(fmt.Sprintf("    ↓ %d more below", len(visible)-end)))
			}
		}
	}

	lines = append(lines, "")
	switch {
	case cm.Saving:
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Render("saving…"))
	case cm.Err != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("error: "+cm.Err))
	}

	lines = append(lines,
		lipgloss.NewStyle().Foreground(theme.FgDim).
			Render("j/k select · enter save · esc cancel"))
	return modalBox(strings.Join(lines, "\n"), w)
}

// handleChoiceModalKey — reducer while the choice modal is open.
func (m Model) handleChoiceModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.choiceModal == nil {
		return m, nil
	}
	cm := m.choiceModal
	if cm.Saving || cm.Loading {
		if msg.String() == "esc" {
			m.choiceModal = nil
		}
		return m, nil
	}
	key := msg.String()

	// Active search input — keystrokes go into the buffer until the
	// user commits with enter or escapes.
	if cm.Searchable && cm.SearchActive {
		switch key {
		case "esc":
			cm.SearchActive = false
			if cm.SearchQuery != "" {
				cm.SearchQuery = ""
				cm.visibleCursor = 0
				choiceModalSyncCursor(cm)
			}
			return m, nil
		case "enter":
			cm.SearchActive = false
			choiceModalSyncCursor(cm)
			return m, nil
		case "backspace":
			if len(cm.SearchQuery) > 0 {
				cm.SearchQuery = cm.SearchQuery[:len(cm.SearchQuery)-1]
				cm.visibleCursor = 0
			}
			return m, nil
		case "ctrl+u":
			cm.SearchQuery = ""
			cm.visibleCursor = 0
			return m, nil
		case "ctrl+c":
			m.choiceModal = nil
			return m, nil
		case "down", "ctrl+n":
			cm.SearchActive = false
			visible := choiceModalVisibleIndices(cm)
			if cm.visibleCursor < len(visible)-1 {
				cm.visibleCursor++
			}
			choiceModalSyncCursor(cm)
			choiceModalSkipHeading(cm, 1)
			return m, nil
		case "up", "ctrl+p":
			cm.SearchActive = false
			if cm.visibleCursor > 0 {
				cm.visibleCursor--
			}
			choiceModalSyncCursor(cm)
			choiceModalSkipHeading(cm, -1)
			return m, nil
		}
		// Append printable single-rune keys to the search buffer.
		if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
			cm.SearchQuery += key
			cm.visibleCursor = 0
			return m, nil
		}
		return m, nil
	}

	if cm.OnAltTyped != nil && cm.AltKeys != "" && len(key) == 1 &&
		strings.Contains(cm.AltKeys, key) {
		if cm.Cursor >= 0 && cm.Cursor < len(cm.Options) {
			opt := cm.Options[cm.Cursor]
			if !opt.Heading && !opt.Cancel {
				m.choiceModal = nil
				return m, cm.OnAltTyped(key, opt.Value)
			}
		}
		return m, nil
	}

	switch key {
	case "esc", "ctrl+c":
		onCancel := cm.OnCancel
		m.choiceModal = nil
		// ctrl+c always closes outright; esc pops one level up when a
		// parent-reopener is wired (drill-down menus), else closes.
		if key == "esc" && onCancel != nil {
			return m, onCancel()
		}
		return m, nil
	case "/":
		if cm.Searchable {
			cm.SearchActive = true
			return m, nil
		}
	case "j", "down":
		visible := choiceModalVisibleIndices(cm)
		if cm.visibleCursor < len(visible)-1 {
			cm.visibleCursor++
		}
		choiceModalSyncCursor(cm)
		choiceModalSkipHeading(cm, 1)
		return m, nil
	case "k", "up":
		if cm.visibleCursor > 0 {
			cm.visibleCursor--
		}
		choiceModalSyncCursor(cm)
		choiceModalSkipHeading(cm, -1)
		return m, nil
	case "g", "home":
		cm.visibleCursor = 0
		choiceModalSyncCursor(cm)
		choiceModalSkipHeading(cm, 1)
		return m, nil
	case "G", "end":
		visible := choiceModalVisibleIndices(cm)
		if len(visible) > 0 {
			cm.visibleCursor = len(visible) - 1
			choiceModalSyncCursor(cm)
			choiceModalSkipHeading(cm, -1)
		}
		return m, nil
	case "enter", "ctrl+s":
		choiceModalSyncCursor(cm)
		return m.submitChoiceModal()
	}
	return m, nil
}

// submitChoiceModal locks the modal and fires the Save closure —
// unless the selected option is marked Cancel, in which case we
// dismiss the modal without calling Save or firing OnSuccess. This
// keeps destructive-confirmation flows simple: "Cancel / Delete"
// with Cancel being a clean no-op.
func (m Model) submitChoiceModal() (Model, tea.Cmd) {
	cm := m.choiceModal
	if cm.Cursor < 0 || cm.Cursor >= len(cm.Options) {
		return m, nil
	}
	if cm.Options[cm.Cursor].Heading {
		// Headings are never selectable; the cursor skip should make
		// this unreachable, but guard anyway so Enter can't fire.
		return m, nil
	}
	if cm.Options[cm.Cursor].Cancel {
		m.choiceModal = nil
		return m, nil
	}
	cm.Saving = true
	cm.Err = ""
	save := cm.Save
	val := cm.Options[cm.Cursor].Value
	return m, func() tea.Msg {
		if save == nil {
			return choiceModalResultMsg{Value: val}
		}
		return choiceModalResultMsg{Value: val, Err: save(val)}
	}
}

// choiceModalResultMsg carries the outcome of a choice-modal save.
// Value is the picked choiceOption.Value, surfaced to the OnSuccess
// hook so multi-step modal flows can hand the selection forward
// without per-flow package-globals (the old pendingX struct pattern).
type choiceModalResultMsg struct {
	Value any
	Err   error
}

// choiceModalLoadedMsg carries the outcome of LoadCurrent. The Value
// is cross-checked against Options; if a match is found the cursor
// jumps to it. On mismatch (shouldn't happen) the cursor stays put.
type choiceModalLoadedMsg struct {
	Value any
	Err   error
}

// openChoiceModal is the canonical way to show a choice modal. Sets
// state, kicks the optional LoadCurrent, returns the tea.Cmd to fire.
func (m *Model) openChoiceModal(state choiceModalState) tea.Cmd {
	if state.LoadCurrent != nil {
		state.Loading = true
	}
	// Seed the visible-cursor from the caller's Cursor so initial
	// rendering highlights the same row regardless of whether the
	// modal is searchable.
	state.visibleCursor = state.Cursor
	s := state
	m.choiceModal = &s
	if s.LoadCurrent == nil {
		return nil
	}
	loader := s.LoadCurrent
	return func() tea.Msg {
		v, err := loader()
		return choiceModalLoadedMsg{Value: v, Err: err}
	}
}
