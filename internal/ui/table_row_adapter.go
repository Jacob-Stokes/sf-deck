package ui

import "github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"

// tableRowAdapter is the shared cursor adapter for table-shaped
// surfaces whose stored cursor lives in raw backing-row coordinates
// while the UI moves in display coordinates.
//
// ListView-backed surfaces do not need this: their cursor already
// lives in the filtered/display slice. Dynamic grids such as records,
// SOQL, and report runs do need it because search/default ordering and
// column sort can all change which raw row appears at display row N.
//
// The adapter speaks the typed row coordinates (RawRow / VisibleRow /
// DisplayRow) at every boundary so a caller cannot hand a display
// index to something expecting a raw index without an explicit —
// and therefore reviewable — conversion.
type tableRowAdapter struct {
	State        *uilayout.ListTableState
	Cols         []uilayout.ListColumn
	N            int
	Cell         func(row, col int) string
	VisibleToRaw []int
	DataKey      string

	RawCursor    func() RawRow
	SetRawCursor func(raw RawRow)
}

func (a tableRowAdapter) RowSpace() RowSpace {
	return newRowSpace(a.VisibleToRaw, a.State, a.Cols, a.N, a.Cell, a.DataKey)
}

func (a tableRowAdapter) Len() int {
	if a.N < 0 {
		return 0
	}
	return a.N
}

func (a tableRowAdapter) rawCursor() RawRow {
	if a.RawCursor == nil {
		return 0
	}
	return a.RawCursor()
}

func (a tableRowAdapter) DisplayCursor() DisplayRow {
	if a.Len() == 0 {
		return 0
	}
	return a.RowSpace().RawToDisplay(a.rawCursor())
}

// MoveDisplay moves the cursor by a row delta in display space. The
// delta is a distance, not a coordinate, so it stays a plain int.
func (a tableRowAdapter) MoveDisplay(delta int) {
	if a.Len() == 0 || a.SetRawCursor == nil {
		return
	}
	space := a.RowSpace()
	cur := int(space.RawToDisplay(a.rawCursor()))
	cur += delta
	cur = clampIndex(cur, space.Len())
	a.SetRawCursor(space.DisplayToRaw(DisplayRow(cur)))
}

func (a tableRowAdapter) ResetDisplayTop() {
	if a.SetRawCursor == nil {
		return
	}
	if a.Len() == 0 {
		a.SetRawCursor(0)
		return
	}
	a.SetRawCursor(a.RowSpace().DisplayToRaw(0))
}

func (a tableRowAdapter) VisibleAtDisplay(display DisplayRow) (VisibleRow, bool) {
	if a.Len() == 0 {
		return 0, false
	}
	v := a.RowSpace().DisplayToVisible(display)
	if v < 0 || int(v) >= a.Len() {
		return 0, false
	}
	return v, true
}

func (a tableRowAdapter) RawAtDisplay(display DisplayRow) (RawRow, bool) {
	if a.Len() == 0 {
		return 0, false
	}
	return a.RowSpace().DisplayToRaw(display), true
}
