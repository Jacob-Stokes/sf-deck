package ui

// Client-side "recently visited" tracking — UI shell.

import (
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/recent"
)

// --- Aliases (preserve legacy ui-package names) --------------------------

// RecentEntry aliases recent.Entry so the dozens of existing call
// sites (chip predicates, list renderers, merged-stream wiring)
// don't need to change.
type RecentEntry = recent.Entry

// Origin + Kind constants — re-exported from internal/recent so existing
// switch arms (e.g. `case RecentKindRecord:`) keep compiling.
const (
	RecentOriginDeck = recent.OriginDeck
	RecentOriginSF   = recent.OriginSF
	RecentOriginBoth = recent.OriginBoth

	RecentKindRecord       = recent.KindRecord
	RecentKindReport       = recent.KindReport
	RecentKindDashboard    = recent.KindDashboard
	RecentKindListView     = recent.KindListView
	RecentKindFlow         = recent.KindFlow
	RecentKindApexClass    = recent.KindApexClass
	RecentKindLWC          = recent.KindLWC
	RecentKindAura         = recent.KindAura
	RecentKindSObject      = recent.KindSObject
	RecentKindField        = recent.KindField
	RecentKindPermSet      = recent.KindPermSet
	RecentKindPermSetGroup = recent.KindPermSetGroup
	RecentKindProfile      = recent.KindProfile
	RecentKindUser         = recent.KindUser
	RecentKindDeploy       = recent.KindDeploy
	RecentKindPackage      = recent.KindPackage
	RecentKindQueue        = recent.KindQueue
	RecentKindPublicGroup  = recent.KindPublicGroup
	RecentKindApexLog      = recent.KindApexLog
)

func recentNameForRow(r RecentEntry) string   { return recent.NameForRow(r) }
func recentDetailForRow(r RecentEntry) string { return recent.DetailForRow(r) }
func entryKindLabel(kind string) string       { return recent.KindLabel(kind) }
func upsertRecent(list []RecentEntry, entry RecentEntry, cap int) []RecentEntry {
	return recent.Upsert(list, entry, cap)
}

func (m *Model) rememberRecentRecord(orgUser, sobject, id, name string) {
	m.rememberRecent(orgUser, RecentKindRecord, id, name, sobject)
}

// recordDrillInForCurrentTab inspects the active tab + per-org cursor
// state and records a recent-visit entry when the user just landed on
// a detail tab. Called from onTabChanged so every transition into a
// drill surface (record, object, field, flow, report, bundle,
// permset/psg/profile) lands in the recent log without each detail
// tab needing its own hook.
//
// Re-entries on the same row are idempotent — rememberRecent dedupes
// on (Kind, ID), so flipping back and forth across tabs just bumps
// VisitedAt rather than spamming the list.
func (m *Model) recordDrillInForCurrentTab() {
	if len(m.orgs) == 0 {
		return
	}
	o := m.orgs[m.selected]
	d := m.data[o.Username]
	if d == nil {
		return
	}
	spec := lookupTabSpec(m.tab())
	if spec == nil || spec.RecordRecentVisit == nil {
		return
	}
	spec.RecordRecentVisit(m, d, o.Username)
}

func recentVisitRecordDetail(m *Model, d *orgData, orgUser string) {
	key := d.RecordDetailCur
	if key == "" {
		return
	}
	colon := strings.IndexByte(key, ':')
	if colon < 0 {
		return
	}
	sobject := key[:colon]
	id := key[colon+1:]
	name := id
	if d.RecordDetails != nil {
		if r := d.RecordDetails[key]; r != nil {
			if v := r.Value(); v != nil {
				name = recordDisplayName(v)
			}
		}
	}
	m.rememberRecent(orgUser, RecentKindRecord, id, name, sobject)
}

func recentVisitObjectDetail(m *Model, d *orgData, orgUser string) {
	if d.DescribeCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindSObject, d.DescribeCur, d.DescribeCur, "")
}

func recentVisitFieldDetail(m *Model, d *orgData, orgUser string) {
	if d.DescribeCur == "" || d.FieldCur == "" {
		return
	}
	id := d.DescribeCur + "." + d.FieldCur
	m.rememberRecent(orgUser, RecentKindField, id, d.FieldCur, d.DescribeCur)
}

