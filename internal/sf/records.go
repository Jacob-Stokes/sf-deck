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
	TotalSize  int
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
	Done    bool
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
	for _, nameField := range []string{"Name", "MasterLabel", "Label", "DeveloperName"} {
		if present[nameField] {
			fields = append(fields, nameField)
			out.HasName = true
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
	if present["CreatedById"] {
		fields = append(fields, "CreatedBy.Name")
	}
	if present["LastModifiedById"] {
		fields = append(fields, "LastModifiedBy.Name")
	}
	if !present["LastModifiedDate"] && present["SystemModstamp"] {
		fields = append(fields, "SystemModstamp")
	}

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
	const chunkSize = 100
	names := make([]string, 0, len(desc.Fields)+1)
	names = append(names, "Id")
	for _, f := range desc.Fields {
		if f.Name == "Id" {
			continue
		}
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
