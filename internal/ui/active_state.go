package ui

// Accessors for per-org UI state (active tab + view-chip / subtab
// cursors). These live on orgData so switching orgs preserves where
// the user was; before any org is selected, reads/writes go to
// Model.noOrgTab etc.
//
// Keep this file minimal — it's the seam for "per-org context."

// tab returns the active Tab for the currently-selected org, or the
// global noOrg fallback if no org is selected yet.
func (m Model) tab() Tab {
	if m.tabOverrideSet {
		return m.tabOverride
	}
	if d := m.activeOrgData(); d != nil {
		return d.Tab
	}
	return m.noOrgTab
}

// setTab writes the active Tab to the currently-selected org's state
// (or noOrgTab if no org is selected). Allocates orgData if needed so
// the per-org context survives the first tab change. Also records the
// tab under its stem so number-key nav can restore drill-in state when
// the user comes back to a family.
func (m *Model) setTab(t Tab) {
	// Maintain the overflow slot: when the new tab's stem isn't on
	// the pinned bar, stash it in slot 0 so the user has a one-key
	// way back. Drill tabs (FieldDetail, etc.) inherit their stem's
	// pinned-ness, so drilling INTO an overflow tab keeps slot 0
	// pointing at the parent.
	stem := m.stemForTab(t)
	if !isPinnedTab(stem) {
		m.overflowTab = stem
		m.overflowSet = true
	}
	// Auto-collapse the left rail when navigating to a different
	// tab — but only if the user didn't pin it open with `ctrl+\`. The
	// rail was opened transiently to pick / inspect orgs; once
	// they navigate elsewhere it should get out of the way.
	if !m.leftPinned && m.leftOpen {
		m.leftOpen = false
		if m.focus == focusOrgs {
			m.focus = focusMain
		}
	}

	if len(m.orgs) == 0 {
		m.noOrgTab = t
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.Tab = t
	if d.LastTabInStem == nil {
		d.LastTabInStem = map[Tab]Tab{}
	}
	// Skip transient drill-ins. LastTabInStem powers "number key
	// restores the last view in this family" — useful for per-entity
	// drills (sObject schema, flow detail, apex class detail) where
	// the user might genuinely want to bounce back. Useless and
	// surprising for transient drills (one-record /record, one-field
	// detail, one-trigger detail) where the drill is gone the moment
	// the user moves on. Recording those means later number-key
	// nav from another stem teleports the user into a stale row
	// they'd already mentally closed.
	if !isTransientDrill(t) {
		d.LastTabInStem[stem] = t
	}
}

// isTransientDrill reports whether t is a drill-in that should not
// be remembered by LastTabInStem. Per-row / per-event detail tabs
// belong here; per-entity drills (TabObjectDetail, TabFlowDetail,
// TabApexDetail, TabUserDetail, TabPermParentDetail, etc.) do not —
// users expect "press the number key, return to that entity I was
// inspecting." See setTab for the broader rationale.
func isTransientDrill(t Tab) bool {
	spec := lookupTabSpec(t)
	return spec != nil && spec.TransientDrill
}

// isPinnedTab reports whether t is currently on the number bar
// (slots 1-8). Falls back to defaults when the user hasn't
// customised the bar.
func isPinnedTab(t Tab) bool {
	for _, p := range TabsForNumbers() {
		if p == t {
			return true
		}
	}
	return false
}

// resolveStem returns the tab to land on when the user invokes the
// given stem (e.g. via the number-key for Objects). Prefers the last
// tab the user was on in that family; falls back to the stem itself.
func (m Model) resolveStem(stem Tab) Tab {
	if d := m.activeOrgData(); d != nil {
		if prev, ok := d.LastTabInStem[stem]; ok && m.stemForTab(prev) == stem {
			return prev
		}
	}
	return stem
}

// stemForTab returns the top-level family for t, including detail tabs
// whose family depends on where the drill started.
//
// For TabRecordDetail and TabTriggerDetail the static Tab.stem() can't
// know where the drill came from — both are reusable from many parent
// tabs (/records, /soql, /reports, /recent for records; /apex,
// /object-detail for triggers). Reading the per-drill return tab off
// the Model and stemming from there is what keeps the tab strip
// highlighted on the parent the user actually came from — without
// it, drilling from /soql looks like the user teleported to /records.
func (m Model) stemForTab(t Tab) Tab {
	// `back != 0` is intentionally not a guard here: TabHome (iota 0)
	// is a valid drill origin. The only thing that's not a valid
	// return target is the drill-in itself (would loop). Tab.stem()
	// is the static fallback when no per-drill state has been set.
	//
	// Records + Triggers are the ONLY drill tabs whose top-strip
	// highlight follows the originator (so drilling a record from
	// /soql keeps the strip on /soql).  Every other detail tab (flow,
	// sobject, apex, lwc, perm parent, etc.) shows the resource's
	// natural family on the strip — drilling a flow from /home shows
	// /flows in the strip, even though Esc takes you back to /home.
	// Top-strip = "where is this resource conceptually"; Esc = "where
	// did I come from."  Two different concerns; only Records +
	// Triggers conflate them, by design.
	if t == TabTriggerDetail {
		if back := m.triggerDetailBackTab(); back != TabTriggerDetail {
			return back.stem()
		}
	}
	if t == TabRecordDetail {
		if back := m.recordDetailReturnTab; back != TabRecordDetail {
			return back.stem()
		}
	}
	return t.stem()
}

// objectsChipIdx / recordsChipIdx / objectSubtab mirror the old
// Model-level fields but now read/write per-org. When no org is
// selected they return 0 / no-op (the corresponding views don't
// render in that state anyway).
func (m Model) objectsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ObjectsChipIdx
	}
	return 0
}

