package ui

import (
	"strconv"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// RawRow is an index in the backing data slice for a surface.
type RawRow int

// VisibleRow is an index in the filtered/default-ordered row slice.
type VisibleRow int

// DisplayRow is an index in the rendered table after column sorting.
type DisplayRow int

// RowSpace centralizes translations between the row coordinate systems
// list-table surfaces use: backing/raw rows, search/default-filtered
// visible rows, and sorted display rows.
type RowSpace struct {
	visibleToRaw []int
	displayToVis []int
	monotonicRaw bool
	identityRaw  bool
	n            int
}

func newRowSpace(
	visibleToRaw []int,
	state *uilayout.ListTableState,
	cols []uilayout.ListColumn,
	n int,
	cell func(row, col int) string,
	dataKey string,
) RowSpace {
	if n < 0 {
		n = 0
	}
	identityRaw := len(visibleToRaw) != n
	return RowSpace{
		visibleToRaw: visibleToRaw,
		displayToVis: sortPermutation(state, cols, n, cell, dataKey),
		monotonicRaw: identityRaw || intsMonotonic(visibleToRaw),
		identityRaw:  identityRaw,
		n:            n,
	}
}

func identityRowSpace(
	state *uilayout.ListTableState,
	cols []uilayout.ListColumn,
	n int,
	cell func(row, col int) string,
	dataKey string,
) RowSpace {
	return newRowSpace(nil, state, cols, n, cell, dataKey)
}

func (s RowSpace) Len() int { return s.n }

func (s RowSpace) RawToVisible(raw RawRow) (VisibleRow, bool) {
	if s.identityRaw {
		if raw < 0 || int(raw) >= s.n {
			return 0, false
		}
		return VisibleRow(raw), true
	}
	for i, r := range s.visibleToRaw {
		if r == int(raw) {
			return VisibleRow(i), true
		}
	}
	return 0, false
}

// RawToNearestVisible maps a stored raw cursor into the current visible
// row space. Search-filtered record lists keep visibleToRaw monotonic,
// so a raw cursor filtered out by search should clamp to the next raw
// row still visible. Permuted views (recency/default order) lose that
// ordering, so they require an exact match and otherwise snap to top.
func (s RowSpace) RawToNearestVisible(raw RawRow) VisibleRow {
	if s.n == 0 {
		return 0
	}
	if v, ok := s.RawToVisible(raw); ok {
		return v
	}
	if s.identityRaw {
		return VisibleRow(clampIndex(int(raw), s.n))
	}
	if !s.monotonicRaw {
		return 0
	}
	for i, r := range s.visibleToRaw {
		if r >= int(raw) {
			return VisibleRow(i)
		}
	}
	return VisibleRow(s.n - 1)
}

func (s RowSpace) VisibleToRaw(v VisibleRow) RawRow {
	if s.n == 0 {
		return 0
	}
	i := clampIndex(int(v), s.n)
	if s.identityRaw {
		return RawRow(i)
	}
	return RawRow(s.visibleToRaw[i])
}

func (s RowSpace) DisplayToVisible(d DisplayRow) VisibleRow {
	n := s.n
	if n == 0 {
		return 0
	}
	i := clampIndex(int(d), n)
	if s.displayToVis == nil || i >= len(s.displayToVis) {
		return VisibleRow(i)
	}
	v := s.displayToVis[i]
	if v < 0 || v >= n {
		return VisibleRow(i)
	}
	return VisibleRow(v)
}

func (s RowSpace) VisibleToDisplay(v VisibleRow) DisplayRow {
	n := s.n
	if n == 0 {
		return 0
	}
	idx := clampIndex(int(v), n)
	if s.displayToVis == nil {
		return DisplayRow(idx)
	}
	for display, src := range s.displayToVis {
		if src == idx {
			return DisplayRow(display)
		}
	}
	return DisplayRow(idx)
}

func (s RowSpace) DisplayToRaw(d DisplayRow) RawRow {
	return s.VisibleToRaw(s.DisplayToVisible(d))
}

func (s RowSpace) RawToDisplay(raw RawRow) DisplayRow {
	return s.VisibleToDisplay(s.RawToNearestVisible(raw))
}

func clampIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

func intsMonotonic(in []int) bool {
	for i := 1; i < len(in); i++ {
		if in[i] < in[i-1] {
			return false
		}
	}
	return true
}

// identityIdx returns [0, 1, …, n-1]. The "no filter applied" case
// uses it so callers can treat the (visible, visibleToRaw) pair
// uniformly without special-casing.
func identityIdx(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// sortPermutation returns the display→visible permutation for the
// given state + columns + cell extractor.
//
// Critical: builds the spec with a non-empty SortCacheKey derived from
// the sort fields, columns, row count, and caller-provided dataKey.
// That hits the permutation cache on ListTableState, so repeat callers
// within the same render frame reuse a single sort rather than
// re-running O(N log N). The dataKey prevents stale permutations when
// a surface swaps to different rows with the same count and sort.
func sortPermutation(state *uilayout.ListTableState, cols []uilayout.ListColumn, n int, cell func(row, col int) string, dataKey string) []int {
	return uilayout.SortedIndices(uilayout.ListTableSpec{
		Cols:         cols,
		N:            n,
		Cell:         cell,
		SortCacheKey: cursorSortCacheKey(state, cols, n, dataKey),
	}, state)
}

// cursorSortCacheKey builds a stable cache key for sort-cursor callers.
// It does not depend on per-frame display state (cursor, scroll offset,
// viewport height), but it must include the row data identity via
// dataKey because the same ListTableState can outlive a refetch, SOQL
// rerun, search change, or recency reshuffle.
func cursorSortCacheKey(state *uilayout.ListTableState, cols []uilayout.ListColumn, n int, dataKey string) string {
	if state == nil || state.SortColumn == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("cursor|")
	b.WriteString(state.SortColumn)
	if state.SortDesc {
		b.WriteString(":desc")
	} else {
		b.WriteString(":asc")
	}
	b.WriteByte('|')
	b.WriteString(dataKey)
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(n))
	for _, c := range cols {
		b.WriteByte('|')
		b.WriteString(c.Name)
	}
	return b.String()
}
