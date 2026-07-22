package ui

// compareScopeModal — a multi-select picker for the comparison scope.
//
// Replaces the old "X only / All types" preset radio list, which
// couldn't express "Apex AND fields AND flows". This is a checkbox
// list: space toggles the cursored type, `a` toggles all/none, enter
// confirms. The result feeds back via onConfirm, so both the New setup
// form and the edit modal reuse it without knowing where the scope is
// stored.
//
// The candidate type list is discovered per-org via describeMetadata,
// which is slow (~1.5s shelling `sf`). So the modal opens IMMEDIATELY in
// a "loading…" state and the type list arrives asynchronously via
// compareTypesLoadedMsg (see openCompareScopeModal + Update). The list
// can be long (~250 types in a mature org), so it renders in a
// fixed-height scroll window and `/` filters it by substring.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// scopeAllRowIdx is the cursor index of the "All types" toggle row,
// which sits above the individual types. Type i (in the FILTERED view)
// is at cursor index i+1.
const scopeAllRowIdx = 0

// scopeVisibleRows is the fixed number of type rows shown at once (the
// scroll window). Keeps the modal a stable height regardless of how many
// types the org has, so it never runs off-screen.
const scopeVisibleRows = 12

// compareScopeModalState is the live multi-select modal.
type compareScopeModalState struct {
	Loading bool            // true until the type list arrives (shows a spinner-ish notice)
	Alias   string          // org whose catalog we requested (guards stale loaded msgs)
	Types   []string        // all candidate metadata types (master list, in order)
	Checked map[string]bool // currently-ticked types

	// Search is the active `/` filter. searchActive means keystrokes go
	// to the query rather than navigation. When non-empty, Filtered()
	// narrows the visible types; the All-row toggles only the filtered set.
	Search       string
	searchActive bool

	Cursor int // cursor over the FILTERED row list (0 = All row)
	Offset int // first visible type index (into the filtered list)

	OnConfirm func(selected []string) // called with the chosen types on enter
}

// compareTypesLoadedMsg delivers the discovered type catalog to the open
// scope modal. Carries the alias it was fetched for so a stale result
// (org switched, modal reopened) is ignored.
type compareTypesLoadedMsg struct {
	Alias string
	Types []string
	Err   error
}

// openCompareScopeModal opens the scope multi-select in its loading
// state and returns a cmd that fetches the org's comparable types
// (cache-or-describe, off the UI loop). onConfirm receives the final
// selection (always ≥1 type — confirming with none keeps the previous
// scope). Callers thread the returned cmd back through Update.
func (m *Model) openCompareScopeModal(current []string, onConfirm func([]string)) tea.Cmd {
	checked := map[string]bool{}
	for _, t := range current {
		checked[t] = true
	}
	alias := ""
	if len(m.orgs) > 0 && m.selected >= 0 && m.selected < len(m.orgs) {
		alias = m.orgs[m.selected].Username
	}
	// No default-all: a fresh comparison opens with nothing ticked, so
	// the user explicitly chooses which types to compare.
	m.compareScope = &compareScopeModalState{
		Loading:   true,
		Alias:     alias,
		Checked:   checked,
		OnConfirm: onConfirm,
	}
	// Fetch off the UI loop; the result lands as compareTypesLoadedMsg.
	mc := *m
	return func() tea.Msg {
		types, err := mc.loadComparableTypes(alias)
		return compareTypesLoadedMsg{Alias: alias, Types: types, Err: err}
	}
}

// applyCompareTypesLoaded fills an open scope modal with its discovered
// types. Ignores stale results (modal closed, or fetched for a different
// org than the one now open).
func (m *Model) applyCompareTypesLoaded(msg compareTypesLoadedMsg) {
	st := m.compareScope
	if st == nil || st.Alias != msg.Alias {
		return
	}
	st.Types = msg.Types
	st.Loading = false
	// Drop pre-ticks for types this org doesn't actually offer (e.g. when
	// reopening a saved comparison against a different org), so the
	// "N of M selected" count stays truthful.
	valid := map[string]bool{}
	for _, t := range st.Types {
		valid[t] = true
	}
	for t := range st.Checked {
		if !valid[t] {
			delete(st.Checked, t)
		}
	}
}

