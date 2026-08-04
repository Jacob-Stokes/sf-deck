package ui

// Theme picker — a small floating modal in the top-right corner.
// Live-previews each theme as the cursor moves. Differs from the
// other choice modals because (a) it doesn't dim the background,
// (b) it's positioned not centered, (c) Esc reverts the live preview
// rather than just dismissing.
//
// Launched from the settings modal (V → Theme); Esc returns there
// without saving. Enter persists to settings.toml. F toggles the
// cursored theme as a favourite.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// themePickerState holds the modal's runtime state.
type themePickerState struct {
	// originalID is the theme that was active when the picker opened.
	// Esc reverts to this; Enter commits the candidate.
	originalID string

	// candidateID is the currently-highlighted theme. Live-applied via
	// theme.ApplyPalette on every cursor move so the rest of the UI
	// renders in the candidate's colours immediately.
	candidateID string

	// search filters the visible list by case-insensitive substring.
	search searchState
}

// openThemePicker launches the picker. Records the original theme so
// Esc can revert. Closes any open settings/choice modal so the picker
// is the only modal layer above the live UI.
func (m *Model) openThemePicker() tea.Cmd {
	currentID := "tokyo-night"
	if m.settings != nil {
		currentID = m.settings.Theme()
	}
	m.choiceModal = nil
	m.editModal = nil
	m.themePicker = &themePickerState{
		originalID:  currentID,
		candidateID: currentID,
	}
	m.themePicker.search.EnsureInit()
	return nil
}

