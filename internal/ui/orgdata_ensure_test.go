package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestEnsureKeyed_BuildOnceOnly is the regression guard for the
// invariant that backs EnsureX's "switching back to a chip reuses the
// prior fetch" UX: build() must run exactly once per missing key, and
// subsequent calls must return the same *Resource[T] pointer.
func TestEnsureKeyed_BuildOnceOnly(t *testing.T) {
	var m map[string]*Resource[sf.RecordsList]
	calls := 0
	build := func() *Resource[sf.RecordsList] {
		calls++
		return &Resource[sf.RecordsList]{Key: "rec:Account"}
	}

	first := ensureKeyed(&m, "Account", build)
	second := ensureKeyed(&m, "Account", build)

	if first != second {
		t.Errorf("expected same *Resource pointer on second call, got %p vs %p", first, second)
	}
	if calls != 1 {
		t.Errorf("build called %d times, want 1", calls)
	}
	if m == nil {
		t.Fatal("nil map should have been allocated")
	}
	if len(m) != 1 {
		t.Errorf("map has %d entries, want 1", len(m))
	}
}

// TestEnsureKeyed_NilMapAllocates confirms that callers that haven't
// initialised their map (e.g. EnsureChipUsers before today's refactor)
// no longer need a manual lazy-init dance.
func TestEnsureKeyed_NilMapAllocates(t *testing.T) {
	var m map[string]*Resource[sf.RecordsList]
	if m != nil {
		t.Fatal("preconditions: map should be nil")
	}
	r := ensureKeyed(&m, "x", func() *Resource[sf.RecordsList] {
		return &Resource[sf.RecordsList]{}
	})
	if r == nil {
		t.Fatal("got nil Resource")
	}
	if m == nil {
		t.Fatal("map should have been allocated")
	}
}
