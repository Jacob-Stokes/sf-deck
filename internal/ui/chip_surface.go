package ui

// Chip surface definitions. Each chipSurface bundles the project-chip
// registry, cursor accessors, and list reset hook for one uniform
// chipped surface.
//
// Most surfaces are built from ListChipSurfaceSpec[T] (see
// chip_surface_spec.go) — the spec declares only the per-surface
// variation; newListChipSurface() wires the common closures.
//
// Bespoke surfaces that don't fit the spec (objects uses Name-keyed
// visit lookup, recent has multi-kind scope membership, users runs
// chips server-side with a no-op client filter) keep hand-rolled
// chipSurface literals.
//
// TabSpec/SubtabSpec entries in tab_registry.go point directly at
// the named vars below. The resolver order is active subtab → parent
// tab → nil; the only remaining runtime bridge is /records before
// it has drilled into a specific sObject, where it reuses
// objectsChipSurface.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/orgproject"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// chipSurface bundles every per-surface hook the chip system needs:
// where to find the chip cursor (ChipIdx / SetChipIdx), where to
// reset the underlying list when the chip changes (ResetList),
// which qchip.Registry to consult, and the two predicate-applying
// closures for chip + project-chip selection.
type chipSurface struct {
	Domain chipDomain

	// ChipIdx + SetChipIdx are the per-org chip-cursor accessors.
	ChipIdx    func(Model) int
	SetChipIdx func(*Model, int)

	// ResetList clears the underlying list cursor; called when the
	// chip selection changes so the user lands on row 0 of the new
	// filtered set instead of the old cursor index that may now
	// point at nothing.
	ResetList func(*orgData)

	// Registry returns the qchip.Registry pointer holding this
	// surface's chips (built-ins + user-defined).
	Registry func(*Model) *qchip.Registry

	// ApplyChip writes the chosen chip's predicate onto the
	// underlying ListView's filter slot. Domain-specific because
	// each List has a different element type.
	ApplyChip func(d *orgData, c qchip.Chip)

	// ApplyProjectChip is the project-chip variant — writes the
	// scope-membership predicate onto the same filter slot. Nil for
	// surfaces where the project chip doesn't apply.
	ApplyProjectChip func(d *orgData, scope *orgproject.Scope)

	// ApplyVisitedChip is the Visited-chip variant — writes a
	// closure that consults the merged-recent stream onto the
	// surface's filter slot. Receives the active Model so it can
	// reach m.recentVisitedRecordIDs / m.recentVisitedSObjects.
	// Nil for surfaces that don't participate in the Visited chip.
	//
	// Implementations should ALSO install a most-recent-first
	// recency order via applyVisitedListOrder so the chip's
	// "Recently viewed" label actually means "most recent at top"
	// (not alphabetical) when no column sort is active.
	ApplyVisitedChip func(m Model, d *orgData)

	// ClearVisitedOrder removes the recency-default-order installed
	// by ApplyVisitedChip. Called by the dispatcher when the user
	// switches away from the Visited chip to a regular chip or the
	// project chip, so the list returns to its natural order. Nil
	// for surfaces that don't install a recency order.
	ClearVisitedOrder func(d *orgData)

	// ScopeCount returns how many items of this surface's kind the
	// scope contains. The chip strip auto-prepends the project chip
	// when this returns > 0.
	ScopeCount func(*orgproject.Scope) int

	// ManagerTitle returns the title shown atop the V (chip
	// manager) modal — e.g. "Views · sObjects". Receives Model so
	// surfaces with per-row context (Records: "Views · Account")
	// can compose the right label.
	ManagerTitle func(Model) string

	// ManagerScope returns the scope key the manager modal opens
	// under. Most surfaces are universal scope ("*"); Records is
	// per-sObject ("Account"). Empty string is treated as "*"; when
	// the closure is nil the dispatcher falls back to "*".
	ManagerScope func(Model) string

	// ImportFromSF flags whether the V modal offers "Import from
	// Salesforce…" — the Lightning list-view import flow. Only
	// meaningful for surfaces backed by ListView entities (Records,
	// Flows). Defaults to false.
	ImportFromSF bool
}

