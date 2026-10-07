package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

type listResourceSpec[T any] struct {
	// Key is the Resource.Key — the routing key applyResourceMsg
	// dispatches on. Must match the Key set when the Resource is
	// constructed in initOrgDataResources.
	Key       string
	Res       func(d *orgData) *resource.Resource[[]T]
	List      func(d *orgData) *ListView[T]
	AfterSync func(m *Model, d *orgData)
}

type listResourceHandler struct {
	key               string
	applyAndSync      func(m *Model, d *orgData, msg resource.UpdatedMsg) bool
	refreshAfterCache func(m *Model, d *orgData) tea.Cmd
	syncOnly          func(d *orgData)
	loadedResource    func(d *orgData) resource.Refreshable
}

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

func syncRegisteredLists(d *orgData) {
	for _, key := range listResourceOrder {
		listResourceHandlers[key].syncOnly(d)
	}
}
