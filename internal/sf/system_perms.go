package sf

// System permissions — ~200 boolean fields on the PermissionSet sobject
// with names starting with "Permissions" (e.g. PermissionsApiEnabled).
//
// We discover them via describe rather than hard-coding the list, so
// newly-added Salesforce permissions surface automatically.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SystemPermission is one named boolean flag on a PermissionSet.
type SystemPermission struct {
	Name  string // "ApiEnabled" (Permissions prefix stripped)
	Label string // human-readable from describe
	Value bool
}

// ListSystemPermissions fetches all system-permission boolean fields from
// the PermissionSet describe, then queries the specific permset record
// to get current values. Returns a sorted slice.
func ListSystemPermissions(target, parentID string) ([]SystemPermission, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}

	// Step 1: describe PermissionSet to get all Permissions* boolean fields.
	desc, err := c.DescribeREST("PermissionSet")
	if err != nil {
		return nil, fmt.Errorf("describe PermissionSet: %w", err)
	}
	type permField struct {
		name  string
		label string
	}
	var fields []permField
	for _, f := range desc.Fields {
		if strings.HasPrefix(f.Name, "Permissions") && f.Type == "boolean" {
			fields = append(fields, permField{
				name:  f.Name,
				label: f.Label,
			})
		}
	}
	if len(fields) == 0 {
		return nil, nil
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].name < fields[j].name
	})

	// Step 2: build chunked SELECT statements (≤ 50 fields each so
	// URLs stay well under any proxy / Salesforce URL-length cap),
	// then ship them all in a single /composite POST. One HTTP
	// round-trip total even for orgs with 600+ system perms.
	//
	// The composite endpoint counts each subrequest separately
	// against the daily API quota (~12 calls for a typical org), so
	// the caller (UI Resource[T]) caches the result aggressively —
	// system perms only change when an admin explicitly toggles them,
	// which is rare. Long TTL keeps the per-visit cost amortised.
	const chunkSize = 50
	values := map[string]bool{}

	var requests []CompositeRequest
	for i := 0; i < len(fields); i += chunkSize {
		end := i + chunkSize
		if end > len(fields) {
			end = len(fields)
		}
		chunk := fields[i:end]
		var fieldNames []string
		for _, f := range chunk {
			fieldNames = append(fieldNames, f.name)
		}
		soql := fmt.Sprintf(
			"SELECT %s FROM PermissionSet WHERE Id = '%s'",
			strings.Join(fieldNames, ", "), sqlEscape(parentID),
		)
		requests = append(requests, CompositeRequest{
			Method:      "GET",
			URL:         c.QueryURL(soql, false),
			ReferenceID: fmt.Sprintf("chunk%d", i/chunkSize),
		})
	}
	responses, err := c.Composite(requests, false)
	if err != nil {
		return nil, fmt.Errorf("query PermissionSet system perms: %w", err)
	}
	results, _ := CompositeQueryResults(responses)
	for _, q := range results {
		if len(q.Records) == 0 {
			continue
		}
		rec := q.Records[0]
		for k, v := range rec {
			if b, ok := v.(bool); ok {
				values[k] = b
			}
		}
	}

	out := make([]SystemPermission, 0, len(fields))
	for _, f := range fields {
		out = append(out, SystemPermission{
			Name:  strings.TrimPrefix(f.name, "Permissions"),
			Label: f.label,
			Value: values[f.name],
		})
	}
	return out, nil
}

// TogglePermissionSetBool patches a single boolean field on a
// PermissionSet record. fieldAPIName is the full Permissions* name
// (e.g. "PermissionsApiEnabled").
func TogglePermissionSetBool(target, parentID, fieldAPIName string, val bool) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		fieldAPIName: val,
	})
	if err != nil {
		return err
	}
	path := c.APIPath("sobjects/PermissionSet/" + parentID)
	if _, err := c.patch(path, body); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}