// surfaceManagerTitle resolves the V (chip manager) modal title for
// a surface. Falls back to a generic "Views" when the surface
// hasn't declared a ManagerTitle — better than crashing or showing
// an empty string.
func surfaceManagerTitle(s chipSurface, m Model) string {
	if s.ManagerTitle != nil {
		return s.ManagerTitle(m)
	}
	return "Views"
}

// surfaceManagerScope resolves the scope key the manager modal
// opens under. Most surfaces use "*" (universal); per-row scopes
// declare ManagerScope explicitly. Empty / nil → "*".
func surfaceManagerScope(s chipSurface, m Model) string {
	if s.ManagerScope == nil {
		return "*"
	}
	if scope := s.ManagerScope(m); scope != "" {
		return scope
	}
	return "*"
}

// --- Per-surface chipSurface values ----------------------------------

// /objects — bespoke because sObject visits are keyed by Name (the
// API name) instead of an Id, and use the dedicated
// recentVisitedSObjects/recentVisitedRankSObjects helpers that
// strict-filter direct sObject visits (a record drill doesn't
// promote the parent to "visited sObject"). The spec builder
// assumes ID-keyed visits via recentVisitedIDsByKind, so this
// surface stays hand-rolled.
var objectsChipSurface = chipSurface{
	Domain:     domainObjects,
	ChipIdx:    func(m Model) int { return m.objectsChipIdx() },
	SetChipIdx: func(m *Model, i int) { m.setObjectsChipIdx(i) },
	ResetList:  func(d *orgData) { d.SObjectList.ResetCursor() },
	Registry:   func(m *Model) *qchip.Registry { return m.chipRegistry(domainObjects) },
	ApplyChip: func(d *orgData, c qchip.Chip) {
		d.SObjectList.SetExtra(chipMatcherFor[sf.SObject](c, chipSubs(d)))
	},
	ApplyProjectChip: func(d *orgData, scope *orgproject.Scope) {
		d.SObjectList.SetExtra(func(o sf.SObject) bool {
			return scope.HasObject(o.Name)
		})
	},
	ApplyVisitedChip: func(m Model, d *orgData) {
		visited := m.recentVisitedSObjects(orgUserOrEmpty(m))
		d.SObjectList.SetExtra(func(o sf.SObject) bool {
			return visited[o.Name]
		})
		rank := m.recentVisitedRankSObjects(orgUserOrEmpty(m))
		applyVisitedListOrder(&d.SObjectList, rank, func(o sf.SObject) string { return o.Name }, "sobject", d.recentGen)
	},
	ClearVisitedOrder: func(d *orgData) { clearVisitedListOrder(&d.SObjectList) },
	ScopeCount:        func(s *orgproject.Scope) int { return len(s.Objects) },
	ManagerTitle:      func(Model) string { return "Views · sObjects" },
}

// /flows
var flowsChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.Flow]{
	Domain:        domainFlows,
	ChipIdx:       func(m Model) int { return m.flowsChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setFlowsChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.Flow] { return &d.FlowList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainFlows) },
	VisitKind:     RecentKindFlow,
	IDOf:          func(f sf.Flow) string { return f.DefinitionID },
	VisitKey:      "flow",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasFlow(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.FlowIDs) },
	ManagerTitle:  "Views · Flows",
	// ImportFromSF intentionally false: Salesforce stores modern
	// Flow list views against FlowDefinitionView, which rejects the
	// /sobjects/<X>/listviews/<id>/describe endpoint we'd need to
	// extract the SOQL. There's no public API that surfaces the
	// underlying predicate for these views, so the import flow is
	// disabled for /flows. Users who want a custom flow chip build
	// one via N (chip wizard) instead.
})

