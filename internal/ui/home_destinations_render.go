package ui

// home_destinations_render.go — render the Lightning Destinations
// grid that sits below the SF-DECK logo on /home Landing.
//
// Layout: 2 columns of sections at >= 100 chars wide, single column
// otherwise. Section headers show "[letter] LABEL"; each row prefixed
// with ↗ to signal "opens in Lightning browser, not in sf-deck."

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderHomeDestinations builds the destinations grid as a list of
// rendered lines. Cursor highlights the currently-focused row; when
// no section is focused the cursor sits on the row but the column-
// header brackets dim to convey "global navigation, type a section
// letter to focus."
//
// Returns the absolute row index (within the returned slice) of the
// cursored entry, or -1 when the cursor isn't visible — the caller
// uses it to scroll the landing view so the cursor stays on screen.
func (m Model) renderHomeDestinations(inner int) ([]string, int) {
	out := []string{}

	// Header bar with footer-style hint about the interaction model.
	// The hint is focus-aware: before focusing a section it explains
	// the two-keystroke model; once a section is focused it explains
	// that item letters open and esc backs out to unfocused (the only
	// way to switch between sections whose letters collide with the
	// focused section's item letters).
	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	var hint string
	if m.homeFocusedSectionLetter != "" {
		hint = "↵ open · " + firstPretty(Keys.YankDefault) + " copy URL · item letter opens · esc back to all sections"
	} else {
		hint = "↵ open · " + firstPretty(Keys.YankDefault) + " copy URL · type [letter] to focus a section, then item letter"
	}
	out = append(out, titleStyle.Render("LIGHTNING DESTINATIONS")+
		"  "+lipgloss.NewStyle().Foreground(theme.FgDim).Render(hint))
	out = append(out, lipgloss.NewStyle().Foreground(theme.Muted).
		Render(strings.Repeat("─", inner)))
	headerRows := len(out) // rows before the grid body starts

	// Two-column layout when there's room. Section widths are derived
	// from the longest label + the longest entry per column so the
	// arrows align cleanly.
	colW := inner / 2
	twoColumn := colW >= 38
	if !twoColumn {
		colW = inner
	}

	// Bucket sections into columns (interleaved: section 0 → col 0,
	// section 1 → col 1, section 2 → col 0, …) so adjacent sections
	// in the catalog sit side by side and the columns balance in
	// height across uneven section sizes.
	var left, right []*homeDestinationSection
	for i := range homeDestinations {
		if twoColumn && i%2 == 1 {
			right = append(right, &homeDestinations[i])
			continue
		}
		left = append(left, &homeDestinations[i])
	}
	if !twoColumn {
		// Single column: just concatenate every section.
		left = nil
		for i := range homeDestinations {
			left = append(left, &homeDestinations[i])
		}
	}

	leftLines, leftCur := m.renderDestinationColumn(left, colW)
	if !twoColumn {
		out = append(out, leftLines...)
		cur := -1
		if leftCur >= 0 {
			cur = headerRows + leftCur
		}
		return out, cur
	}
	rightLines, rightCur := m.renderDestinationColumn(right, colW)
	// Zip the two columns row-by-row, padding the shorter side with
	// blanks so the join lines up.
	for i := 0; i < max(len(leftLines), len(rightLines)); i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		// Pad left column to colW so the right column starts at the
		// same X offset on every row.
		pad := colW - ansi.StringWidth(l)
		if pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		out = append(out, l+r)
	}
	// The cursor lives in whichever column reported it; its zip-row is
	// the column-local row index (both columns share the row index i).
	colCur := leftCur
	if colCur < 0 {
		colCur = rightCur
	}
	cur := -1
	if colCur >= 0 {
		cur = headerRows + colCur
	}
	return out, cur
}

// renderDestinationColumn renders one vertical column of sections.
// Each section is: header line + entry lines + blank separator.
//
// Cursor highlighting compares against the CATALOG flat index for
// each entry (homeDestFlatIndex(sectionIdx, entryIdx) where
// sectionIdx is the catalog position, not the column-local one) —
// otherwise both columns would highlight row 0 simultaneously when
// cursorIdx == 0, producing two visible cursors.
func (m Model) renderDestinationColumn(sections []*homeDestinationSection, w int) ([]string, int) {
	out := []string{}
	focused := m.homeFocusedSectionLetter
	cursorIdx := m.homeDestCursor
	cursorRow := -1

	for sectionIdx, s := range sections {
		// Section header: "[a] ADMIN" — letter bold when focused,
		// dim otherwise.
		letterStyle := lipgloss.NewStyle().Foreground(theme.Magenta).Bold(true)
		if focused != "" && focused != s.Letter {
			letterStyle = lipgloss.NewStyle().Foreground(theme.Muted)
		}
		labelStyle := lipgloss.NewStyle().Foreground(theme.FgDim).Bold(true)
		if focused == s.Letter {
			labelStyle = lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
		}
		header := "  " + letterStyle.Render("["+s.Letter+"]") + " " + labelStyle.Render(s.Label)
		out = append(out, header)

		catalogSectionIdx := homeDestSectionIndex(s.Letter)
		for entryIdx, e := range s.Entries {
			flatIdx := homeDestFlatIndex(catalogSectionIdx, entryIdx)
			isCursor := flatIdx == cursorIdx
			isFocused := focused == s.Letter

			prefix := "    "
			if isCursor {
				prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("  ▌ ")
				cursorRow = len(out) // this entry's row in this column
			}

			// Per-section letter — bold only when section is focused.
			keyStyle := lipgloss.NewStyle().Foreground(theme.Muted)
			if isFocused {
				keyStyle = lipgloss.NewStyle().Foreground(theme.Magenta).Bold(true)
			}
			labelColor := theme.Fg
			if !isFocused {
				labelColor = theme.FgDim
			}
			arrow := lipgloss.NewStyle().Foreground(theme.FgDim).Render("↗")
			label := lipgloss.NewStyle().Foreground(labelColor).Render(e.Label)
			line := prefix + keyStyle.Render(e.Key) + " " + arrow + " " + label
			out = append(out, ansi.Truncate(line, w, "…"))
		}
		// Section separator (skip after the last section in this column).
		if sectionIdx < len(sections)-1 {
			out = append(out, "")
		}
	}
	return out, cursorRow
}

// homeDestFlatIndex returns the flat row count for a given
// (sectionIdx, entryIdx). Used by section-focus to land the cursor
// on the first entry of the focused section.
func homeDestFlatIndex(sectionIdx, entryIdx int) int {
	row := 0
	for i, s := range homeDestinations {
		if i == sectionIdx {
			return row + entryIdx
		}
		row += len(s.Entries)
	}
	return row
}

// homeDestSectionEntryAtIndex returns (section, entry, ok) for a
// flat row index. Used by Enter handling to fire the cursored row.
func homeDestSectionEntryAtIndex(idx int) (*homeDestinationSection, *homeDestination, bool) {
	if idx < 0 {
		return nil, nil, false
	}
	row := 0
	for si := range homeDestinations {
		s := &homeDestinations[si]
		for ei := range s.Entries {
			if row == idx {
				return s, &s.Entries[ei], true
			}
			row++
		}
	}
	return nil, nil, false
}

// homeDestTotalRows is the total entry count across all sections —
// used to clamp cursor movement.
func homeDestTotalRows() int {
	n := 0
	for _, s := range homeDestinations {
		n += len(s.Entries)
	}
	return n
}