func (m *Model) setObjectsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ObjectsChipIdx = i
}

func (m Model) recordsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.RecordsChipIdx
	}
	return 0
}

func (m *Model) setRecordsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).RecordsChipIdx = i
}

func (m Model) flowsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.FlowsChipIdx
	}
	return 0
}

func (m *Model) setFlowsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).FlowsChipIdx = i
}

func (m Model) apexChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ApexChipIdx
	}
	return 0
}

func (m *Model) setApexChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ApexChipIdx = i
}

func (m Model) apexTriggersChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ApexTriggersChipIdx
	}
	return 0
}

func (m *Model) setApexTriggersChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ApexTriggersChipIdx = i
}

func (m Model) lwcChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.LWCChipIdx
	}
	return 0
}

func (m *Model) setLWCChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).LWCChipIdx = i
}

func (m Model) auraChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.AuraChipIdx
	}
	return 0
}

func (m *Model) setAuraChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).AuraChipIdx = i
}

func (m Model) permsetsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.PermSetsChipIdx
	}
	return 0
}

func (m *Model) setPermSetsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).PermSetsChipIdx = i
}

func (m Model) psgsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.PSGsChipIdx
	}
	return 0
}

func (m *Model) setPSGsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).PSGsChipIdx = i
}

func (m Model) profilesChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ProfilesChipIdx
	}
	return 0
}

func (m *Model) setProfilesChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ProfilesChipIdx = i
}

func (m Model) queuesChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.QueuesChipIdx
	}
	return 0
}

func (m *Model) setQueuesChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).QueuesChipIdx = i
}

func (m Model) publicGroupsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.PublicGroupsChipIdx
	}
	return 0
}

func (m *Model) setPublicGroupsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).PublicGroupsChipIdx = i
}

func (m Model) soqlSavedChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.SOQLSavedChipIdx
	}
	return 0
}

func (m *Model) setSOQLSavedChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).SOQLSavedChipIdx = i
}

func (m Model) soqlHistoryChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.SOQLHistoryChipIdx
	}
	return 0
}

func (m *Model) setSOQLHistoryChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).SOQLHistoryChipIdx = i
}

