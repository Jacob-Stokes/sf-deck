package sf

// Field-Level Security (FLS) helpers — list + update FieldPermissions
// records via the regular REST API.
//
// FLS in Salesforce is stored on the FieldPermissions sobject. Each
// row carries (Field, ParentId, PermissionsRead, PermissionsEdit).
// Parent is always a PermissionSet — profiles own an implicit
// PermissionSet whose ProfileId is non-null, so filtering by
// ParentId handles both "a profile's FLS" and "a perm set's FLS"
// uniformly.
//
// Gotcha: FieldPermissions rows exist only for fields that have
// been explicitly set. When a field was created with its default
// FLS and never touched, there's no row — callers render that as
// "read=false, edit=false" (which IS the effective permission for
// a missing row).

import (
	"encoding/json"
	"fmt"
)

// FieldPermissionRow is one row in the FLS grid.
type FieldPermissionRow struct {
	ID       string // FieldPermissions record Id — empty means "no row exists yet"
	Field    string // "<sobject>.<fieldname>", matches the Field column
	ParentID string // PermissionSet Id
	Read     bool
	Edit     bool
}

// FLSPickerEntry is one selectable scope for the FLS grid. Either
// a profile (ProfileID non-empty) or a permission set (IsPermSet
// true). Surface both in one list so callers can present a single
// picker.
type FLSPickerEntry struct {
	ID        string // PermissionSet Id (always)
	Name      string // internal api name — stable key
	Label     string // user-facing label
	ProfileID string // non-empty when this permset is a profile's implicit permset
	// IsPermSet == true means standalone PermissionSet (not a profile).
	IsPermSet bool
}

// ListFLSPickerEntries returns every permset + profile (the latter via
// their implicit permset) in the org. Sorted by (IsPermSet, Label)
// so profiles surface first in the picker.
//
// This is a lightweight shape specifically for populating the FLS
// scope picker. For full-fidelity permission-set metadata (used by
// the /perms tab) see ListPermissionSets in permsets.go.
func ListFLSPickerEntries(target string) ([]FLSPickerEntry, error) {
	return queryRows(target,
		"SELECT Id, Name, Label, ProfileId, Profile.Name, IsOwnedByProfile "+
			"FROM PermissionSet "+
			"ORDER BY IsOwnedByProfile DESC, Label",
		false, mapFLSPickerEntry)
}

func mapFLSPickerEntry(r map[string]any) FLSPickerEntry {
	row := FLSPickerEntry{
		ID:        asString(r["Id"]),
		Name:      asString(r["Name"]),
		Label:     asString(r["Label"]),
		ProfileID: asString(r["ProfileId"]),
	}
	// IsOwnedByProfile is a bool; when true, this permset is a
	// profile's implicit one, and we should surface the profile's
	// label instead (which is what the admin actually recognizes).
	if b, ok := r["IsOwnedByProfile"].(bool); ok && b {
		row.IsPermSet = false
		if p, ok := r["Profile"].(map[string]any); ok {
			if n := asString(p["Name"]); n != "" {
				row.Label = n
			}
		}
	} else {
		row.IsPermSet = true
	}
	return row
}

// ListFieldPermissions returns every FieldPermissions row for a
// given (sobject, parentID) pair. Parent is a PermissionSet Id —
// either a standalone permset or a profile's implicit one.
func ListFieldPermissions(target, sobject, parentID string) ([]FieldPermissionRow, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, Field, ParentId, PermissionsRead, PermissionsEdit "+
				"FROM FieldPermissions "+
				"WHERE SobjectType = '%s' AND ParentId = '%s'",
			sqlEscape(sobject), sqlEscape(parentID),
		),
		false, mapFieldPermissionRow)
}

func mapFieldPermissionRow(r map[string]any) FieldPermissionRow {
	row := FieldPermissionRow{
		ID:       asString(r["Id"]),
		Field:    asString(r["Field"]),
		ParentID: asString(r["ParentId"]),
	}
	if b, ok := r["PermissionsRead"].(bool); ok {
		row.Read = b
	}
	if b, ok := r["PermissionsEdit"].(bool); ok {
		row.Edit = b
	}
	return row
}

// UpsertFieldPermission creates or updates one FLS row. When id is
// empty, POSTs a new FieldPermissions row; otherwise PATCHes the
// existing one. Returns the resulting row Id.
//
// Salesforce rule: Edit=true implies Read=true. We enforce that
// here so callers can set either flag without worrying about the
// invariant.
func UpsertFieldPermission(target, id, sobject, field, parentID string, read, edit bool) (string, error) {
	if edit {
		read = true
	}
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"SobjectType":     sobject,
		"Field":           field,
		"ParentId":        parentID,
		"PermissionsRead": read,
		"PermissionsEdit": edit,
	})
	if err != nil {
		return "", err
	}
	if id == "" {
		path := c.APIPath("sobjects/FieldPermissions")
		raw, err := c.post(path, body)
		if err != nil {
			return "", upgradeToSFError(err)
		}
		var resp struct {
			ID      string `json:"id"`
			Success bool   `json:"success"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", fmt.Errorf("decode FieldPermissions create: %w", err)
		}
		if !resp.Success || resp.ID == "" {
			return "", fmt.Errorf("create FieldPermissions: unexpected response: %s", string(raw))
		}
		return resp.ID, nil
	}
	// PATCH the minimum fields (Sobject/Field/Parent are immutable
	// after creation, so they're omitted from the update body).
	patchBody, err := json.Marshal(map[string]any{
		"PermissionsRead": read,
		"PermissionsEdit": edit,
	})
	if err != nil {
		return "", err
	}
	path := c.APIPath("sobjects/FieldPermissions/" + id)
	if _, err := c.patch(path, patchBody); err != nil {
		return "", upgradeToSFError(err)
	}
	return id, nil
}

// DeleteFieldPermission removes one FLS row. Used when both
// Read and Edit go back to false — the row becomes superfluous
// and Salesforce prefers "no row" to "row with everything false"
// for cleanliness + reduced storage.
func DeleteFieldPermission(target, id string) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.APIPath("sobjects/FieldPermissions/" + id)
	if _, err := c.delete(path); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}
