package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// decorateRowsWithScrollbar appends a 1-char scroll bar to each row
// line in `rowBlock`, anchored at the pane's inner right edge (column
// inner-1) rather than wherever the table's last column happens to
// end. The bar shows `│` (track) for off-thumb positions and `█`
// (thumb) for on-thumb positions. Mutates rowBlock in place.
//
// `rowBlock` is the output of renderRows: rows followed by a trailing
// scrollIndicator line. The indicator is left untouched.
//
// `cur` is the cursor index, `n` is total rows, `viewport` is the row
// budget renderRows was called with (must match — otherwise window
// math drifts). `inner` is the pane's inner width.
func decorateRowsWithScrollbar(rowBlock []string, cur, n, viewport, inner int) {
	if len(rowBlock) == 0 || viewport <= 0 || n <= 0 || inner < 2 {
		return
	}
	rowCount := len(rowBlock) - 1 // last entry is the scrollIndicator
	if rowCount <= 0 {
		return
	}
	start, _ := scrollWindow(cur, n, viewport)
	thumbStart, thumbEnd := scrollThumb(cur, n, viewport)
	trackStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	thumbStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	track := trackStyle.Render("│")
	thumb := thumbStyle.Render("█")
	for vp := 0; vp < rowCount; vp++ {
		glyph := track
		if vp >= thumbStart && vp < thumbEnd {
			glyph = thumb
		}
		_ = start // reserved for future absolute-row indexing if needed
		rowBlock[vp] = fitToScrollbarWidth(rowBlock[vp], inner) + glyph
	}
}

// decoratePagedRowsWithScrollbar is the paginated counterpart to
// decorateRowsWithScrollbar. Bar position reflects the cursor's
// position over the FULL list (not within the active page) so the
// thumb advances continuously as pages turn — matching what the user
// expects from a scroll bar across discrete page jumps.
//
// `rowBlock` is the output of RenderRowsPaged: rows for the active
// page followed by a trailing pageIndicator. The indicator is left
// untouched. Always decorates (no "fits on one page → hide" branch)
// since paginated callers want a stable visual anchor.
func decoratePagedRowsWithScrollbar(rowBlock []string, cur, n, pageSize, inner int) {
	if len(rowBlock) == 0 || pageSize <= 0 || n <= 0 || inner < 2 {
		return
	}
	rowCount := len(rowBlock) - 1 // last entry is the pageIndicator
	if rowCount <= 0 {
		return
	}
	thumbStart, thumbEnd := scrollThumb(cur, n, pageSize)
	trackStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	thumbStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	track := trackStyle.Render("│")
	thumb := thumbStyle.Render("█")
	for vp := 0; vp < rowCount; vp++ {
		glyph := track
		if vp >= thumbStart && vp < thumbEnd {
			glyph = thumb
		}
		rowBlock[vp] = fitToScrollbarWidth(rowBlock[vp], inner) + glyph
	}
}

// fitToScrollbarWidth prepares `s` so that appending a 1-char glyph
// produces a final string of visible width exactly `inner`. The row
// renderer already truncates to `inner`; without this step, appending
// our glyph spills over and the terminal wraps the overflow onto a
// new line — producing the "ghost row" effect the user reported on
// wide tables.
//
// Truncate to `inner-1`, then pad with plain spaces to `inner-1`.
// Empty-tail padding lets the caller append exactly one glyph.
func fitToScrollbarWidth(s string, inner int) string {
	target := inner - 1
	if target < 0 {
		target = 0
	}
	w := ansi.StringWidth(s)
	if w > target {
		s = ansi.Truncate(s, target, "")
		w = ansi.StringWidth(s)
	}
	if w < target {
		s += strings.Repeat(" ", target-w)
	}
	return s
}

// scrollWindow mirrors uilayout.window's [start, end) computation so
// the scrollbar decorator can match the same visible slice renderRows
// drew. Keep in lockstep with internal/ui/uilayout/viewport.go::window —
// if that algorithm changes, this must follow.
func scrollWindow(sel, n, visible int) (int, int) {
	if visible <= 0 || n <= 0 {
		return 0, 0
	}
	if n <= visible {
		return 0, n
	}
	start := sel - visible/3
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

// scrollThumb returns the viewport-relative [thumbStart, thumbEnd)
// range for the bright block. Height is proportional to the viewport
// size over the total list, clamped to at least 1 so very long lists
// still draw a visible thumb. Position interpolates from 0 at sel=0
// to (visible-height) at sel=n-1.
func scrollThumb(sel, n, visible int) (int, int) {
	if visible <= 0 || n <= 0 {
		return 0, 0
	}
	if n <= visible {
		return 0, visible
	}
	h := (visible*visible + n/2) / n
	if h < 1 {
		h = 1
	}
	if h > visible {
		h = visible
	}
	denom := n - 1
	if denom <= 0 {
		return 0, h
	}
	span := visible - h
	pos := (sel*span + denom/2) / denom
	if pos < 0 {
		pos = 0
	}
	if pos > span {
		pos = span
	}
	return pos, pos + h
}
