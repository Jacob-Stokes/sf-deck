package ui

// Per-surface footer hints (the dim "o → Flow Builder · ^o → pick
// target · …" line at the bottom of a list surface).
//
// Problem: when the right sidebar is open beside the main pane, the
// main pane narrows and a long hint line gets ellipsis-truncated
// ("^o → p…"), hiding affordances. Fix: in beside-the-main sidebar
// mode, wrap the hint onto a second (third, …) line instead of
// truncating, splitting on the " · " separators so an affordance is
// never cut mid-word.
//
// Only beside-the-main mode wraps: when the sidebar is closed or
// stacked-below, the main pane is full-width and the hint fits on one
// line anyway, so the original single-line render is kept (matching
// the long-standing look on those layouts).

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// footerHint renders a "·"-separated hint as one or more dim lines,
// fitting width. text should already carry its leading indent (e.g.
// "  o → open · …"), matching the existing dimLine call sites. Returns
// the joined block ready to append to a render's line slice.
func (m Model) footerHint(text string, width int) string {
	// Fits, or not in beside-the-main mode → single line (truncated by
	// dimLine exactly as before on the rare full-width overflow).
	if ansi.StringWidth(text) <= width || !m.sidebarBeside() {
		return dimLine(text, width)
	}
	return strings.Join(wrapHintOnSeparator(text, width), "\n")
}

// sidebarBeside reports whether the sidebar is open AND to the right of
// the main pane (not stacked below, not closed) — the only layout
// where the main pane is narrowed enough to clip footer hints.
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
	// Preserve the leading whitespace indent for continuation lines.
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
