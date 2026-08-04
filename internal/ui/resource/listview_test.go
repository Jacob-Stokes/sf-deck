package resource

import "testing"

func TestSearchStateDebounceLifecycle(t *testing.T) {
	var search SearchState
	if got := search.Buffer(); got != "" {
		t.Fatalf("zero-value Buffer() = %q, want empty", got)
	}

	search.SetBuffer("account")
	search.Active = true
	if !search.Applied() {
		t.Fatal("non-empty active search should be applied")
	}
	if got := search.Effective(); got != "account" {
		t.Fatalf("Effective() = %q, want account", got)
	}

	search.SetLastFilterDurationMs(50)
	if got := search.LastFilterDurationMs(); got != 50 {
		t.Fatalf("LastFilterDurationMs() = %d, want 50", got)
	}
	search.Input.SetValue("account owner")
	search.NoteBufferChanged(10)
	if !search.DebouncePending() {
		t.Fatal("slow filter should leave a debounce pending")
	}
	if got := search.Effective(); got != "account" {
		t.Fatalf("pending Effective() = %q, want previous value", got)
	}

	search.SyncEffective()
	if search.DebouncePending() {
		t.Fatal("SyncEffective should clear pending debounce")
	}
	if got := search.Effective(); got != "account owner" {
		t.Fatalf("synced Effective() = %q, want account owner", got)
	}
}

func TestListViewSetCursorClampsOverflowToLastRow(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{10, 20, 30})

	lv.SetCursor(99)

	if got, want := lv.Cursor(), 2; got != want {
		t.Fatalf("Cursor() = %d, want %d", got, want)
	}
}

// TestListViewSetSameSliceKeepsCursor reproduces the /objects/schema
// "cursor up/down does nothing" bug: syncFieldList re-passes the SAME
// describe field slice to Set() on every render and every cursor move.
// Set must treat an identical backing slice as a no-op so it doesn't
// reset the cursor a frame after MoveBy advanced it.
func TestListViewSetSameSliceKeepsCursor(t *testing.T) {
	fields := []int{10, 20, 30, 40}
	lv := ListView[int]{}
	lv.Set(fields)

	lv.MoveBy(2) // cursor → 2
	if got := lv.Cursor(); got != 2 {
		t.Fatalf("after MoveBy(2) Cursor() = %d, want 2", got)
	}

	// Re-Set the same slice (what a render pass does). Cursor must hold.
	lv.Set(fields)
	if got := lv.Cursor(); got != 2 {
		t.Fatalf("after re-Set(same slice) Cursor() = %d, want 2 (cursor wiped)", got)
	}

	// A genuinely new slice still resets — Set's documented behaviour.
	lv.Set([]int{1, 2, 3, 4})
	if got := lv.Cursor(); got != 0 {
		t.Fatalf("after Set(new slice) Cursor() = %d, want 0", got)
	}
}

func TestListViewSetCursorClampsEmptyListToZero(t *testing.T) {
	lv := ListView[int]{}

	lv.SetCursor(99)

	if got := lv.Cursor(); got != 0 {
		t.Fatalf("Cursor() = %d, want 0", got)
	}
}

func TestListViewOrderSelectedUsesDisplayOrder(t *testing.T) {
	lv := ListView[string]{}
	lv.Set([]string{"b", "c", "a"})
	lv.SetOrder(func(items []string) []int {
		return []int{2, 0, 1}
	}, "name:asc")

	got := lv.Filtered()
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Filtered()[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
	if lv.items[0] != "b" || lv.items[1] != "c" || lv.items[2] != "a" {
		t.Fatalf("items mutated: %v", lv.items)
	}
	lv.SetCursor(1)
	selected, ok := lv.Selected()
	if !ok || selected != "b" {
		t.Fatalf("Selected() = %q, %v; want b,true", selected, ok)
	}
}

func TestListViewOrderKeyInvalidatesCache(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3})
	lv.SetOrder(func(items []int) []int { return []int{2, 1, 0} }, "desc")
	if got := lv.Filtered(); got[0] != 3 {
		t.Fatalf("desc Filtered() = %v", got)
	}
	lv.SetOrder(func(items []int) []int { return []int{0, 1, 2} }, "asc")
	if got := lv.Filtered(); got[0] != 1 {
		t.Fatalf("asc Filtered() = %v", got)
	}
}

func TestListViewSetOrderDoesNotBumpDataVersion(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3})
	before := lv.Version()

	lv.SetOrder(func(items []int) []int { return []int{2, 1, 0} }, "desc")

	if got := lv.Version(); got != before {
		t.Fatalf("Version() = %d after SetOrder, want %d", got, before)
	}
}

func TestListViewMalformedOrderFallsBackToIdentity(t *testing.T) {
	lv := ListView[int]{}
	lv.Set([]int{1, 2, 3})
	lv.SetOrder(func(items []int) []int { return []int{0, 0, 2} }, "bad")
	got := lv.Filtered()
	want := []int{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Filtered()[%d] = %d, want %d (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestListViewOrderAppliesAfterRelevance(t *testing.T) {
	lv := ListView[string]{}
	lv.Set([]string{"a", "ccc", "bb"})
	lv.SetMatch(func(s, q string) bool { return true })
	lv.SetScorer(func(s, q string) int { return len(s) })
	lv.Search.SetBuffer("x")
	lv.Search.Active = true
	lv.SetOrder(func(items []string) []int {
		// Relevance would produce ccc, bb, a. This order then puts
		// the shortest displayed row first.
		return []int{2, 1, 0}
	}, "short-first")
	got := lv.Filtered()
	want := []string{"a", "bb", "ccc"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Filtered()[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}
