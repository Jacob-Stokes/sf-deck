package ui

// Bottom chrome row: the persistent status bar.
//
// Left side is the org-focus shortcut. Tab nav lives in the top tab
// bar now (see render_tabs.go). Right side is canonical shortcuts
// (/ search, ↵ drill, o open, y yank, r refresh, q quit, etc.).
// Every label and every key reads from the configurable keymap
// (Keys), so remapping a binding automatically updates the hint text.

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderStatusBar() string {
	// Chord mode alert: while active, the whole status bar shows CHORD +
	// the available next-letters, so the q-<letter> namespace is
	// discoverable in place. Wins over the normal shortcut bar.
	if m.chordActive {
		return m.renderChordBar()
	}
	if time.Now().Before(m.bannerUntil) && m.banner != "" {
		content := " " +
			lipgloss.NewStyle().Foreground(theme.Yellow).Background(theme.Panel).Render("● ") +
			m.banner + " "
		if lipgloss.Width(content) > m.width {
			content = ansi.Truncate(content, m.width, "")
		}
		return lipgloss.NewStyle().
			Foreground(theme.Fg).
			Background(theme.Panel).
			Width(m.width).
			MaxHeight(1).
			Render(content)
	}

	// Left side: in-flight export activity indicator. Empty when nothing
	// is running. While exports are in flight this owns the left third of
	// the status bar so the user can navigate freely without wondering
	// whether their report download is still chugging.
	left := m.renderExportActivity()

	// Right side: canonical shortcuts.
	//
	// Notes on what lives here vs the per-surface hint bar:
	//   - Movement (j/k) is dropped — users learn this from any
	//     other TUI; repeating it in the footer is noise.
	//   - "/ search" is dropped because the SearchBar at the top
	//     of every list surface already advertises it; duplicating
	//     in the footer was redundant chrome.
	//   - "ctrl+f global search" earns a slot because it's the
	//     non-obvious cross-cutting search and users won't discover
	//     it otherwise.
	//   - List-table-only keys (sort, page, zen) move to the
	//     per-surface hint bar (see internal/ui/list_table_hint.go)
	//     so they don't claim global footer space when irrelevant.
	keys := footerShortcuts(m)
	// View-system shortcuts (L = source, V = manage views) are surfaced
	// on the chip bar itself — see chip_strip.go's hint suffix — so the
	// status bar stays focused on global keys only.
	//
	// Each key glyph renders as a "keycap chip" — a styled inline
	// rectangle with its own bg, distinct from the bar background.
	// This gives the footer the same agent-deck-style two-tone look:
	// pale chips for keys, plain text labels between them.
	//
	// Why chips rather than just Foreground+Bold on the keys: lipgloss
	// v2 emits a [0m full-reset after Bold spans, which wipes any
	// inherited Background on the surrounding text.  Without chips
	// you get visible "splotches" of page-bg behind each key.  A
	// chip's own explicit bg renders cleanly because it never tries
	// to inherit — and the gaps between chips are pure spaces with
	// no styling, so they pick up the outer bar's bg correctly.
	// Render each key as a "keycap chip" on the page background —
	// no separate bar bg fill.  The chip's own Border-colored
	// background is the only colored region; descriptions are plain
	// muted text floating on whatever sits behind the bar (the
	// terminal's default background).  Matches the agent-deck
	// aesthetic where the row reads as "chips + labels" rather than
	// "tinted bar containing chips".
	keycap := lipgloss.NewStyle().
		Foreground(theme.Fg).
		Background(theme.Border).
		Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	var rightParts []string
	for _, k := range keys {
		rightParts = append(rightParts,
			keycap.Render(" "+k.k+" ")+" "+descStyle.Render(k.d))
	}

	// Build the bar by hand so we control the exact rendered width;
	// theme.StatusBar's own Padding(0,1) + ANSI resets can interact
	// with longer right-clusters and wrap onto a 2nd line. The approach:
	//
	//   1. Drop shortcuts from the LEFT of the right cluster until the
	//      joined string fits in the usable width (total minus left +
	//      the 2 leading/trailing spaces we add ourselves).
	//   2. Compose content inside [" "+left + ...pad... + right +" "].
	//   3. Force the final rendered visible width to exactly m.width
	//      using ansi.Truncate as a safety net for edge cases.
	usable := m.width - lipgloss.Width(left) - 2
	if usable < 0 {
		usable = 0
	}
	right, dropped := joinRightShortcuts(rightParts, usable)
	if dropped > 0 {
		// Some hints didn't fit — flag it so users know the full
		// list lives behind ? (which is sticky at the tail and
		// survives every truncation).
		marker := descStyle.Render("+" + itoaSimple(dropped) + "… ")
		if lipgloss.Width(marker)+lipgloss.Width(right) <= usable {
			right = marker + right
		}
	}

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	content := " " + left + strings.Repeat(" ", pad) + right + " "
	// Last-resort clamp: if rounding/wide-char math still leaves us
	// overflowing, truncate the visible tail. Prefer dropping content
	// to ever wrapping to a second row.
	if lipgloss.Width(content) > m.width {
		content = ansi.Truncate(content, m.width, "")
	}
	// Use NewStyle (no Padding) so our explicit padding inside `content`
	// is the only padding applied. StatusBar's Padding(0,1) was the
	// culprit that pushed content past m.width.
	//
	// No Background here: the bar reads as "chips on the page"
	// rather than "tinted strip containing chips".  Matches the
	// reference agent-deck-style design.  The chips' own bg is the
	// only colored region.
	return lipgloss.NewStyle().
		Foreground(theme.Fg).
		Width(m.width).
		MaxHeight(1).
		Render(content)
}

