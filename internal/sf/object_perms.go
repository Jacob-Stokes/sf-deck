package sf

// ObjectPermissions helpers — list and upsert/delete ObjectPermissions
// records via the regular REST API.
//
// ObjectPermissions are sparse like FieldPermissions: a row only exists
// when at least one of the six booleans has been explicitly set. When no
// row exists, all six flags are effectively false.

import (
	"encoding/json"
	"fmt"
)

// ObjectPermission is one row in the object-permissions grid. When ID
// is empty, no row exists yet for this parent + sobject combination.
type ObjectPermission struct {
	ID               string // "" when no row exists yet
	ParentID         string // PermissionSet Id
	SObjectType      string
	Read             bool
	Create           bool
	Edit             bool
	Delete           bool
	ViewAllRecords   bool
	ModifyAllRecords bool
}

// ListObjectPermissions returns every ObjectPermissions row for a given
// PermissionSet (by parentID). Sorted by SobjectType.
func ListObjectPermissions(target, parentID string) ([]ObjectPermission, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, ParentId, SobjectType, "+
				"PermissionsRead, PermissionsCreate, PermissionsEdit, PermissionsDelete, "+
				"PermissionsViewAllRecords, PermissionsModifyAllRecords "+
				"FROM ObjectPermissions "+
				"WHERE ParentId = '%s' "+
				"ORDER BY SobjectType",
			sqlEscape(parentID),
		),
		false, mapObjectPermission)
}

func mapObjectPermission(r map[string]any) ObjectPermission {
	row := ObjectPermission{
		ID:          asString(r["Id"]),
		ParentID:    asString(r["ParentId"]),
		SObjectType: asString(r["SobjectType"]),
	}
	if b, ok := r["PermissionsRead"].(bool); ok {
		row.Read = b
	}
	if b, ok := r["PermissionsCreate"].(bool); ok {
		row.Create = b
	}
	if b, ok := r["PermissionsEdit"].(bool); ok {
		row.Edit = b
	}
	if b, ok := r["PermissionsDelete"].(bool); ok {
		row.Delete = b
	}
	if b, ok := r["PermissionsViewAllRecords"].(bool); ok {
		row.ViewAllRecords = b
	}
	if b, ok := r["PermissionsModifyAllRecords"].(bool); ok {
		row.ModifyAllRecords = b
	}
	return row
}

// UpsertObjectPermission creates or updates one ObjectPermissions row.
// When id is empty, POSTs a new row; otherwise PATCHes the existing one.
// Returns the resulting row Id.
func UpsertObjectPermission(target, id, parentID, sobject string, r, c2, e, d, va, ma bool) (string, error) {
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	if id == "" {
		body, err := json.Marshal(map[string]any{
			"SobjectType":                 sobject,
			"ParentId":                    parentID,
			"PermissionsRead":             r,
			"PermissionsCreate":           c2,
			"PermissionsEdit":             e,
			"PermissionsDelete":           d,
			"PermissionsViewAllRecords":   va,
			"PermissionsModifyAllRecords": ma,
		})
		if err != nil {
			return "", err
		}
		path := c.APIPath("sobjects/ObjectPermissions")
		raw, err := c.post(path, body)
		if err != nil {
			return "", upgradeToSFError(err)
		}
		var resp struct {
			ID      string `json:"id"`
			Success bool   `json:"success"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("decode ObjectPermissions create: %w", err)
		}
		if !resp.Success || resp.ID == "" {
			return "", fmt.Errorf("create ObjectPermissions: unexpected response: %s", string(raw))
		}
		return resp.ID, nil
	}
	// PATCH — SobjectType and ParentId are immutable after creation.
	patchBody, err := json.Marshal(map[string]any{
		"PermissionsRead":             r,
		"PermissionsCreate":           c2,
		"PermissionsEdit":             e,
		"PermissionsDelete":           d,
		"PermissionsViewAllRecords":   va,
		"PermissionsModifyAllRecords": ma,
	})
	if err != nil {
		return "", err
	}
	path := c.APIPath("sobjects/ObjectPermissions/" + id)
	if _, err := c.patch(path, patchBody); err != nil {
		return "", upgradeToSFError(err)
	}
	return id, nil
}

// DeleteObjectPermission removes one ObjectPermissions row. Used when
// all six flags are being turned off — Salesforce prefers "no row"
// over an all-false row.
func DeleteObjectPermission(target, id string) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.APIPath("sobjects/ObjectPermissions/" + id)
	if _, err := c.delete(path); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}
