package devproject

// Dev Projects — the only first-class project concept.
//
// Hierarchy:
//
//   DevProject (cross-org, top-level)
//     └── Item (sObject / Flow / Record / ApexClass / …)
//
// A DevProject is the unit a user thinks in: "Q2 migration." Items
// hang directly off it. Each item carries the org it was collected
// from in OrgUser, so a single project can hold things from multiple
// orgs (dev sandbox + UAT + prod) without a separate per-org row.
//
// Earlier the schema had an OrgProject intermediate so each org got
// its own per-project container. The split was theoretically clean
// but made every common operation two-step (collect → which project?
// → which org-instance?) and the rail had to surface both DevProject
// and OrgProject panels. Items-with-origin-org collapses that to one
// concept without losing the per-org Scope filtering — the Scope
// hydrator just filters items by OrgUser at lookup time.
//
// Items are discriminated by Kind. Each kind stores a stable
// identifier in Ref:
//   - "sobject"        → API name (e.g. "Account")
//   - "field"          → "<sObject>.<FieldApiName>" (e.g. "Account.Phone")
//   - "flow"           → DefinitionId
//   - "flow_version"   → Flow version Id (Type carries DefinitionId)
//   - "record"         → Id (sobject API name in Type)
//   - "apex_class"     → ApexClass.Id
//   - "report"         → Report Id
//   - "permset"        → PermissionSet Id
//   - "permset_group"  → PermissionSetGroup Id
//   - "profile"        → Profile Id (Type carries the implicit permset Id)
//
// More kinds (Dashboards, ApexTrigger, ValidationRule, …) drop in by
// adding a new Kind constant + handling in the resolver/renderer. The
// underlying schema is type-agnostic.

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
	// Future: KindDashboard, KindLayout, …
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
	// Namespace is the managed-package prefix when the item belongs
	// to a managed package (e.g. "sf_devops" for DevOps Center
	// classes). Empty for native / unmanaged components. Captured at
	// collect time from the source query's NamespacePrefix column;
	// stored so we can flag managed items in the UI + skip them at
	// export without re-querying the org.
	Namespace string
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
