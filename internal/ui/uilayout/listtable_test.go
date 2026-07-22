package uilayout

import "testing"

func TestSortedIndicesNoSortUsesIdentityWithoutAllocation(t *testing.T) {
	spec := ListTableSpec{
		Cols: []ListColumn{{Name: "Name"}},
		N:    3,
		Cell: func(row, col int) string { return "" },
	}

	if got := SortedIndices(spec, &ListTableState{}); got != nil {
		t.Fatalf("SortedIndices without active sort = %#v, want nil identity sentinel", got)
	}
}

func TestSortedIndicesSortsWhenColumnActive(t *testing.T) {
	values := []string{"b", "c", "a"}
	spec := ListTableSpec{
		Cols: []ListColumn{{Name: "Name"}},
		N:    len(values),
		Cell: func(row, col int) string { return values[row] },
	}

	got := SortedIndices(spec, &ListTableState{SortColumn: "Name"})
	want := []int{2, 0, 1}
	if len(got) != len(want) {
		t.Fatalf("len(SortedIndices) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortedIndices[%d] = %d, want %d (full=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestSortedIndicesCachesWhenKeyStable(t *testing.T) {
	values := []string{"b", "c", "a"}
	calls := 0
	spec := ListTableSpec{
		Cols:         []ListColumn{{Name: "Name"}},
		N:            len(values),
		SortCacheKey: "v1",
		Cell: func(row, col int) string {
			calls++
			return values[row]
		},
	}
	state := &ListTableState{SortColumn: "Name"}

	first := SortedIndices(spec, state)
	if calls != len(values) {
		t.Fatalf("first sort Cell calls = %d, want %d", calls, len(values))
	}
	second := SortedIndices(spec, state)
	if calls != len(values) {
		t.Fatalf("cached sort made extra Cell calls: %d", calls)
	}
	if len(first) != len(second) || first[0] != second[0] {
		t.Fatalf("cached permutation changed: first=%v second=%v", first, second)
	}

	spec.SortCacheKey = "v2"
	_ = SortedIndices(spec, state)
	if calls != len(values)*2 {
		t.Fatalf("changed cache key calls = %d, want %d", calls, len(values)*2)
	}
}
