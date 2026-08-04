package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestTableRowAdapterMovesInDisplaySpaceAndStoresRaw(t *testing.T) {
	values := []string{"c", "a", "b"}
	visibleToRaw := []int{10, 11, 12}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}
	raw := RawRow(12)
	adapter := tableRowAdapter{
		State:        state,
		Cols:         cols,
		N:            len(values),
		Cell:         func(row, col int) string { return values[row] },
		VisibleToRaw: visibleToRaw,
		DataKey:      "records:v1",
		RawCursor:    func() RawRow { return raw },
		SetRawCursor: func(next RawRow) { raw = next },
	}

	if got, want := adapter.DisplayCursor(), DisplayRow(1); got != want {
		t.Fatalf("DisplayCursor = %d, want %d", got, want)
	}

	adapter.MoveDisplay(-1)
	if got, want := raw, RawRow(11); got != want {
		t.Fatalf("MoveDisplay(-1) stored raw %d, want %d", got, want)
	}

	raw = 10
	adapter.ResetDisplayTop()
	if got, want := raw, RawRow(11); got != want {
		t.Fatalf("ResetDisplayTop stored raw %d, want %d", got, want)
	}
}

func TestTableRowAdapterMapsDisplayToVisibleAndRaw(t *testing.T) {
	values := []string{"c", "a", "b"}
	visibleToRaw := []int{10, 11, 12}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}
	adapter := tableRowAdapter{
		State:        state,
		Cols:         cols,
		N:            len(values),
		Cell:         func(row, col int) string { return values[row] },
		VisibleToRaw: visibleToRaw,
		DataKey:      "records:v1",
	}

	if got, ok := adapter.VisibleAtDisplay(0); !ok || got != 1 {
		t.Fatalf("VisibleAtDisplay(0) = %d/%v, want 1/true", got, ok)
	}
	if got, ok := adapter.RawAtDisplay(0); !ok || got != 11 {
		t.Fatalf("RawAtDisplay(0) = %d/%v, want 11/true", got, ok)
	}
}
