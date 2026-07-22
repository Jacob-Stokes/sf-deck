package ui

import "sync"

// tabsForNumbersCache holds the resolved slot-1-through-8 list
// after settings load. Read by TabsForNumbers() on every call;
// rebuilt via RebuildTabsForNumbers when the user changes pins.
var (
	tabsForNumbersMu    sync.RWMutex
	tabsForNumbersCache []Tab
)

// Tab is one of the switchable top-level destinations (what lives on
// the number keys). Called "Tab" rather than "View" because a View
// in Salesforce parlance is a user-defined record filter inside one
// of these — future features will have actual Salesforce list views
// as sub-tabs inside a Tab, so we kept the "view" word free for them.
type Tab int

const (
	TabHome Tab = iota
	TabSOQL
	TabObjects
	TabObjectDetail
	TabFieldDetail
	TabValidationDetail
	TabRecordTypeDetail
	TabTriggerDetail
	TabFlows
	TabFlowDetail
	TabFlowVersionDetail // drill-in: one flow version's definition (JSON)
	TabRecords
	TabPackages
	TabProjects
	TabSetup
	TabPerms             // top-level /perms list view (permsets / PSGs / profiles)
	TabPermParentDetail  // drill-in for one permset, PSG, or profile
	TabReports           // saved-reports browser
	TabReportDetail      // single report's preview (cached run by default)
	TabRecent            // client-side history of recently-visited records
	TabRecordDetail      // single record drill-in (fields + actions); reusable from /records, /soql, /reports, /recent
	TabSystem            // unified observability: apex logs + deploys + api usage (subtabs)
	TabDevProjects       // list of dev projects — the only project concept
	TabDevProjectDetail  // drill-in: one dev project's items (filtered to active org by default)
	TabBundleDetail      // drill-in: one bundle's preview tables + retrieve/deploy actions
	TabApex              // /apex — list of ApexClasses + (subtab) flat triggers list
	TabApexDetail        // drill-in: single ApexClass body + actions
	TabLWC               // /components — list of LWC + Aura bundles on the active org
	TabLWCDetail         // drill-in: one bundle's resources
	TabMeta              // /meta — hub for metadata types without a dedicated tab
	TabTags              // tag manager — list/edit/recolor/delete every user-defined tag
	TabTagDetail         // drill-in: items carrying one tag across orgs, kind-chip filtered
	TabQueueDetail       // drill-in: queue members (resolved User + nested Group rows)
	TabPublicGroupDetail // drill-in: public-group members (same shape as Queue)
	TabUsers             // top-level /users — recent-login list (was /home → Users)
	TabUserDetail        // drill-in: single User card + sidebar action menu
	TabExec              // /exec — anonymous Apex editor + saved + history + log subtabs
	TabCompare           // /compare — org-to-org metadata compare (New / Saved / History subtabs)
	TabDeployDetail      // drill-in: one deploy's component failures + test results
	TabMetaTypeDetail    // drill-in: one metadata type's components (/meta Browse)
	TabUserSessions      // drill-in: one user's live sessions (from /users → Active)
	TabCommunities       // /communities — Experience sites list
	TabCommunityDetail   // drill-in: one community's pages + config
)

// tabSlugs is the single source for every tab's stable string id —
// breadcrumbs, settings pins, zone ids. Slugs are persistent data
// (changing one breaks saved settings).
//
// Deliberately a plain data table rather than a TabSpec field: several
// package-level vars (the chip surfaces) transitively reference
// String() through their initializers, so routing String() via
// lookupTabSpec would create a compile-time initialization cycle with
// the registry map. A map literal is a dead end for the initializer
// graph. TestEveryTabHasSlugAndSpec keeps this table and the registry
// in lockstep.
var tabSlugs = map[Tab]string{
	TabHome:              "home",
	TabSOQL:              "soql",
	TabObjects:           "objects",
	TabObjectDetail:      "object",
	TabFieldDetail:       "field",
	TabValidationDetail:  "validation",
	TabRecordTypeDetail:  "recordtype",
	TabTriggerDetail:     "trigger",
	TabFlows:             "flows",
	TabFlowDetail:        "flow",
	TabFlowVersionDetail: "flow-version",
	TabRecords:           "records",
	TabDeployDetail:      "deploy",
	TabMetaTypeDetail:    "meta-type",
	TabPackages:          "packages",
	TabProjects:          "projects",
	TabSetup:             "setup",
	TabPerms:             "perms",
	TabPermParentDetail:  "perm",
	TabReports:           "reports",
	TabReportDetail:      "report",
	TabRecent:            "recent",
	TabRecordDetail:      "record",
	TabSystem:            "system",
	TabDevProjects:       "dev-projects",
	TabDevProjectDetail:  "dev-project",
	TabBundleDetail:      "bundle",
	TabApex:              "apex",
	TabApexDetail:        "apex-class",
	TabLWC:               "components",
	TabLWCDetail:         "component",
	TabMeta:              "meta",
	TabTags:              "tags",
	TabTagDetail:         "tag",
	TabQueueDetail:       "queue",
	TabPublicGroupDetail: "public-group",
	TabUsers:             "users",
	TabUserDetail:        "user-detail",
	TabUserSessions:      "user-sessions",
	TabCommunities:       "communities",
	TabCommunityDetail:   "community",
	TabExec:              "exec",
	TabCompare:           "compare",
}

func (v Tab) String() string {
	if s, ok := tabSlugs[v]; ok {
		return s
	}
	return "?"
}

