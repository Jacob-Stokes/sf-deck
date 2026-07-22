package ui

// Open surface registry — second migration step on the path to a
// declarative TabSpec. Companion to chipSurface (chip_surface.go)
// and listSurface (list_surface.go).
//
// Each openSurface answers two questions for a given (Tab, Subtab):
//
//   1. What's under the cursor right now, as an sf.Openable?
//      (drives o, ctrl+o, O, y, ctrl+y, the open-menu modal)
//   2. What does Enter do — drill into a detail tab? Or no-op?
//      (drives the activate() handler)
//
// Each surface lives as a named package-level var below, populated
// in init(). The init() indirection breaks an otherwise unavoidable
// initialization cycle: many Drill closures call m.onTabChanged(),
// which transitively reaches lookupTabSpec → tabRegistry → these
// same surface vars. Go's package-init walker follows function
// bodies, so a direct `var X = openSurface{...}` form would cycle.
// init() side-steps that because the var has no initializer
// expression — the assignment happens at runtime, after all package
// vars are declared.
//
// TabSpec entries point at these vars via Subtabs[i].Open or
// TabSpec.Open — see tab_registry.go.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"

	tea "charm.land/bubbletea/v2"
)

// openSurface is the per-(Tab, Subtab) record. Both fields are
// optional — a surface that has only `Openable` (cursor → URL via o)
// without `Drill` (Enter → no drill) is fine; a surface with only
// `Drill` (Enter drills, but no o target) is also fine.
type openSurface struct {
	// Openable returns the cursored row's Openable, or nil when
	// there's nothing meaningful under the cursor (empty list,
	// loading state, etc.). Nil result tells the caller to fall
	// back to the org's default.
	//
	// Relationship to TabSpec.Identity: if a tab's Identity already
	// returns an Openable, leave THIS field nil — cursorOpenable
	// consults Identity first and an Identity-provided Openable
	// makes this duplicate redundant. Set Openable here only when
	// the openable shape doesn't fit the (kind, ref, label, openable)
	// tuple Identity carries (e.g. records-list rows that synthesize
	// an attributes block, /reports' "report row vs folder row"
	// branch, multi-target opens with bespoke logic).
	Openable func(m Model) sf.Openable

	// Drill is the Enter handler. Returns the (target tab, cmd)
	// pair to apply, plus ok=false if Enter should be a no-op
	// (e.g. cursor on an unloaded row, no detail tab for this kind
	// yet). The closure mutates per-surface state on the *Model
	// (DescribeCur / FlowCur / ApexCur etc.) before returning the
	// target tab — same pattern the legacy activate() switch used.
	Drill func(m *Model) (tea.Cmd, bool)
}

// Surface vars — declared without initializer so they don't
// participate in package-init cycle detection. Populated in init()
// below.

var (
	objectsOpenSurface      openSurface
	flowsOpenSurface        openSurface
	apexClassesOpenSurface  openSurface
	apexTriggersOpenSurface openSurface
	lwcOpenSurface          openSurface
	auraOpenSurface         openSurface
	permsetsOpenSurface     openSurface
	psgsOpenSurface         openSurface
	profilesOpenSurface     openSurface
	queuesOpenSurface       openSurface
	publicGroupsOpenSurface openSurface

	homeRecentOpenSurface        openSurface
	homeNotificationsOpenSurface openSurface
	homeLimitsOpenSurface        openSurface
	homeLicensesOpenSurface      openSurface

	apexLogsOpenSurface        openSurface
	setupAuditOpenSurface      openSurface
	flowInterviewsOpenSurface  openSurface
	asyncJobsOpenSurface       openSurface
	scheduledJobsOpenSurface   openSurface
	activeUsersOpenSurface     openSurface
	userSessionsOpenSurface    openSurface
	communitiesOpenSurface     openSurface
	communityPagesOpenSurface  openSurface
	deploysOpenSurface         openSurface
	deployDetailOpenSurface    openSurface
	metaBrowseOpenSurface      openSurface
	cmtOpenSurface             openSurface
	customLabelsOpenSurface    openSurface
	customSettingsOpenSurface  openSurface
	staticResourcesOpenSurface openSurface
	namedCredsOpenSurface      openSurface
	remoteSitesOpenSurface     openSurface

	queueDetailOpenSurface openSurface
	usersOpenSurface       openSurface
)

