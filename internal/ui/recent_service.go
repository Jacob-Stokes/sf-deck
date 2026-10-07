package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/recent"
)

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

func activeRecentListPtr(d *orgData) *ListView[RecentEntry] {
	if d == nil {
		return nil
	}
	if d.HomeRecentMode == ChipModeSalesforce {
		return &d.RecentSFList
	}
	return &d.RecentList
}

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

func orgUserOrEmpty(m Model) string {
	if len(m.orgs) == 0 {
		return ""
	}
	return m.orgs[m.selected].Username
}

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
