package sf

// Record edit (UpdateRecord) + SOSL record search primitives.
//
// UpdateRecord PATCHes /services/data/vNN/sobjects/<Type>/<Id> with
// a JSON body of {field: value} pairs. The server coerces strings
// to typed values per the field's describe — so we send strings and
// rely on SF to parse dates / numbers / booleans / etc.
//
// SearchRecords runs a SOSL FIND query to back the reference-lookup
// editor. Returns Id + Name (or whatever the object's name-field
// is) of matching records for the requested sObject.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// FieldError describes one rejection from a PATCH response. Salesforce
// returns a JSON array of objects with errorCode, message, and an
// optional fields array (the field names the error applies to).
type FieldError struct {
	Fields    []string `json:"fields"`
	Message   string   `json:"message"`
	ErrorCode string   `json:"errorCode"`
}

// String renders the error for flash-bar / banner display. Falls
// back gracefully when fields is empty (some errors are record-level
// not field-level).
func (e FieldError) String() string {
	if len(e.Fields) > 0 {
		return strings.Join(e.Fields, ",") + ": " + e.Message
	}
	if e.ErrorCode != "" {
		return e.ErrorCode + ": " + e.Message
	}
	return e.Message
}

// UpdateRecord PATCHes a single record. fields is the diff — every
// key becomes a JSON property in the request body. Empty diff is a
// no-op (returns nil, nil) so callers don't need to guard.
//
// On 2xx returns (nil, nil). On 4xx the SF JSON error array is
// parsed into []FieldError and returned. On network / transport
// failures returns the raw error.
//
// Type coercion is server-side. Send strings; SF parses based on
// each field's describe type. Sending `null` for a value clears the
// field (record this in your caller; we don't transform).
func (c *Client) UpdateRecord(sobject, id string, fields map[string]any) ([]FieldError, error) {
	if sobject == "" || id == "" {
		return nil, fmt.Errorf("sobject and id are required")
	}
	if len(fields) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode record body: %w", err)
	}
	path := c.APIPath("sobjects/" + sobject + "/" + id)
	resp, err := c.patch(path, body)
	if err != nil {
		// Most failures land as a 400-class with a JSON error array.
		// doWithRetry wraps non-2xx as sfHTTPError; try to parse the
		// body before giving up.
		var httpErr *sfHTTPError
		if asHTTPError(err, &httpErr) {
			if parsed := parseFieldErrors(httpErr.Body); len(parsed) > 0 {
				return parsed, nil
			}
		}
		return nil, err
	}
	// Successful PATCH returns 204 No Content (resp is empty).
	_ = resp
	return nil, nil
}

// UpdateRecordAlias is the alias-flavoured entry point.
func UpdateRecordAlias(alias, sobject, id string, fields map[string]any) ([]FieldError, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return nil, err
	}
	return c.UpdateRecord(sobject, id, fields)
}

// CreateRecord POSTs a new record. fields is the body (every key
// becomes a JSON property). Returns the new record's id on success
// or []FieldError when SF rejects the create — same error shape as
// UpdateRecord so callers handle field-level + record-level
// validation failures uniformly.
//
// On transport / 5xx failure returns (nil, "", err); on field
// validation returns (errors, "", nil) with id empty. On success
// returns (nil, "<id>", nil).
func (c *Client) CreateRecord(sobject string, fields map[string]any) ([]FieldError, string, error) {
	if sobject == "" {
		return nil, "", fmt.Errorf("sobject is required")
	}
	if len(fields) == 0 {
		return nil, "", fmt.Errorf("at least one field is required")
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, "", fmt.Errorf("encode record body: %w", err)
	}
	path := c.APIPath("sobjects/" + sobject)
	resp, err := c.post(path, body)
	if err != nil {
		var httpErr *sfHTTPError
		if asHTTPError(err, &httpErr) {
			if parsed := parseFieldErrors(httpErr.Body); len(parsed) > 0 {
				return parsed, "", nil
			}
		}
		return nil, "", err
	}
	// SF returns {"id": "001…", "success": true, "errors": []} on 201.
	var out struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, "", fmt.Errorf("decode create response: %w", err)
	}
	if !out.Success {
		return nil, "", fmt.Errorf("create rejected without id")
	}
	return nil, out.ID, nil
}

