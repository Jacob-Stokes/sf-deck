package ui

// Picker — a small anchored dropdown overlay for "type-to-filter, pick
// one item" workflows. Generic over the item type so the same widget
// serves field pickers (in the chip wizard), sObject pickers, picklist
// value pickers, user pickers, etc.
//
// Shape:
//   - Anchored at a caller-supplied (x, y) screen position so the
//     dropdown opens under the trigger row rather than centering
//     globally. Caller computes the anchor from whatever cursor /
//     row state it owns.
//   - Search input at the top, populated as the user types.
//   - Filtered list below with viewport scrolling — defaults to
//     12 rows visible.
//   - Enter picks the highlighted item; Esc cancels.
//   - j/k or arrow keys move the highlight; tab/shift+tab too.
//
// Generics: pickerSpec[T] carries the items + match + render
// closures. Different call sites have different item types and
// different "what to display per row" rules; the closures keep this
// flexible without forcing every caller to adapt to an Item interface.
//
// The runtime state lives in pickerState which is *not* generic — it
// holds the closures already bound to the concrete type. This keeps
// Model from needing a generic field (Go doesn't allow generic
// methods on concrete types either). The trade-off: the spec gets
// "compiled" through pickerStateFromSpec at open time, type-erasing
// to func(string) bool / func(int) string for the runtime.

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// pickerSpec[T] declares one invocation of the picker.
type pickerSpec[T any] struct {
	// Title is the bold headline above the search input.
	Title string

	// Items are the picker's full source list. Filtered live by Match
	// against the user's typed query.
	Items []T

	// Match reports whether an item passes the filter. Called per-item
	// per-keystroke, so cheap is fine. Empty query is the "match all"
	// case which the implementation handles before calling Match.
	Match func(item T, query string) bool

	// RenderRow draws a single item. Receives a focused flag so the
	// caller can apply highlight styling for the cursored row.
	RenderRow func(item T, focused bool) string

	// OnPick fires when the user hits enter on a row. Returns a
	// tea.Cmd the runtime fires after the picker dismisses.
	OnPick func(item T) tea.Cmd

	// AnchorX, AnchorY are the screen cell where the dropdown's top-
	// left corner anchors. Caller supplies these from whatever cursor
	// state they own. The picker clamps to fit on screen.
	AnchorX, AnchorY int

	// Width is the dropdown width in cells. Default 40 if zero.
	Width int

	// MaxRows caps the visible portion of the list. Default 12.
	MaxRows int

	// Placeholder is shown in the search input when empty.
	Placeholder string

	// EmptyHint shows when the filtered list has zero rows. Default
	// "no matches".
	EmptyHint string
}

// pickerState is the runtime state, type-erased so Model can hold a
// single non-generic field. See the package docstring for the
// rationale.
type pickerState struct {
	title     string
	width     int
	maxRows   int
	anchorX   int
	anchorY   int
	emptyHint string

	// Compiled / closed-over slices and closures, type-erased to
	// indices into the underlying source list. The picker only ever
	// indexes into items; the dispatch closures handle the original
	// typed values internally.
	itemCount int
	matches   func(idx int, query string) bool
	render    func(idx int, focused bool) string
	pick      func(idx int) tea.Cmd

	// Live state.
	search  textinput.Model
	visible []int // indices passing the current filter, in source order
	cursor  int   // position into visible (NOT into items)
	viewTop int   // first visible-row index drawn (for viewport clamp)
}

// openPicker[T] is the canonical entry point — the type parameter
// keeps the spec strongly-typed at the call site even though the
// runtime is type-erased.
func openPicker[T any](m *Model, spec pickerSpec[T]) tea.Cmd {
	state := pickerStateFromSpec(spec)
	m.picker = state
	return nil
}

// pickerStateFromSpec compiles the typed spec into the type-erased
// runtime state.
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

// recomputeVisible rebuilds the indices passing the current query.
// Resets cursor + scroll when the previously-cursored item filters
// out so the picker doesn't show a phantom selection.
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

// stylePickerInput themes the search input. Same convention as the
// theme picker / chip wizard so the look stays consistent.
func stylePickerInput(ti *textinput.Model) {
	s := ti.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Cursor.Color = theme.BorderHi
	ti.SetStyles(s)
}

// renderPicker draws the picker overlay, or "" when not open.
// Returned positioned by the main frame compositor using anchorX/anchorY.
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

// handlePickerKey is the reducer while the picker is open.
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

	// Forward to the textinput widget — every printable key + backspace
	// + ctrl+u etc. flows through it.
	before := s.search.Value()
	newInput, cmd := s.search.Update(msg)
	s.search = newInput
	if s.search.Value() != before {
		s.recomputeVisible()
	}
	return m, cmd
}
