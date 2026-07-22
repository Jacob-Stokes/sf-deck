package sf

// record_relationships.go — SOQL helpers that resolve, for a given
// drilled-in record:
//
//   1. The Name (or fallback display field) of every record this
//      record references via a Lookup / Master-Detail field. ONE
//      round-trip per record load — drives the RELATIONSHIPS section
//      of the record detail page so the user sees "→ Account
//      Acme Corp" instead of just the raw 0014I... Id.
//
//   2. The count of child records pointing AT this record (one
//      (SELECT COUNT() FROM RelN) subquery per child relationship,
//      capped to avoid SOQL limits). Drives the RELATED panel so
//      the user can see "5 Opportunities, 12 Contacts" at a
//      glance.
//
// Both helpers consult the parent sObject's describe (passed in)
// rather than re-fetching it — the UI layer's per-sObject describe
// cache is the single source of truth.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// MaxChildRelationshipCounts caps the COUNT() subqueries per
// record-load. SOQL allows up to 20 subqueries per outer query;
// we leave headroom for the relationship-name selects on the same
// SOQL when we eventually merge the two helpers.
const MaxChildRelationshipCounts = 20

// ReferenceFieldName is the resolved-name slot for one reference
// field on a record. Field is the source field's API name (e.g.
// "AccountId"); Name is the related record's Name (or the parent
// sObject's name-field equivalent). Empty Name means the field
// value was null OR the related record's Name field wasn't
// projected (rare — usually means the user lacks read on the
// related record).
type ReferenceFieldName struct {
	Field string
	Name  string
}

// BuildReferenceNameSOQL returns the SOQL that, when run, projects
// every resolvable reference field's relationship Name onto one
// row. Fields without a RelationshipName (system fields like
// RecordVisibilityId) or pointing at a polymorphic target (no
// single Name field guaranteed) are skipped — the UI falls back to
// the raw Id for those.
//
// Returns the empty string when there's nothing to resolve.
//
// Example for Request__c with two lookups:
//
//	SELECT Account__r.Name, Contact__r.Name FROM Request__c WHERE Id = 'a0u...'
func BuildReferenceNameSOQL(d SObjectDescribe, recordID string) string {
	if recordID == "" {
		return ""
	}
	var selects []string
	for _, f := range d.Fields {
		if f.Type != "reference" {
			continue
		}
		if f.RelationshipName == "" {
			continue
		}
		// Polymorphic lookups can't be safely projected — the
		// referenced object's Name field differs per target type
		// and SOQL will error on the wrong one. Skip and let the
		// UI key-prefix-resolve client-side then drill if the
		// user asks.
		if len(f.ReferenceTo) != 1 {
			continue
		}
		// Skip targets that don't have a queryable Name field. SF
		// rejects the whole SOQL with INVALID_FIELD if a single
		// `<ref>.Name` is invalid — one bad reference would null
		// out resolution for every OTHER reference on the record.
		if !referenceTargetHasNameField(f.ReferenceTo[0]) {
			continue
		}
		selects = append(selects, f.RelationshipName+".Name")
	}
	if len(selects) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"SELECT %s FROM %s WHERE Id = '%s'",
		strings.Join(selects, ", "), d.Name, sqlEscape(recordID),
	)
}

// referenceTargetHasNameField reports whether the given sObject
// has a queryable `Name` field projection. Salesforce's internal /
// system objects (UserRecordAccess, RecordVisibility, etc.) reject
// `Foo.Name` SOQL with INVALID_FIELD even though they appear as
// referenceTo targets on standard fields.
//
// Approach: deny-list of known offenders. Faster + simpler than
// fetching every target's describe to check Fields[].Name. The
// list is short and grows additively — any new INVALID_FIELD
// surface lands here.
func referenceTargetHasNameField(sobject string) bool {
	switch sobject {
	case "UserRecordAccess",
		"RecordVisibility",
		"ContentDocumentLink",
		"AccountContactRelation":
		return false
	}
	// Anything ending in __mdt (custom metadata) has DeveloperName
	// not Name — skip. Same with __e (platform events).
	if strings.HasSuffix(sobject, "__mdt") || strings.HasSuffix(sobject, "__e") {
		return false
	}
	return true
}

