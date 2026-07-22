package sf

// Saved Reports — read-only browse + preview via the Analytics REST API.
//
// Phase 1 of the reports feature: list saved reports, fetch their cached
// run for inline preview. Export + force-rerun + headless live in later
// phases (see vault: salesforce-deck/research/reports-feature-research).
//
// Endpoints used:
//   GET /services/data/vXX/analytics/reports          — list saved reports
//   GET /services/data/vXX/analytics/reports/<id>     — run, returns cached
//
// Both honour Run Reports + the report's own folder/object permissions
// for the running user.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
)

// ReportSummary is the catalogue row for a saved report. One per stored
// definition. Folder + describe info comes free with the list endpoint
// so we don't need a second round-trip per row to populate the grid.
type ReportSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FolderID   string `json:"folderId,omitempty"`
	FolderName string `json:"folderName,omitempty"`
	// Format is one of "Tabular" / "Summary" / "Matrix" / "MultiBlock"
	// (the SF spelling for joined). Phase 1 previews only the first
	// three cleanly; MultiBlock surfaces as "open in SF" only.
	Format      string `json:"format,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Description string `json:"desc,omitempty"`
	LastRunDate string `json:"lastRun,omitempty"`
}

// ReportRun is the flattened shape of a single report execution. Detail
// rows live under "T!T" in factMap for tabular reports; summary/matrix
// reports have one entry per group. Phase 1 collapses everything into
// .Rows for tabular presentation; grouped previews come later.
type ReportRun struct {
	ID         string
	Name       string
	Format     string
	Columns    []ReportColumn
	Rows       []map[string]any // detail rows from factMap["T!T"].rows
	Aggregates map[string]any   // grandTotal etc.
	AllData    bool             // false when SF's 2000-row cap kicked in
	Cached     bool             // true when the result came from SF's cache
	RanAt      time.Time
}

// ReportColumn is one column on the report — API name + label so we
// can render either depending on screen width.
type ReportColumn struct {
	APIName string
	Label   string
	Type    string // string / int / double / currency / date / datetime / picklist / boolean
}

// ReportFolder is one row from the Folder sObject filtered to
// Type='Report'. Folder.Id is the FK that Report.OwnerId points at.
//
// Hierarchy: ParentID is "" for root folders, else the parent
// folder's Id. Trees can be arbitrarily deep in practice though
// most orgs run shallow (1-2 levels).
//
// AccessType:
//
//	"Public"  - visible to all users
//	"Hidden"  - private (owner-only)
//	"Shared"  - shared with specific roles / groups
type ReportFolder struct {
	ID            string
	Name          string
	DeveloperName string
	ParentID      string
	AccessType    string
}

// ListAllReports returns every Report record visible to the running
// user via SOQL. Replaces the older /analytics/reports REST call
// which only returned ~7 recently-viewed reports per user (felt like
// "My Reports" because that's effectively what it was).
//
// The full listing can be 10k+ rows on a busy org; the function
// streams via QueryREST's nextRecordsUrl path. OwnerId is the
// Report→Folder FK; the field name is FolderName (a denormalised
// string) plus OwnerId (the actual Folder.Id).
func ListAllReports(orgAlias string) ([]ReportSummary, error) {
	soql := "SELECT Id, Name, DeveloperName, FolderName, OwnerId, " +
		"Format, LastRunDate, LastModifiedDate, Description " +
		"FROM Report ORDER BY Name"
	q, err := Query(orgAlias, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]ReportSummary, 0, len(q.Records))
	for _, r := range q.Records {
		owner, _ := r["OwnerId"].(string)
		out = append(out, ReportSummary{
			ID:          asString(r["Id"]),
			Name:        asString(r["Name"]),
			Format:      asString(r["Format"]),
			Description: asString(r["Description"]),
			LastRunDate: asString(r["LastRunDate"]),
			FolderID:    owner, // Report.OwnerId IS the Folder.Id
			FolderName:  asString(r["FolderName"]),
		})
	}
	return out, nil
}

// ListReportFolders fetches every Folder record of Type='Report'
// with a non-null DeveloperName (filters out the system "00l..." rows
// SF creates without a DevName, which are private/orphaned).
//
// Hierarchy comes from ParentId — the registry walks up to build
// breadcrumbs.
func ListReportFolders(orgAlias string) ([]ReportFolder, error) {
	soql := "SELECT Id, Name, DeveloperName, ParentId, AccessType " +
		"FROM Folder WHERE Type='Report' AND DeveloperName != null " +
		"ORDER BY Name"
	q, err := Query(orgAlias, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]ReportFolder, 0, len(q.Records))
	for _, r := range q.Records {
		parent, _ := r["ParentId"].(string)
		out = append(out, ReportFolder{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			DeveloperName: asString(r["DeveloperName"]),
			ParentID:      parent,
			AccessType:    asString(r["AccessType"]),
		})
	}
	return out, nil
}

// ReportExportFormat is which output Salesforce should serialise the
// report run into. Mirrors the four-option matrix the Lightning
// export modal exposes: {Formatted, Details Only} × {xlsx, csv}.
type ReportExportFormat struct {
	// View "formatted" includes report header, groupings, and filter
	// settings; "details" emits detail rows only. Maps to the SF UI's
	// "Formatted Report" vs "Details Only" radio.
	View string // "formatted" | "details"
	// File "xlsx" or "csv".
	File string // "xlsx" | "csv"
}

// Ext returns the on-disk file extension for the format.
func (f ReportExportFormat) Ext() string {
	if f.File == "csv" {
		return "csv"
	}
	return "xlsx"
}

// MimeAccept returns the Accept-header value SF expects for this
// format. Wrong header = JSON response or UNSUPPORTED_MEDIA_TYPE.
func (f ReportExportFormat) MimeAccept() string {
	if f.File == "csv" {
		return "text/csv"
	}
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

// IsZipMagic returns true if body looks like the expected binary for
// this format. xlsx = "PK"; csv has no magic so we just check it's
// not HTML (login redirect detection).
func (f ReportExportFormat) IsValid(body []byte) bool {
	if f.File == "xlsx" {
		return len(body) >= 2 && body[0] == 'P' && body[1] == 'K'
	}
	// csv: anything that isn't an HTML page or a JSON error envelope.
	if len(body) == 0 {
		return false
	}
	if body[0] == '<' {
		return false
	}
	if body[0] == '{' || body[0] == '[' {
		// Could be a SF error JSON — but a single-cell csv could
		// also start with [. Tighten: real Analytics JSON errors
		// come back with Content-Type application/json, which the
		// caller already used to pick this code path. So: if the
		// caller asked for csv and got JSON, the caller already
		// handled it via the Content-Type detector below. Treat
		// non-HTML as csv-shaped.
	}
	return true
}

// ExportReport returns SF's xlsx bytes for the chosen report. Always
// the Formatted layout — that's the only thing SF's Analytics REST
// endpoint emits via Bearer auth. Up to 100k rows for tabular/summary
// (20k joined, 2k matrix).
//
// Conversion to Details-Only / csv happens client-side via the
// internal/postprocess package — SF doesn't expose those variants on
// the REST API regardless of query params (verified empirically; see
// vault notes). The legacy classic export servlet would but it's
// session-cookie gated.
//
// The format struct here is informational only — it's used by the UI
// to decide which post-processors to run, not by SF itself. The bytes
// returned are always SF's Formatted xlsx.
func ExportReport(orgAlias, reportID string, fmt_ ReportExportFormat) ([]byte, error) {
	c, err := RESTClient(orgAlias)
	if err != nil {
		return nil, err
	}
	path := c.APIPath("analytics/reports/" + reportID)
	q := url.Values{}
	q.Set("includeDetails", "true")
	// Bigger reports take 60-120s server-side to serialize; the default
	// 30s client timeout is the #1 reason this path falls through to
	// the (verification-prone) frontdoor cookie-session route. Give
	// the analytics endpoint headroom — if SF really hangs longer than
	// 3min the report wasn't going to complete usefully anyway.
	body, err := c.getWithAcceptTimeout(path, q,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		3*time.Minute)
	if err != nil {
		applog.Warn("export.rest_failed", map[string]any{
			"err":  err.Error(),
			"view": fmt_.View, "file": fmt_.File,
		})
		return c.classicExportViaFrontdoor(reportID)
	}
	if len(body) >= 2 && body[0] == 'P' && body[1] == 'K' {
		applog.Info("export.rest_ok", map[string]any{
			"bytes": len(body),
			"view":  fmt_.View, "file": fmt_.File,
		})
		return body, nil
	}
	dump := applog.Dump([]string{"export", "rest-invalid"}, "bin", body)
	applog.Warn("export.rest_returned_unexpected", map[string]any{
		"bytes": len(body), "dump": dump,
		"view": fmt_.View, "file": fmt_.File,
	})
	return c.classicExportViaFrontdoor(reportID)
}

// RunReport runs the saved report and returns the result. Honours SF's
// cached snapshot by default (cheaper, instant); pass forceRerun=true to
// add ?cache=false and force a real re-run server-side.
//
// Detail rows are capped at 2000 by Salesforce on the sync endpoint —
// we propagate that via ReportRun.AllData. Aggregates come back fully
// regardless.
func RunReport(orgAlias, reportID string, forceRerun bool) (ReportRun, error) {
	c, err := RESTClient(orgAlias)
	if err != nil {
		return ReportRun{}, err
	}
	path := c.APIPath("analytics/reports/" + reportID)
	q := url.Values{}
	q.Set("includeDetails", "true")
	if forceRerun {
		// SF treats this exactly like the UI's Refresh button —
		// re-runs and persists, so the next caller sees the fresh
		// result whether they asked for it or not.
		q.Set("cache", "false")
	}
	body, err := c.get(path, q)
	if err != nil {
		return ReportRun{}, err
	}
	return parseReportRun(reportID, body)
}

// parseReportRun extracts the bits of the analytics-results payload
// we render in the preview. The payload is large + structured by
// report format; we pull what's universally useful and ignore the rest
// (groupings, filter info, original definition) until later phases.
func parseReportRun(reportID string, body []byte) (ReportRun, error) {
	var raw struct {
		Attributes struct {
			ReportName string `json:"reportName"`
			Format     string `json:"reportFormat"`
		} `json:"attributes"`
		AllData        bool `json:"allData"`
		ReportMetadata struct {
			DetailColumns []string `json:"detailColumns"`
		} `json:"reportMetadata"`
		ReportExtendedMetadata struct {
			DetailColumnInfo map[string]struct {
				Label    string `json:"label"`
				DataType string `json:"dataType"`
			} `json:"detailColumnInfo"`
		} `json:"reportExtendedMetadata"`
		FactMap map[string]struct {
			Rows []struct {
				DataCells []struct {
					Label string `json:"label"`
					Value any    `json:"value"`
				} `json:"dataCells"`
			} `json:"rows"`
			Aggregates []struct {
				Label string `json:"label"`
				Value any    `json:"value"`
			} `json:"aggregates"`
		} `json:"factMap"`
		HasDetailRows bool `json:"hasDetailRows"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return ReportRun{}, fmt.Errorf("decode report run: %w", err)
	}

	out := ReportRun{
		ID:      reportID,
		Name:    raw.Attributes.ReportName,
		Format:  raw.Attributes.Format,
		AllData: raw.AllData,
		RanAt:   time.Now(),
	}
	for _, api := range raw.ReportMetadata.DetailColumns {
		info := raw.ReportExtendedMetadata.DetailColumnInfo[api]
		out.Columns = append(out.Columns, ReportColumn{
			APIName: api,
			Label:   info.Label,
			Type:    info.DataType,
		})
	}
	// factMap layout differs by format:
	//   Tabular: every detail row lives under "T!T".
	//   Summary: "T!T" holds the grand-total aggregates only; detail
	//            rows are under per-group keys ("0!T", "1!T", "0_0!T",
	//            etc. — the suffix "!T" marks a leaf row bucket).
	//   Matrix:  rows live under "<rowGroup>!<colGroup>" leaf cells;
	//            same "!T" detail-leaf convention applies for the
	//            row-axis grand totals when includeDetails=true.
	// Walk every leaf bucket and accumulate rows in the order SF
	// returned them. Aggregates (grand total) come from "T!T" if
	// present.
	keys := make([]string, 0, len(raw.FactMap))
	for k := range raw.FactMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		bucket := raw.FactMap[k]
		// Skip pure-aggregate buckets (no rows). For summary reports the
		// "T!T" bucket has no rows but does have the grand totals.
		if k == "T!T" && len(bucket.Aggregates) > 0 {
			out.Aggregates = map[string]any{}
			for _, a := range bucket.Aggregates {
				out.Aggregates[a.Label] = a.Value
			}
		}
		for _, r := range bucket.Rows {
			row := map[string]any{}
			for i, c := range r.DataCells {
				if i >= len(out.Columns) {
					break
				}
				key := out.Columns[i].APIName
				// Prefer label when present (picklist values, lookup
				// names) — that's what SF would render in the UI.
				if c.Label != "" {
					row[key] = c.Label
				} else {
					row[key] = c.Value
				}
			}
			out.Rows = append(out.Rows, row)
		}
	}
	return out, nil
}

// Field implements query.Row so the chip strip *could* eventually filter
// the reports list. Keep it minimal — Reports aren't sObject-shaped, so
// the practical chips are "by folder", "owned by me", "format = X".
// Field names match the Analytics SF surface names.
func (r ReportSummary) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Name":
		return r.Name, true
	case "FolderName":
		return r.FolderName, true
	case "FolderId":
		return r.FolderID, true
	case "Format":
		return r.Format, true
	case "Owner":
		return r.Owner, true
	case "Description":
		return r.Description, true
	case "LastRunDate":
		return r.LastRunDate, true
	}
	return nil, false
}

// String returns a short rendering used by debug logs.
func (r ReportSummary) String() string {
	parts := []string{r.Name}
	if r.FolderName != "" {
		parts = append(parts, "in "+r.FolderName)
	}
	if r.Format != "" {
		parts = append(parts, "("+strings.ToLower(r.Format)+")")
	}
	return strings.Join(parts, " ")
}
