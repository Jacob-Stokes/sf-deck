// Package recent owns the data types + pure-logic transforms behind
// sf-deck's "recently visited" log.
//
// The package holds nothing UI-specific: just the Entry struct, the
// merge/sort/filter pipeline that combines the local log with
// Salesforce's RecentlyViewed feed, the per-row formatters, and the
// settings round-trip helpers. The UI shell in internal/ui owns the
// per-org log mutation, persistence triggers, and the per-tab visit
// closures (recentVisitX).
//
// Origin tagging: every Entry carries an Origin tag (deck / sf / both)
// set by Merge so the renderer can show a small badge and chips can
// filter by source.
package recent

import (
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// Entry is one row in the per-org recent-visits log. Stored in MRU
// order — most recent first. The same item visited again moves to the
// head (no duplicate).
//
// Kind discriminates entry types so a future "Recent reports" tab can
// share the same persistence layer.
type Entry struct {
	Kind      string    `toml:"kind"`           // "record" today; "report" / "flow" / "dashboard" later
	ID        string    `toml:"id"`             // Salesforce Id (or other stable identifier)
	Name      string    `toml:"name,omitempty"` // user-visible label for the row
	Type      string    `toml:"type,omitempty"` // sObject API name for records; report folder etc. later
	OrgUser   string    `toml:"-"`              // username scope; never persisted (the per-org map handles that)
	VisitedAt time.Time `toml:"visited_at"`

	// Origin is set at merge time, not persisted. One of:
	//   "deck" — sf-deck-only (this user opened it via sf-deck, and
	//            either it's a kind Salesforce doesn't track —
	//            flows / apex / etc. — or the SF fetch hadn't seen
	//            it yet at merge time)
	//   "sf"   — Salesforce-only (came from RecentlyViewed; the
	//            user hasn't touched it via sf-deck this session)
	//   "both" — present in both sources; sf-deck stream's timestamp
	//            wins at dedupe time
	// Empty when set by persistence load (rendered as "deck" since
	// the persisted stream is sf-deck's view).
	Origin string `toml:"-"`
}

// Field implements the query.Row interface so Entry can be filtered by
// chip predicates. The chip system only knows five names per row:
//
//	"Kind"   — the entry kind constant ("record", "report", "flow", …)
//	"Type"   — the kind-specific secondary slot (sObject for records,
//	           namespace for managed components, etc.)
//	"Name"   — the human-readable display label
//	"Id"     — the entry's primary identifier
//	"Origin" — the source-tag ("deck" / "sf" / "both")
//
// Anything else is unknown — caller treats that as "predicate doesn't
// match" via the (any, false) protocol.
func (e Entry) Field(name string) (any, bool) {
	switch name {
	case "Kind":
		return e.Kind, true
	case "Type":
		return e.Type, true
	case "Name":
		return e.Name, true
	case "Id", "ID":
		return e.ID, true
	case "Origin":
		// Default to "deck" when unset — entries loaded from the
		// persisted log have no Origin set; treat them as deck-tracked.
		if e.Origin == "" {
			return OriginDeck, true
		}
		return e.Origin, true
	}
	return nil, false
}

// Origin constants for Entry.Origin. Set by Merge at render time so
// the renderer can show a small badge and chips can filter "just
// sf-deck" / "just Salesforce".
const (
	OriginDeck = "deck" // tracked by sf-deck only
	OriginSF   = "sf"   // tracked by Salesforce RecentlyViewed only
	OriginBoth = "both" // tracked by both sources
)

// Entry-kind constants. Stored as Entry.Kind on disk; the renderer +
// persistence layer treat them as opaque strings, so adding a kind is
// "add a const + write a helper to record it" with no schema migration.
//
// Kind defines what the row IS; the Type field on Entry carries the
// kind's secondary label (sObject API name for records, namespace for
// managed components, etc.).
const (
	KindRecord       = "record"
	KindReport       = "report"
	KindDashboard    = "dashboard"
	KindListView     = "listview"
	KindFlow         = "flow"
	KindApexClass    = "apex_class"
	KindLWC          = "lwc"
	KindAura         = "aura"
	KindSObject      = "sobject"
	KindField        = "field"
	KindPermSet      = "permset"
	KindPermSetGroup = "permset_group"
	KindProfile      = "profile"
	KindUser         = "user"
	KindDeploy       = "deploy"
	KindPackage      = "package"
	KindQueue        = "queue"
	KindPublicGroup  = "public_group"
	KindApexLog      = "apex_log"
)

// SFRow is the shape Merge consumes for Salesforce-side RecentlyViewed
// rows. Just the fields actually used; the caller (UI shell) is
// responsible for projecting whatever sObject the SF REST client
// returns down to this shape.
type SFRow struct {
	ID             string
	Name           string
	SObjectType    string
	LastViewedDate time.Time
}

// Merge combines the local visit log with Salesforce's server-side
// RecentlyViewed list into one MRU-sorted stream.
//
// Dedupe rule: same `(Kind=record, sObject, ID)` collapses into a
// single entry. The local timestamp wins (you opened it more recently
// in sf-deck than Salesforce noticed). Origin = "both".
//
// Records present in only one source carry the source's origin
// ("deck" or "sf"). Non-record kinds (flow, apex, etc.) only ever
// come from sf-deck, so they're always Origin="deck".
//
// `local` is the in-memory log; `sf` is the cached RecentlyViewed
// payload. Both may be nil/empty.
func Merge(local []Entry, sf []SFRow) []Entry {
	out := make([]Entry, 0, len(local)+len(sf))

	// Index local entries by (kind, id) so dedupe works across every
	// kind, not just records. Keeps the SF-only branch from
	// double-emitting things the user already has in their local log
	// (e.g. an apex class opened via sf-deck AND via Lightning).
	type entryKey struct{ kind, id string }
	localByKey := make(map[entryKey]int, len(local))
	for i, e := range local {
		if e.ID != "" {
			localByKey[entryKey{e.Kind, e.ID}] = i
		}
	}

	taken := make(map[int]bool, len(local))
	for _, r := range sf {
		if r.SObjectType == "" || r.ID == "" {
			continue
		}
		kind := KindForSFType(r.SObjectType)
		if i, ok := localByKey[entryKey{kind, r.ID}]; ok {
			// Already in local — promote to "both" but keep local's
			// timestamp (the user's last sf-deck open is the better
			// "when did I last touch this" signal).
			taken[i] = true
			continue
		}
		out = append(out, Entry{
			Kind:      kind,
			ID:        r.ID,
			Name:      r.Name,
			Type:      r.SObjectType,
			VisitedAt: r.LastViewedDate,
			Origin:    OriginSF,
		})
	}

	// Walk local: copy each entry, tagging records that appeared in
	// SF as "both"; everything else stays "deck".
	for i, e := range local {
		out = append(out, Entry{
			Kind:      e.Kind,
			ID:        e.ID,
			Name:      e.Name,
			Type:      e.Type,
			VisitedAt: e.VisitedAt,
			Origin: func() string {
				if taken[i] {
					return OriginBoth
				}
				return OriginDeck
			}(),
		})
	}

	SortMRU(out)
	return out
}

// SortMRU sorts in place by VisitedAt descending (newest first).
// Stable so equal timestamps preserve input order.
func SortMRU(es []Entry) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].VisitedAt.After(es[j-1].VisitedAt); j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
}

