package sf

import (
	"fmt"
	"sort"
	"strings"
)

// SObject is a lightweight entry for the object browser. We pull
// every EntityDefinition field that's useful for filtering / display
// up front so the filter spec has something to bite on.
//
// All fields except Name + Label are best-effort — older orgs and
// niche entities sometimes return null for any of these. The omitempty
// JSON tags keep the cache file compact.
type SObject struct {
	Name               string `json:"n"`
	Label              string `json:"l"`
	IsCustomizable     bool   `json:"c,omitempty"`
	KeyPrefix          string `json:"kp,omitempty"`
	Namespace          string `json:"ns,omitempty"` // NamespacePrefix
	DeploymentStatus   string `json:"dep,omitempty"`
	ApexTriggerable    bool   `json:"apxt,omitempty"`
	WorkflowEnabled    bool   `json:"wfe,omitempty"`
	LastModifiedDate   string `json:"lm,omitempty"`
	LastModifiedByName string `json:"lmb,omitempty"`
}

// Field implements the structural query.Row interface — looks up a
// SOQL column name on this row and returns the value. Names match
// EntityDefinition columns so a SOQL expression imported from a
// list view evaluates against the same identifiers.
//
// "IsCustom" is *not* an EntityDefinition column — it's the
// derived suffix-check we expose so the user can write
// `IsCustom = true` in queries against /objects.
func (s SObject) Field(name string) (any, bool) {
	switch name {
	case "QualifiedApiName", "Name", "DeveloperName":
		return s.Name, true
	case "Label", "MasterLabel":
		return s.Label, true
	case "IsCustomizable":
		return s.IsCustomizable, true
	case "KeyPrefix":
		return s.KeyPrefix, true
	case "NamespacePrefix", "Namespace":
		return s.Namespace, true
	case "DeploymentStatus":
		return s.DeploymentStatus, true
	case "IsApexTriggerable":
		return s.ApexTriggerable, true
	case "IsWorkflowEnabled":
		return s.WorkflowEnabled, true
	case "LastModifiedDate":
		return s.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return s.LastModifiedByName, true
	case "IsCustom":
		return IsCustom(s.Name), true
	case "IsManaged":
		return IsManagedName(s.Name), true
	}
	return nil, false
}

// IsManagedName reports whether an sObject API name carries a managed-
// package prefix. Standard SObjects never carry one; managed-installed
// customs always do. Synthetic companions (ChangeEvent / History /
// Share / OwnerSharingRule) inherit their parent's prefix in the name
// even when EntityDefinition.NamespacePrefix is null on the row, so a
// name-based check is the only reliable way to classify them.
func IsManagedName(name string) bool {
	for _, suf := range []string{"__c", "__e", "__mdt", "__b", "__x"} {
		if strings.HasSuffix(name, suf) {
			name = name[:len(name)-len(suf)]
			break
		}
	}
	return strings.Contains(name, "__")
}

