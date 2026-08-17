package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

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

func formatINValue(v any, formatted string) string {
	switch v.(type) {
	case string:
		return "'" + strings.ReplaceAll(formatted, "'", "''") + "'"
	case bool, float64, int, int64, float32:
		return formatted
	}
	return "'" + strings.ReplaceAll(formatted, "'", "''") + "'"
}
