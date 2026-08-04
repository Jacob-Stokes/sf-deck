package ui

// /perms list surfaces (PermSets, PSGs, Profiles, Queues, Public
// Groups). All declared via ListViewTableSpec[T]. Each surface uses
// kindRefGutters for the tag/project annotations (one gutter call,
// no separate bulk-tag map per surface — kindRefGutters does the
// store lookup internally).

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// idGutters returns a Gutters closure that wraps m.kindRefGutters
// for the "ID-keyed" perms case. idOf extracts the row's ID
// (varies per type — PermissionSet.ID, Profile.ID, etc.).
func idGutters[T any](kind devproject.ItemKind, idOf func(T) string) func(m Model, items []T) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
	return func(m Model, items []T) ([]uilayout.GutterSpec, []uilayout.GutterSpec) {
		return m.kindRefGutters(kind, len(items),
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return idOf(items[row])
			})
	}
}

var permsetsTableSpec = ListViewTableSpec[sf.PermissionSet]{
	Schema:     permSetColumnSchema(),
	RowKindRef: func(p sf.PermissionSet) (devproject.ItemKind, string) { return devproject.KindPermissionSet, p.ID },
	ListPtr:    func(d *orgData) *ListView[sf.PermissionSet] { return &d.PermSetList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.PermSetsTableState },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.PermissionSet) string {
		return standardListTitle("PERMISSION SETS", d.PermSetList.Len(), &d.PermSets)
	},
	ResErr:  func(d *orgData) error { return d.PermSets.Err() },
	Marks:   marksForPermSetList,
	Gutters: idGutters(devproject.KindPermissionSet, func(p sf.PermissionSet) string { return p.ID }),
	Empty:   "  no permission sets",
}

var permsetsListSurface = listSurfaceFromSpec(permsetsTableSpec)

var psgsTableSpec = ListViewTableSpec[sf.PermissionSetGroup]{
	Schema: psgColumnSchema(),
	RowKindRef: func(g sf.PermissionSetGroup) (devproject.ItemKind, string) {
		return devproject.KindPermissionSetGroup, g.ID
	},
	ListPtr:    func(d *orgData) *ListView[sf.PermissionSetGroup] { return &d.PSGList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.PSGsTableState },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.PermissionSetGroup) string {
		return standardListTitle("PERMISSION SET GROUPS", d.PSGList.Len(), &d.PSGs)
	},
	ResErr:  func(d *orgData) error { return d.PSGs.Err() },
	Marks:   marksForPSGList,
	Gutters: idGutters(devproject.KindPermissionSetGroup, func(g sf.PermissionSetGroup) string { return g.ID }),
	Empty:   "  no permission set groups",
}

var psgsListSurface = listSurfaceFromSpec(psgsTableSpec)

var profilesTableSpec = ListViewTableSpec[sf.Profile]{
	Schema:     profileColumnSchema(),
	RowKindRef: func(p sf.Profile) (devproject.ItemKind, string) { return devproject.KindProfile, p.ID },
	ListPtr:    func(d *orgData) *ListView[sf.Profile] { return &d.ProfileList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.ProfilesTableState },
	FlagsAware: true,
	Title: func(m Model, d *orgData, items []sf.Profile) string {
		return standardListTitle("PROFILES", d.ProfileList.Len(), &d.Profiles)
	},
	ResErr:  func(d *orgData) error { return d.Profiles.Err() },
	Marks:   marksForProfileList,
	Gutters: idGutters(devproject.KindProfile, func(p sf.Profile) string { return p.ID }),
	Empty:   "  no profiles",
}

var profilesListSurface = listSurfaceFromSpec(profilesTableSpec)

var queuesTableSpec = ListViewTableSpec[sf.QueueRow]{
	Schema:     queueColumnSchema(),
	RowKindRef: func(q sf.QueueRow) (devproject.ItemKind, string) { return devproject.KindQueue, q.ID },
	ListPtr:    func(d *orgData) *ListView[sf.QueueRow] { return &d.QueueList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.QueuesTableState },
	Title: func(m Model, d *orgData, items []sf.QueueRow) string {
		return standardListTitle("QUEUES", d.QueueList.Len(), &d.Queues)
	},
	ResErr:  func(d *orgData) error { return d.Queues.Err() },
	Gutters: idGutters(devproject.KindQueue, func(q sf.QueueRow) string { return q.ID }),
	Empty:   "  no queues",
}

var queuesListSurface = listSurfaceFromSpec(queuesTableSpec)

var publicGroupsTableSpec = ListViewTableSpec[sf.PublicGroupRow]{
	Schema:     publicGroupColumnSchema(),
	RowKindRef: func(g sf.PublicGroupRow) (devproject.ItemKind, string) { return devproject.KindPublicGroup, g.ID },
	ListPtr:    func(d *orgData) *ListView[sf.PublicGroupRow] { return &d.PublicGroupList },
	StatePtr:   func(d *orgData) *uilayout.ListTableState { return &d.PublicGroupsTableState },
	Title: func(m Model, d *orgData, items []sf.PublicGroupRow) string {
		return standardListTitle("PUBLIC GROUPS", d.PublicGroupList.Len(), &d.PublicGroups)
	},
	ResErr:  func(d *orgData) error { return d.PublicGroups.Err() },
	Gutters: idGutters(devproject.KindPublicGroup, func(g sf.PublicGroupRow) string { return g.ID }),
	Empty:   "  no public groups",
}

var publicGroupsListSurface = listSurfaceFromSpec(publicGroupsTableSpec)