func (m Model) recentChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.RecentChipIdx
	}
	return 0
}

func (m *Model) setRecentChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).RecentChipIdx = i
}

func (m Model) allUsersChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.AllUsersChipIdx
	}
	return 0
}

func (m *Model) setAllUsersChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.AllUsersChipIdx = i
	// Resolve the chip ID against the live registry so listSurface
	// closures (which only have d) can read the active chip without
	// re-walking the strip. See activeUsersChipID for the rationale.
	if m.chipRegistry(domainUsers) != nil {
		chips := m.chipRegistry(domainUsers).ChipsFor("*")
		if i >= 0 && i < len(chips) {
			d.ActiveUsersChipID = chips[i].ID
		}
	}
}

func (m Model) objectSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.ObjectSubtab
	}
	return 0
}

func (m *Model) setObjectSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ObjectSubtab = i
}

func (m Model) homeSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.HomeSubtab
	}
	return 0
}

func (m *Model) setHomeSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.HomeSubtab = i
}

func (m Model) devProjectsSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.DevProjectsSubtab
	}
	return 0
}

func (m *Model) setDevProjectsSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.DevProjectsSubtab = i
}

func (m Model) devProjectDetailSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.DevProjectDetailSubtab
	}
	return 0
}

func (m *Model) setDevProjectDetailSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	d.DevProjectDetailSubtab = i
}

func (m Model) permsDashboardSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.PermsDashboardSubtab
	}
	return 0
}

func (m *Model) setPermsDashboardSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).PermsDashboardSubtab = i
}

func (m Model) systemSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.SystemSubtab
	}
	return 0
}

func (m *Model) setSystemSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).SystemSubtab = i
}

func (m Model) apexSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.ApexSubtab
	}
	return 0
}

func (m *Model) setApexSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ApexSubtab = i
}

func (m Model) componentsSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.ComponentsSubtab
	}
	return 0
}

func (m *Model) setComponentsSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ComponentsSubtab = i
}

func (m Model) metaSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.MetaSubtab
	}
	return 0
}

func (m *Model) setMetaSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).MetaSubtab = i
}

func (m Model) reportsSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.ReportsSubtab
	}
	return 0
}

func (m *Model) setReportsSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ReportsSubtab = i
}

func (m Model) usersSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.UsersSubtab
	}
	return 0
}

func (m *Model) setUsersSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).UsersSubtab = i
}

func (m Model) permParentSubtab() int {
	if d := m.activeOrgData(); d != nil {
		return d.PermParentSubtab
	}
	return 0
}

func (m *Model) setPermParentSubtab(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).PermParentSubtab = i
}

// activeOrgData returns the orgData for the currently-selected org,
// or nil if no org is selected yet. Non-allocating — for read paths.
// Setters above use ensureOrgData instead so the entry exists before
// they write to it.
func (m Model) activeOrgData() *orgData {
	if len(m.orgs) == 0 {
		return nil
	}
	return m.data[m.orgs[m.selected].Username]
}

func (m Model) dashboardsChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.DashboardsChipIdx
	}
	return 0
}

func (m *Model) setDashboardsChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).DashboardsChipIdx = i
}

func (m Model) reportTypesChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ReportTypesChipIdx
	}
	return 0
}

func (m *Model) setReportTypesChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ReportTypesChipIdx = i
}

func (m Model) deploysChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.DeploysChipIdx
	}
	return 0
}

func (m *Model) setDeploysChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).DeploysChipIdx = i
}

func (m Model) activeUsersChipIdx() int {
	if d := m.activeOrgData(); d != nil {
		return d.ActiveUsersChipIdx
	}
	return 0
}

func (m *Model) setActiveUsersChipIdx(i int) {
	if len(m.orgs) == 0 {
		return
	}
	m.ensureOrgData(m.orgs[m.selected].Username).ActiveUsersChipIdx = i
}
