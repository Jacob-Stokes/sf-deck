package ui

// Client-side "recently visited" tracking — UI shell.
//
// The data types + pure-logic transforms (merge, sort, filter, kind
// mapping, formatters, settings round-trip) live in internal/recent.
// This file keeps the UI-coupled pieces: per-tab visit closures
// wired into TabSpec.RecordRecentVisit, plus the orgData mutators
// that touch d.Recent and trigger persistence.
//
// Type/const aliases below preserve the legacy ui-package names so
// 11 sibling files don't need rewriting. New code should reference
// the recent.* names directly.

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

// Thin shims for the recent-package pure helpers. Kept as functions
// (not aliases) so they retain their ui-package visibility shape and
// callers don't need to thread the recent. qualifier.

func recentNameForRow(r RecentEntry) string   { return recent.NameForRow(r) }
func recentDetailForRow(r RecentEntry) string { return recent.DetailForRow(r) }
func entryKindLabel(kind string) string       { return recent.KindLabel(kind) }
func upsertRecent(list []RecentEntry, entry RecentEntry, cap int) []RecentEntry {
	return recent.Upsert(list, entry, cap)
}

// --- UI-coupled helpers (mutate orgData, talk to settings) ---------------

// rememberRecentRecord records a visit to a Salesforce record.
// Convenience wrapper that delegates to rememberRecent with
// Kind=record so existing call sites don't need to change.
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

// --- Recent-visit closures used by TabSpec.RecordRecentVisit -----------

// recentVisitRecordDetail captures a record drill on TabRecordDetail.
// d.RecordDetailCur is "<sobject>:<id>". Resolves the display name
// from the cached detail map when possible; falls back to the id
// alone when the detail isn't loaded yet.
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

// recentVisitObjectDetail captures an sObject drill on TabObjectDetail.
// Identity uses the API name as id (mirrors what recordRecentVisit
// does for sf.SObject).
func recentVisitObjectDetail(m *Model, d *orgData, orgUser string) {
	if d.DescribeCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindSObject, d.DescribeCur, d.DescribeCur, "")
}

// recentVisitFieldDetail captures a field drill on TabFieldDetail.
func recentVisitFieldDetail(m *Model, d *orgData, orgUser string) {
	if d.DescribeCur == "" || d.FieldCur == "" {
		return
	}
	id := d.DescribeCur + "." + d.FieldCur
	m.rememberRecent(orgUser, RecentKindField, id, d.FieldCur, d.DescribeCur)
}

// recentVisitFlowDetail captures a flow drill on TabFlowDetail.
// Resolves the human label from the cached flow list when available.
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

// recentVisitReportDetail captures a report drill on TabReportDetail.
// Name resolution is deferred — the renderer falls back to id when
// Name is blank, and the treechip registry doesn't expose a by-id
// lookup yet.
func recentVisitReportDetail(m *Model, d *orgData, orgUser string) {
	if d.ReportCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindReport, d.ReportCur, "", "")
}

// recentVisitBundleDetail captures an LWC / Aura bundle drill on
// TabBundleDetail. Bundle id doubles as the display name.
func recentVisitBundleDetail(m *Model, d *orgData, orgUser string) {
	if m.bundleCur == "" {
		return
	}
	m.rememberRecent(orgUser, RecentKindLWC, m.bundleCur, m.bundleCur, "")
}

// recentVisitPermParentDetail captures a permset / PSG / profile drill
// on TabPermParentDetail. Maps the kind to the right RecentKind*
// constant. Name resolution is deferred (same reasoning as reports).
// recentVisitUserDetail captures a user drill on TabUserDetail.
// Closes the ratchet exemption from 2026-06-13 — drills now feed
// the recents stream the same way o (browser open) always did.
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

// recentVisitLWCDetail captures a component drill on TabLWCDetail —
// which serves BOTH kinds; the drilled id lives in whichever bundle
// list contains it. Without this, only `o` (browser open) recorded
// component visits, so the Recently-viewed chip ignored Enter drills
// entirely (field bug 2026-06-12).
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
	// Keep the renderer-facing wrapper in sync with the underlying
	// slice. Without this, the /recent tab shows a stale list until
	// some other event triggers SyncListViews — manifested as "press
	// tab a few times, then it appears."
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

// loadRecent restores the per-org recent list from settings on first
// access. Called lazily from ensureOrgData so newly-attached orgs
// pick up their persisted history without a refactor of the boot path.
func loadRecent(m *Model, d *orgData, orgUser string) {
	if m.settings == nil {
		return
	}
	cfgs := m.settings.RecentForOrg(orgUser)
	d.Recent = recent.FromConfig(cfgs, orgUser)
}
