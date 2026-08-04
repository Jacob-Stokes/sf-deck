package sf

import "fmt"

// RecordsList is the result of "give me recent records for this sObject."
// Each row is a map because the column set is sObject-specific.
type RecordsList struct {
	SObject    string           // API name, e.g. "Account"
	HasName    bool             // true if the sObject has a Name field
	HasModDate bool             // true if it has LastModifiedDate
	Records    []map[string]any // raw rows from the SOQL response
	Query      string           // the SOQL we actually ran (for display)
	// TotalSize is the row count Salesforce reports for the WHERE
	// clause — distinct from len(Records) when a LIMIT capped the
	// fetch. Renderers compare them to surface a "showing X of Y"
	// truncation hint so users know the chip is a slice, not a
	// complete view.
	TotalSize int
	// Done reports whether the SOQL cursor walked to completion. False
	// means the fetch was cut short either by the chip's LIMIT clause
	// or by the requested row cap — there are more rows on the server
	// the user could pull. Renderers use this to show a "preview" hint
	// + the ctrl+x full-export affordance.
	//
	// Subtle: a LIMIT clause that exactly matches the unbounded row
	// count still returns Done=true. We can't tell apart "you got
	// everything" from "your LIMIT happened to be the right number"
	// without a second query, so the false-positive-but-cheap case is
	// to trust SF's Done flag.
	Done bool
	// Columns is the projected field list, in SOQL SELECT order. The
	// renderer uses it to know which fields to render and in what
	// order — without it, the records table only ever showed Id +
	// Name + LastModifiedDate regardless of what the chip projected.
	// Always at least ["Id"]; "Name" / "LastModifiedDate" appended
	// when the sObject has them and the caller asked for defaults.
	Columns []string
}

// Record wraps one row from a RecordsList so it can implement the
// structural query.Row interface. Used by client-side filtering of
// records (rare: /records is server-side filtered today, but the
// engine fallback uses this when a small enough lens runs locally).
type Record map[string]any