// filtered returns the types matching the active `/` search (all types
// when the query is empty), preserving master order.
func (st *compareScopeModalState) filtered() []string {
	if st.Search == "" {
		return st.Types
	}
	q := strings.ToLower(st.Search)
	var out []string
	for _, t := range st.Types {
		if strings.Contains(strings.ToLower(t), q) {
			out = append(out, t)
		}
	}
	return out
}

func (m Model) renderCompareScopeModal() string {
	st := m.compareScope
	if st == nil {
		return ""
	}
	w := modalWidth(m.width, 50, 80)
	inner := w - 4

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render("Comparison scope"))

	if st.Loading {
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render("  "+compareSpinner(m.compareFrame)+" loading metadata types…"))
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render("  esc cancel"))
		return modalBox(strings.Join(lines, "\n"), w)
	}

	filtered := st.filtered()
	n := st.countChecked()
	lines = append(lines, theme.Subtle.Render(itoa(n)+" of "+itoa(len(st.Types))+" types selected"))

	// Search line: an editable query when `/` is active, else a hint.
	if st.searchActive || st.Search != "" {
		q := st.Search
		if st.searchActive {
			q += "▌"
		}
		label := "/" + q
		if len(filtered) != len(st.Types) {
			label += theme.Subtle.Render("   " + itoa(len(filtered)) + " match")
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.BorderHi).Render(label))
	}
	lines = append(lines, strings.Repeat("─", inner))

	checkbox := func(on bool) string {
		if on {
			return lipgloss.NewStyle().Foreground(theme.Green).Render("[x]")
		}
		return "[ ]"
	}
	rowLine := func(active bool, box, label string, bold bool) string {
		prefix := "   "
		nameStyle := theme.Subtle
		if active {
			prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" ▌ ")
			nameStyle = lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
		} else if bold {
			nameStyle = lipgloss.NewStyle().Foreground(theme.Fg)
		}
		return prefix + box + " " + nameStyle.Render(label)
	}

	// Row 0 = the "All types" toggle (operates on the FILTERED set when a
	// search is active); rows 1..N = the visible (windowed) filtered types.
	allOn := len(filtered) > 0 && st.allFilteredChecked(filtered)
	allLabel := "All types"
	if st.Search != "" {
		allLabel = "All matching"
	}
	lines = append(lines, rowLine(st.Cursor == scopeAllRowIdx, checkbox(allOn), allLabel, true))

	if len(filtered) == 0 {
		lines = append(lines, theme.Subtle.Render("   (no types match)"))
	} else {
		start := st.Offset
		end := start + scopeVisibleRows
		if end > len(filtered) {
			end = len(filtered)
		}
		if start > 0 {
			lines = append(lines, theme.Subtle.Render("   ↑ more"))
		} else {
			lines = append(lines, "")
		}
		for i := start; i < end; i++ {
			t := filtered[i]
			lines = append(lines, rowLine(st.Cursor == i+1, checkbox(st.Checked[t]), t, false))
		}
		if end < len(filtered) {
			lines = append(lines, theme.Subtle.Render("   ↓ more"))
		} else {
			lines = append(lines, "")
		}
	}

	lines = append(lines, "")
	if st.searchActive {
		lines = append(lines, theme.Subtle.Render("  type to filter · enter apply · esc clear search"))
	} else {
		lines = append(lines, theme.Subtle.Render("  space toggle · / search · a all · enter confirm · esc cancel"))
	}
	return modalBox(strings.Join(lines, "\n"), w)
}

// allFilteredChecked reports whether every type in the filtered set is
// currently ticked.
func (st *compareScopeModalState) allFilteredChecked(filtered []string) bool {
	for _, t := range filtered {
		if !st.Checked[t] {
			return false
		}
	}
	return true
}

