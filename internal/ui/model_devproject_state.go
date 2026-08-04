package ui

// Dev-project list + drill state.
//
// Extracted from model.go. modelDevProjectState is embedded into Model
// so existing field access (m.devProjectList, m.devProjectCur, …) keeps
// working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// modelDevProjectState owns dev-project list and drill state.
type modelDevProjectState struct {
	// DevProject list view + drill state. Populated lazily on first
	// /dev-projects entry (and refreshed on org switch / collect).
	// Items hang directly off DevProjects now — there's no separate
	// per-org "OrgProject" intermediate. The detail view shows items
	// filtered to the active org by default; devProjectShowAllOrgs
	// toggles to "all orgs" so users can see the project's full reach.
	devProjectList        ListView[devproject.DevProject]
	devProjectCur         string // ID of the drilled-in dev project (TabDevProjectDetail)
	devProjectShowAllOrgs bool   // when true, detail view shows items from every org

	// Kind-filter chip on the Items subtab. Auto-generated from the
	// loaded item set — kinds with zero items aren't in the strip,
	// so the cursor space is dense.
	//
	// devProjectKindChip is the active kind ("" means "All"); the
	// cursor is the index into the visible chip list. Both reset on
	// detail-view entry, scope toggle, and item removal — anything
	// that can shift kind counts or drop the active kind out of the
	// strip. Per-session only; no persistence.
	devProjectKindChip       devproject.ItemKind
	devProjectKindChipCursor int
}
