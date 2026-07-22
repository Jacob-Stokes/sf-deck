package uilayout

import "testing"

func TestPageSizeFor(t *testing.T) {
	cases := []struct {
		budget, n, want int
	}{
		{20, 100, 20}, // budget fits within data → use budget
		{20, 5, 5},    // tiny dataset → cap at n
		{2, 100, 5},   // tiny budget → floor at minViewportRows
		{0, 100, 5},   // zero budget → floor
		{50, 0, 50},   // n=0 → leave budget alone (TotalPages handles emptiness)
	}
	for _, tc := range cases {
		got := PageSizeFor(tc.budget, tc.n)
		if got != tc.want {
			t.Errorf("PageSizeFor(budget=%d, n=%d) = %d, want %d", tc.budget, tc.n, got, tc.want)
		}
	}
}

func TestTotalPages(t *testing.T) {
	cases := []struct {
		n, pageSize, want int
	}{
		{0, 50, 0},
		{50, 50, 1},
		{51, 50, 2},
		{100, 50, 2},
		{101, 50, 3},
		{10, 0, 1}, // pageSize 0 → defensive 1 page
		{10, -1, 1},
	}
	for _, tc := range cases {
		got := TotalPages(tc.n, tc.pageSize)
		if got != tc.want {
			t.Errorf("TotalPages(n=%d, pageSize=%d) = %d, want %d", tc.n, tc.pageSize, got, tc.want)
		}
	}
}

func TestSetPage_Clamps(t *testing.T) {
	s := ListTableState{}
	s.SetPage(5, 3)
	if s.Page != 2 {
		t.Errorf("SetPage(5, 3) clamp: got %d, want 2", s.Page)
	}
	s.SetPage(-1, 3)
	if s.Page != 0 {
		t.Errorf("SetPage(-1, 3) clamp: got %d, want 0", s.Page)
	}
	s.SetPage(7, 0)
	if s.Page != 0 {
		t.Errorf("SetPage(7, 0) → page 0 for empty totalPages: got %d", s.Page)
	}
}

func TestRenderRowsPaged_HappyPath(t *testing.T) {
	rows, total := RenderRowsPaged(25, 0, 1, 10, 80, func(i int) string {
		return "row " + string(rune('A'+i))
	})
	// 25 rows, pageSize 10 → 3 pages. Page 1 → rows 10..19 + indicator.
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	// 10 rows + 1 indicator
	if len(rows) != 11 {
		t.Errorf("rows = %d (want 11): %#v", len(rows), rows)
	}
	if rows[0] != "row K" { // 'A' + 10
		t.Errorf("first row of page 1 = %q, want 'row K'", rows[0])
	}
}

func TestRenderRowsPaged_LastPagePartial(t *testing.T) {
	// 25 rows, pageSize 10, page 2 → rows 20..24 (5 rows) + indicator
	rows, total := RenderRowsPaged(25, 0, 2, 10, 80, func(i int) string {
		return "row"
	})
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 6 {
		t.Errorf("partial last page len = %d, want 6 (5 rows + indicator)", len(rows))
	}
}

func TestRenderRowsPaged_PageOutOfRange(t *testing.T) {
	// page = 99 → clamped to last (page 2 of 0..2)
	rows, _ := RenderRowsPaged(25, 0, 99, 10, 80, func(i int) string { return "x" })
	if len(rows) != 6 { // last page is 5 rows + indicator
		t.Errorf("out-of-range page should clamp to last; got %d rows", len(rows))
	}
}

func TestRenderRowsPaged_EmptyN(t *testing.T) {
	rows, total := RenderRowsPaged(0, 0, 0, 10, 80, func(i int) string { return "x" })
	if total != 0 || rows != nil {
		t.Errorf("n=0 should return (nil, 0); got (%#v, %d)", rows, total)
	}
}
