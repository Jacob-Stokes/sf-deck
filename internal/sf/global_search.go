package sf

// global_search.go — multi-object SOSL primitive backing the
// records mode of the global search modal.  See
// docs/global-record-search-plan.md for the design.
//
// SOSL's FIND {...} RETURNING Obj1(...), Obj2(...), ... primitive
// runs one server-side search across many sObjects in one call.
// Returned `searchRecords` array carries each row's attributes.type
// so callers can render mixed results in a flat list.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// GlobalSearchTarget describes one sObject in a multi-object SOSL
// query: the API name + the fields to project for previews.  The
// FIND clause runs against IN NAME FIELDS for every target; the
// RETURNING(...) clause carries the per-target projection.
//
// Fields is the projection list (Id is always implicit).  When
// Secondary is non-empty it names the field to show in the
// modal's "secondary" column for this kind — typically the field
// most useful as a disambiguator (Industry for Account, Email for
// Contact, etc.).  Callers can hand-pick this per object instead
// of letting the modal guess.
type GlobalSearchTarget struct {
	Sobject   string
	Fields    []string // Id always implicit; Name usually first; rest are preview
	NameField string   // display field; defaults to "Name"
	Secondary string   // field name whose value populates Secondary on the hit
}

// GlobalSearchHit is one row from a multi-object SOSL query.
// Sobject identifies which target produced it (from the row's
// attributes.type).  Fields carries every projected field's raw
// value keyed by field name; callers render whichever subset they
// care about.
type GlobalSearchHit struct {
	Sobject string
	ID      string
	Name    string
	// Secondary is the value of the target's Secondary field (if
	// declared) — already string-ified.  Empty when Secondary
	// wasn't requested or the value was nil.
	Secondary string
	// Fields holds the raw projected field map for callers that
	// want more than (Id, Name, Secondary).  Read-only.
	Fields map[string]any
}

