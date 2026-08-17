package ui

// Open surface registry — second migration step on the path to a
// declarative TabSpec. Companion to chipSurface (chip_surface.go)
// and listSurface (list_surface.go).

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"

	tea "charm.land/bubbletea/v2"
)

type openSurface struct {
	Openable func(m Model) sf.Openable

	// Drill is the Enter handler. Returns the (target tab, cmd)
	// pair to apply, plus ok=false if Enter should be a no-op
	// (e.g. cursor on an unloaded row, no detail tab for this kind
	// yet). The closure mutates per-surface state on the *Model
	// (DescribeCur / FlowCur / ApexCur etc.) before returning the
	// target tab — same pattern the legacy activate() switch used.
	Drill func(m *Model) (tea.Cmd, bool)
}

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

	apexTriggersOpenSurface = openSurface{
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

	deploysOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if d := m.activeOrgData(); d != nil {
				if v, ok := d.DeployList.Selected(); ok {
					return v
				}
			}
			return nil
		},
		Drill: func(m *Model) (tea.Cmd, bool) {
			return m.drillIntoDeploy(), true
		},
	}

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
	homeFallbackOpenSurface = openSurface{
		Openable: func(m Model) sf.Openable {
			if o, ok := m.currentOrg(); ok {
				return o
			}
			return nil
		},
	}

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
			if _, ok := rec["attributes"]; !ok {
				if _, hasID := rec["Id"]; hasID {
					rec = copyRecordWithAttrs(rec, d.DescribeCur)
				}
			}
			return m.newRecordRef(rec)
		},
	}

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