// CreateRecordAlias is the alias-flavoured entry point for CreateRecord.
func CreateRecordAlias(alias, sobject string, fields map[string]any) ([]FieldError, string, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return nil, "", err
	}
	return c.CreateRecord(sobject, fields)
}

// DeleteRecord removes a record by id. SF returns 204 No Content
// on success. Errors are surfaced verbatim — there's no per-field
// validation channel for delete (the whole record either goes or
// it doesn't), so the FieldError path is intentionally absent
// here.
func (c *Client) DeleteRecord(sobject, id string) error {
	if sobject == "" || id == "" {
		return fmt.Errorf("sobject and id are required")
	}
	path := c.APIPath("sobjects/" + sobject + "/" + id)
	if _, err := c.delete(path); err != nil {
		return err
	}
	return nil
}

// DeleteRecordAlias is the alias-flavoured entry point for DeleteRecord.
func DeleteRecordAlias(alias, sobject, id string) error {
	c, err := RESTClient(alias)
	if err != nil {
		return err
	}
	return c.DeleteRecord(sobject, id)
}

// parseFieldErrors decodes the JSON array Salesforce returns on a
// PATCH rejection. Empty body or unparseable JSON returns nil so
// callers fall back to the raw HTTP error.
func parseFieldErrors(body []byte) []FieldError {
	if len(body) == 0 {
		return nil
	}
	var out []FieldError
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

// asHTTPError is a small wrapper around errors.As(err, &target)
// kept local to avoid pulling in the errors package across files
// that already use a plain `fmt.Errorf` style.
func asHTTPError(err error, target **sfHTTPError) bool {
	for err != nil {
		if e, ok := err.(*sfHTTPError); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// SearchHit is one result row from SearchRecords. Id is the 18-char
// record id; Name is the human-readable label (the object's name
// field — usually Name, sometimes Subject / Title / etc).
type SearchHit struct {
	ID   string
	Name string
}

// SearchRecords runs a SOSL FIND query against one sObject and
// returns up to `limit` (Id, Name) tuples. Backs the reference-
// lookup field editor.
//
// nameField is the object's display field — "Name" on most objects,
// "Subject" on Task / Case, etc. Caller resolves this via the
// describe's nameField property; we don't pull the describe here
// to keep this primitive lightweight.
func (c *Client) SearchRecords(sobject, nameField, term string, limit int) ([]SearchHit, error) {
	if sobject == "" || term == "" {
		return nil, nil
	}
	if nameField == "" {
		nameField = "Name"
	}
	if limit <= 0 {
		limit = 20
	}
	// SOSL FIND requires the term wrapped in braces. Salesforce
	// auto-applies wildcard behaviour for word-stems; for exact
	// substring match the caller can include '*' wildcards.
	sosl := fmt.Sprintf("FIND {%s} IN NAME FIELDS RETURNING %s(Id, %s) LIMIT %d",
		escapeSOSLBraces(term), sobject, nameField, limit)
	q := url.Values{}
	q.Set("q", sosl)
	raw, err := c.get(c.APIPath("search"), q)
	if err != nil {
		return nil, fmt.Errorf("sosl search: %w", err)
	}
	var parsed struct {
		SearchRecords []map[string]any `json:"searchRecords"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode sosl: %w", err)
	}
	out := make([]SearchHit, 0, len(parsed.SearchRecords))
	for _, r := range parsed.SearchRecords {
		id, _ := r["Id"].(string)
		name, _ := r[nameField].(string)
		if id == "" {
			continue
		}
		out = append(out, SearchHit{ID: id, Name: name})
	}
	return out, nil
}

// SearchRecordsAlias is the alias-flavoured entry point.
func SearchRecordsAlias(alias, sobject, nameField, term string, limit int) ([]SearchHit, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return nil, err
	}
	return c.SearchRecords(sobject, nameField, term, limit)
}

// escapeSOSLBraces escapes characters that have special meaning
// inside SOSL FIND {...} clauses. The full reserved set is
// documented at https://developer.salesforce.com/docs/atlas.en-us.soql_sosl.meta/soql_sosl/sforce_api_calls_sosl_find.htm
// — we cover the common ones; users passing literal SOSL operators
// stay responsible for their own escaping.
func escapeSOSLBraces(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`{`, `\{`,
		`}`, `\}`,
	)
	return r.Replace(s)
}
