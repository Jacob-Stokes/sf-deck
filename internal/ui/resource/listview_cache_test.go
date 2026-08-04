package resource

import "testing"

// TestFilteredCacheReturnsSameSliceOnRepeatCall is the headline win:
// repeated Filtered() calls without intervening mutations return the
// exact same slice header — no allocation, no re-scan. That's what
// makes per-frame call sites (MoveBy, BuildRenderModel, Selected)
// share one O(N) scan rather than re-running it 2-3 times.
func TestFilteredCacheReturnsSameSliceOnRepeatCall(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3, 4, 5})
	lv.SetExtra(func(i int) bool { return i%2 == 0 })

	a := lv.Filtered()
	b := lv.Filtered()
	if len(a) != 2 || a[0] != 2 || a[1] != 4 {
		t.Fatalf("Filtered() = %v, want [2 4]", a)
	}
	// Slice headers point at same backing array — cache hit.
	if &a[0] != &b[0] {
		t.Fatal("expected identical backing array on repeat Filtered() call (cache miss)")
	}
}

// TestFilteredCacheInvalidatesOnSet — Set replaces items, cache must
// rebuild on next Filtered() call.
func TestFilteredCacheInvalidatesOnSet(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3})
	lv.SetExtra(func(i int) bool { return i > 1 })
	first := lv.Filtered()
	if len(first) != 2 {
		t.Fatalf("first Filtered() len = %d, want 2", len(first))
	}

	lv.Set([]int{10, 20, 30})
	second := lv.Filtered()
	if len(second) != 3 {
		t.Fatalf("after Set, Filtered() len = %d, want 3", len(second))
	}
	if &first[0] == &second[0] {
		t.Fatal("expected fresh slice after Set; got cached one")
	}
}

// TestFilteredCacheInvalidatesOnSetExtra — swapping the predicate
// must rebuild.
func TestFilteredCacheInvalidatesOnSetExtra(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3, 4})
	lv.SetExtra(func(i int) bool { return i%2 == 0 }) // [2, 4]
	if got := lv.Filtered(); len(got) != 2 {
		t.Fatalf("first Filtered() = %v, want 2 rows", got)
	}
	lv.SetExtra(func(i int) bool { return i%2 == 1 }) // [1, 3]
	got := lv.Filtered()
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("after SetExtra, Filtered() = %v, want [1 3]", got)
	}
}

// TestFilteredCacheInvalidatesOnSetExtraNil — nil predicate clears
// the filter, cache must rebuild and return raw items.
func TestFilteredCacheInvalidatesOnSetExtraNil(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3, 4})
	lv.SetExtra(func(i int) bool { return i%2 == 0 })
	if got := lv.Filtered(); len(got) != 2 {
		t.Fatalf("first Filtered() = %v, want 2 rows", got)
	}
	lv.SetExtra(nil)
	got := lv.Filtered()
	if len(got) != 4 {
		t.Fatalf("after SetExtra(nil), Filtered() len = %d, want 4", len(got))
	}
}

// TestFilteredCacheInvalidatesOnSearchChange — typing in the search
// buffer must trigger a rebuild even though Set/SetExtra weren't
// called. The cache key folds in Search.Buffer().
func TestFilteredCacheInvalidatesOnSearchChange(t *testing.T) {
	lv := ListView[string]{}
	lv.Set([]string{"alpha", "beta", "gamma", "delta"})
	lv.SetMatch(func(s, q string) bool { return contains(s, q) })
	first := lv.Filtered()
	if len(first) != 4 {
		t.Fatalf("unfiltered Filtered() len = %d, want 4", len(first))
	}
	lv.Search.SetBuffer("a")
	lv.Search.Active = true
	got := lv.Filtered()
	// "a" matches alpha, beta, gamma, delta — all four.
	if len(got) != 4 {
		t.Fatalf("after search 'a', Filtered() = %v, want 4", got)
	}
	lv.Search.SetBuffer("alp")
	got = lv.Filtered()
	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("after search 'alp', Filtered() = %v, want [alpha]", got)
	}
}

// TestFilteredCacheNoExtraNoSearch — fast path returns the items
// slice directly. Cache must still mark it cached so the next call
// avoids the early-return check + reuses the same pointer.
func TestFilteredCacheNoExtraNoSearch(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3})
	a := lv.Filtered()
	b := lv.Filtered()
	if &a[0] != &b[0] {
		t.Fatal("expected same backing array on repeat call with no filter")
	}
}

// helper for the search test to avoid pulling in strings dependency
// at test time (keep the test file dependency-free).
func contains(s, q string) bool {
	for i := 0; i+len(q) <= len(s); i++ {
		if s[i:i+len(q)] == q {
			return true
		}
	}
	return false
}
