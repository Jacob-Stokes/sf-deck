package sf

// Apex trigger helpers — Tooling-API read + update for ApexTrigger
// records scoped to an sObject. Same shape as validation_rules.go
// and record_types.go.
//
// Tooling's ApexTrigger carries the full body as a column (not via
// the Metadata envelope), so GET is a light sobject read rather
// than a Metadata round-trip. Updates use the generic
// UpdateToolingMetadata path with the Metadata envelope — Salesforce
// wants {Metadata: {...}, Body} for body changes.

import (
	"encoding/json"
	"fmt"
)

// TriggerRow is one row in the list of a sobject's triggers.
// Lightweight for list rendering — drill for Body + full column set.
type TriggerRow struct {
	ID                 string
	Name               string
	NamespacePrefix    string // empty for unmanaged; set for triggers from installed packages
	Table              string // parent sObject API name; populated by ListAllTriggers / blank when fetched per-sObject
	Status             string // "Active" / "Inactive" / "Deleted"
	Events             string // pre-composed human-readable events string ("before insert, after update")
	Valid              bool
	Len                int // LengthWithoutComments
	ApiVer             float64
	LastModifiedDate   string
	LastModifiedByName string
}

// Field implements query.Row for chip predicates (used by /apex's
// Triggers chip when surfacing the flat trigger list).
func (t TriggerRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return t.ID, true
	case "Name":
		return t.Name, true
	case "NamespacePrefix", "Namespace":
		return t.NamespacePrefix, true
	case "Table", "SObject", "EntityDefinition":
		return t.Table, true
	case "Status":
		return t.Status, true
	case "IsValid":
		return t.Valid, true
	case "Events":
		return t.Events, true
	case "ApiVersion", "APIVersion":
		return t.ApiVer, true
	case "LengthWithoutComments":
		return t.Len, true
	case "LastModifiedDate":
		return t.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return t.LastModifiedByName, true
	}
	return nil, false
}

// ListAllTriggers returns every ApexTrigger in the org regardless of
// parent sObject. Used by the flat "/apex Triggers" view. Returns
// managed-package triggers too — chip filters in the UI handle the
// "managed vs unmanaged" cut.
func ListAllTriggers(target string) ([]TriggerRow, error) {
	// Sort by Name only — Tooling rejects ORDER BY EntityDefinition.<col>
	// with an internal server error once the result set spans multiple
	// namespaces (managed packages), which it does now that we don't
	// filter on NamespacePrefix. EntityDefinition.QualifiedApiName is
	// still selected for the per-row display; clients can re-sort
	// in-memory if they want grouping by parent sObject.
	return queryRows(target,
		"SELECT Id, Name, NamespacePrefix, Status, ApiVersion, LengthWithoutComments, IsValid, "+
			"LastModifiedDate, LastModifiedBy.Name, "+
			"UsageBeforeInsert, UsageAfterInsert, "+
			"UsageBeforeUpdate, UsageAfterUpdate, "+
			"UsageBeforeDelete, UsageAfterDelete, "+
			"UsageAfterUndelete, "+
			"EntityDefinition.QualifiedApiName "+
			"FROM ApexTrigger ORDER BY Name",
		true, mapTriggerRow)
}

// ListTriggers returns every ApexTrigger defined on the given
// sobject. Queries Tooling; read-only.
func ListTriggers(target, sobject string) ([]TriggerRow, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, Name, NamespacePrefix, Status, ApiVersion, LengthWithoutComments, IsValid, "+
				"LastModifiedDate, LastModifiedBy.Name, "+
				"UsageBeforeInsert, UsageAfterInsert, "+
				"UsageBeforeUpdate, UsageAfterUpdate, "+
				"UsageBeforeDelete, UsageAfterDelete, "+
				"UsageAfterUndelete "+
				"FROM ApexTrigger "+
				"WHERE EntityDefinition.QualifiedApiName = '%s' "+
				"ORDER BY Name",
			sqlEscape(sobject)),
		true, mapTriggerRow)
}

