package treechip

// Registry is the live state of one (org, domain) treechip surface.
// One per (org, domain) pair — sharing across orgs would mean pins
// from one org colliding with another's tree IDs, so the UI layer
// constructs a fresh Registry per ensureOrgData and persists per-org.
//
// Mutators (Drill, Up, JumpTo, Reset, TogglePin) take pointer
// receivers and update in place. Readers (Path, Pins, IsPinned,
// StripModel, MainModel) take value receivers — safe for renderers
// that hold the registry behind a Model copy.

import (
	"fmt"
	"sync"
)

// Persister is the minimal settings-shaped interface the registry
// needs — keeps the package free of a settings import. The UI layer
// hands in an adapter.
type Persister interface {
	Load() (pins []string, lastPath []string)
	Save(pins []string, lastPath []string)
}

// Registry owns the current path + pins for one tree surface.
type Registry struct {
	domain  string
	src     TreeSource
	persist Persister

	mu       sync.Mutex
	path     TreePath
	pins     []string            // node IDs, MRU order
	pinNodes map[string]TreeNode // id → node, populated lazily so pin-jump
	// doesn't re-fetch every render.
}

// NewRegistry constructs a registry for one (domain, source) pair.
// On creation it loads any persisted state (pins + last path) from
// the persister; both are best-effort — failure to resolve a pin or
// path entry just means the pin/segment is dropped.
func NewRegistry(domain string, src TreeSource, persist Persister) *Registry {
	r := &Registry{
		domain:   domain,
		src:      src,
		persist:  persist,
		pinNodes: map[string]TreeNode{},
	}
	if persist != nil {
		pins, last := persist.Load()
		r.pins = append([]string(nil), pins...)
		// Resolve last path lazily — call sites that want it will hit
		// HydrateLastPath. Persisting raw IDs keeps the constructor
		// fast (zero network) and lets the UI decide whether to pay
		// for the lookup on entry.
		_ = last
	}
	return r
}

// Domain returns the registry's domain string.
func (r *Registry) Domain() string { return r.domain }

// Source returns the underlying TreeSource. Exposed so renderers
// that want sub-counts or item fetches can reuse it without
// re-resolving from the model.
func (r *Registry) Source() TreeSource { return r.src }

// Path returns the current breadcrumb path. Empty = at the
// synthetic "root" view.
func (r *Registry) Path() TreePath {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := TreePath{Nodes: make([]TreeNode, len(r.path.Nodes))}
	copy(out.Nodes, r.path.Nodes)
	return out
}

// Drill appends a child to the path. The caller normally pulls the
// child out of the main pane's Subnodes — that node's ParentID
// should match the current path's CurrentID. The registry doesn't
// strictly enforce this (some flows skip levels via JumpTo).
func (r *Registry) Drill(child TreeNode) {
	r.mu.Lock()
	r.path.Nodes = append(r.path.Nodes, child)
	pinsCopy := append([]string(nil), r.pins...)
	pathIDs := r.path.IDs()
	r.mu.Unlock()
	r.persistState(pinsCopy, pathIDs)
}

// Up pops the leaf of the path. No-op at root.
func (r *Registry) Up() {
	r.mu.Lock()
	if len(r.path.Nodes) == 0 {
		r.mu.Unlock()
		return
	}
	r.path.Nodes = r.path.Nodes[:len(r.path.Nodes)-1]
	pinsCopy := append([]string(nil), r.pins...)
	pathIDs := r.path.IDs()
	r.mu.Unlock()
	r.persistState(pinsCopy, pathIDs)
}

// Reset clears the path back to root (synthetic top level).
func (r *Registry) Reset() {
	r.mu.Lock()
	r.path.Nodes = nil
	pinsCopy := append([]string(nil), r.pins...)
	r.mu.Unlock()
	r.persistState(pinsCopy, nil)
}

// JumpTo replaces the path with one going from root to the given
// node. The registry walks ancestors via TreeSource.Item to build
// the breadcrumb. If any ancestor lookup fails, the path is set to
// just the destination node — the user can still navigate from
// there.
func (r *Registry) JumpTo(nodeID string) error {
	if nodeID == "" {
		r.Reset()
		return nil
	}
	chain, err := r.ancestorChain(nodeID)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.path.Nodes = chain
	pinsCopy := append([]string(nil), r.pins...)
	pathIDs := r.path.IDs()
	r.mu.Unlock()
	r.persistState(pinsCopy, pathIDs)
	return nil
}

