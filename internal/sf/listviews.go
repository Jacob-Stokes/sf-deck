package sf

import (
	"encoding/json"
	"fmt"
)

// ListView is a Salesforce-defined list view on an sObject. Each user
// sees their own + shared ones per SF's permission rules, so the same
// query run as different users returns different lists.
type ListView struct {
	ID               string `json:"Id"`
	Name             string `json:"Name"`
	DeveloperName    string `json:"DeveloperName"`
	SobjectType      string `json:"SobjectType"`
	IsSoqlCompatible bool   `json:"IsSoqlCompatible"`
}

// ListViews lists the accessible list views for a given sObject via
// SOQL. Read-only.
func ListViews(orgAlias, sobjectName string) ([]ListView, error) {
	soql := fmt.Sprintf(
		"SELECT Id, Name, DeveloperName, SobjectType, IsSoqlCompatible "+
			"FROM ListView WHERE SobjectType = '%s' ORDER BY Name",
		sqlEscape(sobjectName),
	)
	q, err := Query(orgAlias, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]ListView, 0, len(q.Records))
	for _, r := range q.Records {
		lv := ListView{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			DeveloperName: asString(r["DeveloperName"]),
			SobjectType:   asString(r["SobjectType"]),
		}
		if b, ok := r["IsSoqlCompatible"].(bool); ok {
			lv.IsSoqlCompatible = b
		}
		out = append(out, lv)
	}
	return out, nil
}

// ListViewColumn is one column entry from a list view's describe /
// results endpoint. We only keep the fields needed to render.
type ListViewColumn struct {
	Name   string `json:"fieldNameOrPath"`
	Label  string `json:"label"`
	Type   string `json:"type"`
	Hidden bool   `json:"hidden"`
}

// ListViewResult is the shape of a list view run — columns + records
// + the truncation indicator from SF's response. Salesforce reports
// total matching rows in `size` and pagination state in `done`; we
// surface both so the UI can flag "showing 200 of N — import the
// view to see all" when the preview cap clipped the slice.
type ListViewResult struct {
	Columns []ListViewColumn `json:"columns"`
	// `records` is a slice of maps. Each map has "columns" which itself
	// is a slice of {fieldNameOrPath, value, label}. We flatten to a
	// simple map[string]any keyed by fieldNameOrPath for display.
	Records []map[string]any `json:"-"`
	// TotalSize is SF's `size` field — the unbounded match count.
	// When TotalSize > len(Records) the preview is truncated by our
	// 200-row request cap (or SF's own paging boundary).
	TotalSize int `json:"size"`
	// Done mirrors SF's `done` field. False means there's more — SF
	// also returns a nextPageUrl which we don't currently follow
	// (preview is by-design discovery, not exhaustive iteration).
	Done bool `json:"done"`
}

// RunListView runs the list view via REST and returns its columns +
// flattened record rows. Read-only.
//
// limit is the row cap; <=0 falls back to DefaultListViewPreviewLimit.
// Caller threads through settings.ListViewPreviewLimit() so the user
// can tune it.
//
// Fast path: REST-direct via the cached bearer token. Falls back to
// `sf api request rest` if REST bootstrap isn't available.
func RunListView(orgAlias, sobjectName, listViewID string, limit int) (ListViewResult, error) {
	if c, err := RESTClient(orgAlias); err == nil {
		return c.ListViewResultsREST(sobjectName, listViewID, limit)
	}
	// CLI-fallback path. Resolve the API version through the same
	// helper the REST client uses so we don't drift behind on
	// pinned-version paths.
	path := fmt.Sprintf("/services/data/v%s/sobjects/%s/listviews/%s/results",
		APIVersionForAlias(orgAlias), sobjectName, listViewID)
	raw, err := runSF("api", "request", "rest", path, "-o", orgAlias)
	if err != nil {
		return ListViewResult{}, err
	}
	var parsed struct {
		Columns []ListViewColumn `json:"columns"`
		Records []struct {
			Columns []struct {
				FieldNameOrPath string `json:"fieldNameOrPath"`
				Value           any    `json:"value"`
				Label           string `json:"label"`
			} `json:"columns"`
			ID string `json:"Id"`
		} `json:"records"`
		Size int  `json:"size"`
		Done bool `json:"done"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ListViewResult{}, fmt.Errorf("parse listview results: %w", err)
	}
	out := ListViewResult{
		Columns:   parsed.Columns,
		TotalSize: parsed.Size,
		Done:      parsed.Done,
	}
	for _, r := range parsed.Records {
		row := map[string]any{"Id": r.ID}
		for _, c := range r.Columns {
			if c.Label != "" {
				row[c.FieldNameOrPath] = c.Label
			} else {
				row[c.FieldNameOrPath] = c.Value
			}
		}
		out.Records = append(out.Records, row)
	}
	return out, nil
}
