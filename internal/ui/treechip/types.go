// Package treechip provides hierarchical-navigation chips for tree-
// shaped Salesforce data. Sibling to internal/ui/qchip (filter chips)
// — they share visual + persistence primitives but have fundamentally
// different selection models.
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
