package ui

// Generic list-resource registry.
//
// Adding a new list-backed org surface used to mean editing ~6 plumbing
// files with byte-identical boilerplate — the applyResourceMsg routing
// case, the SyncXList method + its registration, the loadedResources
// entry, and the listtable-prefs key — each guarded by its own drift
// test because it was so easy to forget one.
//
// registerListResource collapses that: ONE registration derives the
// sync, the resource-message routing, and the loaded-resource
// enumeration. The remaining per-surface work (the sf fetcher, column
// schema, sidebar, registry entry, render func) is the actual feature.
//
// Heterogeneous resources are stored behind type-erased closures so a
// single []listResourceHandler can hold them all. Each closure captures
// the concrete Resource[[]T]/ListView[T] via the accessor funcs on the
// typed spec, so no reflection is involved.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

// listResourceSpec declares one list-backed per-org resource. T is the
// row type. The accessors return the live Resource/ListView on a given
// orgData so the generic machinery can drive them.
type listResourceSpec[T any] struct {
	// Key is the Resource.Key — the routing key applyResourceMsg
	// dispatches on. Must match the Key set when the Resource is
	// constructed in initOrgDataResources.
	Key string
	// Res / List return the backing Resource and its ListView on d.
	Res  func(d *orgData) *resource.Resource[[]T]
	List func(d *orgData) *ListView[T]
	// AfterSync runs after a successful Apply+sync (optional). Use for
	// surface-specific side effects — the SObject catalogue busts the
	// SOQL autocomplete memo here, for example. Nil for the common case.
	AfterSync func(m *Model, d *orgData)
}

// listResourceHandler is the type-erased form registerListResource
// produces. One per registered resource.
type listResourceHandler struct {
	key string
	// applyAndSync applies the message to the resource and, on a real
	// change, mirrors it into the ListView + runs AfterSync. Returns
	// whether the resource changed.
	applyAndSync func(m *Model, d *orgData, msg resource.UpdatedMsg) bool
	// refreshAfterCache returns the post-cache-load refresh command.
	refreshAfterCache func(m *Model, d *orgData) tea.Cmd
	// syncOnly mirrors the resource into its ListView with no message
	// (used by the bulk SyncListViews path).
	syncOnly func(d *orgData)
	// loadedResource returns the resource as a type-erased Refreshable
	// so loadedResources() can enumerate it for a global refresh.
	loadedResource func(d *orgData) resource.Refreshable
}

// listResourceHandlers is the global registry, built once at init by
// the registerListResource calls in list_resource_registrations.go.
var listResourceHandlers = map[string]listResourceHandler{}
var listResourceOrder []string // stable iteration order (registration order)

// registerListResource records a spec. Called from init(); a duplicate
// Key panics (a programming error — two resources can't share a key).
func registerListResource[T any](spec listResourceSpec[T]) {
	if spec.Key == "" || spec.Res == nil || spec.List == nil {
		panic("registerListResource: Key, Res and List are required")
	}
	if _, dup := listResourceHandlers[spec.Key]; dup {
		panic("registerListResource: duplicate key " + spec.Key)
	}
	h := listResourceHandler{
		key: spec.Key,
		applyAndSync: func(m *Model, d *orgData, msg resource.UpdatedMsg) bool {
			changed := spec.Res(d).Apply(msg)
			if changed {
				spec.List(d).Set(spec.Res(d).Value())
				if spec.AfterSync != nil {
					spec.AfterSync(m, d)
				}
			}
			return changed
		},
		refreshAfterCache: func(m *Model, d *orgData) tea.Cmd {
			return spec.Res(d).MaybeRefreshAfterCacheLoad(m.cache)
		},
		syncOnly: func(d *orgData) {
			spec.List(d).Set(spec.Res(d).Value())
		},
		loadedResource: func(d *orgData) resource.Refreshable {
			return spec.Res(d)
		},
	}
	listResourceHandlers[spec.Key] = h
	listResourceOrder = append(listResourceOrder, spec.Key)
}

// routeListResource handles a resource-update message for any registered
// list resource. Returns (handled, refreshCmd). handled=false means the
// key isn't a registered generic resource — the caller keeps its
// explicit switch cases for surfaces with bespoke apply logic.
func (m *Model) routeListResource(d *orgData, msg resource.UpdatedMsg) (handled bool, refresh tea.Cmd) {
	h, ok := listResourceHandlers[msg.Key]
	if !ok {
		return false, nil
	}
	h.applyAndSync(m, d, msg)
	if msg.FromCache {
		refresh = h.refreshAfterCache(m, d)
	}
	return true, refresh
}

// syncRegisteredLists mirrors every registered resource into its
// ListView. Called from SyncListViews so a full re-sync (e.g. after an
// org switch) covers registered surfaces without a per-resource line.
func syncRegisteredLists(d *orgData) {
	for _, key := range listResourceOrder {
		listResourceHandlers[key].syncOnly(d)
	}
}