func recentVisitFlowDetail(m *Model, d *orgData, orgUser string) {
	if d.FlowCur == "" {
		return
	}
	name := d.FlowCur
	if list := d.FlowList.Items(); len(list) > 0 {
		for _, f := range list {
			if f.DefinitionID == d.FlowCur {
				if f.MasterLabel != "" {
					name = f.MasterLabel
				} else if f.DeveloperName != "" {
					name = f.DeveloperName
				}
				break
			}
		}
	}
	m.rememberRecent(orgUser, RecentKindFlow, d.FlowCur, name, "")
}

func recentVisitReportDetail(m *Model, d *orgData, orgUser string) {
	if d.ReportCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindReport, d.ReportCur, "", "")
}

func recentVisitBundleDetail(m *Model, d *orgData, orgUser string) {
	if m.bundleCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindLWC, m.bundleCur, m.bundleCur, "")
}

func recentVisitUserDetail(m *Model, d *orgData, orgUser string) {
	id := d.UserCur
	if id == "" {
		return
	}
	row := m.cursoredUserRow(d, id)
	if row.ID == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindUser, row.ID, row.Name, row.Username)
}

func recentVisitLWCDetail(m *Model, d *orgData, orgUser string) {
	id := d.LWCCur
	if id == "" {
		return
	}
	for _, b := range d.LWCBundleList.Items() {
		if b.ID == id {
			name := b.MasterLabel
			if name == "" {
				name = b.DeveloperName
			}
			m.rememberRecent(orgUser, RecentKindLWC, id, name, b.DeveloperName)
			return
		}
	}
	for _, b := range d.AuraBundleList.Items() {
		if b.ID == id {
			name := b.MasterLabel
			if name == "" {
				name = b.DeveloperName
			}
			m.rememberRecent(orgUser, RecentKindAura, id, name, b.DeveloperName)
			return
		}
	}
}

// recentVisitApexDetail captures a class drill on TabApexDetail —
// same gap as LWC: drills never fed the Recent chip.
func recentVisitApexDetail(m *Model, d *orgData, orgUser string) {
	id := d.ApexCur
	if id == "" {
		return
	}
	for _, a := range d.ApexClassList.Items() {
		if a.ID == id {
			m.rememberRecent(orgUser, RecentKindApexClass, id, a.Name, a.NamespacePrefix)
			return
		}
	}
}

func recentVisitPermParentDetail(m *Model, d *orgData, orgUser string) {
	if d.PermParentID == "" {
		return
	}
	var kind string
	switch d.PermParentKind {
	case "permset":
		kind = RecentKindPermSet
	case "psg":
		kind = RecentKindPermSetGroup
	case "profile":
		kind = RecentKindProfile
	default:
		return
	}
	m.rememberRecent(orgUser, kind, d.PermParentID, d.PermParentID, "")
}

// rememberRecent is the generic entry recorder — every kind funnels
// through here. Idempotent on (Kind, ID): same item visited twice in
// a row just bumps VisitedAt; older duplicates are dropped during
// upsert. After mutating d.Recent we sync the wrapping ListView and
// fire persistRecent so settings reflects the new MRU order.
func (m *Model) rememberRecent(orgUser, kind, id, name, secondary string) {
	if id == "" || orgUser == "" || kind == "" {
		return
	}
	d := m.data[orgUser]
	if d == nil {
		return
	}
	entry := RecentEntry{
		Kind:      kind,
		ID:        id,
		Name:      strings.TrimSpace(name),
		Type:      secondary,
		OrgUser:   orgUser,
		VisitedAt: time.Now(),
	}
	d.Recent = upsertRecent(d.Recent, entry, m.settings.RecentMaxEntries())
	d.recentGen++
	d.RecentList.Set(d.Recent)
	persistRecent(m, orgUser, d.Recent)
}

// persistRecent writes the current recent list to settings.toml.
// The in-memory list remains authoritative for the session; save
// failures are surfaced through the status flash.
func persistRecent(m *Model, orgUser string, list []RecentEntry) {
	if m.settings == nil {
		return
	}
	m.settings.SetRecentForOrg(orgUser, recent.ToConfig(list))
	m.saveSettings("")
}

func loadRecent(m *Model, d *orgData, orgUser string) {
	if m.settings == nil {
		return
	}
	cfgs := m.settings.RecentForOrg(orgUser)
	d.Recent = recent.FromConfig(cfgs, orgUser)
}