// ParseReferenceNames extracts the resolved-Name values from a
// QueryResult row produced by the SOQL BuildReferenceNameSOQL
// generated. Returns map[fieldApiName]string keyed by the source
// field name (NOT the relationship name) for direct lookup
// against describe fields.
//
// Example: for `SELECT Account__r.Name FROM Request__c …`, the
// returned map has key "Account__c" (the source field) → value
// "Acme Corp" (the resolved name).
func ParseReferenceNames(d SObjectDescribe, row map[string]any) map[string]string {
	out := map[string]string{}
	for _, f := range d.Fields {
		if f.Type != "reference" || f.RelationshipName == "" {
			continue
		}
		if len(f.ReferenceTo) != 1 {
			continue
		}
		nested, ok := row[f.RelationshipName].(map[string]any)
		if !ok {
			continue
		}
		name, ok := nested["Name"].(string)
		if !ok || name == "" {
			continue
		}
		out[f.Name] = name
	}
	return out
}

// ChildRelationshipCount is one row of the resolved counts. Name
// matches the RelationshipName on the describe so callers can
// join back.
type ChildRelationshipCount struct {
	RelationshipName string
	Count            int
}

// ChildCountQuery is one batched count query: the child relationship
// name (key the caller will join back on) and the SOQL that computes
// its count. SF rejects COUNT() inside subqueries so each child needs
// its own root query; we batch them via /composite/batch.
type ChildCountQuery struct {
	RelationshipName string
	SOQL             string
}

// BuildChildCountQueries returns up to MaxChildRelationshipCounts
// COUNT() SOQL queries — one per queryable child relationship — for
// the given parent record. The caller batches them via the existing
// Composite helper (25 subrequests per batch).
//
// Excludes child relationships with no RelationshipName, anything
// flagged deprecatedAndHidden, and standard noise (FeedSubscriptions,
// LookedUpFromActivities, etc. — system-only that admins never want
// to see). Excluded names live in the unwanted-children list below.
func BuildChildCountQueries(d SObjectDescribe, recordID string) []ChildCountQuery {
	if recordID == "" {
		return nil
	}
	var out []ChildCountQuery
	for _, c := range d.ChildRelationships {
		if len(out) >= MaxChildRelationshipCounts {
			break
		}
		if c.RelationshipName == "" || c.DeprecatedAndHidden {
			continue
		}
		if isSystemChildRelationship(c.RelationshipName) {
			continue
		}
		soql := fmt.Sprintf(
			"SELECT COUNT() FROM %s WHERE %s = '%s'",
			c.ChildSObject, c.Field, sqlEscape(recordID),
		)
		out = append(out, ChildCountQuery{
			RelationshipName: c.RelationshipName,
			SOQL:             soql,
		})
	}
	return out
}

// RunChildCountBatch fires the COUNT() queries from
// BuildChildCountQueries via /composite/batch. One round-trip per
// 25 queries; the helper auto-chunks via Client.Composite when the
// caller exceeds the cap (we already cap at
// MaxChildRelationshipCounts=20 so this is single-trip in practice).
//
// Returns map[relationshipName]count keyed on the same
// RelationshipName the caller passed in. Sub-queries that fail
// (deleted parent, permission denied, etc.) are silently dropped
// rather than failing the whole batch — partial data is more useful
// than no data on the RELATED panel.
func RunChildCountBatch(orgTarget string, queries []ChildCountQuery) (map[string]int, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	c, err := RESTClient(orgTarget)
	if err != nil {
		return nil, err
	}
	reqs := make([]CompositeRequest, 0, len(queries))
	for i, q := range queries {
		reqs = append(reqs, CompositeRequest{
			Method:      "GET",
			URL:         c.APIPath("query") + "?q=" + url.QueryEscape(q.SOQL),
			ReferenceID: fmt.Sprintf("c%d", i),
		})
	}
	resps, err := c.Composite(reqs, false)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for i, r := range resps {
		if i >= len(queries) {
			break
		}
		if r.HTTPStatusCode < 200 || r.HTTPStatusCode >= 300 {
			continue
		}
		var parsed struct {
			TotalSize int `json:"totalSize"`
		}
		if err := json.Unmarshal(r.Body, &parsed); err != nil {
			continue
		}
		out[queries[i].RelationshipName] = parsed.TotalSize
	}
	return out, nil
}

// isSystemChildRelationship filters out the long tail of platform
// child relationships every sObject inherits — Feeds, ChangeEvents,
// History, ProcessInstance, FieldHistory, share tables, RecordAction.
// These dominate the noise on the RELATED panel without telling the
// user anything actionable. The standard "Notes & Attachments,
// Tasks, Open Activities" set still gets through because their
// relationship names don't match.
func isSystemChildRelationship(name string) bool {
	suffixes := []string{
		"Feeds", "ChangeEvents", "Histories", "Histories__r",
		"Shares", "__Share", "RecordActions",
		"FeedSubscriptionsForEntity", "ProcessInstance", "ProcessSteps",
		"DuplicateRecordItems",
	}
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) || name == s {
			return true
		}
	}
	return false
}
