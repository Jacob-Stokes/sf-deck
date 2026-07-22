package ui

// recent_service.go — /home Recent two-mode source + the cross-tab
// "visited-set" helpers that drive the Recently Viewed chip on every
// other surface.
//
// /home → Recent picks ONE source based on d.HomeRecentMode:
//   - ChipModeLocal      → d.Recent (sf-deck visit log), surfaced via
//                          d.RecentList.
//   - ChipModeSalesforce → d.RecentlyViewed.Value() converted to
//                          []RecentEntry, surfaced via d.RecentSFList.
//
// Previously these were merged into a single stream with Origin
// glyphs.  The merge added a UI tax (decode the glyph to know the
// source) for a workflow users don't actually need.  Two distinct
// modes answer two distinct questions; L toggles between them.
//
// The cross-tab Recently Viewed chips (on /objects, /flows, /apex,
// /lwc, /aura, /perms, /queues, /public-groups) still want the
// UNION of both sources — "any record I've touched recently
// regardless of where" — so they consult both d.Recent and the
// converted SF payload via the helpers below.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/recent"
)

// syncRecentSFList rebuilds d.RecentSFList when the SF
// RecentlyViewed payload has changed since the last sync.  Called
// from the /home Recent surface's BuildRenderModel before reading
// the list; cheap O(1) when the generation hasn't advanced.
func (m Model) syncRecentSFList(orgUser string) {
	d := m.data[orgUser]
	if d == nil {
		return
	}
	if d.recentSFGen == d.recentGen && d.RecentSFList.Len() > 0 {
		return
	}
	rows := d.RecentlyViewed.Value()
	entries := make([]RecentEntry, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" || r.SObjectType == "" {
			continue
		}
		entries = append(entries, RecentEntry{
			Kind:      recent.KindForSFType(r.SObjectType),
			ID:        r.ID,
			Name:      r.Name,
			Type:      r.SObjectType,
			VisitedAt: r.LastViewedDate,
			OrgUser:   orgUser,
			Origin:    RecentOriginSF,
		})
	}
	d.RecentSFList.Set(entries)
	d.recentSFGen = d.recentGen
}

// activeRecentListPtr returns a pointer to the active ListView for
// /home Recent.  Surfaces wire SearchPtr / MoveCursor / ResetCursor
// through this so cursor / search state lives on whichever list is
// currently driving the view.
func activeRecentListPtr(d *orgData) *ListView[RecentEntry] {
	if d == nil {
		return nil
	}
	if d.HomeRecentMode == ChipModeSalesforce {
		return &d.RecentSFList
	}
	return &d.RecentList
}

// recentUnionStream returns the deduped union of the local visit
// log + the SF RecentlyViewed payload for visited-set lookups on
// cross-tab Recently Viewed chips.  Dedupes on (Kind, ID) preferring
// local entries (they carry the user's actual sf-deck visit
// timestamp, which is the better "when did I last touch this"
// signal).
//
// Allocates on every call.  Callers that consume it should derive
// their maps in-place rather than holding the slice; the slice is
// throwaway scratch.  Cached on orgData so per-frame readers can
// hit the same instance — invalidated when d.recentGen advances.
func recentUnionStream(d *orgData) []RecentEntry {
	if d == nil {
		return nil
	}
	type key struct{ kind, id string }
	seen := make(map[key]bool, len(d.Recent))
	out := make([]RecentEntry, 0, len(d.Recent))
	for _, e := range d.Recent {
		if e.ID == "" || e.Kind == "" {
			continue
		}
		seen[key{e.Kind, e.ID}] = true
		out = append(out, e)
	}
	for _, r := range d.RecentlyViewed.Value() {
		if r.ID == "" || r.SObjectType == "" {
			continue
		}
		k := recent.KindForSFType(r.SObjectType)
		if seen[key{k, r.ID}] {
			continue
		}
		out = append(out, RecentEntry{
			Kind:      k,
			ID:        r.ID,
			Name:      r.Name,
			Type:      r.SObjectType,
			VisitedAt: r.LastViewedDate,
			Origin:    RecentOriginSF,
		})
	}
	recent.SortMRU(out)
	return out
}