func init() {
	// /objects
	objectsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if s, ok := d.SObjectList.Selected(); ok {
					return s
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			selected, ok := d.SObjectList.Selected()
			if !ok {
				return nil, false
			}
			d.DescribeCur = selected.Name
			if s := d.SObjectList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			m.objectActionCur = 0
			m.setTab(TabObjectDetail)
			return m.onTabChanged(), true
		},
	}

	// /flows
	flowsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if f, ok := d.FlowList.Selected(); ok {
					return f
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			f, ok := d.FlowList.Selected()
			if !ok {
				return nil, false
			}
			d.FlowCur = f.DefinitionID
			if s := d.FlowList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			m.setTab(TabFlowDetail)
			return m.onTabChanged(), true
		},
	}

	// /apex Classes
	apexClassesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if a, ok := d.ApexClassList.Selected(); ok {
					return a
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			a, ok := d.ApexClassList.Selected()
			if !ok {
				return nil, false
			}
			if s := d.ApexClassList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerOpenApexClass(a.ID), true
		},
	}

	// /apex Triggers (flat cross-sObject list)
	apexTriggersOpenSurface = openSurface{
		// Trigger rows aren't directly Openable in Lightning today —
		// nil; o falls through to the org default.
		Openable: func(m Model) sf.Openable { return nil },
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			t, ok := d.ApexTriggerList.Selected()
			if !ok {
				return nil, false
			}
			if s := d.ApexTriggerList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerDetailDrill(t.Table, t.ID, TabApex), true
		},
	}

	// /components LWC
	lwcOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if b, ok := d.LWCBundleList.Selected(); ok {
					return b
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			b, ok := d.LWCBundleList.Selected()
			if !ok {
				return nil, false
			}
			if s := d.LWCBundleList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerOpenLWCBundle(b.ID), true
		},
	}

	// /components Aura
	auraOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if b, ok := d.AuraBundleList.Selected(); ok {
					return b
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			b, ok := d.AuraBundleList.Selected()
			if !ok {
				return nil, false
			}
			if s := d.AuraBundleList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerOpenAuraBundle(b.ID), true
		},
	}

	// /perms PermSets
	permsetsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if p, ok := d.PermSetList.Selected(); ok {
					return p
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			p, ok := d.PermSetList.Selected()
			if !ok {
				return nil, false
			}
			d.PermParentKind = "permset"
			d.PermParentID = p.ID
			d.PermParentPermSetID = p.ID
			d.PermParentSubtab = 0
			d.PermFieldsSObject = ""
			if s := d.PermSetList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			m.setTab(TabPermParentDetail)
			return m.onTabChanged(), true
		},
	}

	// /perms PSGs
	psgsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if g, ok := d.PSGList.Selected(); ok {
					return g
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			g, ok := d.PSGList.Selected()
			if !ok {
				return nil, false
			}
			d.PermParentKind = "psg"
			d.PermParentID = g.ID
			d.PermParentPermSetID = ""
			d.PermParentSubtab = 0
			d.PermFieldsSObject = ""
			if s := d.PSGList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			m.setTab(TabPermParentDetail)
			return m.onTabChanged(), true
		},
	}

	// /perms Profiles
	profilesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if p, ok := d.ProfileList.Selected(); ok {
					return p
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			p, ok := d.ProfileList.Selected()
			if !ok {
				return nil, false
			}
			d.PermParentKind = "profile"
			d.PermParentID = p.ID
			d.PermParentPermSetID = p.PermissionSetID
			d.PermParentSubtab = 0
			d.PermFieldsSObject = ""
			if s := d.ProfileList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			m.setTab(TabPermParentDetail)
			return m.onTabChanged(), true
		},
	}

	// /perms Queues
	queuesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if q, ok := d.QueueList.Selected(); ok {
					return q
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			return m.activateQueue(), true
		},
	}

	// /perms Public Groups
	publicGroupsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if g, ok := d.PublicGroupList.Selected(); ok {
					return g
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			return m.activatePublicGroup(), true
		},
	}

	// /perms Queue / Public Group detail — drill on a member opens
	// the underlying user/group's Lightning record (same browser-hop
	// shape as notifications + apex logs).
	queueDetailOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if lv, ok := d.GroupMemberList[d.GroupMemberID]; ok {
					if r, ok := lv.Selected(); ok {
						return r
					}
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	// /home Recent subtab Open surface.  Rows are list-shaped with
	// both Openable (yank / open-in-Lightning) and Drill (in-TUI
	// navigation to the canonical detail tab for the row's kind).
	// Drill dispatches by RecentEntry.Kind via the shared drillByKind
	// helper; kinds with no in-TUI detail surface (dashboard, listview,
	// package, deploy, apex_log) silently no-op — users can press `o`
	// to open them in Lightning instead.  See
	// docs/recent-kinds-drill-audit.md.
	homeRecentOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil {
				return nil
			}
			lv := activeRecentListPtr(d)
			if lv == nil {
				return nil
			}
			r, ok := lv.Selected()
			if !ok {
				return nil
			}
			rec := map[string]any{
				"Id":         r.ID,
				"attributes": map[string]any{"type": r.Type},
			}
			if r.Name != "" {
				rec["Name"] = r.Name
			}
			return m.newRecordRef(rec)
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			lv := activeRecentListPtr(d)
			if lv == nil {
				return nil, false
			}
			r, ok := lv.Selected()
			if !ok {
				return nil, false
			}
			return drillByKind(m, r.Kind, r.ID, r.Type, r.Name, TabHome)
		},
	}

	homeNotificationsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.HomeNotifList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		// Drill on a notification = open its target in Lightning.
		// Notifications don't have a useful in-TUI detail surface
		// (the body is short and already rendered in the row); the
		// expected gesture is "take me to the thing the notification
		// is about." The openable's first target is exactly that.
		Drill: drillToFirstOpenTarget,
	}

	homeLimitsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.HomeLimitList.Selected(); ok {
					return v
				}
			}
			return nil
		},
	}

	homeLicensesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.HomeLicenseList.Selected(); ok {
					return v
				}
			}
			return nil
		},
	}

	// /apex-logs (top-level tab)
	apexLogsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.ApexLogList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	setupAuditOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.SetupAuditList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	flowInterviewsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.FlowInterviewList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	// Jobs surfaces expose Openable (o → the Setup page) but NO Drill —
	// Enter drills into the job's Apex class via the subtab's Activate
	// hook (activateAsyncJob / activateScheduledJob), not into Lightning.
	asyncJobsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.AsyncJobList.Selected(); ok {
					return v
				}
			}
			return nil
		},
	}

	scheduledJobsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.ScheduledJobList.Selected(); ok {
					return v
				}
			}
			return nil
		},
	}

	activeUsersOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.ActiveUserList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		// No Drill: on the Active subtab, Enter must reach the subtab's
		// Activate hook (drill into the user's sessions). A Drill closure
		// here would short-circuit Enter before Activate (see activate()),
		// opening the user in Lightning instead — o still does that via
		// Openable.
	}

	userSessionsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.UserSessionList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	// Communities: o opens the community (live/builder/setup); Enter
	// drills into its pages (Drill omitted so activate hits the tab's
	// Activate hook — same pattern as Active Users).
	communitiesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.CommunityList.Selected(); ok {
					return v
				}
			}
			return nil
		},
	}

	communityPagesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.CommunityPageList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: drillToFirstOpenTarget,
	}

	// /deploys (top-level)
	deploysOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.DeployList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		// Enter drills into the component/test breakdown — the
		// browser stays on o/O per the global Enter policy. (This
		// replaced an early drillToFirstOpenTarget which opened the
		// browser from Enter.)
		Drill: func(m *Model) (tea.Cmd, bool) {
			return m.drillIntoDeploy(), true
		},
	}

	// /deploy drill: o opens the drilled deploy's Setup detail page.
	deployDetailOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.DeployCur == "" {
				return nil
			}
			for _, r := range d.Deploys.Value() {
				if r.ID == d.DeployCur {
					return r
				}
			}
			return sf.DeployRow{ID: d.DeployCur}
		},
	}

	// /users · Recent logins. Drill enters the in-TUI User detail
	// surface (action menu lives in the sidebar). The browser-open
	// path stays available via o / ctrl+o.
	usersOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.HomeUserList.Selected(); ok {
					return m.enrichUserRowTargets(v)
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			d := m.activeOrgData()
			if d == nil {
				return nil, false
			}
			u, ok := d.HomeUserList.Selected()
			if !ok {
				return nil, false
			}
			if s := d.HomeUserList.SearchPtr(); s.Active {
				s.Active = false
				s.Committed = s.Buffer() != ""
			}
			return m.triggerOpenUser(u.ID), true
		},
	}
}

