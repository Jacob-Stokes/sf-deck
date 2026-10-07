package devproject

// Dev Projects — the only first-class project concept.

import "time"

// DevProject is the cross-org collection. Pure metadata.
type DevProject struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	TouchedAt   time.Time
}

// ItemKind discriminates collectible items. Added incrementally as
// new types want collecting — the registry pattern in collect.go
// looks at item.Targets()/type-switch to pick a Kind.
type ItemKind string

const (
	KindSObject     ItemKind = "sobject"
	KindField       ItemKind = "field"
	KindFlow        ItemKind = "flow"
	KindFlowVersion ItemKind = "flow_version"
	KindRecord      ItemKind = "record"
	KindApexClass   ItemKind = "apex_class"
	// KindReport's Ref is the report id (a SF Id). Folder collects
	// resolve to KindReport items at collect time — projects never
	// store folder references directly. See devproject/collect.go's
	// commentary on the "static bag" model.
	KindReport             ItemKind = "report"
	KindPermissionSet      ItemKind = "permset"
	KindPermissionSetGroup ItemKind = "permset_group"
	KindProfile            ItemKind = "profile"
	KindValidationRule     ItemKind = "validation_rule" // Type carries parent sobject
	KindRecordType         ItemKind = "record_type"     // Type carries parent sobject
	KindApexTrigger        ItemKind = "apex_trigger"    // Type carries parent sobject
	KindLWC                ItemKind = "lwc"             // Type carries the bundle DeveloperName
	KindAura               ItemKind = "aura"            // Type carries the bundle DeveloperName
	KindQueue              ItemKind = "queue"           // Group with Type='Queue'
	KindPublicGroup        ItemKind = "public_group"    // Group with Type='Regular'
	// KindSOQLQuery's Ref is the saved_queries.id (a "sq_<ulid>"
	// string). Saved queries are org-agnostic by default — pin
	// rows store org_user='' so a single saved query can be
	// associated with a project regardless of which org spawned
	// it. Tags bind the same way.
	KindSOQLQuery ItemKind = "soql_query"
	// KindApexSnippet's Ref is the saved_apex.id (a "ax_<base32>"
	// string). Same shape + semantics as KindSOQLQuery: org-
	// agnostic, taggable, pinnable to DevProjects.
	KindApexSnippet ItemKind = "apex_snippet"
)

// Item is one entry in a DevProject's collected set.
//
// DevProjectID + OrgUser + Kind + Ref together form the primary key
// — the same item ID can legitimately appear under different orgs
// (e.g. the "Account" sObject is in scope for both dev and prod).
//
// Ref is the stable identifier for the kind (see Kind doc). Type is
// supplementary context — for KindRecord it's the sObject name; for
// other kinds it's typically empty.
//
// Name is the user-visible label captured at collect time. It's not
// guaranteed to stay current (the underlying record's Name field can
// change), but it lets us render a row even when the live data isn't
// loaded yet.
type Item struct {
	DevProjectID string
	OrgUser      string // origin org username; "" only for tests
	Kind         ItemKind
	Ref          string
	Type         string
	Name         string
	AddedAt      time.Time
	Notes        string // user's freeform note on why this item is in the project
	Namespace    string
}

// Managed reports whether this item belongs to a managed package
// (i.e. its source can't be retrieved into a project bundle).
// Cheap computed property — managed = non-empty namespace prefix.
func (it Item) Managed() bool {
	return it.Namespace != ""
}

// Field satisfies the internal/query.Row interface so the UI
// surface can run sort/search/chip predicates against items. The
// field names match what chip authors would write in a clause:
// Kind / Name / Ref / Type / OrgUser / Namespace / AddedAt.
//
// This package can't import internal/query (UI dependency), so the
// signature is duplicated rather than referenced. Caller-side
// asserts row.Field(name) (any, bool) which is the structural shape
// query.Row needs.
func (it Item) Field(name string) (any, bool) {
	switch name {
	case "Kind":
		return string(it.Kind), true
	case "Name":
		return it.Name, true
	case "Ref":
		return it.Ref, true
	case "Type":
		return it.Type, true
	case "OrgUser":
		return it.OrgUser, true
	case "Namespace":
		return it.Namespace, true
	case "AddedAt":
		return it.AddedAt.Format(time.RFC3339), true
	case "Notes":
		return it.Notes, true
	case "Managed":
		return it.Managed(), true
	}
	return nil, false
}

// Counts is the aggregated stats one DevProject view wants — total
// items split by kind, plus distinct org count (how many orgs have
// contributed items to this project).
type Counts struct {
	Orgs   int
	Items  int
	ByKind map[ItemKind]int
}
