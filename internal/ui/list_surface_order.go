package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func installListViewOrder[T any](
	lv *resource.ListView[T],
	state *uilayout.ListTableState,
	cols []uilayout.ListColumn,
	cell func(T, int) string,
) {
	installListViewOrderRows(lv, state, cols, func(items []T, row, col int) string {
		if row < 0 || row >= len(items) {
			return ""
		}
		return cell(items[row], col)
	})
}

func installListViewOrderRows[T any](
	lv *resource.ListView[T],
	state *uilayout.ListTableState,
	cols []uilayout.ListColumn,
	cell func([]T, int, int) string,
) {
	if state != nil {
		state.RowsOrdered = false
	}
	if lv == nil || state == nil || state.SortColumn == "" || cell == nil {
		if lv != nil {
			lv.SetOrder(nil, "")
		}
		return
	}
	col := -1
	for i, c := range cols {
		if c.Name == state.SortColumn {
			col = i
			break
		}
	}
	if col < 0 {
		lv.SetOrder(nil, "")
		return
	}
	state.RowsOrdered = true
	desc := state.SortDesc
	key := listViewOrderKey(cols, state.SortColumn, desc)
	// SetOrder short-circuits on a stable key but the closure literal
	// below still escapes every frame; skip construction entirely.
	if lv.OrderKey() == key {
		return
	}
	lv.SetOrder(func(items []T) []int {
		out := make([]int, len(items))
		for i := range out {
			out[i] = i
		}
		keys := make([]string, len(items))
		for i := range items {
			keys[i] = cell(items, i, col)
		}
		sort.SliceStable(out, func(i, j int) bool {
			cmp := uilayout.CompareCells(keys[out[i]], keys[out[j]])
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
		return out
	}, key)
}

func listViewOrderKey(cols []uilayout.ListColumn, sortColumn string, desc bool) string {
	var b strings.Builder
	b.WriteString(sortColumn)
	if desc {
		b.WriteString(":desc")
	} else {
		b.WriteString(":asc")
	}
	for _, c := range cols {
		b.WriteByte('|')
		b.WriteString(c.Name)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(c.Min))
	}
	return b.String()
}