// toggleAtCursor toggles whatever the cursor is on: the All row (cursor
// 0 → toggle the whole FILTERED set) or a single filtered type.
func (st *compareScopeModalState) toggleAtCursor() {
	filtered := st.filtered()
	if st.Cursor == scopeAllRowIdx {
		on := st.allFilteredChecked(filtered)
		for _, t := range filtered {
			st.Checked[t] = !on
		}
		return
	}
	idx := st.Cursor - 1 // index into filtered
	if idx >= 0 && idx < len(filtered) {
		t := filtered[idx]
		st.Checked[t] = !st.Checked[t]
	}
}

func (st *compareScopeModalState) countChecked() int {
	n := 0
	for _, t := range st.Types {
		if st.Checked[t] {
			n++
		}
	}
	return n
}

// clampCursorScroll keeps Cursor within the filtered list and slides the
// scroll window so the cursored type stays visible. Cursor 0 (All row)
// pins the window to the top.
func (st *compareScopeModalState) clampCursorScroll() {
	filtered := st.filtered()
	maxCursor := len(filtered) // last type is at index len (All row is 0)
	if st.Cursor < scopeAllRowIdx {
		st.Cursor = scopeAllRowIdx
	}
	if st.Cursor > maxCursor {
		st.Cursor = maxCursor
	}
	if st.Cursor == scopeAllRowIdx {
		st.Offset = 0
		return
	}
	typeIdx := st.Cursor - 1
	if typeIdx < st.Offset {
		st.Offset = typeIdx
	}
	if typeIdx >= st.Offset+scopeVisibleRows {
		st.Offset = typeIdx - scopeVisibleRows + 1
	}
	if st.Offset < 0 {
		st.Offset = 0
	}
}

func (m Model) handleCompareScopeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	st := m.compareScope
	if st == nil {
		return m, nil
	}

	// While loading, only esc does anything (cancel).
	if st.Loading {
		if s := msg.String(); s == "esc" || s == "ctrl+c" {
			m.compareScope = nil
		}
		return m, nil
	}

	// Search-entry mode: keystrokes edit the query.
	if st.searchActive {
		switch msg.String() {
		case "esc":
			// Clear the search and leave entry mode.
			st.searchActive = false
			st.Search = ""
			st.Cursor = scopeAllRowIdx
			st.Offset = 0
			return m, nil
		case "enter":
			// Apply the filter: leave entry mode, keep the query.
			st.searchActive = false
			st.Cursor = scopeAllRowIdx
			st.Offset = 0
			return m, nil
		case "backspace":
			if st.Search != "" {
				st.Search = st.Search[:len(st.Search)-1]
			}
			st.Cursor = scopeAllRowIdx
			st.Offset = 0
			return m, nil
		default:
			if s := msg.String(); len(s) == 1 {
				st.Search += s
				st.Cursor = scopeAllRowIdx
				st.Offset = 0
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "esc", "ctrl+c":
		if st.Search != "" {
			// First esc clears an applied filter; next esc closes.
			st.Search = ""
			st.Cursor = scopeAllRowIdx
			st.Offset = 0
			return m, nil
		}
		m.compareScope = nil
		return m, nil
	case "/":
		st.searchActive = true
		return m, nil
	case "up", "k":
		st.Cursor--
		st.clampCursorScroll()
		return m, nil
	case "down", "j":
		st.Cursor++
		st.clampCursorScroll()
		return m, nil
	case " ", "space":
		st.toggleAtCursor()
		return m, nil
	case "a":
		// Toggle the whole FILTERED set regardless of cursor position.
		filtered := st.filtered()
		on := st.allFilteredChecked(filtered)
		for _, t := range filtered {
			st.Checked[t] = !on
		}
		return m, nil
	case "enter":
		selected := make([]string, 0, len(st.Types))
		for _, t := range st.Types { // preserve master order, ignore filter
			if st.Checked[t] {
				selected = append(selected, t)
			}
		}
		if len(selected) == 0 {
			// Refuse empty scope — keep the modal open with a nudge.
			m.flash("select at least one metadata type")
			return m, nil
		}
		onConfirm := st.OnConfirm
		m.compareScope = nil
		if onConfirm != nil {
			onConfirm(selected)
		}
		return m, nil
	}
	return m, nil
}
