package uilayout

// RowMark — generic per-row visual annotation system.
//
// Existing surfaces tinted/badged rows in ad-hoc ways: /objects tinted
// custom-sObject names cyan, /dev-project-detail appended a yellow
// `[ns]` badge to managed-package items, /home Limits row-tinted
// thresholds, etc. Six different implementations all reaching for
// theme colors and stitching the result into the row body.
//
// RowMark is the shared primitive: a named rule pairing a per-row
// match closure with a Treatment. Specs declare their marks once;
// the renderer applies all matching marks per row, composing
// treatments deterministically. Adding a new visual annotation
// becomes one struct literal instead of six lines spread across
// the row builder.
//
// Treatments compose:
//   - NameColor: last-applied wins (caller orders marks by precedence)
//   - Badges: stack as `[mark1] [mark2]` after the name
//   - Dim: any matching mark with Dim=true wins (all-or-nothing)
//
import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// RowMark is one rule that may apply to a row in a list-table.
// Each tab declares the marks it wants on its ListTableSpec; the
// renderer walks them per row and composes the matching ones.
type RowMark struct {
	// ID is a stable identifier ("custom-sobject", "managed-package",
	// "loaded-project-member"). Used for legend deduplication and
	// future settings UI / TOML config. Lower-kebab-case by convention.
	ID string

	// Label is the human-readable name shown in the auto-generated
	// legend chip ("custom", "managed package", "in loaded project").
	// Should fit in a chip — short.
	Label string

	// Matches is the per-row predicate. row is the row index passed
	// to the surrounding ListTableSpec.Cell. Should be fast (no I/O,
	// no lookups against the network); typically reads a slice index
	// or map lookup the caller closed over.
	Matches func(row int) bool

	// Treatment is what to apply when Matches returns true.
	Treatment Treatment
}

// Treatment is the visual change a mark applies to a matched row.
// Optional fields default to no-op so partial treatments work
// (e.g. a name-tint-only mark sets only NameColor).
//
// Treatments used to include an inline Badge string appended to the
// primary identifier column ("[managed]", "★"). That visualisation
// has been replaced by a dedicated Marks column on the listSurface;
// `RowMark.Label` is now the canonical mark text and `BadgeColor`
// stays as the per-mark text color used in that column. Surfaces
// that still want column-0 tinting (cyan name for custom sObjects)
// use NameColor for the row tint and emit no extra column text.
type Treatment struct {
	// NameColor tints the primary identifier column (column 0). Nil
	// = leave the column's declared style alone.
	NameColor color.Color

	// Dim renders the whole row in muted shade — used for "this row
	// exists but isn't actionable" semantics (e.g. stale bundles).
	Dim bool

	// BadgeColor tints the mark's label text in the dedicated Marks
	// column. Nil falls back to NameColor when set, otherwise to a
	// sensible muted default.
	BadgeColor color.Color
}

// ApplyMarks walks the spec's marks for one row and returns the
// effective NameColor + dim flag. Returns (nil, false) when no
// marks fire.
//
// Composition rules:
//   - NameColor: last-matching mark wins. Mark order on the spec
//     defines precedence — most specific last (e.g. put
//     "loaded-project-member" after "custom-sobject" if both fire).
//   - Dim: any matching mark with Dim=true forces the whole row
//     to render in a muted shade (caller decides how).
//
// The dedicated Marks column reads matching marks via MatchingMarks;
// this function intentionally doesn't compose mark labels itself.
func ApplyMarks(marks []RowMark, row int) (nameColor color.Color, dim bool) {
	for _, m := range marks {
		if m.Matches == nil || !m.Matches(row) {
			continue
		}
		if m.Treatment.NameColor != nil {
			nameColor = m.Treatment.NameColor
		}
		if m.Treatment.Dim {
			dim = true
		}
	}
	return
}

// MatchingMarks returns the subset of `marks` whose Matches predicate
// fires for the given row. Used by the dedicated Marks column on
// list surfaces — render one styled label per matching mark, joined
// with space.
//
// Mark ordering is preserved so per-surface declarations control
// the display order ("custom" before "managed" if both fire).
func MatchingMarks(marks []RowMark, row int) []RowMark {
	out := make([]RowMark, 0, len(marks))
	for _, m := range marks {
		if m.Matches == nil || !m.Matches(row) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// MarksCellMode controls per-cell rendering shape — the FLAGS
// column toggles between these via Ctrl+F.
type MarksCellMode int

const (
	// MarksCellModeFull renders each mark as its full Label, joined
	// by spaces ("managed session"). Default.
	MarksCellModeFull MarksCellMode = iota
	// MarksCellModeLetter renders each mark as its first letter,
	// concatenated ("ms" for managed+session). Compact view.
	MarksCellModeLetter
)

// RenderMarksCellMode is RenderMarksCell + an explicit mode. Lets
// the FLAGS column compact down to one-letter glyphs without each
// surface knowing the cycle state.
func RenderMarksCellMode(marks []RowMark, row int, mode MarksCellMode) string {
	matching := MatchingMarks(marks, row)
	if len(matching) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matching))
	sep := " "
	if mode == MarksCellModeLetter {
		sep = ""
	}
	for _, m := range matching {
		label := m.Label
		if mode == MarksCellModeLetter {
			label = firstLetterUpper(m.Label)
			if label == "" {
				continue
			}
		}
		parts = append(parts, renderMarkLabel(label, m.Treatment.BadgeColor, m.Treatment.NameColor))
	}
	return strings.Join(parts, sep)
}

// firstLetterUpper returns the first rune of s uppercased. Skips
// leading whitespace; returns "" for empty input. Used for
// MarksCellModeLetter glyphs.
func firstLetterUpper(s string) string {
	for _, r := range s {
		if r == ' ' || r == '\t' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			r = r - 'a' + 'A'
		}
		return string(r)
	}
	return ""
}

// renderMarkLabel styles one mark label for the Marks column. Picks
// BadgeColor when set, falls back to NameColor, then to bold default
// so a mark always reads as a distinct annotation (not body text).
func renderMarkLabel(text string, badgeColor, nameColor color.Color) string {
	c := badgeColor
	if c == nil {
		c = nameColor
	}
	if c == nil {
		return lipgloss.NewStyle().Bold(true).Render(text)
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(text)
}