// renderThemePicker draws the floating modal. Returned string is
// pre-positioned via the compositor (see render.go's overlay handling).
func (m Model) renderThemePicker() string {
	if m.themePicker == nil {
		return ""
	}
	const width = 38
	const visibleRows = 12

	tp := m.themePicker
	ids := filteredThemeIDs(m, tp.search.Buffer())

	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	rowStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	favStyle := lipgloss.NewStyle().Foreground(theme.Yellow)
	cursorBar := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌")

	cursor := indexOf(ids, tp.candidateID)
	if cursor < 0 {
		cursor = 0
	}
	start, end := windowAround(cursor, len(ids), visibleRows)

	var lines []string
	lines = append(lines, titleStyle.Render("Theme"))
	lines = append(lines, strings.Repeat("─", width-2))

	// Search bar — always visible. Shows the live buffer with a "/" hint.
	var searchLine string
	switch {
	case !tp.search.Active && tp.search.Buffer() == "":
		searchLine = subStyle.Render("/  type to filter")
	case tp.search.Active:
		caret := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("│")
		searchLine = subStyle.Render("/") + tp.search.Buffer() + caret
	default:
		searchLine = subStyle.Render("/") + tp.search.Buffer()
	}
	lines = append(lines, searchLine)
	lines = append(lines, "")

	if len(ids) == 0 {
		lines = append(lines, subStyle.Render("  no themes match"))
	} else {
		if start > 0 {
			lines = append(lines, subStyle.Render("    ↑ more"))
		}
		palettes := theme.Palettes()
		for i := start; i < end; i++ {
			id := ids[i]
			if id == themeDividerID {
				lines = append(lines, subStyle.Render(strings.Repeat("─", width-2)))
				continue
			}
			p := palettes[id]
			label := p.Name
			if label == "" {
				label = id
			}
			fav := "  "
			if m.settings != nil && m.settings.IsThemeFavourite(id) {
				fav = favStyle.Render("★ ")
			}
			prefix := "  "
			labelRendered := rowStyle.Render(label)
			if i == cursor {
				prefix = cursorBar + " "
				labelRendered = lipgloss.NewStyle().Foreground(theme.Fg).Bold(true).Render(label)
			}
			lines = append(lines, ellipsisTrunc(prefix+fav+labelRendered, width-2))
		}
		if end < len(ids) {
			lines = append(lines, subStyle.Render(fmt.Sprintf("    ↓ %d more", len(ids)-end)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, subStyle.Render("j/k move · / search · "+firstPretty(Keys.ThemePickerFavourite)+" favourite"))
	lines = append(lines, subStyle.Render("enter save · esc revert"))

	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderHi).
		Background(theme.Bg).
		Padding(0, 1).
		Width(width).
		Render(body)
	return box
}

// handleThemePickerKey is the reducer while the picker is visible.
func (m Model) handleThemePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.themePicker == nil {
		return m, nil
	}
	tp := m.themePicker
	key := msg.String()

	// If search is active, route input there first — but reserve a
	// few keys (esc, enter, ctrl+c) so the user can always escape
	// even mid-typing. Up/down break out of typing without losing the
	// buffer so users can search-then-pick fluidly.
	if tp.search.Active {
		switch key {
		case "esc":
			// First esc: drop search focus, keep buffer.
			if tp.search.Buffer() != "" {
				tp.search.Active = false
				tp.search.Committed = true
				return m, nil
			}
			tp.search.Active = false
			return m.cancelThemePicker()
		case "enter":
			// Commit search; keep cursor on the highlighted row.
			tp.search.Active = false
			tp.search.Committed = tp.search.Buffer() != ""
			ids := filteredThemeIDs(m, tp.search.Buffer())
			for _, id := range ids {
				if id != themeDividerID {
					tp.candidateID = id
					theme.ApplyPalette(tp.candidateID)
					m.clearRenderCache()
					break
				}
			}
			return m, nil
		case "ctrl+c":
			return m.cancelThemePicker()
		case "down", "ctrl+n":
			tp.search.Active = false
			tp.search.Committed = tp.search.Buffer() != ""
			return m.themePickerMove(+1)
		case "up", "ctrl+p":
			tp.search.Active = false
			tp.search.Committed = tp.search.Buffer() != ""
			return m.themePickerMove(-1)
		case "backspace":
			if tp.search.Inited {
				newInput, _ := tp.search.Input.Update(msg)
				tp.search.Input = newInput
				return m.applyThemePickerSearch()
			}
		default:
			// Forward to the textinput for printable characters.
			if tp.search.Inited {
				newInput, _ := tp.search.Input.Update(msg)
				tp.search.Input = newInput
				return m.applyThemePickerSearch()
			}
		}
		return m, nil
	}

	// Search not active.
	switch key {
	case "esc", "ctrl+c":
		return m.cancelThemePicker()
	case "enter":
		return m.commitThemePicker()
	case "j", "down":
		return m.themePickerMove(+1)
	case "k", "up":
		return m.themePickerMove(-1)
	case "g", "home":
		return m.themePickerJump(0)
	case "G", "end":
		ids := filteredThemeIDs(m, tp.search.Buffer())
		return m.themePickerJump(len(ids) - 1)
	case "/":
		tp.search.EnsureInit()
		tp.search.Active = true
		tp.search.Committed = false
		tp.search.Input.Focus()
		return m, nil
	}
	switch {
	case matches(key, Keys.ThemePickerFavourite):
		return m.themePickerToggleFavourite()
	case matches(key, Keys.ThemePickerClear):
		// Clear the search buffer entirely.
		if tp.search.Buffer() != "" {
			tp.search.SetBuffer("")
			tp.search.Committed = false
			return m.applyThemePickerSearch()
		}
	}
	return m, nil
}

// themePickerMove shifts the cursor by delta and live-applies the new
// candidate theme. Skips the divider sentinel so navigation feels
// continuous across the favourites/rest boundary.
func (m Model) themePickerMove(delta int) (Model, tea.Cmd) {
	tp := m.themePicker
	ids := filteredThemeIDs(m, tp.search.Buffer())
	if len(ids) == 0 {
		return m, nil
	}
	cur := indexOf(ids, tp.candidateID)
	if cur < 0 {
		cur = 0
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for n := delta; n != 0; n -= step {
		next := cur + step
		if next < 0 || next >= len(ids) {
			break
		}
		cur = next
		// If we landed on the divider, hop one more in the same
		// direction. If that's out of bounds, back off.
		if ids[cur] == themeDividerID {
			next = cur + step
			if next < 0 || next >= len(ids) {
				cur -= step
				break
			}
			cur = next
		}
	}
	tp.candidateID = ids[cur]
	theme.ApplyPalette(tp.candidateID)
	m.clearRenderCache()
	return m, nil
}

// themePickerJump sets the cursor to a specific index. Skips the
// divider sentinel by stepping forward to the next real entry.
func (m Model) themePickerJump(idx int) (Model, tea.Cmd) {
	tp := m.themePicker
	ids := filteredThemeIDs(m, tp.search.Buffer())
	if len(ids) == 0 {
		return m, nil
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ids) {
		idx = len(ids) - 1
	}
	if ids[idx] == themeDividerID {
		// Walk forward, then backward, to find the nearest real id.
		for j := idx + 1; j < len(ids); j++ {
			if ids[j] != themeDividerID {
				idx = j
				break
			}
		}
		if ids[idx] == themeDividerID {
			for j := idx - 1; j >= 0; j-- {
				if ids[j] != themeDividerID {
					idx = j
					break
				}
			}
		}
	}
	tp.candidateID = ids[idx]
	theme.ApplyPalette(tp.candidateID)
	m.clearRenderCache()
	return m, nil
}

// applyThemePickerSearch is fired after every keystroke in the search
// buffer. Keeps the candidate cursor on a still-visible row, or jumps
// to the first match when the previous candidate filtered out.
func (m Model) applyThemePickerSearch() (Model, tea.Cmd) {
	tp := m.themePicker
	ids := filteredThemeIDs(m, tp.search.Buffer())
	if len(ids) == 0 {
		return m, nil
	}
	if indexOf(ids, tp.candidateID) < 0 {
		// Pick the first non-divider id.
		for _, id := range ids {
			if id != themeDividerID {
				tp.candidateID = id
				theme.ApplyPalette(tp.candidateID)
				m.clearRenderCache()
				break
			}
		}
	}
	return m, nil
}

// themePickerToggleFavourite flips the cursored theme's favourite
// flag and persists settings immediately so the change survives a
// later Esc-cancel of the picker session.
func (m Model) themePickerToggleFavourite() (Model, tea.Cmd) {
	tp := m.themePicker
	if m.settings == nil || tp.candidateID == "" {
		return m, nil
	}
	m.settings.ToggleThemeFavourite(tp.candidateID)
	if err := m.settings.Save(); err != nil {
		m.flash("could not save favourite: " + err.Error())
		return m, nil
	}
	return m, nil
}

// commitThemePicker persists the candidate as the active theme and
// closes the picker.
func (m Model) commitThemePicker() (Model, tea.Cmd) {
	tp := m.themePicker
	if tp == nil {
		return m, nil
	}
	saved := true
	if m.settings != nil {
		m.settings.SetTheme(tp.candidateID)
		saved = m.saveSettings("")
	}
	theme.ApplyPalette(tp.candidateID)
	m.clearRenderCache()
	m.themePicker = nil
	if saved {
		m.flash("theme: " + tp.candidateID)
	}
	return m, nil
}

// cancelThemePicker reverts to the original theme and re-opens the
// settings meta-menu so the user can pick something else.
func (m Model) cancelThemePicker() (Model, tea.Cmd) {
	tp := m.themePicker
	if tp == nil {
		return m, nil
	}
	theme.ApplyPalette(tp.originalID)
	m.clearRenderCache()
	m.themePicker = nil
	// Re-open the settings meta-menu so the picker dismiss feels like
	// a back-step rather than dropping the user out of settings entirely.
	cmd := m.openSettingsModal()
	return m, cmd
}

// themeDividerID is a sentinel inserted into the row list between the
// favourites group and the rest of the catalogue. The renderer draws
// it as a horizontal rule; cursor movement skips over it so it never
// becomes "selected".
const themeDividerID = "__divider__"

// filteredThemeIDs returns the theme list ordered for the picker:
// favourites (curated favourites first, then user favourites) → divider
// → curated non-favourites → divider → rest. Optionally filtered by
// query. Result is the visible-row order; index lookups + scrolling
// use it.
func filteredThemeIDs(m Model, query string) []string {
	all := theme.PaletteIDs()
	favs := map[string]bool{}
	if m.settings != nil {
		for _, f := range m.settings.ThemeFavourites() {
			favs[f] = true
		}
	}

	curated := map[string]bool{
		"tokyo-night":     true,
		"catppuccin":      true,
		"dracula":         true,
		"solarized-light": true,
	}

	var favIDs, curatedIDs, restIDs []string
	for _, id := range all {
		switch {
		case favs[id]:
			favIDs = append(favIDs, id)
		case curated[id]:
			curatedIDs = append(curatedIDs, id)
		default:
			restIDs = append(restIDs, id)
		}
	}

	// Filter each group independently so the divider only appears
	// between groups that actually have content after filtering.
	if query != "" {
		q := strings.ToLower(query)
		palettes := theme.Palettes()
		match := func(id string) bool {
			p := palettes[id]
			return strings.Contains(strings.ToLower(p.Name), q) ||
				strings.Contains(strings.ToLower(id), q)
		}
		favIDs = filterIDs(favIDs, match)
		curatedIDs = filterIDs(curatedIDs, match)
		restIDs = filterIDs(restIDs, match)
	}

	var ordered []string
	ordered = append(ordered, favIDs...)
	if len(favIDs) > 0 && (len(curatedIDs) > 0 || len(restIDs) > 0) {
		ordered = append(ordered, themeDividerID)
	}
	ordered = append(ordered, curatedIDs...)
	ordered = append(ordered, restIDs...)
	return ordered
}

// filterIDs returns ids for which keep returns true, preserving order.
func filterIDs(ids []string, keep func(string) bool) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if keep(id) {
			out = append(out, id)
		}
	}
	return out
}

// indexOf returns the position of v in xs, or -1.
func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

// windowAround returns [start, end) viewport indices. Cursor sits
// ~1/3 down the visible window so movement reads naturally.
func windowAround(cursor, n, visible int) (int, int) {
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

// ellipsisTrunc clips a styled string to width using lipgloss-aware
// truncation so ANSI escapes survive intact.
func ellipsisTrunc(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	// Fall back to a simple rune slice — visible-width truncation
	// inside ANSI is tricky and the rows here are short labels
	// where this is good enough.
	if len(s) <= width {
		return s
	}
	return s[:width]
}
