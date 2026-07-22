package ui

// Central "can this sObject back a Records view?" gate.
//
// Salesforce surfaces plenty of entities in the object list that can't
// actually serve records the way a normal sObject does:
//
//   - Platform Events (__e), Big Objects (__b), External Objects (__x)
//     and assorted system entities report queryable=false and reject
//     SOQL with INVALID_TYPE_FOR_OPERATION.
//   - Setup/metadata entities (BatchProcessJobDefinition, …) report
//     mruEnabled=false and have no LastViewedDate, so the "Recently
//     Viewed" query throws INVALID_FIELD.
//
// sf-deck repeatedly learned this the hard way by firing a query and
// surfacing the raw API error. This is the single source of truth: ask
// recordsCapabilityFor before offering any records operation, rather
// than discovering unsupported-ness from an error. Every records entry
// point (render gate + fetch gate) reads it, so a new surface inherits
// the same behaviour for free.

import "strings"

// recordsCapability is the resolved records-capability of one sObject,
// derived from its (possibly-not-yet-loaded) describe.
type recordsCapability struct {
	// DescribeLoaded is false when the describe hasn't landed yet. While
	// false the other fields are meaningless and callers must NOT gate —
	// wait for the describe (it's ensured in the same batch and re-fires
	// this once it arrives). This is what stops a transient "not
	// queryable" flash before the describe loads.
	DescribeLoaded bool
	// Queryable mirrors describe.queryable — whether SOQL works at all.
	Queryable bool
	// MruEnabled mirrors describe.mruEnabled — whether the "Recently
	// Viewed" chip is meaningful (object has LastViewedDate tracking).
	MruEnabled bool
}

// recordsCapabilityFor resolves the records-capability for an sObject.
// Returns DescribeLoaded=false when the describe isn't cached yet.
func (m Model) recordsCapabilityFor(sobject string) recordsCapability {
	desc, ok := m.cachedDescribe(sobject)
	if !ok {
		return recordsCapability{DescribeLoaded: false}
	}
	return recordsCapability{
		DescribeLoaded: true,
		Queryable:      desc.Queryable,
		MruEnabled:     desc.MruEnabled,
	}
}

// recordsCapabilityForData is the orgData-side variant used by the fetch
// gate (which has d in hand, not the live Model's cachedDescribe path).
// Same contract: DescribeLoaded=false means "don't gate yet".
func recordsCapabilityForData(d *orgData, sobject string) recordsCapability {
	r, ok := d.Describes[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return recordsCapability{DescribeLoaded: false}
	}
	v := r.Value()
	return recordsCapability{DescribeLoaded: true, Queryable: v.Queryable, MruEnabled: v.MruEnabled}
}

// nonQueryableReason classifies a non-queryable sObject by API-name
// suffix and returns (short kind phrase, one-line explanation) for the
// Records-tab empty state. The describe's queryable=false is the
// authoritative gate; the suffix just lets us word it specifically.
func nonQueryableReason(sobject string) (kind, why string) {
	switch {
	case strings.HasSuffix(sobject, "__e"):
		return "is a Platform Event", "pub/sub only — events are published + subscribed, never stored or queried"
	case strings.HasSuffix(sobject, "__b"):
		return "is a Big Object", "not SOQL-queryable here — use an async/indexed query against its index fields"
	case strings.HasSuffix(sobject, "__x"):
		return "is an External Object", "records live in an external system (OData) — querying isn't supported here"
	default:
		return "isn't queryable", "Salesforce reports queryable=false for this entity, so it has no records to list"
	}
}
