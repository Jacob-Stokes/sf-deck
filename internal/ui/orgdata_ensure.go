package ui

// Shared scaffolding for orgData's "ensure a Resource exists for this
// key" pattern. The 16+ EnsureX methods on orgData all follow the same
// shape:
//
//   1. Check the per-key map for an existing Resource — return it.
//   2. Otherwise build a fresh Resource (TTL, NoCache, Fetch closure).
//   3. Store it in the map.
//   4. Return the new Resource.
//
// Before this helper, every EnsureX inlined steps 1, 3, 4 manually,
// leaving only the build closure as the meaningful per-call code. The
// scaffolding read as ~40% of model.go's bulk for the EnsureX section.
//
// ensureKeyed collapses 1, 3, 4 into a single call. Callers supply the
// map, the key, and a build func that produces the fresh Resource; the
// helper handles lookup + insertion.

// ensureKeyed returns the Resource at (*mp)[key], creating it via
// build() when absent. The map pointer is taken so the helper can
// allocate the map when it's nil (rather than only some EnsureX
// methods doing the lazy-init dance manually).
//
// Generic over T (the Resource's payload type). The build callback is
// invoked exactly once per missing key — subsequent calls return the
// stored *Resource[T] without re-invoking build.
//
// Usage:
//
//	r := ensureKeyed(&d.Records, sobject, func() *Resource[sf.RecordsList] {
//	    return &Resource[sf.RecordsList]{
//	        Scope: d.username,
//	        Key:   "records:" + sobject,
//	        TTL:   d.ttl("records", 24*time.Hour),
//	        Fetch: ...,
//	    }
//	})
func ensureKeyed[T any](mp *map[string]*Resource[T], key string, build func() *Resource[T]) *Resource[T] {
	if *mp == nil {
		*mp = map[string]*Resource[T]{}
	}
	if r, ok := (*mp)[key]; ok {
		return r
	}
	r := build()
	(*mp)[key] = r
	return r
}