// /apex Classes
var apexClassesChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.ApexClassRow]{
	Domain:        domainApex,
	ChipIdx:       func(m Model) int { return m.apexChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setApexChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.ApexClassRow] { return &d.ApexClassList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainApex) },
	VisitKind:     RecentKindApexClass,
	IDOf:          func(a sf.ApexClassRow) string { return a.ID },
	VisitKey:      "apex_class",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasApex(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.ApexIDs) },
	ManagerTitle:  "Views · Apex Classes",
})

// /apex Triggers (flat cross-sObject list).
// Triggers ride on the apex_trigger kind in the scope — see Scope.HasTrigger.
var apexTriggersChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.TriggerRow]{
	Domain:        domainTriggers,
	ChipIdx:       func(m Model) int { return m.apexTriggersChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setApexTriggersChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.TriggerRow] { return &d.ApexTriggerList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainTriggers) },
	VisitKind:     RecentKindApexClass,
	IDOf:          func(t sf.TriggerRow) string { return t.ID },
	VisitKey:      "apex_trigger",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasTrigger(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.TriggerIDs) },
	ManagerTitle:  "Views · Apex Triggers",
})

// /components LWC
var lwcChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.LWCBundle]{
	Domain:        domainLWC,
	ChipIdx:       func(m Model) int { return m.lwcChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setLWCChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.LWCBundle] { return &d.LWCBundleList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainLWC) },
	VisitKind:     RecentKindLWC,
	IDOf:          func(b sf.LWCBundle) string { return b.ID },
	VisitKey:      "lwc",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasLWC(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.LWCIDs) },
	ManagerTitle:  "Views · LWC",
})

// /components Aura
var auraChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.AuraBundle]{
	Domain:        domainAura,
	ChipIdx:       func(m Model) int { return m.auraChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setAuraChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.AuraBundle] { return &d.AuraBundleList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainAura) },
	VisitKind:     RecentKindAura,
	IDOf:          func(b sf.AuraBundle) string { return b.ID },
	VisitKey:      "aura",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasAura(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.AuraIDs) },
	ManagerTitle:  "Views · Aura",
})

// /perms PermSets
var permsetsChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.PermissionSet]{
	Domain:        domainPermSets,
	ChipIdx:       func(m Model) int { return m.permsetsChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setPermSetsChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.PermissionSet] { return &d.PermSetList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainPermSets) },
	VisitKind:     RecentKindPermSet,
	IDOf:          func(p sf.PermissionSet) string { return p.ID },
	VisitKey:      "permset",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasPermSet(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.PermSets) },
	ManagerTitle:  "Views · Permission Sets",
})

// /perms PSGs
var psgsChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.PermissionSetGroup]{
	Domain:        domainPSGs,
	ChipIdx:       func(m Model) int { return m.psgsChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setPSGsChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.PermissionSetGroup] { return &d.PSGList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainPSGs) },
	VisitKind:     RecentKindPermSetGroup,
	IDOf:          func(g sf.PermissionSetGroup) string { return g.ID },
	VisitKey:      "psg",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasPSG(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.PSGs) },
	ManagerTitle:  "Views · Permission Set Groups",
})

// /perms Profiles
var profilesChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.Profile]{
	Domain:        domainProfiles,
	ChipIdx:       func(m Model) int { return m.profilesChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setProfilesChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.Profile] { return &d.ProfileList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainProfiles) },
	VisitKind:     RecentKindProfile,
	IDOf:          func(p sf.Profile) string { return p.ID },
	VisitKey:      "profile",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasProfile(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.Profiles) },
	ManagerTitle:  "Views · Profiles",
})

// /perms Queues
var queuesChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.QueueRow]{
	Domain:        domainQueues,
	ChipIdx:       func(m Model) int { return m.queuesChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setQueuesChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.QueueRow] { return &d.QueueList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainQueues) },
	VisitKind:     RecentKindQueue,
	IDOf:          func(q sf.QueueRow) string { return q.ID },
	VisitKey:      "queue",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasQueue(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.Queues) },
	ManagerTitle:  "Views · Queues",
})

