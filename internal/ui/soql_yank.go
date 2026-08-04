package ui

// SOQL result yank gestures.
//
// Three flavours, each binding a copy semantic to "what cells does
// the user want?":
//
//   y       (SOQLYankCell)   — cursored cell value (single string)
//   Y       (SOQLYankRow)    — cursored row as TSV (one tab-joined line)
//   ctrl+y  (SOQLYankColumn) — cursored column as ('id1','id2',…)
//                              ready to paste into another query's
//                              WHERE … IN clause.
//
// Active only on the SOQL Editor with results loaded. The Saved /
// History subtabs route the same keys to their own actions
// (duplicate / refresh) so the dispatcher checks subtab before
// firing these handlers.
//
// "Cursored column" comes from soqlTable.ColCursor — always the
// highlighted column. Defaults to column 0 (Id when present) when
// the user hasn't yet h-scrolled. Matches every other listSurface
// in sf-deck — yank-column targets the highlighted column, which
// the user picks via ←/→.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// soqlActiveResultsContext returns the cursored-row + columns +
// active column index for the current SOQL editor results, or
// ok=false when the user isn't in a state where yank applies.
func (m Model) soqlActiveResultsContext() (rec map[string]any, cols []string, colIdx int, ok bool) {
	if m.tab() != TabSOQL || m.currentSubtab() != SubtabSOQLEditor {
		return nil, nil, 0, false
	}
	if len(m.soqlResult.Records) == 0 {
		return nil, nil, 0, false
	}
	cols = collectColumns(m.soqlResult.Records, m.soqlInput.Value())
	if len(cols) == 0 {
		return nil, nil, 0, false
	}
	rec, ok = m.soqlSelectedRecord()
	if !ok {
		return nil, nil, 0, false
	}
	colIdx = m.soqlTable.ColCursor
	if colIdx < 0 || colIdx >= len(cols) {
		colIdx = 0
	}
	return rec, cols, colIdx, true
}

// handleSOQLYankCell copies the cursored cell's value to the
// system clipboard.
func (m Model) handleSOQLYankCell() (Model, tea.Cmd) {
	rec, cols, colIdx, ok := m.soqlActiveResultsContext()
	if !ok {
		return m, nil
	}
	col := cols[colIdx]
	val, _ := sf.Record(rec).Field(col)
	s := formatCell(val) // existing helper used by the renderer
	return m, m.yankToClipboard(s, "yanked "+col+": "+truncate(s, 30))
}

// handleSOQLYankRow copies the cursored row as a TSV-joined line.
// Headers aren't included — the user can paste into a single row
// of a spreadsheet without re-aligning columns.
func (m Model) handleSOQLYankRow() (Model, tea.Cmd) {
	rec, cols, _, ok := m.soqlActiveResultsContext()
	if !ok {
		return m, nil
	}
	parts := make([]string, len(cols))
	for i, c := range cols {
		v, _ := sf.Record(rec).Field(c)
		parts[i] = formatCell(v)
	}
	s := strings.Join(parts, "\t")
	return m, m.yankToClipboard(s, fmt.Sprintf("yanked row (%d cols)", len(cols)))
}

// handleSOQLYankColumn copies every value in the cursored column
// across every result row, formatted as ('v1','v2',…). Strings get
// single-quoted with apostrophe-doubling; non-strings render bare.
// Empty values are skipped — including ” in a WHERE … IN clause
// usually indicates a query bug rather than intent.
func (m Model) handleSOQLYankColumn() (Model, tea.Cmd) {
	_, cols, colIdx, ok := m.soqlActiveResultsContext()
	if !ok {
		return m, nil
	}
	col := cols[colIdx]
	parts := make([]string, 0, len(m.soqlResult.Records))
	for _, rec := range m.soqlResult.Records {
		v, _ := sf.Record(rec).Field(col)
		s := formatCell(v)
		if s == "" {
			continue
		}
		parts = append(parts, formatINValue(v, s))
	}
	if len(parts) == 0 {
		m.flash("nothing to yank — column " + col + " is empty")
		return m, nil
	}
	out := "(" + strings.Join(parts, ",") + ")"
	return m, m.yankToClipboard(out, fmt.Sprintf("yanked %s × %d as IN-clause", col, len(parts)))
}

// yankToClipboard writes s to the system clipboard and returns a
// no-op tea.Cmd that flashes the success/failure message. Failure
// case is rare on macOS (pbcopy is always present) but possible on
// Linux when no clipboard tool is installed.
func (m *Model) yankToClipboard(s, successFlash string) tea.Cmd {
	if err := writeClipboard(s); err != nil {
		applog.Error("soql.yank.failed", map[string]any{"err": err.Error()})
		m.flash("yank failed: " + err.Error())
		return nil
	}
	m.flash(successFlash)
	return nil
}

// formatINValue stringifies v for inclusion in a SOQL IN-clause.
// Strings get single-quoted with apostrophes doubled (SOQL escape).
// Numbers / bools render bare. Falls through to a quoted version of
// the formatted-cell string for unknown shapes — safer than
// emitting an unquoted unknown-typed value into a SOQL clause.
func formatINValue(v any, formatted string) string {
	switch v.(type) {
	case string:
		return "'" + strings.ReplaceAll(formatted, "'", "''") + "'"
	case bool, float64, int, int64, float32:
		return formatted
	}
	// Fallback: quote it. The user can always edit the result
	// before pasting if the type was something exotic.
	return "'" + strings.ReplaceAll(formatted, "'", "''") + "'"
}
