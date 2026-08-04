package uilayout

// Small reusable table primitive for the read-only tables on /home,
// /flows, /perms-overview, etc. — anywhere the UI shows fixed columns
// without cursor / drill / search interaction.
//
// Not for the bigger interactive lists (objects, fields, records) —
// those have their own widths-from-state machinery (fieldTableLayout
// etc.) since they need to coordinate with cursor / FLAGS column /
// scroll indicator.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// Column declares one column. Width is the rendered cell width (cells
// wider than this get truncated with …; narrower get right-padded).
// Negative Width means "fill the leftover space" — at most one column
// can be flex.
type Column struct {
	Header string
	Width  int
	Style  lipgloss.Style // applied to body cells; zero-value means default theme.Fg
}

// Sep is the rendered column separator. Exposed so callers can match
// the existing field-table look without re-deriving.
var Sep = lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")

// RenderTableHeader renders the header row using the same column widths
// as the interactive row renderers. Pass the same []Column slice to both.
func RenderTableHeader(cols []Column, inner int) string {
	resolved := resolveWidths(cols, inner)
	hdr := lipgloss.NewStyle().Foreground(theme.Muted).Bold(true)
	parts := make([]string, len(cols))
	for i, c := range cols {
		w := resolved[i]
		parts[i] = hdr.Width(w).Render(ansi.Truncate(c.Header, w, "…"))
	}
	return ansi.Truncate("  "+strings.Join(parts, Sep), inner, "…")
}

// RenderInteractiveTableRow renders a cursored table row. It prepends
// the standard "▌ " selection bar (or "  " gutter)
// and bolds the first cell when selected, leaving every column's own
// Style intact. cells[0] is treated as the "name"-style column that
// pops bold on cursor.
//
// Highlight terms aren't passed here; this is the cleaner-to-call
// shape for non-search surfaces. RenderInteractiveTableRowHighlight
// is the variant that applies match highlighting per cell.
func RenderInteractiveTableRow(cols []Column, cells []string, selected, focused bool, inner int) string {
	return RenderInteractiveTableRowHighlight(cols, cells, selected, focused, inner, nil)
}

// RenderInteractiveTableRowHighlight is RenderInteractiveTableRow with
// per-cell match highlighting. Each cell goes through HighlightInStyle
// using the column's foreground style, so the highlight's yellow
// background sits cleanly inside the column colour and the surrounding
// fg colour survives the ANSI reset that comes after each match.
//
// terms is the slice produced by uilayout.SearchTerms (or any
// equivalent per-row search-term parser). Empty / nil → identical to
// the no-highlight variant; cheap to call regardless.
func RenderInteractiveTableRowHighlight(cols []Column, cells []string, selected, focused bool, inner int, terms []string) string {
	resolved := resolveWidths(cols, inner)
	parts := make([]string, len(cols))
	for i, c := range cols {
		w := resolved[i]
		s := c.Style
		if s.GetForeground() == nil {
			s = lipgloss.NewStyle().Foreground(theme.Fg)
		}
		if selected && i == 0 {
			s = s.Bold(true)
		}
		truncated := ansi.Truncate(cells[i], w, "…")
		if len(terms) == 0 {
			parts[i] = s.Width(w).Render(truncated)
		} else {
			// Render the highlighted-in-style cell first (so highlight
			// + base alternate cleanly), then pad to the column width.
			body := HighlightInStyle(truncated, terms, s)
			parts[i] = lipgloss.NewStyle().Width(w).Render(body)
		}
	}
	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !focused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	return ansi.Truncate(prefix+strings.Join(parts, Sep), inner, "…")
}

// resolveWidths fills any flex (negative) column with leftover space.
// Reserves "  " gutter + (n-1) " │ " separators against inner.
func resolveWidths(cols []Column, inner int) []int {
	const gutter = 2
	const sepW = 3 // " │ "
	resolved := make([]int, len(cols))
	used := gutter + sepW*(len(cols)-1)
	flex := -1
	for i, c := range cols {
		if c.Width < 0 {
			flex = i
			continue
		}
		resolved[i] = c.Width
		used += c.Width
	}
	if flex >= 0 {
		w := inner - used
		if w < 6 {
			w = 6
		}
		resolved[flex] = w
	}
	return resolved
}
