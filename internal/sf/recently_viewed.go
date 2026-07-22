package sf

// recently_viewed.go — generic RecentlyViewed reader.
//
// Salesforce tracks "things this user recently viewed" server-side
// and exposes them via the RecentlyViewed standard sObject. The
// list is records-only (no flows / apex / setup) but is global —
// it captures Lightning views, mobile views, sf-deck views (since
// those open Lightning), API-driven opens, etc.
//
// We expose this as a small, generic API rather than baking SOQL
// strings into the UI layer:
//
//	rows, err := sf.ListRecentlyViewed(orgUser, sf.RecentlyViewedOpts{
//	    Limit: 50,
//	})
//
// or per-sObject:
//
//	rows, err := sf.ListRecentlyViewed(orgUser, sf.RecentlyViewedOpts{
//	    SObject: "Account",
//	    Limit:   25,
//	})
//
// Per-sObject queries use the `RECENTLY VIEWED` SOQL clause on the
// target sObject; universal queries hit the `RecentlyViewed`
// standard sObject directly so a single round-trip returns mixed
// kinds. Both paths yield the same RecentlyViewedRow shape so
// callers don't branch.

import (
	"fmt"
	"strings"
	"time"
)

// RecentlyViewedRow is one entry from the server's recently-viewed
// list. Mirrors the Salesforce wire shape but with parsed types
// (LastViewedDate as time.Time) and a normalised Type field.
//
// SObjectType is the API name ("Account", "MyCustomObject__c", …).
// For per-sObject queries the renderer already knows the type, but
// keeping the field on every row means callers don't have to
// special-case the universal vs per-sObject paths.
type RecentlyViewedRow struct {
	ID             string
	Name           string
	SObjectType    string // "Type" on the wire
	LastViewedDate time.Time
}

// RecentlyViewedOpts controls the query shape.
type RecentlyViewedOpts struct {
	// SObject narrows the query to a single sObject. Empty = universal
	// (cross-sObject) — uses the RecentlyViewed standard object.
	SObject string

	// Limit caps the number of rows. 0 → 50 (Salesforce-side default
	// for RECENTLY VIEWED is 200; we surface a smaller default to keep
	// the rail tight, callers override when they want more).
	Limit int

	// IncludeName is a hint for per-sObject queries: when true, the
	// SOQL also pulls `Name` (or the sObject's name field). Set
	// false when the caller only needs IDs and wants to avoid the
	// extra column projection. Defaults to true.
	IncludeName bool

	// ExcludeTypes is the sObject-type noise list for the universal
	// (cross-sObject) query — dropped via WHERE Type NOT IN (…). Nil
	// falls back to the built-in defaultRecentNoiseSFTypes so a
	// zero-value opt still filters. Pass an explicit empty slice to
	// filter nothing. Ignored on the per-sObject path.
	ExcludeTypes []string
}

// ListRecentlyViewed runs the RecentlyViewed query against the given
// org and returns the rows in server-supplied order (most recent
// first). Errors propagate from the underlying Query helper.
//
// Universal path (opts.SObject == ""): one SOQL against the
// `RecentlyViewed` sObject. Yields mixed-kind results.
//
// Per-sObject path: SOQL on the target sObject filtered by
// LastViewedDate. That avoids the global top-N starvation from the
// RecentlyViewed object while still using Salesforce's per-record
// viewed timestamp.
func ListRecentlyViewed(orgTarget string, opts RecentlyViewedOpts) ([]RecentlyViewedRow, error) {
	soql := recentlyViewedSOQL(opts)
	res, err := Query(orgTarget, soql, false)
	if err != nil {
		return nil, err
	}

	out := make([]RecentlyViewedRow, 0, len(res.Records))
	for _, rec := range res.Records {
		row := RecentlyViewedRow{
			ID:   asString(rec["Id"]),
			Name: asString(rec["Name"]),
		}
		row.SObjectType = asString(rec["Type"])
		if row.SObjectType == "" && opts.SObject != "" {
			row.SObjectType = opts.SObject
		}
		if v, ok := rec["LastViewedDate"]; ok {
			row.LastViewedDate = parseSFDate(v)
		}
		out = append(out, row)
	}
	return out, nil
}

func recentlyViewedSOQL(opts RecentlyViewedOpts) string {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	if opts.SObject == "" {
		// nil → built-in defaults; explicit empty slice → filter nothing.
		exclude := opts.ExcludeTypes
		if exclude == nil {
			exclude = defaultRecentNoiseSFTypes
		}
		where := ""
		if len(exclude) > 0 {
			where = "WHERE Type NOT IN (" + soqlStringInList(exclude) + ") "
		}
		return fmt.Sprintf(
			"SELECT Id, Name, Type, LastViewedDate "+
				"FROM RecentlyViewed "+
				"%s"+
				"ORDER BY LastViewedDate DESC NULLS LAST "+
				"LIMIT %d",
			where, limit,
		)
	}
	return fmt.Sprintf(
		"SELECT Id, Name, LastViewedDate "+
			"FROM %s "+
			"WHERE LastViewedDate != NULL "+
			"ORDER BY LastViewedDate DESC NULLS LAST "+
			"LIMIT %d",
		opts.SObject, limit,
	)
}

// defaultRecentNoiseSFTypes is the fallback exclusion list for the
// universal RecentlyViewed query — used when a caller doesn't supply
// opts.ExcludeTypes. The user-facing SSOT is
// settings.RecentExcludedSFTypes (which mirrors this default and is
// threaded in via opts); this copy just keeps a zero-value opt honest.
// Flow / OmniStudio builder internals + admin artifacts; ListView /
// Report / Dashboard are excluded from THIS list (they're real, with
// their own recent chips).
var defaultRecentNoiseSFTypes = []string{
	"FlowRecordElement", "FlowRecordVersion", "FlowRecord",
	"OmniProcessElement", "OmniDataTransformItem",
	"BatchJob", "CalculationMatrix", "ActionPlanTemplateVersion",
	"DataLakeObjectInstance", "DataSpace",
}

// soqlStringInList renders a slice as a quoted, comma-separated SOQL
// IN-list body. Values are sqlEscape'd so a stray quote can't break the
// query. Returns "" for an empty slice.
func soqlStringInList(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = "'" + sqlEscape(v) + "'"
	}
	return strings.Join(quoted, ", ")
}

// Targets implements Openable. A RecentlyViewedRow is functionally
// a record reference, so it routes to the same Lightning destinations
// as any other record (record detail, edit, related, etc.).
func (r RecentlyViewedRow) Targets() []OpenTarget {
	if r.SObjectType == "" || r.ID == "" {
		return []OpenTarget{{ID: "home", Label: "Home", Path: "/lightning/page/home"}}
	}
	return []OpenTarget{
		{ID: "view", Label: "Record detail", Shortcut: "r",
			Path: "/lightning/r/" + r.SObjectType + "/" + r.ID + "/view"},
		{ID: "edit", Label: "Edit record", Shortcut: "e",
			Path: "/lightning/r/" + r.SObjectType + "/" + r.ID + "/edit"},
		{ID: "list", Label: "Records list",
			Path: "/lightning/o/" + r.SObjectType + "/list"},
	}
}

// parseSFDate normalises Salesforce's RFC3339-with-zone date strings
// into time.Time. Returns the zero value when the input isn't
// parseable — callers fall back to displaying "—".
func parseSFDate(v any) time.Time {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
