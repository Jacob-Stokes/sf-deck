package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

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

type choiceOption struct {
	Label string
	Hint  string
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

type choiceModalState struct {
	Title string
	Hint  string

	Options []choiceOption
	Cursor  int

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

	OnCancel func() tea.Cmd

	Searchable bool

	AltKeys    string
	OnAltTyped func(key string, val any) tea.Cmd

	// Wide opts the modal into a larger size (~80% of terminal,
	// clamped to 80..140) for browse-shaped modals like the chip
	// manager that need room for tabular row content. Default is
	// the compact 48..70 confirm size.
	Wide bool

	Loading       bool
	Saving        bool
	Err           string
	SearchActive  bool   // true while the user is typing a filter
	SearchQuery   string // committed filter buffer
	visibleCursor int    // cursor position in the filtered slice
}

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

type choiceModalResultMsg struct {
	Value any
	Err   error
}

type choiceModalLoadedMsg struct {
	Value any
	Err   error
}

func (m *Model) openChoiceModal(state choiceModalState) tea.Cmd {
	if state.LoadCurrent != nil {
		state.Loading = true
	}
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