// /perms Public Groups
var publicGroupsChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.PublicGroupRow]{
	Domain:        domainPublicGroup,
	ChipIdx:       func(m Model) int { return m.publicGroupsChipIdx() },
	SetChipIdx:    func(m *Model, i int) { m.setPublicGroupsChipIdx(i) },
	ListPtr:       func(d *orgData) *ListView[sf.PublicGroupRow] { return &d.PublicGroupList },
	Registry:      func(m *Model) *qchip.Registry { return m.chipRegistry(domainPublicGroup) },
	VisitKind:     RecentKindPublicGroup,
	IDOf:          func(g sf.PublicGroupRow) string { return g.ID },
	VisitKey:      "public_group",
	ScopeContains: func(s *orgproject.Scope, id string) bool { return s.HasPublicGroup(id) },
	ScopeCount:    func(s *orgproject.Scope) int { return len(s.PublicGroups) },
	ManagerTitle:  "Views · Public Groups",
})

// /deploys
var deploysChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.DeployRow]{
	Domain:       domainDeploys,
	ChipIdx:      func(m Model) int { return m.deploysChipIdx() },
	SetChipIdx:   func(m *Model, i int) { m.setDeploysChipIdx(i) },
	ListPtr:      func(d *orgData) *ListView[sf.DeployRow] { return &d.DeployList },
	Registry:     func(m *Model) *qchip.Registry { return m.chipRegistry(domainDeploys) },
	IDOf:         func(r sf.DeployRow) string { return r.ID },
	ManagerTitle: "Views · Deploys",
})

// /users → Active: session-derived "who's live" rows, chip-filtered by
// security / recency / integration lens.
var activeUsersChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.ActiveUserRow]{
	Domain:       domainActiveUsers,
	ChipIdx:      func(m Model) int { return m.activeUsersChipIdx() },
	SetChipIdx:   func(m *Model, i int) { m.setActiveUsersChipIdx(i) },
	ListPtr:      func(d *orgData) *ListView[sf.ActiveUserRow] { return &d.ActiveUserList },
	Registry:     func(m *Model) *qchip.Registry { return m.chipRegistry(domainActiveUsers) },
	IDOf:         func(r sf.ActiveUserRow) string { return r.UserID },
	ManagerTitle: "Views · Active users",
})

// /reports Dashboards
var dashboardsChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.DashboardRow]{
	Domain:       domainDashboards,
	ChipIdx:      func(m Model) int { return m.dashboardsChipIdx() },
	SetChipIdx:   func(m *Model, i int) { m.setDashboardsChipIdx(i) },
	ListPtr:      func(d *orgData) *ListView[sf.DashboardRow] { return &d.DashboardList },
	Registry:     func(m *Model) *qchip.Registry { return m.chipRegistry(domainDashboards) },
	IDOf:         func(d sf.DashboardRow) string { return d.ID },
	ManagerTitle: "Views · Dashboards",
})

// /reports Report Types
var reportTypesChipSurface = newListChipSurface(ListChipSurfaceSpec[sf.ReportTypeRow]{
	Domain:       domainReportTypes,
	ChipIdx:      func(m Model) int { return m.reportTypesChipIdx() },
	SetChipIdx:   func(m *Model, i int) { m.setReportTypesChipIdx(i) },
	ListPtr:      func(d *orgData) *ListView[sf.ReportTypeRow] { return &d.ReportTypeList },
	Registry:     func(m *Model) *qchip.Registry { return m.chipRegistry(domainReportTypes) },
	IDOf:         func(r sf.ReportTypeRow) string { return r.Type },
	ManagerTitle: "Views · Report Types",
})

