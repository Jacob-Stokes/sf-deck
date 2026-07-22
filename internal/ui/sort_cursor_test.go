package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestSortCursorTranslatesBetweenDisplayAndData(t *testing.T) {
	values := []string{"b", "c", "a"}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}
	cell := func(row, col int) string { return values[row] }
	space := identityRowSpace(state, cols, len(values), cell, "rows:v1")

	if got, want := space.VisibleToDisplay(2), DisplayRow(0); got != want {
		t.Fatalf("VisibleToDisplay(2) = %d, want %d", got, want)
	}
	if got, want := space.DisplayToVisible(0), VisibleRow(2); got != want {
		t.Fatalf("DisplayToVisible(0) = %d, want %d", got, want)
	}
}

func TestSortCursorNoSortUsesIdentity(t *testing.T) {
	values := []string{"b", "c", "a"}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{}
	cell := func(row, col int) string { return values[row] }
	space := identityRowSpace(state, cols, len(values), cell, "rows:v1")

	if got, want := space.VisibleToDisplay(2), DisplayRow(2); got != want {
		t.Fatalf("VisibleToDisplay without sort = %d, want %d", got, want)
	}
	if got, want := space.DisplayToVisible(0), VisibleRow(0); got != want {
		t.Fatalf("DisplayToVisible without sort = %d, want %d", got, want)
	}
}

func TestRowSpaceTranslatesRawVisibleAndDisplay(t *testing.T) {
	values := []string{"c", "a", "b"}
	visibleToRaw := []int{3, 5, 8}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}
	cell := func(row, col int) string { return values[row] }

	space := newRowSpace(visibleToRaw, state, cols, len(values), cell, "rows:v1")
	if got, want := space.DisplayToRaw(0), RawRow(5); got != want {
		t.Fatalf("DisplayToRaw(0) = %d, want %d", got, want)
	}
	if got, want := space.RawToDisplay(8), DisplayRow(1); got != want {
		t.Fatalf("RawToDisplay(8) = %d, want %d", got, want)
	}
	if got, want := space.RawToNearestVisible(6), VisibleRow(2); got != want {
		t.Fatalf("RawToNearestVisible(6) = %d, want %d", got, want)
	}
}

func TestRowSpacePermutedRawRowsSnapMissingRawToTop(t *testing.T) {
	space := newRowSpace([]int{8, 3, 5}, nil, nil, 3, nil, "recency:v1")
	if got, want := space.RawToNearestVisible(6), VisibleRow(0); got != want {
		t.Fatalf("RawToNearestVisible on permuted rows = %d, want %d", got, want)
	}
	if got, want := space.RawToDisplay(5), DisplayRow(2); got != want {
		t.Fatalf("RawToDisplay exact match on permuted rows = %d, want %d", got, want)
	}
}

func TestSortCursorCacheInvalidatesOnDataKey(t *testing.T) {
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}

	first := []string{"b", "a"}
	firstCell := func(row, col int) string { return first[row] }
	firstSpace := identityRowSpace(state, cols, len(first), firstCell, "rows:first")
	if got, want := firstSpace.DisplayToRaw(0), RawRow(1); got != want {
		t.Fatalf("first DisplayToRaw(0) = %d, want %d", got, want)
	}

	second := []string{"a", "b"}
	secondCell := func(row, col int) string { return second[row] }
	secondSpace := identityRowSpace(state, cols, len(second), secondCell, "rows:second")
	if got, want := secondSpace.DisplayToRaw(0), RawRow(0); got != want {
		t.Fatalf("second DisplayToRaw(0) = %d, want %d", got, want)
	}
}

// TestSortCursorCachesPermutation regression-tests the perf fix:
// every j/k on sorted /records used to rebuild the sort permutation
// twice (once display→data, once back).  cursorSortCacheKey now
// threads a stable key through, so the second call hits the cache
// on ListTableState.  Count cell-extractor invocations to verify.
func TestSortCursorCachesPermutation(t *testing.T) {
	values := []string{"b", "c", "a", "d", "e", "f"}
	cols := []uilayout.ListColumn{{Name: "Name"}}
	state := &uilayout.ListTableState{SortColumn: "Name"}
	cellCalls := 0
	cell := func(row, col int) string {
		cellCalls++
		return values[row]
	}

	// First call populates the cache.
	_ = identityRowSpace(state, cols, len(values), cell, "rows:v1").VisibleToDisplay(2)
	first := cellCalls

	// Second call (the recordsMoveCursor pattern: translate
	// display→data, then later data→display) must NOT re-invoke
	// cell.  Each invocation costs O(N) cell extractions on a real
	// surface where cell() does work; the cache check should
	// short-circuit before any cell call.
	_ = identityRowSpace(state, cols, len(values), cell, "rows:v1").DisplayToVisible(0)
	if cellCalls != first {
		t.Errorf("second call re-invoked cell %d times; want cache hit (0 additional)",
			cellCalls-first)
	}

	// Third call (e.g. drill reads the cursor row): still cached.
	_ = identityRowSpace(state, cols, len(values), cell, "rows:v1").VisibleToDisplay(0)
	if cellCalls != first {
		t.Errorf("third call re-invoked cell %d times; want cache hit",
			cellCalls-first)
	}
}
