package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/recent"
)

func rememberDrillReturn(d *orgData, detailTab, originator Tab) {
	if d == nil || detailTab == originator {
		return
	}
	if d.DrillReturnTab == nil {
		d.DrillReturnTab = map[Tab]Tab{}
	}
	d.DrillReturnTab[detailTab] = originator
}

func drillByKind(m *Model, kind, ref, typeField, name string, originator Tab) (tea.Cmd, bool) {
	if len(m.orgs) == 0 || ref == "" {
		return nil, false
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	switch kind {

	case recent.KindSObject:
		d.DescribeCur = ref
		rememberDrillReturn(d, TabObjectDetail, originator)
		m.setTab(TabObjectDetail)
		return m.onTabChanged(), true

	case recent.KindField:
		sobj := typeField
		fname := ref
		if i := indexOfRune(fname, '.'); i >= 0 {
			sobj = fname[:i]
			fname = fname[i+1:]
		}
		if sobj == "" {
			return nil, false
		}
		d.DescribeCur = sobj
		d.FieldCur = fname
		m.fieldActionCur = 0
		rememberDrillReturn(d, TabFieldDetail, originator)
		m.setTab(TabFieldDetail)
		return tea.Batch(m.onTabChanged(), m.ensureFieldDescriptionCmd()), true

	case recent.KindRecord:
		return m.triggerRecordDrill(typeField, ref, name, originator), true

	case recent.KindFlow:
		d.FlowCur = ref
		rememberDrillReturn(d, TabFlowDetail, originator)
		m.setTab(TabFlowDetail)
		return m.onTabChanged(), true

	case recent.KindApexClass:
		rememberDrillReturn(d, TabApexDetail, originator)
		return m.triggerOpenApexClass(ref), true

	case recent.KindLWC:
		rememberDrillReturn(d, TabLWCDetail, originator)
		return m.triggerOpenLWCBundle(ref), true

	case recent.KindAura:
		rememberDrillReturn(d, TabLWCDetail, originator)
		return m.triggerOpenAuraBundle(ref), true

	case recent.KindReport:
		d.ReportCur = ref
		rememberDrillReturn(d, TabReportDetail, originator)
		m.setTab(TabReportDetail)
		return m.onTabChanged(), true

	case recent.KindPermSet:
		d.PermParentKind = "permset"
		d.PermParentID = ref
		d.PermParentPermSetID = ref
		rememberDrillReturn(d, TabPermParentDetail, originator)
		m.setTab(TabPermParentDetail)
		return m.onTabChanged(), true

	case recent.KindPermSetGroup:
		d.PermParentKind = "psg"
		d.PermParentID = ref
		d.PermParentPermSetID = ""
		rememberDrillReturn(d, TabPermParentDetail, originator)
		m.setTab(TabPermParentDetail)
		return m.onTabChanged(), true

	case recent.KindProfile:
		d.PermParentKind = "profile"
		d.PermParentID = ref
		d.PermParentPermSetID = typeField
		rememberDrillReturn(d, TabPermParentDetail, originator)
		m.setTab(TabPermParentDetail)
		return m.onTabChanged(), true

	case recent.KindQueue:
		d.GroupMemberKind = "queue"
		d.GroupMemberID = ref
		rememberDrillReturn(d, TabQueueDetail, originator)
		m.setTab(TabQueueDetail)
		return m.onTabChanged(), true

	case recent.KindPublicGroup:
		d.GroupMemberKind = "public_group"
		d.GroupMemberID = ref
		rememberDrillReturn(d, TabPublicGroupDetail, originator)
		m.setTab(TabPublicGroupDetail)
		return m.onTabChanged(), true

	case recent.KindUser:
		rememberDrillReturn(d, TabUserDetail, originator)
		return m.triggerOpenUser(ref), true

	case string(devproject.KindFlowVersion):
		if typeField == "" {
			return nil, false
		}
		d.FlowCur = typeField
		rememberDrillReturn(d, TabFlowDetail, originator)
		m.setTab(TabFlowDetail)
		return m.onTabChanged(), true

	case string(devproject.KindValidationRule):
		d.DescribeCur = typeField
		d.ValidationRules.DrillID = ref
		m.validationActionCur = 0
		rememberDrillReturn(d, TabValidationDetail, originator)
		m.setTab(TabValidationDetail)
		return m.onTabChanged(), true

	case string(devproject.KindRecordType):
		d.DescribeCur = typeField
		d.RecordTypes.DrillID = ref
		m.recordTypeActionCur = 0
		rememberDrillReturn(d, TabRecordTypeDetail, originator)
		m.setTab(TabRecordTypeDetail)
		return m.onTabChanged(), true

	case string(devproject.KindApexTrigger):
		rememberDrillReturn(d, TabTriggerDetail, originator)
		return m.triggerDetailDrill(typeField, ref, originator), true

	case recent.KindDashboard,
		recent.KindListView,
		recent.KindPackage,
		recent.KindDeploy,
		recent.KindApexLog:
		return nil, false

	// ---- Devproject-only kinds that drillByKind doesn't own ----
	// KindSOQLQuery is handled by openItemFromDevProject directly
	// because it loads the query body into the soql editor — needs
	// devproject store + editor state the dispatcher shouldn't pull
	// in.  KindApexSnippet isn't wired anywhere yet (drill is a
	// no-op both pre- and post-refactor); a fall-through here keeps
	// it from accidentally hitting the default `return nil, false`
	// silently — explicit case + comment so the gap is visible.
	case string(devproject.KindSOQLQuery), string(devproject.KindApexSnippet):
		return nil, false
	}
	return nil, false
}
