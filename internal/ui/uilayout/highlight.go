package uilayout

// Search-match highlighting — the visual "this is where your query
// hit" cue applied to list-view cells. Pure function; takes already-
// truncated text + the active search terms and returns the same
// text with ANSI escapes wrapping each match.
//
// Why post-truncation? If we highlighted source strings, then
// truncated, we'd either:
//   - cut the highlight wrapper open (mid-ANSI truncation breaks
//     the rendering); or
//   - over-highlight content the user can't see, wasting visual
//     attention.
// Operating on the already-truncated cell guarantees the highlight
// always lands on visible characters.
//
// Why case-insensitive but preserve original case? Users type
// lowercase queries reflexively; matching "acc" to "Account" is
// the expected behaviour. The output preserves the row's original
// case so column data still reads correctly.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// HighlightStyle is the lipgloss style applied to matched substrings.
// Yellow background + theme background foreground gives strong
// contrast without clobbering the surrounding cell colour. Same
// shape as the chip-strip's transient slot so the visual language
// stays consistent across the app.
var HighlightStyle = lipgloss.NewStyle().
	Foreground(theme.Bg).
	Background(theme.Yellow).
	Bold(true)

// HighlightInStyle is Highlight with re-application of `base` after
// each match. Used for table cells that have a per-column foreground
// colour: the highlight's background looks correct AND the bytes
// after the highlight stay in the column's colour rather than
// reverting to the terminal default.
//
// `base` should be the foreground/colour-only style the caller would
// otherwise pass to lipgloss Style.Render. This helper does the
// non-match runs through base and the match runs through HighlightStyle.
// Result is a single concatenated string with no width-padding —
// callers still wrap with .Width(w).Render(...) afterward to pad.
func HighlightInStyle(text string, terms []string, base lipgloss.Style) string {
	if text == "" {
		return text
	}
	cleaned := cleanedTerms(terms)
	if len(cleaned) == 0 {
		return base.Render(text)
	}
	mark := markRanges(text, cleaned)

	var b strings.Builder
	b.Grow(len(text) + 32*len(cleaned))
	i := 0
	for i < len(text) {
		if !mark[i] {
			j := i
			for j < len(text) && !mark[j] {
				j++
			}
			b.WriteString(base.Render(text[i:j]))
			i = j
			continue
		}
		j := i
		for j < len(text) && mark[j] {
			j++
		}
		b.WriteString(HighlightStyle.Render(text[i:j]))
		i = j
	}
	return b.String()
}

// cleanedTerms strips empty / whitespace-only entries.
func cleanedTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// markRanges computes a per-byte boolean mark for which positions in
// text are inside any term's case-insensitive match. Shared by
// Highlight + HighlightInStyle.
func markRanges(text string, terms []string) []bool {
	mark := make([]bool, len(text))
	lower := strings.ToLower(text)
	for _, t := range terms {
		lt := strings.ToLower(t)
		if lt == "" {
			continue
		}
		off := 0
		for off < len(lower) {
			i := strings.Index(lower[off:], lt)
			if i < 0 {
				break
			}
			start := off + i
			end := start + len(lt)
			for j := start; j < end && j < len(mark); j++ {
				mark[j] = true
			}
			off = end
		}
	}
	return mark
}

// SearchTerms parses the active search query into a slice of bare
// terms suitable for Highlight(). Supports the same fielded shorthand
// the records subtab uses: `field:value`. The field prefix is
// stripped — Highlight just needs the literal search strings; whether
// to scope a particular term to a column is the caller's decision.
//
// Empty query → empty slice. Multiple whitespace-separated tokens →
// multi-term match (each token highlights independently).
func SearchTerms(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	fields := strings.Fields(q)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if i := strings.IndexByte(f, ':'); i > 0 && i < len(f)-1 {
			out = append(out, f[i+1:])
			continue
		}
		out = append(out, f)
	}
	return out
}