// FilterByKinds drops entries whose Kind is in the excluded slice.
// Linear scan; the slice is short (<10 kinds typically). Returns the
// input unchanged when excluded is empty.
func FilterByKinds(in []Entry, excluded []string) []Entry {
	if len(excluded) == 0 || len(in) == 0 {
		return in
	}
	skip := make(map[string]bool, len(excluded))
	for _, k := range excluded {
		skip[k] = true
	}
	out := make([]Entry, 0, len(in))
	for _, e := range in {
		if skip[e.Kind] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// NameForRow returns the row's display name with a sane fallback when
// Name is missing. Prior shape fell back to "" which downstream
// rendered as "—"; better to always show something useful — a
// truncated Id when no Name exists.
func NameForRow(r Entry) string {
	if name := strings.TrimSpace(r.Name); name != "" {
		return name
	}
	if id := strings.TrimSpace(r.ID); id != "" {
		// Short Id form — first 4 chars + "…" for visual compactness.
		// The full Id is in the ID column anyway.
		if len(id) > 6 {
			return id[:4] + "…"
		}
		return id
	}
	return ""
}

// DetailForRow returns the kind-specific secondary slot, suppressing
// values that would just duplicate KIND or NAME.
//
// For records: the sObject API name (Account, Request__c).
// For fields: the parent sObject.
// For managed components: the namespace prefix.
// For everything else (listview, user, report, dashboard, sobject):
// blank — the kind label already says what it is.
func DetailForRow(r Entry) string {
	switch r.Kind {
	case KindRecord, KindField:
		return r.Type
	case KindLWC, KindAura, KindApexClass:
		return r.Type
	}
	return ""
}

// KindForSFType maps Salesforce's RecentlyViewed.Type (the sObject API
// name on the wire) to one of the Kind* constants. Salesforce returns
// mixed types — ListView, User, Report, Dashboard, Group, FlowRecord,
// ApexClass, etc., not just data records — so labelling them all
// "record" is misleading.
//
// Anything we don't recognise stays Record — that's the default for
// actual data records (Account, Request__c, custom objects).
func KindForSFType(t string) string {
	switch t {
	case "ListView":
		return KindListView
	case "User":
		return KindUser
	case "Report":
		return KindReport
	case "Dashboard":
		return KindDashboard
	case "Group":
		return KindPublicGroup
	case "FlowRecord", "FlowDefinition", "Flow", "FlowRecordVersion":
		return KindFlow
	case "ApexClass":
		return KindApexClass
	case "ApexTrigger":
		// Reuse apex_class — we don't have a separate trigger kind
		// on the persistence side and they live in the Apex chip
		// together.
		return KindApexClass
	case "LightningComponentBundle":
		return KindLWC
	case "AuraDefinitionBundle":
		return KindAura
	case "PermissionSet":
		return KindPermSet
	case "PermissionSetGroup":
		return KindPermSetGroup
	case "Profile":
		return KindProfile
	case "InstalledSubscriberPackage":
		return KindPackage
	}
	return KindRecord
}

// KindLabel returns a short, human-friendly label for the KIND column.
// Falls back to the raw kind string when unknown so new kinds added by
// a future build don't crash the older renderer.
func KindLabel(kind string) string {
	switch kind {
	case KindRecord:
		return "record"
	case KindReport:
		return "report"
	case KindDashboard:
		return "dashboard"
	case KindListView:
		return "listview"
	case KindFlow:
		return "flow"
	case KindApexClass:
		return "apex"
	case KindLWC:
		return "lwc"
	case KindAura:
		return "aura"
	case KindSObject:
		return "sobject"
	case KindField:
		return "field"
	case KindPermSet:
		return "permset"
	case KindPermSetGroup:
		return "psg"
	case KindProfile:
		return "profile"
	case KindUser:
		return "user"
	case KindDeploy:
		return "deploy"
	case KindPackage:
		return "package"
	case KindQueue:
		return "queue"
	case KindPublicGroup:
		return "pubgroup"
	case KindApexLog:
		return "log"
	}
	return kind
}

// Upsert inserts entry at position 0, removing any existing entry with
// the same Kind+ID first. The slice is capped at `cap` (caller passes
// settings.RecentMaxEntries()) — older entries fall off the tail.
// cap <= 0 disables the cap entirely.
func Upsert(list []Entry, entry Entry, cap int) []Entry {
	out := make([]Entry, 0, len(list)+1)
	out = append(out, entry)
	for _, e := range list {
		if e.Kind == entry.Kind && e.ID == entry.ID {
			continue
		}
		out = append(out, e)
		if cap > 0 && len(out) >= cap {
			break
		}
	}
	return out
}

// ToConfig converts the in-memory list to the settings persistence
// form. Drops the OrgUser field — the per-org map handles scoping.
func ToConfig(list []Entry) []settings.RecentConfig {
	out := make([]settings.RecentConfig, len(list))
	for i, e := range list {
		out[i] = settings.RecentConfig{
			Kind:      e.Kind,
			ID:        e.ID,
			Name:      e.Name,
			Type:      e.Type,
			VisitedAt: e.VisitedAt,
		}
	}
	return out
}

// FromConfig is the inverse: settings → in-memory. The orgUser arg
// fills in OrgUser on each entry so callers don't need to track scope
// separately.
func FromConfig(cfgs []settings.RecentConfig, orgUser string) []Entry {
	out := make([]Entry, len(cfgs))
	for i, c := range cfgs {
		out[i] = Entry{
			Kind:      c.Kind,
			ID:        c.ID,
			Name:      c.Name,
			Type:      c.Type,
			OrgUser:   orgUser,
			VisitedAt: c.VisitedAt,
		}
	}
	return out
}
