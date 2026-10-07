package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type pickerSpec[T any] struct {
	Title string

	Items []T

	Match func(item T, query string) bool

	RenderRow func(item T, focused bool) string

	OnPick func(item T) tea.Cmd

	// AnchorX, AnchorY are the screen cell where the dropdown's top-
	// left corner anchors. Caller supplies these from whatever cursor
	// state they own. The picker clamps to fit on screen.
	AnchorX, AnchorY int

	Width int

	MaxRows int

	Placeholder string

	EmptyHint string
}

type pickerState struct {
	title     string
	width     int
	maxRows   int
	anchorX   int
	anchorY   int
	emptyHint string

	itemCount int
	matches   func(idx int, query string) bool
	render    func(idx int, focused bool) string
	pick      func(idx int) tea.Cmd

	search  textinput.Model
	visible []int // indices passing the current filter, in source order
	cursor  int   // position into visible (NOT into items)
	viewTop int   // first visible-row index drawn (for viewport clamp)
}

func openPicker[T any](m *Model, spec pickerSpec[T]) tea.Cmd {
	state := pickerStateFromSpec(spec)
	m.picker = state
	return nil
}

func pickerStateFromSpec[T any](spec pickerSpec[T]) *pickerState {
	if spec.Width == 0 {
		spec.Width = 40
	}
	if spec.MaxRows == 0 {
		spec.MaxRows = 12
	}
	if spec.EmptyHint == "" {
		spec.EmptyHint = "no matches"
	}

	items := append([]T(nil), spec.Items...)

	st := &pickerState{
		title:     spec.Title,
		width:     spec.Width,
		maxRows:   spec.MaxRows,
		anchorX:   spec.AnchorX,
		anchorY:   spec.AnchorY,
		emptyHint: spec.EmptyHint,
		itemCount: len(items),
		matches: func(idx int, q string) bool {
			if q == "" {
				return true
			}
			return spec.Match(items[idx], q)
		},
		render: func(idx int, focused bool) string {
			return spec.RenderRow(items[idx], focused)
		},
		pick: func(idx int) tea.Cmd {
			if spec.OnPick == nil {
				return nil
			}
			return spec.OnPick(items[idx])
		},
	}

	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	if spec.Placeholder != "" {
		ti.Placeholder = spec.Placeholder
	}
	stylePickerInput(&ti)
	ti.Focus()
	st.search = ti

	st.recomputeVisible()
	return st
}

func (s *pickerState) recomputeVisible() {
	q := strings.TrimSpace(s.search.Value())
	prev := -1
	if s.cursor < len(s.visible) {
		prev = s.visible[s.cursor]
	}
	out := make([]int, 0, s.itemCount)
	for i := 0; i < s.itemCount; i++ {
		if s.matches(i, q) {
			out = append(out, i)
		}
	}
	s.visible = out

	// Try to keep the cursor on the same item if it survived the
	// filter; otherwise reset to top.
	if prev >= 0 {
		for i, idx := range s.visible {
			if idx == prev {
				s.cursor = i
				s.clampViewport()
				return
			}
		}
	}
	s.cursor = 0
	s.viewTop = 0
}

// clampViewport ensures viewTop keeps the cursor inside [viewTop,
// viewTop+maxRows). Called after every cursor move.
func (s *pickerState) clampViewport() {
	if s.cursor < s.viewTop {
		s.viewTop = s.cursor
		return
	}
	if s.cursor >= s.viewTop+s.maxRows {
		s.viewTop = s.cursor - s.maxRows + 1
	}
	if s.viewTop < 0 {
		s.viewTop = 0
	}
}

func stylePickerInput(ti *textinput.Model) {
	s := ti.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Cursor.Color = theme.BorderHi
	ti.SetStyles(s)
}

func (m Model) renderPicker() string {
	if m.picker == nil {
		return ""
	}
	s := m.picker
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)

	var lines []string
	if s.title != "" {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(s.title))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.FgDim).Render("/")+s.search.View())
	lines = append(lines, strings.Repeat("─", s.width-2))

	if len(s.visible) == 0 {
		lines = append(lines, subStyle.Italic(true).Render("  "+s.emptyHint))
	} else {
		end := s.viewTop + s.maxRows
		if end > len(s.visible) {
			end = len(s.visible)
		}
		if s.viewTop > 0 {
			lines = append(lines, subStyle.Render(fmt.Sprintf("    ↑ %d more", s.viewTop)))
		}
		for i := s.viewTop; i < end; i++ {
			focused := i == s.cursor
			lines = append(lines, s.render(s.visible[i], focused))
		}
		if end < len(s.visible) {
			lines = append(lines, subStyle.Render(fmt.Sprintf("    ↓ %d more", len(s.visible)-end)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, subStyle.Render("type to filter · enter pick · esc cancel"))

	body := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderHi).
		Background(theme.Bg).
		Padding(0, 1).
		Width(s.width).
		Render(body)
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.picker == nil {
		return m, nil
	}
	s := m.picker
	key := msg.String()

	switch key {
	case "esc", "ctrl+c":
		m.picker = nil
		return m, nil
	case "enter":
		if len(s.visible) == 0 {
			return m, nil
		}
		idx := s.visible[s.cursor]
		cmd := s.pick(idx)
		m.picker = nil
		return m, cmd
	case "down", "ctrl+n", "tab":
		if len(s.visible) > 0 && s.cursor < len(s.visible)-1 {
			s.cursor++
			s.clampViewport()
		}
		return m, nil
	case "up", "ctrl+p", "shift+tab":
		if s.cursor > 0 {
			s.cursor--
			s.clampViewport()
		}
		return m, nil
	}

	before := s.search.Value()
	newInput, cmd := s.search.Update(msg)
	s.search = newInput
	if s.search.Value() != before {
		s.recomputeVisible()
	}
	return m, cmd
}