// drillToFirstOpenTarget is a generic Drill closure that opens the
// cursored row's first OpenTarget in the user's default browser.
//
// Use for surfaces that don't have a useful in-TUI detail tab — the
// natural drill is "take me to where this thing lives in
// Salesforce." Notifications, limit rows, deploy rows that point at
// a Lightning page, etc.
//
// Returns ok=false when there's nothing under the cursor (empty
// list, loading state) so the dispatcher falls through to whatever
// next handler exists.
func drillToFirstOpenTarget(m *Model) (tea.Cmd, bool) {
	surf := m.resolveOpenSurface()
	if surf == nil || surf.Openable == nil {
		return nil, false
	}
	target := surf.Openable(*m)
	if target == nil {
		return nil, false
	}
	targets := target.Targets()
	if len(targets) == 0 {
		return nil, false
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil, false
	}
	m.recordRecentVisit(o.Username, target)
	m.flash("opening " + targets[0].Label + "…")
	return m.openInBrowserCmd(o, targets[0]), true
}

// ---- surfaces migrated from cursorOpenable's legacy tab switch ----
//
// Same init() indirection as above (see the file header comment):
// these closures reach Model methods that transitively touch the
// registry, so direct var initializers would cycle.

var (
	homeFallbackOpenSurface      openSurface
	objectRecordsOpenSurface     openSurface
	reportsOpenSurface           openSurface
	dashboardsOpenSurface        openSurface
	reportTypesOpenSurface       openSurface
	reportDetailOpenSurface      openSurface
	flowDetailOpenSurface        openSurface
	flowVersionDetailOpenSurface openSurface
	soqlOpenSurface              openSurface
	recordsTabOpenSurface        openSurface
	packagesOpenSurface          openSurface
	setupOpenSurface             openSurface
	permParentOpenSurface        openSurface
)

