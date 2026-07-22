package sf

// Composite requests.
//
// Salesforce's /composite endpoint accepts up to 25 subrequests in one
// round-trip. Used here to batch SOQL queries that would otherwise be
// issued serially (multiple describes, a PermissionSet+Profile lookup
// pair, /home's five independent data queries, etc.). Round-trip
// latency dominates each REST call, so bundling is a near-linear
// speedup for parallel-queryable work.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// CompositeMaxSubrequests is the hard Salesforce cap.
const CompositeMaxSubrequests = 25

// CompositeRequest is one subrequest in a batch. ReferenceID is the
// key under which the matching CompositeResponse arrives, so callers
// can identify responses independent of submission order.
type CompositeRequest struct {
	Method      string // "GET", "POST", "PATCH", "DELETE"
	URL         string // e.g. APIPath("query") + "?q=..." or APIPath("sobjects/FieldPermissions")
	ReferenceID string // caller-chosen, must be unique within the batch
	Body        any    // JSON-encodable; ignored for GET/DELETE
}

// CompositeResponse is one subresult.
type CompositeResponse struct {
	Body           json.RawMessage   `json:"body"`
	HTTPHeaders    map[string]string `json:"httpHeaders"`
	HTTPStatusCode int               `json:"httpStatusCode"`
	ReferenceID    string            `json:"referenceId"`
}

// Composite issues a batch. allOrNone=false lets successful subrequests
// commit even when others fail — we default to false because /home-style
// dashboards prefer partial data to none. Switches to true for
// transactional writes (not used today but the knob is cheap).
//
// Subrequests beyond the 25-cap are auto-chunked across multiple HTTP
// round-trips. Responses are merged in submission order.
func (c *Client) Composite(requests []CompositeRequest, allOrNone bool) ([]CompositeResponse, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	var out []CompositeResponse
	for start := 0; start < len(requests); start += CompositeMaxSubrequests {
		end := start + CompositeMaxSubrequests
		if end > len(requests) {
			end = len(requests)
		}
		chunk, err := c.compositeOnce(requests[start:end], allOrNone)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// compositeOnce issues a single /composite HTTP call (≤25 subrequests).
func (c *Client) compositeOnce(requests []CompositeRequest, allOrNone bool) ([]CompositeResponse, error) {
	type subreq struct {
		Method      string `json:"method"`
		URL         string `json:"url"`
		ReferenceID string `json:"referenceId"`
		Body        any    `json:"body,omitempty"`
	}
	payload := struct {
		AllOrNone        bool     `json:"allOrNone"`
		CompositeRequest []subreq `json:"compositeRequest"`
	}{AllOrNone: allOrNone}
	for _, r := range requests {
		if r.ReferenceID == "" {
			return nil, fmt.Errorf("composite: empty ReferenceID on %s %s", r.Method, r.URL)
		}
		payload.CompositeRequest = append(payload.CompositeRequest, subreq(r))
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	raw, err := c.post(c.APIPath("composite"), body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		CompositeResponse []CompositeResponse `json:"compositeResponse"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("composite: decode: %w", err)
	}
	return resp.CompositeResponse, nil
}

// QueryURL builds a subrequest URL for a SOQL query against the given
// /composite endpoint. Callers stitch this into a CompositeRequest's
// URL field. Tooling SOQL uses a different base path.
func (c *Client) QueryURL(soql string, tooling bool) string {
	if tooling {
		return c.ToolingPath("query") + "?q=" + url.QueryEscape(soql)
	}
	return c.APIPath("query") + "?q=" + url.QueryEscape(soql)
}

// CompositeQueryResults is a helper that decodes every subresponse as
// a QueryResult, keyed by ReferenceID. Subrequests that failed
// (HTTPStatusCode >= 400) are reported via the returned errors map
// rather than aborting the whole batch.
func CompositeQueryResults(responses []CompositeResponse) (results map[string]QueryResult, errs map[string]error) {
	results = make(map[string]QueryResult, len(responses))
	errs = make(map[string]error, 0)
	for _, r := range responses {
		if r.HTTPStatusCode >= 400 {
			errs[r.ReferenceID] = fmt.Errorf("subrequest %s: %d %s",
				r.ReferenceID, r.HTTPStatusCode, strings.TrimSpace(string(r.Body)))
			continue
		}
		var q QueryResult
		if err := json.Unmarshal(r.Body, &q); err != nil {
			errs[r.ReferenceID] = fmt.Errorf("subrequest %s: decode: %w", r.ReferenceID, err)
			continue
		}
		results[r.ReferenceID] = q
	}
	return results, errs
}
