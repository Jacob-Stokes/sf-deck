package ui

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

func (d *orgData) ensureGutterCache() *gutterCacheState {
	if d.gutterCache == nil {
		d.gutterCache = &gutterCacheState{
			tags:     map[string]gutterEntry[map[string][]devproject.Tag]{},
			projects: map[string]gutterEntry[map[string][]devproject.DevProject]{},
		}
	}
	return d.gutterCache
}

func slicePtr[T any](s []T) uintptr {
	if len(s) == 0 {
		return 0
	}
	return reflect.ValueOf(s).Pointer()
}

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
			applog.Warn("gutter.tags_fetch_failed", map[string]any{
				"domain": domain, "err": err.Error(),
			})
			return nil
		}
		return out
	})
}

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

var _ = unsafe.Sizeof(0)
