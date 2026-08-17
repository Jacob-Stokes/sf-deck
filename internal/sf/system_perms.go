package sf

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