func init() {
	// Home subtabs without their own open surface (Landing, Downloads,
	// placeholders): o opens Lightning home for the org.
	homeFallbackOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if o, ok := m.currentOrg(); ok {
				return o
			}
			return nil
		},
	}

	// Records subtab of the object drill. The openable is built from
	// filtered record-list rows + a synthetic attributes.type, which
	// doesn't fit the Identity tuple shape.
	objectRecordsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.DescribeCur == "" {
				return nil
			}
			idx := recordsCursorDisplay(d, d.DescribeCur)
			rec, ok := currentRecordAt(d, d.DescribeCur, idx)
			if !ok {
				return nil
			}
			// List-view rows don't carry the standard `attributes`
			// block, so RecordRef's URL-parser would fail without a
			// synthetic attributes.type.
			if _, ok := rec["attributes"]; !ok {
				if _, hasID := rec["Id"]; hasID {
					rec = copyRecordWithAttrs(rec, d.DescribeCur)
				}
			}
			return m.newRecordRef(rec)
		},
	}

	// /reports: cursor lives in d.Cursors keyed by current folder, not
	// in d.ReportList (which holds the full unscoped row set). Resolve
	// via visibleReportsItems so o opens what the user sees. Folder
	// rows return nil — folders have no canonical Lightning URL.
	reportsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.ReportFolders == nil {
				return nil
			}
			subs, reps := m.visibleReportsItems()
			row := m.reportsRowCursor()
			if row < len(subs) {
				return nil
			}
			reportIdx := row - len(subs)
			if reportIdx >= 0 && reportIdx < len(reps) {
				return reps[reportIdx]
			}
			return nil
		},
	}

	// /reports Dashboards subtab: plain ListView selection.
	dashboardsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.DashboardList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	// /reports Report Types subtab: rows only carry a Setup-home
	// URL (report types have no per-record Lightning page).
	reportTypesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.ReportTypeList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	// /meta Browse: Enter drills into the type's component list; o
	// has no target on the catalogue itself (types aren't pages).
	metaBrowseOpenSurface = openSurface{
		Drill: func(m *Model) (tea.Cmd, bool) {
			return m.drillIntoMetaType(), true
		},
	}

	cmtOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.CMTList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	customLabelsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.CustomLabelList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	customSettingsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.CustomSettingList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	staticResourcesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.StaticResourceList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	namedCredsOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.NamedCredList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	remoteSitesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if row, ok := d.RemoteSiteList.Selected(); ok {
					return row
				}
			}
			return nil
		},
	}

	// /report drill: same target list as the list-row form.
	reportDetailOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.ReportCur == "" {
				return nil
			}
			for _, r := range d.Reports.Value() {
				if r.ID == d.ReportCur {
					return r
				}
			}
			return sf.ReportSummary{ID: d.ReportCur}
		},
	}

	// /flow drill: open the cursored version row.
	flowDetailOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.FlowCur == "" {
				return nil
			}
			r, ok := d.FlowVersions[d.FlowCur]
			if !ok {
				return nil
			}
			versions := r.Value()
			if len(versions) == 0 {
				return nil
			}
			idx := d.Cursors.Get(cursorKindFlowVersion, len(versions), d.FlowCur)
			return versions[idx]
		},
	}

	// Flow-version viewer: o still opens THIS version in Flow Builder
	// (the in-terminal JSON is for reading; the browser is for editing).
	flowVersionDetailOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil || d.FlowCur == "" || d.FlowVersionCur == "" {
				return nil
			}
			r, ok := d.FlowVersions[d.FlowCur]
			if !ok {
				return nil
			}
			for _, v := range r.Value() {
				if v.ID == d.FlowVersionCur {
					return v
				}
			}
			return nil
		},
	}

	// /soql: the cursored result row as a record ref.
	soqlOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if len(m.soqlResult.Records) == 0 {
				return nil
			}
			rec, ok := m.soqlSelectedRecord()
			if !ok {
				return nil
			}
			return m.newRecordRef(rec)
		},
	}

	// /records: picker mode opens the cursored sObject (same openable
	// as /objects); record-list mode opens the visible (filtered)
	// record under the cursor.
	recordsTabOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil {
				return nil
			}
			if d.RecordsSObjectCur == "" {
				if s, ok := d.SObjectList.Selected(); ok {
					return s
				}
				return nil
			}
			idx := recordsCursorDisplay(d, d.RecordsSObjectCur)
			rec, ok := currentRecordAt(d, d.RecordsSObjectCur, idx)
			if !ok {
				return nil
			}
			if _, hasAttrs := rec["attributes"]; !hasAttrs {
				if _, hasID := rec["Id"]; hasID {
					rec = copyRecordWithAttrs(rec, d.RecordsSObjectCur)
				}
			}
			return m.newRecordRef(rec)
		},
	}

	packagesOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil {
				return nil
			}
			if p, ok := d.PackageList.Selected(); ok {
				return p
			}
			return nil
		},
	}

	setupOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if l, ok := m.setupList.Selected(); ok {
				return l
			}
			return nil
		},
	}

	// /perm drill: on Overview (and any subtab without a row-level
	// Openable of its own), o opens the parent permset/PSG/profile in
	// Setup.
	permParentOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			d := m.activeOrgData()
			if d == nil {
				return nil
			}
			switch d.PermParentKind {
			case "permset":
				for _, p := range d.PermSets.Value() {
					if p.ID == d.PermParentID {
						return p
					}
				}
			case "psg":
				for _, g := range d.PSGs.Value() {
					if g.ID == d.PermParentID {
						return g
					}
				}
			case "profile":
				for _, p := range d.Profiles.Value() {
					if p.ID == d.PermParentID {
						return p
					}
				}
			}
			return nil
		},
	}
}
