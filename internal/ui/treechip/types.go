// Package treechip provides hierarchical-navigation chips for tree-
// shaped Salesforce data. Sibling to internal/ui/qchip (filter chips)
// — they share visual + persistence primitives but have fundamentally
// different selection models.
//
// What treechip is for
// ====================
//
// qchip filters: each chip is a (label, predicate). Selecting one
// filters a flat row list. Chips have no relation to each other.
//
// treechip navigates: each chip is a position in a tree. Selecting a
// chip means "I'm at this branch." The main pane changes shape based
// on position (showing subnodes + items), not just content.
//
// Use treechip when:
//   - The data has parent/child relationships (folders, account
//     hierarchy, custom self-referential lookups, file trees, …)
//   - The user expects to drill from outer to inner branches AND
//     pin individual branches as favourites for one-keypress jumps.
//
// Use qchip when filtering a flat list with predicates.
//
// Public API surface
// ==================
//
// Domain implementations only need to provide a TreeSource (and an
// ItemSource for the leaf rows at each node). Everything else — path
// management, pinning, persistence, view-state for renderers — is
// owned by Registry.
//
// Adding a new tree-shaped data source:
//
//  1. Implement TreeSource (Roots, Children, Item) for the domain.
//  2. Implement ItemSource[T] for the leaf items at a node.
//  3. Build a Registry with NewRegistry(domain, src, items, settings).
//  4. Use Registry.Path() / Drill() / Up() / JumpTo() / Reset() to
//     move around. Read StripModel() and MainModel() to render.
//
// First implementation: internal/sf/report_folders.go (Reports by
// folder). Designed to be a pattern for future trees: Account
// hierarchy, PSG component graphs, custom self-refs.
//
// Why a separate package from qchip
// ==================================
//
// Selection model is fundamentally different — position vs filter.
// Trying to bolt hierarchy onto qchip would either complicate qchip's
// existing flat-filter callers (Records / Objects / Flows) or
// degrade treechip into "filters with parent pointers." A future
// internal/ui/chips/ extraction will share the rendering primitives
// (pill widget, overflow modal, favourite-store) without forcing
// either model on the other.
package treechip

// TreeNode is one node in the tree. Identity is the ID; Label is the
// display name; ParentID is "" for root nodes. Implementations can
// hang arbitrary payload on Data — the registry doesn't inspect it.
//
// Stable IDs are required: pins persist by ID across sessions, so
// rerunning sf-deck and seeing the underlying tree must still
// resolve the user's pinned favourites. SF object IDs satisfy this
// trivially.
type TreeNode struct {
	ID       string
	Label    string
	ParentID string
	Data     any
}

// TreeSource is what a domain implements. Three reads cover every
// access pattern the registry needs:
//
//   - Roots()         — top-level nodes (used at first paint)
//   - Children(id)    — direct children of a node (used on drill)
//   - Item(id)        — fetch one node by ID (used to hydrate pins
//     and last-path on session restore — we have
//     the IDs in settings, but need labels to
//     render).
//
// Implementations decide eager vs lazy: eager loaders cache the
// whole tree in their constructor and answer all three reads from
// memory; lazy loaders fetch on demand and cache as they go. The
// registry doesn't care.
type TreeSource interface {
	Roots() ([]TreeNode, error)
	Children(parentID string) ([]TreeNode, error)
	Item(id string) (TreeNode, error)
}

// ItemSource lists the leaf items at a given node. Generic over the
// item type so each domain can return its own typed rows
// (sf.ReportSummary for report folders, sf.Account for an account
// hierarchy, etc.).
//
// Items returns the items belonging to nodeID specifically, NOT the
// recursive subtree. Subtree counts are computed by the registry
// from per-node counts as it walks.
//
// Empty nodeID ("") means the synthetic "root" — used when the user
// hasn't drilled into a real node yet. Domains that don't have a
// "all items" view can return an empty slice for "".
type ItemSource[T any] interface {
	Items(nodeID string) ([]T, error)
}

// TreePath is a list of nodes from root to current position. Empty
// path = "at the synthetic root level."
type TreePath struct {
	Nodes []TreeNode
}

// CurrentID returns the ID of the leaf-end of the path (i.e. where
// the user currently is), or "" when the path is empty.
func (p TreePath) CurrentID() string {
	if len(p.Nodes) == 0 {
		return ""
	}
	return p.Nodes[len(p.Nodes)-1].ID
}

// Depth returns the path length. 0 = at root.
func (p TreePath) Depth() int { return len(p.Nodes) }

// IDs returns the path as a flat slice of IDs, suitable for
// persistence. Empty when the path is at the synthetic root.
func (p TreePath) IDs() []string {
	out := make([]string, len(p.Nodes))
	for i, n := range p.Nodes {
		out[i] = n.ID
	}
	return out
}