// ancestorChain walks parent pointers from nodeID up to a root,
// returning the chain in root → leaf order. Cycles are guarded by a
// depth limit (admin-side data corruption shouldn't infinite-loop us).
func (r *Registry) ancestorChain(nodeID string) ([]TreeNode, error) {
	const maxDepth = 64
	chainRev := []TreeNode{}
	cur := nodeID
	seen := map[string]bool{}
	for cur != "" && len(chainRev) < maxDepth {
		if seen[cur] {
			return nil, fmt.Errorf("cycle detected at node %s", cur)
		}
		seen[cur] = true
		n, err := r.src.Item(cur)
		if err != nil {
			// Node missing — return what we have so far, leaf-first.
			break
		}
		chainRev = append(chainRev, n)
		cur = n.ParentID
	}
	// Reverse to root → leaf.
	out := make([]TreeNode, len(chainRev))
	for i, n := range chainRev {
		out[len(chainRev)-1-i] = n
	}
	return out, nil
}

// Pins returns the user's pinned-favourite node list, MRU-first.
// Each pin is hydrated to a full TreeNode via the source so renderers
// can show labels without an extra round trip.
//
// Failed pin lookups are dropped silently — node was deleted or the
// user lost permission. They reappear if the underlying data comes
// back (we don't auto-clean settings).
func (r *Registry) Pins() []TreeNode {
	r.mu.Lock()
	ids := append([]string(nil), r.pins...)
	r.mu.Unlock()
	out := make([]TreeNode, 0, len(ids))
	for _, id := range ids {
		if cached, ok := r.pinCache(id); ok {
			out = append(out, cached)
			continue
		}
		n, err := r.src.Item(id)
		if err != nil {
			continue
		}
		r.cachePin(id, n)
		out = append(out, n)
	}
	return out
}

func (r *Registry) pinCache(id string) (TreeNode, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.pinNodes[id]
	return n, ok
}

func (r *Registry) cachePin(id string, n TreeNode) {
	r.mu.Lock()
	r.pinNodes[id] = n
	r.mu.Unlock()
}

// IsPinned reports whether the given node ID is in the pin list.
func (r *Registry) IsPinned(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.pins {
		if p == id {
			return true
		}
	}
	return false
}

// TogglePin flips the pin state for a node. Returns the new state
// (true = pinned). New pins go to the MRU front; un-pinning removes
// from the list. Persists immediately.
func (r *Registry) TogglePin(node TreeNode) bool {
	if node.ID == "" {
		return false
	}
	r.mu.Lock()
	for i, p := range r.pins {
		if p == node.ID {
			// un-pin: drop from list
			r.pins = append(r.pins[:i], r.pins[i+1:]...)
			delete(r.pinNodes, node.ID)
			pathIDs := r.path.IDs()
			pinsCopy := append([]string(nil), r.pins...)
			r.mu.Unlock()
			r.persistState(pinsCopy, pathIDs)
			return false
		}
	}
	// pin: insert at MRU front
	r.pins = append([]string{node.ID}, r.pins...)
	r.pinNodes[node.ID] = node
	pathIDs := r.path.IDs()
	pinsCopy := append([]string(nil), r.pins...)
	r.mu.Unlock()
	r.persistState(pinsCopy, pathIDs)
	return true
}

// HydrateLastPath restores the path from the persisted last-path IDs
// passed in. Best-effort — drops segments that fail to resolve. UI
// layer calls this once on tab entry if it wants to restore state.
func (r *Registry) HydrateLastPath(ids []string) {
	if len(ids) == 0 {
		return
	}
	chain := make([]TreeNode, 0, len(ids))
	for _, id := range ids {
		n, err := r.src.Item(id)
		if err != nil {
			break
		}
		chain = append(chain, n)
	}
	r.mu.Lock()
	r.path.Nodes = chain
	r.mu.Unlock()
}

func (r *Registry) persistState(pins []string, path []string) {
	if r.persist == nil {
		return
	}
	r.persist.Save(pins, path)
}
