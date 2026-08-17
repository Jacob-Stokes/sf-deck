package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// activeListYankContext returns the active list's cell accessor, row
// count, and columns when the surface exposes them; ok=false otherwise.
func (m *Model) activeListYankContext() (cell func(row, col int) string, rows int, cols []uilayout.ListColumn, ok bool) {
	ctx := m.activeListTableContext()
	if ctx.Cell == nil || ctx.RowCount <= 0 || len(ctx.RenderCols) == 0 {
		return nil, 0, nil, false
	}
	return ctx.Cell, ctx.RowCount, ctx.RenderCols, true
}

func yankableListColumns(cols []uilayout.ListColumn) []int {
	var idx []int
	for i, c := range cols {
		if c.Name == "" || c.Name == "Marks" {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func yankListTableChord() chordSpec {
	return chordSpec{
		Letter:    "y",
		Label:     "y yank list as table",
		Available: func(m Model) bool { _, _, _, ok := (&m).activeListYankContext(); return ok },
		Do: func(m Model) (Model, tea.Cmd) {
			cell, rows, cols, ok := (&m).activeListYankContext()
			if !ok {
				m.flash("no list to yank here")
				return m, nil
			}
			colIdx := yankableListColumns(cols)
			if len(colIdx) == 0 {
				m.flash("no columns to yank")
				return m, nil
			}
			var b strings.Builder
			for j, ci := range colIdx {
				if j > 0 {
					b.WriteByte('\t')
				}
				b.WriteString(strings.TrimSpace(ansi.Strip(cols[ci].Header)))
			}
			b.WriteByte('\n')
			for r := 0; r < rows; r++ {
				for j, ci := range colIdx {
					if j > 0 {
						b.WriteByte('\t')
					}
					b.WriteString(strings.TrimSpace(ansi.Strip(cell(r, ci))))
				}
				b.WriteByte('\n')
			}
			m.flash("copied list · " + intToStr(rows) + " rows")
			return m, yankValueCmd(strings.TrimRight(b.String(), "\n"))
		},
	}
}

func yankIDListChord() chordSpec {
	idCol := func(m *Model) (func(row, col int) string, int, int, bool) {
		cell, rows, cols, ok := m.activeListYankContext()
		if !ok {
			return nil, 0, -1, false
		}
		for i, c := range cols {
			if c.Name == "Id" {
				return cell, rows, i, true
			}
		}
		return nil, 0, -1, false
	}
	return chordSpec{
		Letter:    "i",
		Label:     "i yank ids (comma / IN-list)",
		Available: func(m Model) bool { _, _, _, ok := idCol(&m); return ok },
		Do: func(m Model) (Model, tea.Cmd) {
			cell, rows, col, ok := idCol(&m)
			if !ok {
				m.flash("no Id column in this view")
				return m, nil
			}
			ids := make([]string, 0, rows)
			for r := 0; r < rows; r++ {
				v := strings.TrimSpace(ansi.Strip(cell(r, col)))
				if v != "" {
					ids = append(ids, v)
				}
			}
			if len(ids) == 0 {
				m.flash("no ids to yank")
				return m, nil
			}
			m.flash("copied " + intToStr(len(ids)) + " ids")
			return m, yankValueCmd(strings.Join(ids, ","))
		},
	}
}
