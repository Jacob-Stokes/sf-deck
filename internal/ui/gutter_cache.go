package ui

// Bulk tag/project gutter cache.
//
// `BuildRenderModel` runs every wheel tick and every keypress; many
// surfaces fetch their tag + project gutters via bulkXxxFor helpers
// that hit SQLite + allocate a wanted-set the size of the items
// slice. On a 5000-row list that's two queries + 10000 allocations
// per tick — visible scroll lag.
//
// This cache memoises the result on *orgData. Lookups are O(1) once
// the cache is warm. Invalidation is automatic:
//
//   - items pointer change (Set on the wrapping ListView replaced
//     the slice header) → cache miss, rebuild
//   - devproject.Store.Generation() advanced (a tag was applied/
//     removed, item was collected/uncollected, project was
//     created/deleted) → cache miss, rebuild
//
// Both checks are fast: pointer compare + int compare. Mismatches
// fall through to the underlying bulk fetch as before.

import (
	"reflect"
	"unsafe"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// gutterDomainSObject and friends are the cache discriminators —
// each surface's items have a different element type, so we key the
// cache by domain to avoid collisions when different surfaces show
// the same number of items by coincidence.
const (
	gutterDomainSObject     = "sobject"
	gutterDomainFlow        = "flow"
	gutterDomainApexClass   = "apex_class"
	gutterDomainApexTrigger = "apex_trigger"
	gutterDomainLWC         = "lwc"
	gutterDomainAura        = "aura"
	gutterDomainRecord      = "record"
)

// ensureGutterCache lazily allocates the cache state on first use.
func (d *orgData) ensureGutterCache() *gutterCacheState {
	if d.gutterCache == nil {
		d.gutterCache = &gutterCacheState{
			tags:     map[string]gutterEntry[map[string][]devproject.Tag]{},
			projects: map[string]gutterEntry[map[string][]devproject.DevProject]{},
		}
	}
	return d.gutterCache
}

// slicePtr returns the header pointer of a generic slice. Combined
// with the slice's len, it uniquely identifies the underlying array
// — which is what we want as a cheap "did items get replaced" check.
// reflect.SliceHeader is deprecated for new code; reflect.Value's
// Pointer() does the same job without the unsafe-pointer-conversion
// lint warnings.
func slicePtr[T any](s []T) uintptr {
	if len(s) == 0 {
		// nil/empty slices have nondeterministic header data; treat
		// them all as one cache slot. Caller still bumps generation
		// when the underlying store changes, which forces a rebuild.
		return 0
	}
	return reflect.ValueOf(s).Pointer()
}

// memoTagsFor checks the cache for a bulk tag map matching the given
// domain + items slice pointer + store generation, returning the
// cached map on hit. On miss it runs `fetch` and stores the result.
//
// fetch is the original bulkXxx helper — closure form so each surface
// keeps its existing key-build + store-call logic, the cache layer is
// purely additive.
func (d *orgData) memoTagsFor(
	store *devproject.Store,
	domain string,
	itemsPtr uintptr,
	fetch func() map[string][]devproject.Tag,
) map[string][]devproject.Tag {
	gen := store.Generation()
	cache := d.ensureGutterCache()
	if entry, ok := cache.tags[domain]; ok {
		if entry.itemsPtr == itemsPtr && entry.generation == gen {
			return entry.value
		}
	}
	v := fetch()
	cache.tags[domain] = gutterEntry[map[string][]devproject.Tag]{
		itemsPtr:   itemsPtr,
		generation: gen,
		value:      v,
	}
	return v
}

// bulkTagsForItems is the shared body behind the per-domain
// bulkTagsFor* helpers whose rows are a simple []T keyed by a single
// (kind, id). It preserves the exact memo keying — slicePtr(items) +
// domain + store generation — that the per-frame gutter render depends
// on, so callers stay off the hot path. Returns nil (empty gutter) when
// the store is unavailable, the tag column is hidden, no org is active,
// or the list is empty. Bundles/records keep bespoke functions because
// their row shapes differ (two slices / map rows).
func bulkTagsForItems[T any](m Model, items []T, domain string, kind devproject.ItemKind, idOf func(T) string) map[string][]devproject.Tag {
	if m.devProjects == nil || len(items) == 0 || !m.settings.TagColumnVisible() {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	return d.memoTagsFor(m.devProjects, domain, slicePtr(items), func() map[string][]devproject.Tag {
		keys := make([]devproject.TagLookupKey, 0, len(items))
		for _, it := range items {
			keys = append(keys, devproject.TagLookupKey{Kind: kind, Ref: idOf(it)})
		}
		out, err := m.devProjects.TagsForItems(o.Username, keys)
		if err != nil {
			// The gutter renders empty on failure (reasonable — it's an
			// annotation, not the data), but a silent nil hid store
			// breakage entirely. Log so "tags vanished" is diagnosable.
			applog.Warn("gutter.tags_fetch_failed", map[string]any{
				"domain": domain, "err": err.Error(),
			})
			return nil
		}
		return out
	})
}

// bulkProjectsForItems is bulkTagsForItems for project membership.
func bulkProjectsForItems[T any](m Model, items []T, domain string, kind devproject.ItemKind, idOf func(T) string) map[string][]devproject.DevProject {
	if m.devProjects == nil || len(items) == 0 || !m.settings.ProjectColumnVisible() {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	return d.memoProjectsFor(m.devProjects, domain, slicePtr(items), func() map[string][]devproject.DevProject {
		keys := make([]devproject.TagLookupKey, 0, len(items))
		for _, it := range items {
			keys = append(keys, devproject.TagLookupKey{Kind: kind, Ref: idOf(it)})
		}
		out, err := m.devProjects.ProjectsForItems(o.Username, keys)
		if err != nil {
			applog.Warn("gutter.projects_fetch_failed", map[string]any{
				"domain": domain, "err": err.Error(),
			})
			return nil
		}
		return out
	})
}

// memoProjectsFor mirrors memoTagsFor for the project-membership map.
func (d *orgData) memoProjectsFor(
	store *devproject.Store,
	domain string,
	itemsPtr uintptr,
	fetch func() map[string][]devproject.DevProject,
) map[string][]devproject.DevProject {
	gen := store.Generation()
	cache := d.ensureGutterCache()
	if entry, ok := cache.projects[domain]; ok {
		if entry.itemsPtr == itemsPtr && entry.generation == gen {
			return entry.value
		}
	}
	v := fetch()
	cache.projects[domain] = gutterEntry[map[string][]devproject.DevProject]{
		itemsPtr:   itemsPtr,
		generation: gen,
		value:      v,
	}
	return v
}

// _ keeps unsafe imported in case future variants need a more
// targeted slice-header read; reflect.Value.Pointer() covers the
// current callers without it.
var _ = unsafe.Sizeof(0)
