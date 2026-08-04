package ui

// Org quick-jump shortcuts — "ultra shortcut" mode for the left-rail
// Orgs panel. The user presses `0` (FocusOrgs) and a single QWERTY
// letter overlays each visible org's row; pressing the letter
// immediately selects that org AND returns focus to the main pane.
// Any non-shortcut action (j/k/up/down/scroll/esc) dismisses the
// overlay and behaves normally.
//
// Letters are laid out as the user's left-hand QWERTY home row —
// fingers are already there from pressing `0`. Two rows: 10 each.
// Indices 20+ have no shortcut; user has to nav with j/k to reach
// them.

// orgQuickJumpLetters is the QWERTY ordering: top row qwertyuiop,
// then home row asdfghjkl;. Indexed by org position.
var orgQuickJumpLetters = []string{
	"q", "w", "e", "r", "t", "y", "u", "i", "o", "p",
	"a", "s", "d", "f", "g", "h", "j", "k", "l", ";",
}

// orgQuickJumpLetterFor returns the shortcut letter for the org at
// position i, or "" when i is past the table (no shortcut). The
// renderer uses "" to skip drawing a letter slot entirely.
func orgQuickJumpLetterFor(i int) string {
	if i < 0 || i >= len(orgQuickJumpLetters) {
		return ""
	}
	return orgQuickJumpLetters[i]
}

// orgQuickJumpIndexFor returns the org index for a key, or -1 when
// the key isn't a quick-jump letter. Keys are matched verbatim
// (lowercase only) — chord forms like ctrl+q never match.
func orgQuickJumpIndexFor(key string) int {
	for i, ltr := range orgQuickJumpLetters {
		if ltr == key {
			return i
		}
	}
	return -1
}