// /soql Saved — bespoke because it has no visited support or
// project chip. Just a registry + cursor + plain chip predicate.
var savedQueriesChipSurface = chipSurface{
	Domain:     domainSOQLSaved,
	ChipIdx:    func(m Model) int { return m.soqlSavedChipIdx() },
	SetChipIdx: func(m *Model, i int) { m.setSOQLSavedChipIdx(i) },
	ResetList:  func(d *orgData) { d.SOQLSavedList.ResetCursor() },
	Registry:   func(m *Model) *qchip.Registry { return m.chipRegistry(domainSOQLSaved) },
	ApplyChip: func(d *orgData, c qchip.Chip) {
		d.SOQLSavedList.SetExtra(chipMatcherFor[devproject.SavedQuery](c, chipSubs(d)))
	},
	ManagerTitle: func(Model) string { return "Views · Saved Queries" },
}

// /soql History — bespoke for the same reasons.
var soqlHistoryChipSurface = chipSurface{
	Domain:     domainSOQLHistory,
	ChipIdx:    func(m Model) int { return m.soqlHistoryChipIdx() },
	SetChipIdx: func(m *Model, i int) { m.setSOQLHistoryChipIdx(i) },
	ResetList:  func(d *orgData) { d.SOQLHistoryList.ResetCursor() },
	Registry:   func(m *Model) *qchip.Registry { return m.chipRegistry(domainSOQLHistory) },
	ApplyChip: func(d *orgData, c qchip.Chip) {
		d.SOQLHistoryList.SetExtra(chipMatcherFor[devproject.SOQLHistoryEntry](c, chipSubs(d)))
	},
	ManagerTitle: func(Model) string { return "Views · SOQL History" },
}

// /recent + /home Recent — bespoke. The chip filters
// d.RecentList.Extra by RecentEntry.Kind; the project chip's
// match function is a per-kind switch over the scope's per-kind
// sets, which doesn't fit the simple ScopeContains(id) shape.
var recentChipSurface = chipSurface{
	Domain:     domainRecent,
	ChipIdx:    func(m Model) int { return m.recentChipIdx() },
	SetChipIdx: func(m *Model, i int) { m.setRecentChipIdx(i) },
	ResetList:  func(d *orgData) { d.RecentList.ResetCursor() },
	Registry:   func(m *Model) *qchip.Registry { return m.chipRegistry(domainRecent) },
	ApplyChip: func(d *orgData, c qchip.Chip) {
		// Filter the ACTIVE list (sf-deck or Salesforce) — chip
		// predicates run against RecentEntry.Field which exposes
		// Kind for the kind chips.
		if lv := activeRecentListPtr(d); lv != nil {
			lv.SetExtra(chipMatcherFor[RecentEntry](c, chipSubs(d)))
		}
	},
	ApplyProjectChip: func(d *orgData, scope *orgproject.Scope) {
		lv := activeRecentListPtr(d)
		if lv == nil {
			return
		}
		lv.SetExtra(func(r RecentEntry) bool {
			// Match each entry against the scope's per-kind sets.
			// Kinds not collected at scope level (users, deploys,
			// packages, apex logs) can't match — return false rather
			// than letting them slip through.
			switch r.Kind {
			case RecentKindRecord:
				// Records are scoped by (sobject, id) — not just the
				// sObject. Both halves must be in the project.
				return scope.HasRecord(r.Type, r.ID)
			case RecentKindSObject:
				return scope.HasObject(r.ID)
			case RecentKindField:
				// Fields aren't tracked individually — the project
				// scopes whole sObjects. Match when the parent
				// sObject (carried on r.Type) is in scope.
				return scope.HasObject(r.Type)
			case RecentKindFlow:
				return scope.HasFlow(r.ID)
			case RecentKindApexClass:
				return scope.HasApex(r.ID) || scope.HasTrigger(r.ID)
			case RecentKindLWC:
				return scope.HasLWC(r.ID)
			case RecentKindAura:
				return scope.HasAura(r.ID)
			case RecentKindReport:
				return scope.HasReport(r.ID)
			case RecentKindPermSet:
				return scope.HasPermSet(r.ID)
			case RecentKindPermSetGroup:
				return scope.HasPSG(r.ID)
			case RecentKindProfile:
				return scope.HasProfile(r.ID)
			case RecentKindQueue:
				return scope.HasQueue(r.ID)
			case RecentKindPublicGroup:
				return scope.HasPublicGroup(r.ID)
			}
			return false
		})
	},
	// ScopeCount is conservative — return a positive number whenever
	// the loaded project has ANYTHING in it, so the project chip
	// always shows for users with a project loaded. Filtering happens
	// inside ApplyProjectChip.
	ScopeCount: func(s *orgproject.Scope) int {
		return len(s.Objects) + len(s.FlowIDs) + len(s.ApexIDs) +
			len(s.TriggerIDs) + len(s.LWCIDs) + len(s.AuraIDs) +
			len(s.ReportIDs) + len(s.Records) + len(s.PermSets) +
			len(s.PSGs) + len(s.Profiles) + len(s.Queues) +
			len(s.PublicGroups)
	},
	ManagerTitle: func(Model) string { return "Views · Recent" },
}

