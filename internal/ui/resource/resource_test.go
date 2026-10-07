package resource

import (
	"testing"
	"time"
)

func TestFetchWithExistingUsesUpdateLoopSnapshot(t *testing.T) {
	r := Resource[int]{
		Scope:   "scope",
		Key:     "key",
		NoCache: true,
		FetchWithExisting: func(existing int) (int, error) {
			return existing + 1, nil
		},
	}
	r.Set(10)
	cmd := r.Refresh(nil)
	if cmd == nil {
		t.Fatal("Refresh returned nil command")
	}

	r.Set(20)
	raw := cmd()
	msg, ok := raw.(UpdatedMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want UpdatedMsg", raw)
	}
	got := *(msg.Payload.(*int))
	if got != 11 {
		t.Fatalf("fetch saw %d, want snapshot-derived 11", got)
	}
}

func TestEnsureColdCacheableResourceSingleFlightsCacheLoad(t *testing.T) {
	r := Resource[int]{
		Scope: "scope",
		Key:   "key",
		TTL:   time.Minute,
		Fetch: func() (int, error) { return 1, nil },
	}

	first := r.Ensure(nil)
	if first == nil {
		t.Fatal("first Ensure returned nil command")
	}
	if !r.Busy() {
		t.Fatal("resource should be busy while cache load is in flight")
	}
	if second := r.Ensure(nil); second != nil {
		t.Fatal("second Ensure returned a duplicate cache-load command")
	}

	if !r.Apply(UpdatedMsg{Scope: "scope", Key: "key", FromCache: true}) {
		t.Fatal("cache-load message was not applied")
	}
	if r.Busy() {
		t.Fatal("resource should not remain busy after cache load applies")
	}
}

func TestCacheLoadApplyClearsBusyAndAllowsRefreshWhenStale(t *testing.T) {
	r := Resource[int]{
		Scope: "scope",
		Key:   "key",
		TTL:   time.Minute,
		Fetch: func() (int, error) { return 1, nil },
	}

	if cmd := r.Ensure(nil); cmd == nil {
		t.Fatal("Ensure returned nil command")
	}
	r.Apply(UpdatedMsg{Scope: "scope", Key: "key", FromCache: true})

	if cmd := r.MaybeRefreshAfterCacheLoad(nil); cmd == nil {
		t.Fatal("expected stale cache miss to schedule refresh")
	}
	if !r.Busy() {
		t.Fatal("resource should be busy while refresh is in flight")
	}
}
