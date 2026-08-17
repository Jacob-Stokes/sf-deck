package ui

// Per-surface footer hints (the dim "o → Flow Builder · ^o → pick
// target · …" line at the bottom of a list surface).

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) footerHint(text string, width int) string {
	// Fits, or not in beside-the-main mode → single line (truncated by
	// dimLine exactly as before on the rare full-width overflow).
	if ansi.StringWidth(text) <= width || !m.sidebarBeside() {
		return dimLine(text, width)
	}
	return strings.Join(wrapHintOnSeparator(text, width), "\n")
}

func (m Model) sidebarBeside() bool {
	return m.sidebarOpen && !m.sidebarStacked
}

// wrapHintOnSeparator splits text on " · " and greedily packs segments
// onto lines no wider than width, each dim-styled. The leading indent
// of the first line is preserved on continuation lines so the wrapped
// block stays left-aligned under the original. A single segment wider
// than width is truncated (can't split an affordance mid-word).
func wrapHintOnSeparator(text string, width int) []string {
	const sep = " · "
	indent := text[:len(text)-len(strings.TrimLeft(text, " "))]
	segs := strings.Split(strings.TrimLeft(text, " "), sep)

	var lines []string
	cur := indent
	curEmpty := true
	for _, seg := range segs {
		cand := cur + seg
		if !curEmpty {
			cand = cur + sep + seg
		}
		if ansi.StringWidth(cand) > width && !curEmpty {
			lines = append(lines, cur)
			cur = indent + seg
			curEmpty = false
			continue
		}
		cur = cand
		curEmpty = false
	}
	if !curEmpty {
		lines = append(lines, cur)
	}
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = dimLine(ln, width)
	}
	return out
}
