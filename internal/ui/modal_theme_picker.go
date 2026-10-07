package ui

// Theme picker — a small floating modal in the top-right corner.
// Live-previews each theme as the cursor moves. Differs from the
// other choice modals because (a) it doesn't dim the background,
// (b) it's positioned not centered, (c) Esc reverts the live preview
// rather than just dismissing.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type themePickerState struct {
	originalID string

	candidateID string

	search searchState
}

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

func (m Model) handleThemePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.themePicker == nil {
		return m, nil
	}
	tp := m.themePicker
	key := msg.String()

	if tp.search.Active {
		switch key {
		case "esc":
			if tp.search.Buffer() != "" {
				tp.search.Active = false
				tp.search.Committed = true
				return m, nil
			}
			tp.search.Active = false
			return m.cancelThemePicker()
		case "enter":
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
			if tp.search.Inited {
				newInput, _ := tp.search.Input.Update(msg)
				tp.search.Input = newInput
				return m.applyThemePickerSearch()
			}
		}
		return m, nil
	}

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

func (m Model) applyThemePickerSearch() (Model, tea.Cmd) {
	tp := m.themePicker
	ids := filteredThemeIDs(m, tp.search.Buffer())
	if len(ids) == 0 {
		return m, nil
	}
	if indexOf(ids, tp.candidateID) < 0 {
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

func (m Model) cancelThemePicker() (Model, tea.Cmd) {
	tp := m.themePicker
	if tp == nil {
		return m, nil
	}
	theme.ApplyPalette(tp.originalID)
	m.clearRenderCache()
	m.themePicker = nil
	cmd := m.openSettingsModal()
	return m, cmd
}

// themeDividerID is a sentinel inserted into the row list between the
// favourites group and the rest of the catalogue. The renderer draws
// it as a horizontal rule; cursor movement skips over it so it never
// becomes "selected".
const themeDividerID = "__divider__"

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

func filterIDs(ids []string, keep func(string) bool) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if keep(id) {
			out = append(out, id)
		}
	}
	return out
}

func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

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

func ellipsisTrunc(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if len(s) <= width {
		return s
	}
	return s[:width]
}
