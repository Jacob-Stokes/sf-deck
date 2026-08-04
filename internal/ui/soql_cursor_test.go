package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestSOQLTableAdapterMovesInSortedDisplaySpace(t *testing.T) {
	records := []map[string]any{
		{"Id": "001C", "Name": "Charlie"},
		{"Id": "001A", "Name": "Alpha"},
		{"Id": "001B", "Name": "Bravo"},
	}
	entry := soqlProjectionFor(nil, records, nil, theme.Current.ID, "")
	var m Model
	m.soqlRowCur = 2
	m.soqlTable = uilayoutListTableStateForTest("Name")

	adapter := soqlTableAdapter(&m, entry)
	if got, want := adapter.DisplayCursor(), DisplayRow(1); got != want {
		t.Fatalf("DisplayCursor = %d, want %d", got, want)
	}

	adapter.MoveDisplay(-1)
	if got, want := m.soqlRowCur, RawRow(1); got != want {
		t.Fatalf("MoveDisplay(-1) stored raw %d, want %d", got, want)
	}

	m.soqlRowCur = 0
	soqlTableAdapter(&m, entry).ResetDisplayTop()
	if got, want := m.soqlRowCur, RawRow(1); got != want {
		t.Fatalf("ResetDisplayTop stored raw %d, want %d", got, want)
	}
}

func TestSOQLTableAdapterHonorsFilteredRawIndexes(t *testing.T) {
	records := []map[string]any{
		{"Id": "001X", "Name": "Skip"},
		{"Id": "001B", "Name": "Keep Bravo"},
		{"Id": "001A", "Name": "Keep Alpha"},
	}
	search := &searchState{}
	search.SetBuffer("keep")
	search.Committed = true
	entry := soqlProjectionFor(nil, records, search, theme.Current.ID, "")
	var m Model
	m.soqlRowCur = 1
	m.soqlTable = uilayoutListTableStateForTest("Name")
	m.soqlSearch = search
	m.soqlResult.Records = records

	adapter := soqlTableAdapter(&m, entry)
	if got, want := adapter.DisplayCursor(), DisplayRow(1); got != want {
		t.Fatalf("DisplayCursor = %d, want %d", got, want)
	}

	adapter.MoveDisplay(-1)
	if got, want := m.soqlRowCur, RawRow(2); got != want {
		t.Fatalf("MoveDisplay(-1) stored raw %d, want %d", got, want)
	}
	rec, ok := m.soqlSelectedRecord()
	if !ok || rec["Name"] != "Keep Alpha" {
		t.Fatalf("selected record = %v/%v, want Keep Alpha/true", rec, ok)
	}
}

func uilayoutListTableStateForTest(sortColumn string) uilayout.ListTableState {
	return uilayout.ListTableState{SortColumn: sortColumn}
}
