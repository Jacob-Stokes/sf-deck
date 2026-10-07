package ui

// Shared scaffolding for orgData's "ensure a Resource exists for this
// key" pattern. The 16+ EnsureX methods on orgData all follow the same
// shape:

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
