package uilayout

// Formatting primitives used across views + sidebar: section titles,
// key/value lines, colored status helpers, date/age formatters,
// limit-bar rendering, etc. No Model state here — pure functions.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

// --- text primitives ----------------------------------------------------

func SectionTitle(s string) string {
	return lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).Render(s)
}

func KvLine(k, v string, width int) string {
	// Key column: pad short keys to 12 cols for alignment, but let
	// longer keys ("external id", "relationship", "precision / scale")
	// run to their natural width WITHOUT wrapping — a fixed Width(12)
	// makes lipgloss wrap a 13-char key onto two lines, which breaks
	// the row layout (and the cursor/scroll line accounting) on narrow
	// panes. We pad manually and never set a wrapping Width.
	label := "  " + k
	if w := ansi.StringWidth(label); w < 12 {
		label += strings.Repeat(" ", 12-w)
	}
	left := lipgloss.NewStyle().Foreground(theme.Muted).Render(label)
	avail := width - ansi.StringWidth(label) - 2
	if avail < 1 {
		avail = 1
	}
	val := ansi.Truncate(v, avail, "…")
	return left + "  " + lipgloss.NewStyle().Foreground(theme.Fg).Render(val)
}

// DimLine renders dim text truncated PER LINE — multi-line blocks
// (wrapped prose, formulas) keep their lines; each line is clamped
// to width. Vertical truncation is deliberately NOT this function's
// job: panes clip at their height budget and stamp the ⚠ truncated
// indicator (sidebar) or scroll (detail panes). Same contract as
// ui.sideDim after the 2026-06-13 collapse bug.
func DimLine(s string, width int) string {
	style := lipgloss.NewStyle().Foreground(theme.FgDim)
	if !strings.Contains(s, "\n") {
		return style.Render(ansi.Truncate(s, width, "…"))
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return style.Render(strings.Join(lines, "\n"))
}

func RedLine(s string) string {
	return lipgloss.NewStyle().Foreground(theme.Red).Render(s)
}

func StateSuffix(busy bool, err error) string {
	if err != nil {
		return "  (error)"
	}
	if busy {
		return "  (syncing…)"
	}
	return ""
}

// --- search-bar + header pill -------------------------------------------

// SearchBar renders the unified search row below a view's title.
// One line covers all three states so the layout stays compact:
//
//	idle:      "  / press / to search"     (dim hint)
//	active:    "  ⌕ Request▌  ↵ commit · esc cancel · ctrl+u clear"
//	committed: "  ⌕ \"Request\"  C: clear"
//
// Yellow ⌕ glyph throughout when there's a query (active or
// committed) so the user has one visual landmark for "this view
// is search-affected." While Active the textinput draws inline
// after the glyph (no quotes — it's a live edit, not a locked-in
// thing). After commit the buffer renders quoted to mark it as
// locked.
//
// The header used to carry a separate pill alongside this bar,
// duplicating the buffer on two consecutive lines. Pill is now
// folded into this one row.
func SearchBar(s resource.SearchState, width int) string {
	dim := lipgloss.NewStyle().Foreground(theme.FgDim)
	switch {
	case s.Active:
		// Live-edit mode. Yellow glyph + the textinput's own
		// cursor + a hint listing the truthful keys.
		glyph := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("  ⌕ ")
		inputView := ""
		if s.Inited {
			inputView = s.Input.View()
		}
		hint := dim.Render("   ↵ commit · esc cancel · ctrl+u clear")
		return ansi.Truncate(glyph+inputView+hint, width, "…")
	case s.Committed:
		// Filter applied, user back in nav. Quoted buffer marks
		// it locked-in; `/` re-opens the input with the existing
		// buffer to amend, `C` clears. Esc is NOT advertised here
		// because esc behaviour is context-dependent — it clears
		// at the top level but pops drill-ins one step up (and
		// preserves the filter on return). Advertising "esc to
		// clear" universally would lie on every drill surface. C
		// is the unambiguous clear that works everywhere.
		glyph := lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).
			Render(fmt.Sprintf("  ⌕ \"%s\"", s.Buffer()))
		hint := dim.Render("  / edit · C clear")
		return ansi.Truncate(glyph+hint, width, "…")
	default:
		// No query yet. Faint hint, no glyph.
		return dim.Render("  press / to search")
	}
}

// HeaderWithSearchPill renders just the section title now —
// the search "pill" used to ride alongside it but has been
// folded into SearchBar so the title and search row don't both
// carry duplicate "⌕ <query>" text.
//
// Kept under this name (rather than renamed to SectionTitle
// directly) because callers all over the UI pass a SearchState
// here; flipping the call shape is a separate refactor with
// blast-radius across every list-bearing surface. The unused
// argument signals the original intent and keeps the merge
// surgical.
func HeaderWithSearchPill(title string, _ resource.SearchState) string {
	return SectionTitle(title)
}

func DashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// CapsFlags renders a compact "Q C U D" badge showing which CRUD verbs
// an sObject supports (Queryable / Creatable / Updateable / Deletable).
func CapsFlags(d sf.SObjectDescribe) string {
	flags := []string{}
	if d.Queryable {
		flags = append(flags, "Q")
	}
	if d.Creatable {
		flags = append(flags, "C")
	}
	if d.Updatable {
		flags = append(flags, "U")
	}
	if d.Deletable {
		flags = append(flags, "D")
	}
	if d.Custom {
		flags = append(flags, "custom")
	}
	return strings.Join(flags, " ")
}

// PrettyDate trims a Salesforce ISO timestamp "2026-04-21T22:00:00.000+0000"
// down to "2026-04-21 22:00" for row display.
func PrettyDate(iso string) string {
	if len(iso) >= 16 {
		return iso[:10] + " " + iso[11:16]
	}
	return iso
}