// Field implements query.Row by reading the column from the
// underlying map. Salesforce returns relationship fields nested
// (e.g. r["LastModifiedBy"]["Name"]) so we honour dotted paths
// transparently — `Field("LastModifiedBy.Name")` walks through.
func (r Record) Field(name string) (any, bool) {
	if v, ok := r[name]; ok {
		return v, true
	}
	// Dotted path: walk through nested map[string]any until we hit
	// the leaf or a non-map.
	parts := splitDotted(name)
	if len(parts) <= 1 {
		return nil, false
	}
	cur := any(map[string]any(r))
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// splitDotted is a tiny helper avoiding strings.Split's allocation in
// the common single-segment path.
func splitDotted(s string) []string {
	out := []string{}
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	out = append(out, s[last:])
	return out
}

// RecentRecords returns the most recently-modified records on a sObject.
// Read-only SOQL. Defensive about fields: we try Name + LastModifiedDate
// and fall back if either is missing (Address has no Name; some system
// objects have no LastModifiedDate).
func RecentRecords(orgAlias, sobjectName string, limit int) (RecordsList, error) {
	if limit <= 0 {
		limit = 50
	}
	// Strategy: probe the describe to decide which fields are available.
	// In practice we already have the describe cached for every sobject
	// the user has opened, but asking again here keeps this function
	// self-contained. Callers that already have the describe can skip
	// the round-trip via RecentRecordsWithDescribe below.
	desc, err := Describe(orgAlias, sobjectName)
	if err != nil {
		return RecordsList{}, err
	}
	return RecentRecordsWithDescribe(orgAlias, desc, limit)
}

// RecentRecordsWithDescribe is the no-extra-round-trip version of
// RecentRecords — caller hands in the (already cached) describe so we
// can skip the probe.
func RecentRecordsWithDescribe(orgAlias string, desc SObjectDescribe, limit int) (RecordsList, error) {
	if limit <= 0 {
		limit = 50
	}
	out := RecordsList{SObject: desc.Name}

	// Some metadata-ish sObjects aren't queryable from the REST API at
	// all (AggregateResult, ActivityHistory, etc.). Fail fast with a
	// useful message.
	if !desc.Queryable {
		return out, fmt.Errorf("%s is not queryable via SOQL", desc.Name)
	}

	// Walk describe once to learn which of the columns we want actually
	// exist on this sObject — selecting only present fields is what keeps
	// the query safe across shapes (a CustomMetadata __mdt has
	// DeveloperName/MasterLabel/Label instead of Name, and while it
	// exposes LastModifiedDate it rejects it in ORDER BY → INVALID_FIELD).
	present := map[string]bool{}
	sortable := map[string]bool{}
	for _, f := range desc.Fields {
		present[f.Name] = true
		sortable[f.Name] = f.Sortable
	}

	fields := []string{"Id"}
	add := func(name string) {
		if present[name] {
			fields = append(fields, name)
		}
	}
	// The human label column: standard objects use Name; CustomMetadata
	// (and a few others) use DeveloperName / MasterLabel / Label. Take
	// the first that exists so every shape shows something readable.
	for _, nameField := range []string{"Name", "MasterLabel", "Label", "DeveloperName"} {
		if present[nameField] {
			fields = append(fields, nameField)
			out.HasName = true
			// MasterLabel/Label shapes (CustomMetadata __mdt and kin)
			// also carry DeveloperName — the API identity a developer
			// actually keys on. Project it alongside the label so CMDT
			// rows aren't label-only. (Name-shaped standard objects
			// skip this — Name IS their identity.)
			if nameField != "Name" && nameField != "DeveloperName" && present["DeveloperName"] {
				fields = append(fields, "DeveloperName")
			}
			break
		}
	}
	if present["LastModifiedDate"] {
		out.HasModDate = true
	}
	add("CreatedDate")
	add("LastModifiedDate")
	// Audit user names are relationship fields: the describe exposes the
	// *Id* column (CreatedById), so gate the CreatedBy.Name projection on
	// that. Objects without the audit relationship (rare) just skip it.
	if present["CreatedById"] {
		fields = append(fields, "CreatedBy.Name")
	}
	if present["LastModifiedById"] {
		fields = append(fields, "LastModifiedBy.Name")
	}
	// CustomMetadata (__mdt) has none of the date/audit fields above but
	// DOES expose SystemModstamp — show it so a __mdt record isn't a bare
	// Id + label.
	if !present["LastModifiedDate"] && present["SystemModstamp"] {
		fields = append(fields, "SystemModstamp")
	}

	// Order by the most-recent activity, but only when the ordering
	// column is actually sortable (CMDT rejects ORDER BY LastModifiedDate;
	// CreatedDate is accepted as a fallback).
	var sortField string
	switch {
	case sortable["LastModifiedDate"]:
		sortField = "LastModifiedDate"
	case sortable["CreatedDate"]:
		sortField = "CreatedDate"
	}

	soql := "SELECT " + joinCommas(fields) + " FROM " + desc.Name
	if sortField != "" {
		soql += " ORDER BY " + sortField + " DESC"
	}
	soql += fmt.Sprintf(" LIMIT %d", limit)
	out.Query = soql
	out.Columns = fields

	q, err := Query(orgAlias, soql, false)
	if err != nil {
		return out, err
	}
	out.Records = q.Records
	out.TotalSize = q.TotalSize
	out.Done = q.Done
	return out, nil
}

// GetRecord fetches every queryable field on a single record and
// returns it as a flat map[string]any. The describe drives the
// projection so we never SELECT a field SF would reject.
//
// Sub-query / relationship traversals (LastModifiedBy.Name etc.) are
// not folded in here — callers that want them should query directly
// with their own SOQL or extend this helper. Phase 1 keeps the
// projection to first-level fields only; that's enough for a usable
// detail page and avoids surprise SOQL aggregate-row-limit errors on
// sObjects with hundreds of fields.
func GetRecord(orgAlias, sobjectName, recordID string) (map[string]any, error) {
	if recordID == "" {
		return nil, fmt.Errorf("recordID required")
	}
	desc, err := Describe(orgAlias, sobjectName)
	if err != nil {
		return nil, err
	}
	return GetRecordWithDescribe(orgAlias, desc, recordID)
}

// GetRecordWithDescribe is the no-extra-round-trip version. Callers
// that already have the describe cached should prefer this so we
// don't re-fetch.
func GetRecordWithDescribe(orgAlias string, desc SObjectDescribe, recordID string) (map[string]any, error) {
	if recordID == "" {
		return nil, fmt.Errorf("recordID required")
	}
	if !desc.Queryable {
		return nil, fmt.Errorf("%s is not queryable via SOQL", desc.Name)
	}
	// Project every Name we know about. SF caps a SELECT at 100 fields
	// per query — chunk if necessary and merge the results client-side.
	const chunkSize = 100
	names := make([]string, 0, len(desc.Fields)+1)
	names = append(names, "Id")
	for _, f := range desc.Fields {
		if f.Name == "Id" {
			continue
		}
		// Skip compound fields (e.g. ShippingAddress) — SOQL rejects
		// them on the projection. The discrete child fields
		// (ShippingStreet, ShippingCity, …) come back individually.
		if f.Type == "address" || f.Type == "location" {
			continue
		}
		names = append(names, f.Name)
	}

	out := map[string]any{}
	for start := 0; start < len(names); start += chunkSize {
		end := start + chunkSize
		if end > len(names) {
			end = len(names)
		}
		chunk := names[start:end]
		if !validSOQLIdentifier(desc.Name) {
			return nil, fmt.Errorf("invalid sobject name %q", desc.Name)
		}
		soql := fmt.Sprintf("SELECT %s FROM %s WHERE Id = '%s' LIMIT 1",
			joinCommas(chunk), desc.Name, sqlEscape(recordID))
		q, err := Query(orgAlias, soql, false)
		if err != nil {
			return nil, err
		}
		if len(q.Records) == 0 {
			return nil, fmt.Errorf("no %s with Id %s", desc.Name, recordID)
		}
		for k, v := range q.Records[0] {
			out[k] = v
		}
	}
	// Make sure the attributes block carries the sObject type — the
	// UI's openable code reads it for URL generation.
	if _, ok := out["attributes"]; !ok {
		out["attributes"] = map[string]any{"type": desc.Name}
	}
	return out, nil
}

// RecordsForSOQL runs an arbitrary SOQL against the org and packages
// the response as a RecordsList. Used by lens-driven fetches where the
// caller has already composed the query (lens.BuildSOQL). Set sobject
// + flags from the caller's describe so the renderer's Name / mod-date
// columns light up correctly. `columns` is the projected SELECT list
// in order — the renderer uses it to drive header + cell layout.
func RecordsForSOQL(orgAlias, sobject, soql string, columns []string, hasName, hasModDate bool, cap int) (RecordsList, error) {
	out := RecordsList{
		SObject:    sobject,
		HasName:    hasName,
		HasModDate: hasModDate,
		Query:      soql,
		Columns:    columns,
	}
	q, err := QueryCapped(orgAlias, soql, false, cap)
	if err != nil {
		return out, err
	}
	out.Records = q.Records
	// TotalSize comes from SF's first response page — it reports the
	// true matching-row count regardless of how many pages the
	// cursor follow walked. Renderers compare this to len(Records)
	// to surface a "showing X of Y · capped" hint.
	out.TotalSize = q.TotalSize
	out.Done = q.Done
	return out, nil
}

func joinCommas(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}

// validSOQLIdentifier reports whether s is safe to interpolate into
// SOQL as an identifier (sObject / field API name). Describe-sourced
// names always pass; the check is defense-in-depth so a corrupted or
// attacker-shaped describe payload can't smuggle SOQL syntax in.
func validSOQLIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
