package ui

var orgQuickJumpLetters = []string{
	"q", "w", "e", "r", "t", "y", "u", "i", "o", "p",
	"a", "s", "d", "f", "g", "h", "j", "k", "l", ";",
}

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
