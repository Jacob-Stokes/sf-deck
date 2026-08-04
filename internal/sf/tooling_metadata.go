package sf

import (
	"encoding/json"
	"fmt"
)

// GetToolingMetadata fetches a single Tooling-sobject row and returns
// its Metadata map. Used by the UI to pre-populate edit modals for
// properties the describe API doesn't expose (e.g. CustomField.
// description). Light wrapper around the same GET that
// UpdateToolingMetadata performs internally.
func GetToolingMetadata(target, sobjectType, id string) (map[string]any, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	path := c.ToolingPath("sobjects/" + sobjectType + "/" + id)
	raw, err := c.get(path, nil)
	if err != nil {
		return nil, upgradeToSFError(err)
	}
	var current struct {
		Metadata map[string]any `json:"Metadata"`
	}
	if err := json.Unmarshal(raw, &current); err != nil {
		return nil, fmt.Errorf("decode %s metadata: %w", sobjectType, err)
	}
	if current.Metadata == nil {
		current.Metadata = map[string]any{}
	}
	return current.Metadata, nil
}

// CreateToolingMetadata POSTs a new Tooling sobject row with
// {FullName, Metadata}. Used to create ValidationRules, CustomFields,
// CustomObjects, etc. from scratch. Returns the new Id on success.
//
//	fullName  — Tooling "full name" for the new row (validation rules
//	            use "<object>.<rule>"; custom fields "<object>.<field>__c")
//	metadata  — the Metadata object literal. The API validates it in
//	            the same strict way as update, so callers should send
//	            everything the new record needs to be well-formed.
func CreateToolingMetadata(target, sobjectType, fullName string, metadata map[string]any) (string, error) {
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	path := c.ToolingPath("sobjects/" + sobjectType)
	body, err := json.Marshal(map[string]any{
		"FullName": fullName,
		"Metadata": metadata,
	})
	if err != nil {
		return "", err
	}
	raw, err := c.post(path, body)
	if err != nil {
		return "", upgradeToSFError(err)
	}
	// Tooling create returns {id, success, errors[]}.
	var out struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode create %s response: %w", sobjectType, err)
	}
	if !out.Success || out.ID == "" {
		return "", fmt.Errorf("create %s: unexpected response shape: %s", sobjectType, string(raw))
	}
	return out.ID, nil
}

// DeleteToolingMetadata DELETEs a Tooling sobject row by Id. Used for
// destructive operations on metadata (delete a validation rule, a
// custom field, etc.). Caller is responsible for the destructive
// confirmation UX + the safety gate.
func DeleteToolingMetadata(target, sobjectType, id string) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.ToolingPath("sobjects/" + sobjectType + "/" + id)
	if _, err := c.delete(path); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}

// UpdateToolingMetadata is the generic read-modify-write pattern used
// by every Tooling-API write that touches a sobject's `Metadata`
// field. The Tooling API validates the whole Metadata object on every
// PATCH, so partial writes fail with "FIELD_INTEGRITY_EXCEPTION" —
// we always fetch the current state, merge the caller's patch, and
// PATCH the merged object back.
//
//	sobjectType  — Tooling sobject name (CustomField, CustomObject,
//	               ValidationRule, ApexTrigger, FlexiPage, …)
//	id           — 18-char Tooling Id of the record
//	patch        — keys to overwrite on Metadata; anything not in the
//	               patch is preserved from the fetched version
//
// Returns an *SFError on 4xx/5xx so callers get the typed Kind +
// Hint without re-parsing. Safety gating is the caller's job.
func UpdateToolingMetadata(target, sobjectType, id string, patch map[string]any) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.ToolingPath("sobjects/" + sobjectType + "/" + id)

	// GET → decode FullName + Metadata.
	raw, err := c.get(path, nil)
	if err != nil {
		return upgradeToSFError(err)
	}
	var current struct {
		FullName string         `json:"FullName"`
		Metadata map[string]any `json:"Metadata"`
	}
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("decode current %s: %w", sobjectType, err)
	}
	if current.Metadata == nil {
		current.Metadata = map[string]any{}
	}

	// Merge on top.
	for k, v := range patch {
		current.Metadata[k] = v
	}

	// PATCH back the whole shape Tooling expects.
	body, err := json.Marshal(map[string]any{
		"FullName": current.FullName,
		"Metadata": current.Metadata,
	})
	if err != nil {
		return err
	}
	if _, err := c.patch(path, body); err != nil {
		return upgradeToSFError(err)
	}
	return nil
}

// upgradeToSFError coerces REST client errors to *SFError if possible
// so callers always get the classified Kind + Hint. Keeping this
// close to the shared write function so every write site looks the
// same.
func upgradeToSFError(err error) error {
	if typed := AsSFError(err); typed != nil {
		return typed
	}
	return err
}