// ListSObjects returns name+label+customizable via EntityDefinition.
// Read-only SOQL against the Tooling API.
//
// EntityDefinition doesn't support queryMore at all, so we page
// manually. We cursor on DurableId (Id) rather than QualifiedApiName
// because Tooling's `ORDER BY QualifiedApiName` uses case-insensitive
// collation while `WHERE QualifiedApiName > X` uses case-sensitive
// binary comparison — the two disagree around the case boundary, so
// `> 'DigitalSignatureChangeEvent'` leaps straight to `acknowltng__…`
// and skips every uppercase row from E through Z. Using DurableId as
// the cursor sidesteps the collation mismatch entirely: the client
// re-sorts by QualifiedApiName (case-insensitively) after collecting
// every row.
//
// Dedupe by DurableId because a cursor row can re-surface if it
// sorts differently on the server than the local re-sort suggests.
func ListSObjects(orgAlias string) ([]SObject, error) {
	const batchSize = 2000
	var all []SObject
	seen := map[string]bool{}
	lastID := ""
	includeModifiedBy := true
restart:
	for {
		where := ""
		if lastID != "" {
			where = fmt.Sprintf(" WHERE DurableId > '%s'", sqlEscape(lastID))
		}
		modifiedBySelect := ""
		if includeModifiedBy {
			modifiedBySelect = ", LastModifiedBy.Name"
		}
		soql := fmt.Sprintf(
			"SELECT DurableId, QualifiedApiName, Label, IsCustomizable, "+
				"KeyPrefix, NamespacePrefix, DeploymentStatus, "+
				"IsApexTriggerable, IsWorkflowEnabled, LastModifiedDate%s "+
				"FROM EntityDefinition%s ORDER BY DurableId LIMIT %d",
			modifiedBySelect, where, batchSize,
		)
		q, err := Query(orgAlias, soql, true)
		if err != nil {
			if includeModifiedBy && unsupportedLastModifiedBy(err) {
				includeModifiedBy = false
				all = nil
				seen = map[string]bool{}
				lastID = ""
				goto restart
			}
			return nil, err
		}
		if len(q.Records) == 0 {
			break
		}
		// Last page: a partial fill (< batchSize rows returned) means
		// we've reached the end. Process it and exit before firing
		// the dead "are we there yet" probe call.
		lastPage := len(q.Records) < batchSize
		prevLast := lastID
		for _, r := range q.Records {
			id := asString(r["DurableId"])
			if id == "" {
				continue
			}
			lastID = id
			if seen[id] {
				continue
			}
			seen[id] = true
			name := asString(r["QualifiedApiName"])
			if name == "" {
				continue
			}
			label := asString(r["Label"])
			if strings.Contains(label, "__MISSING LABEL__") {
				label = ""
			}
			cust, _ := r["IsCustomizable"].(bool)
			apxt, _ := r["IsApexTriggerable"].(bool)
			wfe, _ := r["IsWorkflowEnabled"].(bool)
			all = append(all, SObject{
				Name:               name,
				Label:              label,
				IsCustomizable:     cust,
				KeyPrefix:          asString(r["KeyPrefix"]),
				Namespace:          asString(r["NamespacePrefix"]),
				DeploymentStatus:   asString(r["DeploymentStatus"]),
				ApexTriggerable:    apxt,
				WorkflowEnabled:    wfe,
				LastModifiedDate:   asString(r["LastModifiedDate"]),
				LastModifiedByName: relationName(r, "LastModifiedBy"),
			})
		}
		// Safety valve: if the cursor didn't advance, bail.
		if lastID == prevLast {
			break
		}
		if lastPage {
			break
		}
	}
	// Re-sort by QualifiedApiName (case-insensitive) so the UI sees
	// alphabetical order; server order was by DurableId for paging.
	sortByName(all)
	return all, nil
}

func unsupportedLastModifiedBy(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "lastmodifiedby") &&
		(strings.Contains(s, "no such column") ||
			strings.Contains(s, "didn't understand relationship") ||
			strings.Contains(s, "invalid field"))
}

// sortByName orders SObjects by QualifiedApiName, case-insensitive,
// so the UI renders alphabetically regardless of server sort order.
func sortByName(xs []SObject) {
	sort.SliceStable(xs, func(i, j int) bool {
		return strings.ToLower(xs[i].Name) < strings.ToLower(xs[j].Name)
	})
}

// sqlEscape escapes a value for interpolation inside a single-quoted
// SOQL string literal. Backslash must be escaped FIRST — otherwise a
// value ending in `\` turns the appended closing quote into an escaped
// quote (`\'`), breaking out of the literal (a SOQL-injection vector on
// the few caller paths that take free-form input, e.g. a headless --id).
func sqlEscape(s string) string {
	return EscapeSOQLString(s)
}

// EscapeSOQLString escapes caller-controlled text for interpolation inside
// a single-quoted SOQL literal. Prefer query parameters/builders where
// available; this helper exists for the Tooling paths that still build SOQL.
func EscapeSOQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "'", `\'`)
}

// IsCustom reports whether an sObject API name looks custom.
func IsCustom(name string) bool {
	return strings.HasSuffix(name, "__c") ||
		strings.HasSuffix(name, "__e") ||
		strings.HasSuffix(name, "__mdt") ||
		strings.HasSuffix(name, "__b") ||
		strings.HasSuffix(name, "__x")
}

// SObjectByKeyPrefix scans objs for the entry whose KeyPrefix matches
// the first 3 chars of recordID. THE shared kernel for "which sObject
// does this Id belong to" — the TUI's polymorphic-reference resolver
// and the headless record commands both wrap it (with their own
// candidate filtering / error shaping) so the matching rule can't
// drift between surfaces.
func SObjectByKeyPrefix(objs []SObject, recordID string) (SObject, bool) {
	if len(recordID) < 3 {
		return SObject{}, false
	}
	prefix := recordID[:3]
	for _, s := range objs {
		if strings.EqualFold(s.KeyPrefix, prefix) {
			return s, true
		}
	}
	return SObject{}, false
}