// TabsForNumbers returns the nav order shown in the status bar. Up to 9
// entries so a single 1-9 key lands on each. Drill-in tabs (ObjectDetail,
// FlowDetail) are reached via Enter, not numbers.
//
// Records is NOT on this list — it's a subtab of the Object drill now.
// TabSetup is still a real tab but no longer has a number slot; reach
// it via the open-menu or bookmarks.
//
// Dev Projects lives off the number bar in the left rail (Orgs / Dev
// Projects panels). TabDevProjectDetail stays reachable via Enter
// from the rail, so the Tab constants still exist; they're just not
// number-addressable.
//
// Layout grouping — code surfaces (Apex, Components) sit next to
// Objects/Flows so the cluster of "things that get deployed" reads as
// one block. Perms gets its number back. /system + /packages dropped
// from the bar — both are reachable as Home subtabs (Logs/Deploys/API
// from /system, Packages from Home/Packages).
//
// Recent + Notifications + Limits live as Home subtabs.
// /apex covers Apex Classes + Triggers + VF (subtabs).
// /components covers LWC + Aura + App Pages + Quick Actions + Web Links.
// /meta is the hub for everything else admin-shaped (labels, templates,
// auth + integration metadata).
// TabsForNumbers returns the 8 user-pinned tabs occupying slots
// 1-8 on the top bar. Slot 9 is always the "More…" overflow modal
// sentinel — every tab not in this list is reachable from there.
//
// The slice is resolved from settings.UI.TabBar.Pinned, with the
// built-in tab list used when unset. Unknown ids (typo'd settings,
// removed tabs) are silently dropped.
func TabsForNumbers() []Tab {
	// Read from the most recent settings snapshot the model has
	// seen. The function is package-level so it doesn't take a
	// Model arg; instead we stash the resolved list when settings
	// load (see RebuildTabsForNumbers).
	tabsForNumbersMu.RLock()
	defer tabsForNumbersMu.RUnlock()
	if len(tabsForNumbersCache) > 0 {
		return tabsForNumbersCache
	}
	return defaultPinnedTabs()
}

// defaultPinnedTabs is the built-in slot-1-through-8 list. Used
// when settings.UI.TabBar.Pinned is unset.
func defaultPinnedTabs() []Tab {
	// System takes slot 8 so the operational surfaces (apex logs,
	// deploys, setup audit trail, flow interviews) are one keystroke
	// away — they're the "what's happening / what broke" tabs you reach
	// for often. Compare and Reports live in the More… overflow; both
	// are lower-frequency and re-pinnable via the overflow modal.
	return []Tab{
		TabHome, TabSOQL, TabObjects, TabFlows,
		TabApex, TabUsers, TabPerms, TabSystem,
	}
}

// RebuildTabsForNumbers swaps the pinned-tab cache. Called by main
// after LoadSettings, and again whenever the user re-orders the
// bar via the overflow modal.
func RebuildTabsForNumbers(ids []string) {
	tabsForNumbersMu.Lock()
	defer tabsForNumbersMu.Unlock()
	tabsForNumbersCache = resolvePinnedIDs(ids)
}

// resolvePinnedIDs maps string IDs to Tab values, dropping unknown
// entries. Caps at 8 slots.
func resolvePinnedIDs(ids []string) []Tab {
	if len(ids) == 0 {
		return defaultPinnedTabs()
	}
	out := make([]Tab, 0, 8)
	for _, id := range ids {
		t, ok := tabByID(id)
		if !ok {
			continue
		}
		out = append(out, t)
		if len(out) >= 8 {
			break
		}
	}
	if len(out) == 0 {
		return defaultPinnedTabs()
	}
	return out
}

// tabByID returns the Tab whose String() matches id. Drill-only
// tabs are excluded so users can't pin them to a slot.
func tabByID(id string) (Tab, bool) {
	for _, t := range allPinnableTabs() {
		if t.String() == id {
			return t, true
		}
	}
	return 0, false
}

// allPinnableTabs lists every top-level tab that's eligible for a
// number-bar slot. Drill-only tabs (record-detail, field-detail,
// etc.) are excluded; the user reaches those via Enter on a row.
func allPinnableTabs() []Tab {
	return []Tab{
		TabHome, TabSOQL, TabObjects, TabFlows, TabApex, TabLWC,
		TabPerms, TabReports, TabMeta, TabUsers,
		TabPackages, TabSetup, TabSystem,
		TabDevProjects, TabTags, TabExec, TabCompare, TabCommunities,
	}
}

// stemOf returns the top-level Tab that a drill-in Tab belongs to.
// For top-level tabs it's the identity. Used so number-key navigation
// can restore a user's last drill position within a family (pressing
// the Objects number from Flows lands back on TabFieldDetail if that's
// where they last were).
//
// The TabSpec registry is the single source of truth: every spec
// declares Stem (identity for top-level tabs); the registry is total
// over the Tab constants, with identity fallback kept for safety.
// TestRegistryStemsAreSane guards against an entry forgetting to set
// Stem, which would silently read as TabHome (the Tab zero value).
//
// Note for TabRecordDetail: the drill is reusable from many tabs
// (/records, /soql, /reports, /recent) — the live return target is
// stashed on the Model as recordDetailReturnTab when opened; the
// registry Stem (TabRecords) is only the fallback.
func (v Tab) stem() Tab {
	if spec := lookupTabSpec(v); spec != nil {
		return spec.Stem
	}
	return v
}