// footerHint is one footer shortcut: keycap text + description.
// Shared between the status bar and the ? modal's "Footer · this
// view" section, so what the modal lists is BY CONSTRUCTION exactly
// what the footer would show given infinite width.
type footerHint struct{ k, d string }

// footerShortcuts builds the contextual footer hint set, in footer
// order. Front entries are the most context-specific (and the first
// to be dropped on narrow terminals — see joinRightShortcuts).
func footerShortcuts(m Model) []footerHint {
	// Trimmed 2026-06-12: open + open-menu share one chip; the orgs
	// hint moved to the header org pill (the "0" keycap lives where
	// the org identity is shown); settings dropped — it's already a
	// permanent right-nav pill in the tab bar. Everything trimmed
	// remains in the ? modal's "Footer · this view" section via
	// footerShortcutsAll.
	keys := []footerHint{
		{firstPretty(Keys.GlobalSearch), "global search"},
		{firstPretty(Keys.Drill), "drill"},
		{firstPretty(Keys.OpenDefault) + "/" + firstPretty(Keys.OpenMenu), "open"},
		{firstPretty(Keys.YankDefault) + "/" + firstPretty(Keys.YankMenu), "yank"},
		{firstPretty(Keys.Refresh) + "/" + firstPretty(Keys.GlobalRefresh), "refresh"},
	}
	// `\ side` only when the sidebar is hidden — when it's visible,
	// the same hint sits inside the sidebar's bottom-right button so
	// the global footer doesn't double up. Saves a slot for tab-local
	// hints.
	if !m.sidebarOpen {
		keys = append(keys, footerHint{firstPretty(Keys.ToggleSidebar), "side"})
	}
	keys = append(keys,
		// The command-menu hint moved to the header (next to the
		// orgs keychip) so the footer has more room for tab-local
		// hints. See renderHeaderOrgPill.
		footerHint{firstPretty(Keys.Help), "help"},
		footerHint{firstPretty(Keys.Quit), "quit"},
	)
	// Zen is the only list-table-mode key still surfaced in the
	// global footer because it's deliberately a "go fullscreen" gesture
	// — useful enough to keep one keystroke away. sort + page live
	// in the per-surface hint bar.
	if state := (&m).activeListTableState(); state != nil {
		keys = append([]footerHint{
			{firstPretty(Keys.ZenMode), "zen"},
		}, keys...)
	}
	// The view-cycle hint ([/] view) lives right-aligned on the chip
	// strip itself — see chip_strip.go's viewStripHint().
	// Surface subtab-cycling shortcut when the active tab has >1 subtab.
	if len(m.tabSubtabs()) > 1 {
		keys = append([]footerHint{
			{firstPretty(Keys.PrevSubtab) + "/" + firstPretty(Keys.NextSubtab), "subtab"},
		}, keys...)
	}
	if s := m.searchStateForTab(m.tab()); s != nil && s.Committed {
		keys = append([]footerHint{{firstPretty(Keys.SearchClear), "clear"}}, keys...)
	}
	// "esc back" only when the user has somewhere to pop to — a drill
	// tab, a recorded drill-return, or a record→record drill stack.
	// Without this the footer advertised "↵ drill" (go deeper) but
	// never how to come back up.
	if m.canEscBack() {
		keys = append([]footerHint{{"esc", "back"}}, keys...)
	}
	return keys
}

// canEscBack reports whether Esc on the current surface pops to a
// parent/previous view (vs. doing nothing). True on any drill tab
// (its stem is a different parent tab), when a record→record drill
// stack is open, or when a dynamic drill-return is recorded for the
// current tab. Drives the "esc back" footer hint.
func (m Model) canEscBack() bool {
	t := m.tab()
	if t.stem() != t {
		return true
	}
	if len(m.recordDrillStack) > 0 {
		return true
	}
	if d := m.activeOrgData(); d != nil && d.DrillReturnTab != nil {
		if _, ok := d.DrillReturnTab[t]; ok {
			return true
		}
	}
	return false
}

// footerShortcutsAll is the ? modal's variant: the live footer set
// PLUS the hints that were deliberately relocated out of the bar
// (orgs → header pill, settings → tab-bar pill) so the modal stays
// the one complete answer to "what can I press here".
func footerShortcutsAll(m Model) []footerHint {
	keys := footerShortcuts(m)
	keys = append(keys,
		footerHint{firstPretty(Keys.FocusOrgs), "orgs rail (also on the header pill)"},
		footerHint{firstPretty(Keys.OpenMenu), "open menu (multi-target)"},
		footerHint{firstPretty(Keys.OpenSettings), "settings (also top-right pill)"},
	)
	return keys
}

// joinRightShortcuts joins as many trailing parts as fit in budget
// cols (double-space separator). Drops leading parts first — callers
// put the most-droppable hints (search-clear, view, subtab) at the
// front of the slice.
func joinRightShortcuts(parts []string, budget int) (string, int) {
	sep := "  "
	for start := 0; start < len(parts); start++ {
		joined := strings.Join(parts[start:], sep)
		if lipgloss.Width(joined) <= budget {
			return joined, start
		}
	}
	return "", len(parts) // nothing fits
}

// itoaSimple avoids pulling strconv into this file for one marker.
func itoaSimple(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
