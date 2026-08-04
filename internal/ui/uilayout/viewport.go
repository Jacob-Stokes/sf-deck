package uilayout

// Viewport is the shared "scroll a list of rows that exceeds the pane"
// primitive. Every list-based view (Flows, Objects, SOQL results, etc.)
// pipes through RenderRows so cursor-following, row windowing, the
// scroll indicator, and the reserved-lines math all live in one place.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// RenderRows is the single entry point every list view should use.
//
// It computes the visible [start, end) slice around `sel`, calls
// renderRow(i) once per visible row, and appends a right-aligned
// "N / M" scroll indicator whenever the viewport is a proper subset
// of the list.
//
//	reserved   — lines already consumed by header/dashboard/search
//	             bar; subtracted from innerH to get row budget
//	trailing   — lines the caller will append after the rows (e.g. a
//	             final "esc back" hint); also subtracted
//	n          — total row count
//	sel        — cursor position
//	innerH     — pane's content height (width is the caller's concern)
//	inner      — pane's content width (used by the scroll indicator)
//	renderRow  — called once per visible row; gets the absolute index
//
// Returns the row block as a []string so the caller can splice it
// between their header/footer — they know their layout, we don't.
func RenderRows(
	n, sel, innerH, reserved, trailing, inner int,
	renderRow func(i int) string,
) []string {
	budget := innerH - reserved - trailing
	if budget < minViewportRows {
		budget = minViewportRows
	}
	start, end := window(sel, n, budget)
	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		out = append(out, renderRow(i))
	}
	// Always render the position indicator — even when the full list
	// fits in the viewport. "33 / 33" gives users a stable count
	// reference; the previous "hide when nothing to scroll" behaviour
	// looked like the count had vanished on small lists.
	if n > 0 {
		out = append(out, scrollIndicator(sel, n, inner))
	}
	return out
}

// RenderRowsN is the multi-line-per-row variant. `rowLines` is how
// many terminal lines one row occupies (e.g. Projects renders each
// project as up to 3 lines). The budget math divides the remaining
// vertical space by rowLines so the viewport budgets by *row count*
// rather than line count.
func RenderRowsN(
	n, sel, innerH, reserved, trailing, inner, rowLines int,
	renderRow func(i int) string,
) []string {
	if rowLines <= 0 {
		rowLines = 1
	}
	budget := (innerH - reserved - trailing) / rowLines
	if budget < minViewportRows {
		budget = minViewportRows
	}
	start, end := window(sel, n, budget)
	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		out = append(out, renderRow(i))
	}
	if n > 0 {
		out = append(out, scrollIndicator(sel, n, inner))
	}
	return out
}

// RenderRowsPaged is the paginated variant. Instead of windowing
// around `sel` (less/vim style), it shows rows for the active page:
// rows [page*pageSize, (page+1)*pageSize). Cursor is rendered
// relative to the page (i == sel still highlights when the absolute
// index sel falls inside the visible page).
//
// Page boundaries are stable: resizing the terminal changes
// pageSize and therefore "what page X holds", but within a single
// frame the page is well-defined.
//
// Returns the row block + the number of pages. Pagination footer
// ("Page 3 / 12") is the caller's job — different surfaces want
// different placement (footer, title, both).
func RenderRowsPaged(
	n, sel, page, pageSize, inner int,
	renderRow func(i int) string,
) (rows []string, totalPages int) {
	totalPages = TotalPages(n, pageSize)
	if totalPages == 0 {
		return nil, 0
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page * pageSize
	end := start + pageSize
	if end > n {
		end = n
	}
	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		out = append(out, renderRow(i))
	}
	// Always render the page indicator so users see a stable
	// "Page 1 / 1" reference even on small lists. Hiding it when
	// totalPages==1 looked like the count had disappeared on
	// short result sets.
	out = append(out, pageIndicator(page, totalPages, inner))
	return out, totalPages
}

// pageIndicator is the right-aligned "Page X / N" footer hint
// shown beneath the rows when paginated mode covers more than one
// page. Mirrors scrollIndicator's positioning so users get the same
// visual anchor in both modes.
func pageIndicator(page, total, width int) string {
	s := fmt.Sprintf("Page %d / %d", page+1, total)
	pad := width - len(s) - 2
	if pad < 0 {
		pad = 0
	}
	return lipgloss.NewStyle().Foreground(theme.Muted).
		Render(strings.Repeat(" ", pad) + s)
}

// minViewportRows is the floor we refuse to go below even on tiny
// terminals — some rows is better than none.
const minViewportRows = 5

// window returns [start, end) for a viewport of `visible` rows around
// `sel`, clamped to [0, n). Puts the cursor ~1/3 down the pane so it
// doesn't stick to the top edge — familiar from less/vim.
func window(sel, n, visible int) (int, int) {
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

// scrollIndicator renders a one-line "N / M" position hint, right-
// aligned to the given inner width. Callers only emit it when the
// viewport is a proper subset of the list; RenderRows handles that
// check automatically.
func scrollIndicator(sel, total, width int) string {
	s := fmt.Sprintf("%d / %d", sel+1, total)
	pad := width - len(s) - 2
	if pad < 0 {
		pad = 0
	}
	return lipgloss.NewStyle().Foreground(theme.Muted).
		Render(strings.Repeat(" ", pad) + s)
}
