package ui

import (
	"fmt"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestRenderListModelSkipsSortWhenRowsAlreadyOrdered(t *testing.T) {
	const rows = 500
	newModel := func(state *uilayout.ListTableState, calls *int) listRenderModel {
		search := &searchState{}
		search.EnsureInit()
		return listRenderModel{
			Title:  "TEST",
			State:  state,
			Search: search,
			Cols:   []uilayout.ListColumn{{Name: "Name", Header: "NAME", Min: 8, Ideal: 12}},
			N:      rows,
			Cursor: 0,
			Cell: func(row, col int) string {
				*calls = *calls + 1
				return fmt.Sprintf("%04d", row)
			},
		}
	}

	unorderedCalls := 0
	unordered := &uilayout.ListTableState{SortColumn: "Name"}
	if got := renderListModel(Model{}, newModel(unordered, &unorderedCalls), focusMain, 80, 12); len(got) == 0 {
		t.Fatal("unordered render returned no lines")
	}
	if unorderedCalls < rows {
		t.Fatalf("unordered render made %d Cell calls, want at least %d for sort-key precompute", unorderedCalls, rows)
	}

	orderedCalls := 0
	ordered := &uilayout.ListTableState{SortColumn: "Name", RowsOrdered: true}
	if got := renderListModel(Model{}, newModel(ordered, &orderedCalls), focusMain, 80, 12); len(got) == 0 {
		t.Fatal("ordered render returned no lines")
	}
	if orderedCalls >= rows {
		t.Fatalf("RowsOrdered render made %d Cell calls, want far fewer than %d", orderedCalls, rows)
	}
}

func TestInstallListViewOrderMarksRowsOrdered(t *testing.T) {
	lv := resource.ListView[string]{}
	lv.Set([]string{"b", "a"})
	state := &uilayout.ListTableState{SortColumn: "Name"}
	cols := []uilayout.ListColumn{{Name: "Name"}}

	installListViewOrder(&lv, state, cols, func(s string, col int) string { return s })

	if !state.RowsOrdered {
		t.Fatal("RowsOrdered = false after installing active ListView sort")
	}
	got := lv.Filtered()
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("Filtered() = %v, want [a b]", got)
	}

	state.SortColumn = ""
	installListViewOrder(&lv, state, cols, func(s string, col int) string { return s })
	if state.RowsOrdered {
		t.Fatal("RowsOrdered = true after clearing sort")
	}
}
