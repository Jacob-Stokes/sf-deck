package treechip

// View-model adapters: thin reads off the registry that renderers
// consume. Renderers take these structs, never the registry itself,
// so they're easy to test and the registry stays mutable-but-private.

// StripModel describes the chip strip at the top of the pane:
// breadcrumb segments for the current path, plus pinned-favourite
// nodes. The renderer composes them into a single horizontal strip.
type StripModel struct {
	// Breadcrumb is the current path, root → leaf. Empty when at the
	// synthetic top. The renderer shows these as clickable chips —
	// click any segment to jump back to it.
	Breadcrumb []TreeNode

	// Pins are favourite nodes anywhere in the tree. Click any to
	// JumpTo() that node directly. Independent of path.
	Pins []TreeNode

	// CurrentID is the leaf-end ID of Breadcrumb (or "" at root).
	// Renderers use this to highlight the active segment.
	CurrentID string
}

// MainModel describes the main pane contents at the current path:
// child nodes for further drilling, plus leaf items at this node
// (whatever the domain's ItemSource produced).
//
// Items is intentionally type-erased here — domain-specific
// renderers re-cast via the typed channel they pre-populated. The
// registry hands back []any so the package itself stays generic;
// the UI layer calls reg.Source() + the typed ItemSource directly
// for the rows.
type MainModel struct {
	Subnodes []TreeNode
	// SubnodeCounts gives per-subnode item-count + descendant-count
	// hints used to render summary lines on each row. Computed lazily
	// by the UI layer; treechip doesn't compute them.
	SubnodeCounts map[string]NodeCount

	// AtRoot is true when the current path is empty.
	AtRoot bool
}

// NodeCount aggregates display-only stats about a node — used for
// "12 items, 2 subfolders" summary lines in the main pane. The UI
// layer fills these in.
type NodeCount struct {
	DirectItems int // items at this node
	Subnodes    int // direct children
}

// StripModel returns the strip view-model — read-only snapshot.
// Safe to call from a render loop.
func (r *Registry) StripModel() StripModel {
	path := r.Path()
	pins := r.Pins()
	return StripModel{
		Breadcrumb: path.Nodes,
		Pins:       pins,
		CurrentID:  path.CurrentID(),
	}
}

// MainModel returns the main-pane view-model: subnodes at the
// current path and an AtRoot flag. The caller is responsible for
// fetching and rendering the leaf items via their ItemSource (we
// don't bake item types into the package).
//
// On error, returns an empty model so renderers can show a "loading"
// or empty state cleanly.
func (r *Registry) MainModel() (MainModel, error) {
	path := r.Path()
	var subnodes []TreeNode
	var err error
	if len(path.Nodes) == 0 {
		subnodes, err = r.src.Roots()
	} else {
		subnodes, err = r.src.Children(path.CurrentID())
	}
	if err != nil {
		return MainModel{}, err
	}
	return MainModel{
		Subnodes: subnodes,
		AtRoot:   len(path.Nodes) == 0,
	}, nil
}