// /users · All users — bespoke because chips run server-side.
//
// Each chip's Query AST compiles to SOQL via qchip.ApplyToSOQL
// inside d.EnsureChipUsers, so cycling between "Active" / "System
// admins" / "Logged in 30d" pulls a fresh chip-bounded slice from
// Salesforce instead of filtering a single over-large fetch.
// Switching chips just changes which Resource the renderer reads —
// there's no client-side predicate to apply, hence the no-op
// ApplyChip.
//
// ImportFromSF is on: User is a regular sObject and Salesforce
// happily describes its list views, so the standard /records-style
// import flow works (V → Import from Salesforce…). Imported chips
// run the SF list view's SOQL against User the same way the
// built-ins do.
var usersChipSurface = chipSurface{
	Domain:     domainUsers,
	ChipIdx:    func(m Model) int { return m.allUsersChipIdx() },
	SetChipIdx: func(m *Model, i int) { m.setAllUsersChipIdx(i) },
	ResetList: func(d *orgData) {
		// Reset every per-chip ListView's cursor — switching chip
		// shouldn't carry over a deep cursor on the previous chip.
		for _, lv := range d.ChipUsersList {
			lv.ResetCursor()
		}
	},
	Registry:     func(m *Model) *qchip.Registry { return m.chipRegistry(domainUsers) },
	ApplyChip:    func(d *orgData, c qchip.Chip) {},
	ManagerTitle: func(Model) string { return "Views · Users" },
	ImportFromSF: true,
}

// allChipSurfaces walks the TabSpec registry and yields every
// declared chipSurface — both TabSpec.Chips and SubtabSpec.Chips.
// Replaces the older chipSurfaces() map (which duplicated what the
// registry already encoded). Used by:
//
//   - domainFromRegistry (chip_helpers.go) — reverse-lookup from a
//     qchip.Registry pointer to its domain.
//   - chipSurfaceForDomain — reverse-lookup from a domain enum to
//     the surface that owns it.
//   - compensateChipCursorsForPrepend (orgproject_load.go) — fan
//     out across every project-chip-aware surface when scope
//     changes.
//
// Returns by value so callers don't accidentally mutate the
// registry's spec entries through the returned slice.
func allChipSurfaces() []chipSurface {
	out := make([]chipSurface, 0, 16)
	seen := map[chipDomain]bool{}
	add := func(s *chipSurface) {
		if s == nil || seen[s.Domain] {
			return
		}
		seen[s.Domain] = true
		out = append(out, *s)
	}
	for _, spec := range tabSpecs() {
		add(spec.Chips)
		for i := range spec.Subtabs {
			add(spec.Subtabs[i].Chips)
		}
	}
	return out
}

// chipSurfaceForDomain reverses the lookup: given a domain, return
// its surface entry. Used by the chip-manager modal which thinks in
// terms of domains, not tabs.
func chipSurfaceForDomain(domain chipDomain) *chipSurface {
	for _, s := range allChipSurfaces() {
		if s.Domain == domain {
			out := s
			return &out
		}
	}
	return nil
}