// salesforceVisitedRecordIDs returns the set of record IDs in
// Salesforce's per-sObject RecentlyViewed payload — EXCLUDING
// sf-deck's local visit log.  Used by the SF-source-mode "Recently
// Viewed" chip on /records so it shows exactly what Salesforce
// considers recently viewed for this sObject, which is what
// Lightning's `/lightning/o/<X>/list?filterName=Recent` shows.
//
// Sources from d.RecentlyViewedPerSObject[sobject] (a SOQL query
// scoped to the target sObject), NOT d.RecentlyViewed (which is a
// GLOBAL top-N across all sObjects and starves per-sObject filters
// for users who recently viewed many other types).
//
// Returns nil for empty inputs so callers can short-circuit.
func (m Model) salesforceVisitedRecordIDs(orgUser, sobject string) map[string]bool {
	if orgUser == "" || sobject == "" {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	r, ok := d.RecentlyViewedPerSObject[sobject]
	if !ok || r == nil {
		return nil
	}
	rows := r.Value()
	if len(rows) == 0 {
		return nil
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		out[row.ID] = true
	}
	return out
}

// recentVisitedRecordIDs returns the set of record IDs in the
// union stream whose containing sObject matches `sobject`.  Used by
// /records' Recently Viewed chip predicate.
//
// Returns nil for empty inputs so callers can short-circuit.
func (m Model) recentVisitedRecordIDs(orgUser, sobject string) map[string]bool {
	if orgUser == "" || sobject == "" {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	stream := recentUnionStream(d)
	if len(stream) == 0 {
		return nil
	}
	out := make(map[string]bool, len(stream))
	for _, e := range stream {
		if e.Kind != RecentKindRecord || e.Type != sobject || e.ID == "" {
			continue
		}
		out[e.ID] = true
	}
	return out
}

// recentVisitedSObjects returns the set of sObject API names the
// user has visited DIRECTLY on /objects (Kind=sobject).  Used by
// /objects' Recently Viewed chip predicate.
//
// Strict: a record drill no longer marks the parent sObject as
// "visited" for /objects purposes.  Earlier versions did transitive
// inclusion ("opened an Account record yesterday → Account is on
// /objects Recently Viewed") but it surprised users — drilling a
// list-view record made the ListView sObject appear at the top of
// /objects Recently Viewed, which is not what the user meant.
// Recently Viewed = "I went to this sObject's page," not "I touched
// something whose parent is this sObject."
func (m Model) recentVisitedSObjects(orgUser string) map[string]bool {
	if orgUser == "" {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	stream := recentUnionStream(d)
	if len(stream) == 0 {
		return nil
	}
	out := make(map[string]bool, len(stream))
	for _, e := range stream {
		if e.Kind != RecentKindSObject || e.ID == "" {
			continue
		}
		out[e.ID] = true
	}
	return out
}

// orgUserOrEmpty returns the active org's username, or "" when no
// org is selected.  Convenience for callers that want to thread the
// active-org scope through without re-checking len(m.orgs).
func orgUserOrEmpty(m Model) string {
	if len(m.orgs) == 0 {
		return ""
	}
	return m.orgs[m.selected].Username
}

// recentVisitedIDsByKind returns the set of IDs in the union stream
// whose Kind matches one of the values in `kinds`.  Used by surfaces
// (flows, apex, lwc, aura, permsets, …) whose Recently Viewed chip
// predicate is "id present in union of any of these kinds."
func (m Model) recentVisitedIDsByKind(orgUser string, kinds ...string) map[string]bool {
	if orgUser == "" || len(kinds) == 0 {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	stream := recentUnionStream(d)
	if len(stream) == 0 {
		return nil
	}
	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		want[k] = true
	}
	out := make(map[string]bool, len(stream))
	for _, e := range stream {
		if !want[e.Kind] || e.ID == "" {
			continue
		}
		out[e.ID] = true
	}
	return out
}

// recentVisitedRankSObjects is the recency-rank counterpart to
// recentVisitedSObjects.  Strict: only direct Kind=sobject visits
// contribute to the rank order — same rationale as the set helper
// above.  Rank 0 = most recent direct sObject visit.
func (m Model) recentVisitedRankSObjects(orgUser string) map[string]int {
	if orgUser == "" {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	stream := recentUnionStream(d)
	if len(stream) == 0 {
		return nil
	}
	out := make(map[string]int, len(stream))
	rank := 0
	for _, e := range stream {
		if e.Kind != RecentKindSObject {
			continue
		}
		id := e.ID
		if id == "" {
			continue
		}
		if _, dup := out[id]; dup {
			continue
		}
		out[id] = rank
		rank++
	}
	return out
}

// recentVisitedRankByKind is the recency-rank counterpart to
// recentVisitedIDsByKind.  Returns map[id]rank where rank 0 is the
// most-recently-visited entry of the requested kind(s).
func (m Model) recentVisitedRankByKind(orgUser string, kinds ...string) map[string]int {
	if orgUser == "" || len(kinds) == 0 {
		return nil
	}
	d := m.data[orgUser]
	if d == nil {
		return nil
	}
	stream := recentUnionStream(d)
	if len(stream) == 0 {
		return nil
	}
	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		want[k] = true
	}
	out := make(map[string]int, len(stream))
	rank := 0
	for _, e := range stream {
		if !want[e.Kind] || e.ID == "" {
			continue
		}
		if _, dup := out[e.ID]; dup {
			continue
		}
		out[e.ID] = rank
		rank++
	}
	return out
}

// rankRecordsFromStream is the pure version of
// recentVisitedRankRecords — no Model receiver, so callers that have
// a stream in hand can compute the rank map without threading Model.
func rankRecordsFromStream(stream []RecentEntry, sobject string) map[string]int {
	if len(stream) == 0 || sobject == "" {
		return nil
	}
	out := make(map[string]int, len(stream))
	rank := 0
	for _, e := range stream {
		if e.Kind != RecentKindRecord || e.Type != sobject || e.ID == "" {
			continue
		}
		if _, dup := out[e.ID]; dup {
			continue
		}
		out[e.ID] = rank
		rank++
	}
	return out
}
