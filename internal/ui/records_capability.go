package ui

// Central "can this sObject back a Records view?" gate.

import "strings"

type recordsCapability struct {
	// DescribeLoaded is false when the describe hasn't landed yet. While
	// false the other fields are meaningless and callers must NOT gate —
	// wait for the describe (it's ensured in the same batch and re-fires
	// this once it arrives). This is what stops a transient "not
	// queryable" flash before the describe loads.
	DescribeLoaded bool
	Queryable      bool
	MruEnabled     bool
}

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