// mapTriggerRow maps one ApexTrigger record to a TriggerRow. Table is
// populated only when the query selected EntityDefinition (the org-wide
// ListAllTriggers does; the per-sobject ListTriggers doesn't — the
// nested lookup falls through harmlessly there).
func mapTriggerRow(r map[string]any) TriggerRow {
	row := TriggerRow{
		ID:                 asString(r["Id"]),
		Name:               asString(r["Name"]),
		NamespacePrefix:    asString(r["NamespacePrefix"]),
		Status:             asString(r["Status"]),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
	if ed, ok := r["EntityDefinition"].(map[string]any); ok {
		row.Table = asString(ed["QualifiedApiName"])
	}
	if b, ok := r["IsValid"].(bool); ok {
		row.Valid = b
	}
	if n, ok := r["LengthWithoutComments"].(float64); ok {
		row.Len = int(n)
	}
	if v, ok := r["ApiVersion"].(float64); ok {
		row.ApiVer = v
	}
	row.Events = composeTriggerEvents(r)
	return row
}

// TriggerDetail is the full body + column set for one trigger.
type TriggerDetail struct {
	ID     string
	Name   string
	Status string
	Body   string
	ApiVer float64
	Valid  bool
	Len    int
	Events string
}

// GetTrigger fetches one ApexTrigger via Tooling including its Body.
// Used when the user drills into a trigger row.
func GetTrigger(target, id string) (TriggerDetail, error) {
	c, err := RESTClient(target)
	if err != nil {
		return TriggerDetail{}, err
	}
	// Tooling's /sobjects/ApexTrigger/<id> returns the full record
	// including Body directly — no Metadata envelope needed.
	path := c.ToolingPath("sobjects/ApexTrigger/" + id)
	body, err := c.get(path, nil)
	if err != nil {
		return TriggerDetail{}, upgradeToSFError(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(body, &rec); err != nil {
		return TriggerDetail{}, err
	}
	det := TriggerDetail{
		ID:     asString(rec["Id"]),
		Name:   asString(rec["Name"]),
		Status: asString(rec["Status"]),
		Body:   asString(rec["Body"]),
	}
	if b, ok := rec["IsValid"].(bool); ok {
		det.Valid = b
	}
	if n, ok := rec["LengthWithoutComments"].(float64); ok {
		det.Len = int(n)
	}
	if v, ok := rec["ApiVersion"].(float64); ok {
		det.ApiVer = v
	}
	det.Events = composeTriggerEvents(rec)
	return det, nil
}

// UpdateTriggerMetadata patches an ApexTrigger. ApexTrigger has two
// kinds of mutable data:
//
//   - Top-level columns — Body, Status, ApiVersion. Salesforce
//     expects these at the envelope root, NOT inside Metadata.
//   - Metadata — packageVersions, formulas, etc. Inside "Metadata".
//
// The caller passes `body` / `status` / `apiVersion` / `metadata`
// in the patch. We sort them into the right slot on the PATCH envelope
// so the caller can think in terms of "a key I want to change".
func UpdateTriggerMetadata(target, id string, patch map[string]any) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.ToolingPath("sobjects/ApexTrigger/" + id)

	body := map[string]any{}
	meta := map[string]any{}
	for k, v := range patch {
		switch k {
		case "body", "Body":
			body["Body"] = v
		case "status", "Status":
			body["Status"] = v
		case "apiVersion", "ApiVersion":
			body["ApiVersion"] = v
		default:
			meta[k] = v
		}
	}
	// If the caller sent any real-Metadata keys, merge them into the
	// fetched Metadata first — Tooling validates the whole envelope on
	// PATCH so partial Metadata writes fail.
	if len(meta) > 0 {
		existing, err := GetToolingMetadata(target, "ApexTrigger", id)
		if err != nil {
			return err
		}
		for k, v := range meta {
			existing[k] = v
		}
		body["Metadata"] = existing
	}
	if len(body) == 0 {
		return nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if _, err := c.patch(path, raw); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}

// composeTriggerEvents walks the UsageXxx bool columns on a raw
// Tooling record and returns a compact human-readable string like
// "before insert, after update". Empty if nothing is set.
func composeTriggerEvents(r map[string]any) string {
	bits := []struct {
		Key   string
		Label string
	}{
		{"UsageBeforeInsert", "before insert"},
		{"UsageAfterInsert", "after insert"},
		{"UsageBeforeUpdate", "before update"},
		{"UsageAfterUpdate", "after update"},
		{"UsageBeforeDelete", "before delete"},
		{"UsageAfterDelete", "after delete"},
		{"UsageAfterUndelete", "after undelete"},
	}
	var out []string
	for _, b := range bits {
		if v, ok := r[b.Key].(bool); ok && v {
			out = append(out, b.Label)
		}
	}
	return joinCSV(out)
}

// joinCSV joins with ", " without importing strings for one call.
func joinCSV(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	s := xs[0]
	for _, x := range xs[1:] {
		s += ", " + x
	}
	return s
}