// GlobalSearch runs a multi-object FIND ... IN NAME FIELDS RETURNING
// query against the given org.  Returns up to `limit` hits across
// ALL targets combined — SF doesn't honor per-target limits in
// SOSL, so the cap applies to the merged result set in arrival
// order (server-ranked).
//
// Empty term, empty targets, or no auth → returns nil, nil so
// callers can render an empty list without an error toast.
func (c *Client) GlobalSearch(term string, targets []GlobalSearchTarget, limit int) ([]GlobalSearchHit, error) {
	if term == "" || len(targets) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	sosl := buildGlobalSearchSOSL(term, targets, limit)
	q := url.Values{}
	q.Set("q", sosl)
	raw, err := c.get(c.APIPath("search"), q)
	if err != nil {
		return nil, fmt.Errorf("sosl global search: %w", err)
	}
	var parsed struct {
		SearchRecords []map[string]any `json:"searchRecords"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode sosl: %w", err)
	}
	byType := indexTargetsBySobject(targets)
	out := make([]GlobalSearchHit, 0, len(parsed.SearchRecords))
	for _, r := range parsed.SearchRecords {
		sobject := sosjectTypeFromAttributes(r)
		if sobject == "" {
			continue
		}
		id, _ := r["Id"].(string)
		if id == "" {
			continue
		}
		tgt, hasTarget := byType[sobject]
		nameField := "Name"
		if hasTarget && tgt.NameField != "" {
			nameField = tgt.NameField
		}
		name, _ := r[nameField].(string)
		secondary := ""
		if hasTarget && tgt.Secondary != "" {
			secondary = stringifyField(r[tgt.Secondary])
		}
		out = append(out, GlobalSearchHit{
			Sobject:   sobject,
			ID:        id,
			Name:      name,
			Secondary: secondary,
			Fields:    r,
		})
	}
	return out, nil
}

// GlobalSearchAlias is the alias-flavoured entry point — mirrors
// SearchRecordsAlias so callers don't have to thread a Client.
func GlobalSearchAlias(alias, term string, targets []GlobalSearchTarget, limit int) ([]GlobalSearchHit, error) {
	c, err := RESTClient(alias)
	if err != nil {
		return nil, err
	}
	return c.GlobalSearch(term, targets, limit)
}

// buildGlobalSearchSOSL composes the FIND ... IN NAME FIELDS
// RETURNING Obj1(F1, F2), Obj2(...) clause from targets.  The
// fields list per target is deduped + sorted so the wire format
// is deterministic (helps cache keys).
func buildGlobalSearchSOSL(term string, targets []GlobalSearchTarget, limit int) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		if t.Sobject == "" {
			continue
		}
		seen := map[string]bool{"Id": true}
		fields := []string{"Id"}
		nameField := t.NameField
		if nameField == "" {
			nameField = "Name"
		}
		if !seen[nameField] {
			fields = append(fields, nameField)
			seen[nameField] = true
		}
		for _, f := range t.Fields {
			if f == "" || seen[f] {
				continue
			}
			fields = append(fields, f)
			seen[f] = true
		}
		if t.Secondary != "" && !seen[t.Secondary] {
			fields = append(fields, t.Secondary)
			seen[t.Secondary] = true
		}
		// Deterministic ordering after Id + nameField so cache
		// keys stay stable across map iteration.
		rest := fields[2:]
		sort.Strings(rest)
		parts = append(parts, fmt.Sprintf("%s(%s)", t.Sobject, strings.Join(fields, ", ")))
	}
	return fmt.Sprintf(
		"FIND {%s} IN NAME FIELDS RETURNING %s LIMIT %d",
		escapeSOSLBraces(term),
		strings.Join(parts, ", "),
		limit,
	)
}

// sosjectTypeFromAttributes pulls the "type" out of the SF row's
// attributes block: {"attributes": {"type": "Account", "url": ...}}.
// Empty string when missing.
func sosjectTypeFromAttributes(row map[string]any) string {
	attrs, _ := row["attributes"].(map[string]any)
	if attrs == nil {
		return ""
	}
	t, _ := attrs["type"].(string)
	return t
}

func indexTargetsBySobject(targets []GlobalSearchTarget) map[string]GlobalSearchTarget {
	out := make(map[string]GlobalSearchTarget, len(targets))
	for _, t := range targets {
		out[t.Sobject] = t
	}
	return out
}

// stringifyField renders a raw JSON-decoded SF field value into the
// human string the modal will show.  Numbers, bools, strings pass
// through.  Nested objects (relationship traversals) flatten to
// their .Name or .Id leaf.  Nil → "".
func stringifyField(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		// JSON decode produces float64 for all numerics; trim a
		// trailing .0 so ints render cleanly.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case map[string]any:
		if name, ok := x["Name"].(string); ok && name != "" {
			return name
		}
		if id, ok := x["Id"].(string); ok && id != "" {
			return id
		}
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// DefaultGlobalSearchTargets returns the curated set of common
// sObjects most orgs have.  Used as the baseline targets list when
// the caller hasn't supplied its own — covers Account, Contact,
// Lead, Opportunity, Case, Task, Event, User, Asset, Order, Quote,
// Product2, Contract, Campaign.  Per-target Secondary fields are
// picked to be the most useful disambiguator.
//
// Augment this list at call time with the user's recently-visited
// sObjects (read from d.Recent) so admins who work in custom
// objects get hits without configuration.
func DefaultGlobalSearchTargets() []GlobalSearchTarget {
	return []GlobalSearchTarget{
		{Sobject: "Account", Secondary: "Industry"},
		{Sobject: "Contact", Secondary: "Email"},
		{Sobject: "Lead", Secondary: "Company"},
		{Sobject: "Opportunity", Fields: []string{"StageName", "Amount"}, Secondary: "StageName"},
		{Sobject: "Case", NameField: "Subject", Fields: []string{"CaseNumber"}, Secondary: "CaseNumber"},
		{Sobject: "Task", NameField: "Subject", Secondary: "ActivityDate"},
		{Sobject: "Event", NameField: "Subject", Secondary: "ActivityDate"},
		{Sobject: "User", Secondary: "Email"},
		{Sobject: "Asset", Secondary: "AccountId"},
		{Sobject: "Order", NameField: "OrderNumber", Secondary: "Status"},
		{Sobject: "Quote", Secondary: "Status"},
		{Sobject: "Product2", Secondary: "ProductCode"},
		{Sobject: "Contract", NameField: "ContractNumber", Secondary: "Status"},
		{Sobject: "Campaign", Secondary: "Type"},
	}
}

// NameFieldFor returns the canonical "label" field for the given
// sObject — the field SOQL should project alongside Id when the user
// wants a human-readable row identifier.
//
// Salesforce's `Name` field is the standard answer, but ~25 system
// objects (Task, Event, Case, Order, Contract, EmailMessage,
// ContentDocument, FeedItem, ServiceAppointment, ...) don't have a
// `Name` and instead carry a domain-specific equivalent
// (Subject, OrderNumber, Title, Body, AppointmentNumber, ...).
// Selecting `Name` on those throws INVALID_FIELD, breaking
// related-record SOQL.
//
// Strategy: check the curated systemSObjectNameFields map below.
// Falls through to "Name" for anything not listed — covers every
// standard CRM object (Account, Contact, Lead, Opportunity, ...)
// AND every custom sObject (custom objects always get a Name).
//
// Callers that have the sObject's describe cached should prefer
// reading Fields[].NameField == true directly; this helper exists
// for the path where the describe isn't loaded yet (or fetching it
// would block UI).
//
// Returns "" when the sObject is known to have NO meaningful label
// field (ProcessInstance, LoginHistory, ContentDocumentLink, etc.).
// Callers should skip the name projection entirely in that case
// (just `SELECT Id FROM X`).
func NameFieldFor(sobject string) string {
	if f, ok := systemSObjectNameFields[sobject]; ok {
		return f
	}
	return "Name"
}

// systemSObjectNameFields maps standard Salesforce sObjects that
// lack a `Name` field to their canonical display field. Sourced
// from the SF Object Reference (developer.salesforce.com/docs/
// atlas.en-us.object_reference.meta/).
//
// Empty-string value means "no meaningful label" — callers should
// project Id only.
//
// Custom objects are never in this map; they always carry a Name.
// Standard objects WITH a Name (Account, Contact, Lead, Opportunity,
// Asset, Quote, Entitlement, CollaborationGroup, Attachment, ...)
// are also absent — the default "Name" fallback handles them.
var systemSObjectNameFields = map[string]string{
	// Activities — Subject is the canonical user-facing label.
	"Task":  "Subject",
	"Event": "Subject",

	// Service Cloud — Subject for cases/emails, auto-numbers for
	// work orders / appointments / contracts.
	"Case":               "Subject",
	"EmailMessage":       "Subject",
	"WorkOrder":          "WorkOrderNumber",
	"WorkOrderLineItem":  "LineItemNumber",
	"ServiceAppointment": "AppointmentNumber",
	"ServiceContract":    "ContractNumber",
	"ContractLineItem":   "LineItemNumber",
	"Solution":           "SolutionName",

	// Order / Contract / Returns — auto-numbers.
	"Order":               "OrderNumber",
	"OrderItem":           "OrderItemNumber",
	"Contract":            "ContractNumber",
	"ReturnOrder":         "ReturnOrderNumber",
	"ReturnOrderLineItem": "ReturnOrderLineItemNumber",
	"AssetRelationship":   "AssetRelationshipNumber",

	// Content / Files — Title is the human label.
	"ContentDocument": "Title",
	"ContentVersion":  "Title",
	"ContentNote":     "Title",
	"Note":            "Title", // legacy notes (pre-Enhanced Notes)

	// Chatter — FeedItem has a Body (Title is only populated for
	// link/content posts), FeedComment is CommentBody.
	"FeedItem":    "Body",
	"FeedComment": "CommentBody",

	// Case feed.
	"CaseComment": "CommentBody",

	// Ideas / Knowledge.
	"Idea":           "Title",
	"Knowledge__kav": "Title",

	// Social / chat.
	"SocialPost":         "Headline",
	"LiveChatTranscript": "LiveChatTranscriptName",

	// Junction / approval / audit objects that have NO meaningful
	// label — explicit empty string so callers know to drop the
	// name column entirely rather than try a fallback.
	"ContentDocumentLink":     "",
	"ProcessInstance":         "",
	"ProcessInstanceStep":     "",
	"ProcessInstanceWorkitem": "",
	"LoginHistory":            "",
	"OpportunityLineItem":     "", // synthesise from Product2.Name when needed
}
