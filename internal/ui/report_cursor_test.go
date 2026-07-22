package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestReportRunTableAdapterMovesInSortedDisplaySpace(t *testing.T) {
	rows := []map[string]any{
		{"Id": "003C", "Name": "Charlie"},
		{"Id": "003A", "Name": "Alpha"},
		{"Id": "003B", "Name": "Bravo"},
	}
	cols := buildReportRunCols([]sf.ReportColumn{
		{APIName: "Id", Label: "Id"},
		{APIName: "Name", Label: "Name"},
	}, rows)
	cell := reportRunCell(rows, cols)
	reportID := "00Oreport"

	var m Model
	m.reportRunTable = uilayout.ListTableState{SortColumn: "Name"}
	d := &orgData{}
	d.Cursors = NewCursorStore()
	d.Cursors.Set(cursorKindReportRow, 2, len(rows), reportID)

	adapter := reportRunTableAdapter(&m, d, reportID, 123, rows, cols, cell)
	if got, want := adapter.DisplayCursor(), DisplayRow(1); got != want {
		t.Fatalf("DisplayCursor = %d, want %d", got, want)
	}

	adapter.MoveDisplay(-1)
	if got, want := d.Cursors.Get(cursorKindReportRow, len(rows), reportID), 1; got != want {
		t.Fatalf("MoveDisplay(-1) stored raw %d, want %d", got, want)
	}

	d.Cursors.Set(cursorKindReportRow, 0, len(rows), reportID)
	adapter.ResetDisplayTop()
	if got, want := d.Cursors.Get(cursorKindReportRow, len(rows), reportID), 1; got != want {
		t.Fatalf("ResetDisplayTop stored raw %d, want %d", got, want)
	}
}

func TestReportRunTableAdapterRawAtDisplay(t *testing.T) {
	rows := []map[string]any{
		{"Id": "003C", "Name": "Charlie"},
		{"Id": "003A", "Name": "Alpha"},
		{"Id": "003B", "Name": "Bravo"},
	}
	cols := buildReportRunCols([]sf.ReportColumn{
		{APIName: "Id", Label: "Id"},
		{APIName: "Name", Label: "Name"},
	}, rows)
	cell := reportRunCell(rows, cols)

	var m Model
	m.reportRunTable = uilayout.ListTableState{SortColumn: "Name"}
	adapter := reportRunTableAdapter(&m, nil, "00Oreport", 123, rows, cols, cell)

	if got, ok := adapter.RawAtDisplay(0); !ok || got != 1 {
		t.Fatalf("RawAtDisplay(0) = %d/%v, want 1/true", got, ok)
	}
}
