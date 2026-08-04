package sf

import (
	"encoding/json"
	"sync"
)

// describeFlight coalesces concurrent Describe calls for the same
// (alias, sobject) so two callers in flight at once share one REST
// describe call instead of firing two. Observed on object drill-in
// where EnsureDescribe and EnsureCustomObjectBaseline both fetch the
// describe in parallel goroutines.
//
// Manual implementation rather than golang.org/x/sync/singleflight to
// avoid adding a dependency for a 30-line piece of code that's only
// used here.
var (
	describeFlightMu sync.Mutex
	describeFlight   = map[string]*describeCall{}
)

type describeCall struct {
	done   chan struct{}
	result SObjectDescribe
	err    error
}

// Field is a subset of the describe-field payload. We pull enough to
// back a full Object-Manager-style detail view without having to
// re-describe: identity, constraints, reference + picklist metadata,
// formula/autonumber/encryption markers, and the SOQL-relevant
// capability flags (filterable / sortable / etc.).
type Field struct {
	// Identity
	Name           string `json:"name"`
	Label          string `json:"label"`
	Type           string `json:"type"`
	SoapType       string `json:"soapType"`
	Length         int    `json:"length"`
	Custom         bool   `json:"custom"`
	InlineHelpText string `json:"inlineHelpText"`

	// Constraints
	Nillable   bool `json:"nillable"`
	Unique     bool `json:"unique"`
	ExternalID bool `json:"externalId"`
	// Permissionable: whether FLS applies to this field at all.
	// System fields (Id, audit stamps) are always-visible and have
	// no FieldPermissions rows — the FLS grid renders them as "—"
	// instead of a misleading deniable-looking [·].
	Permissionable bool `json:"permissionable"`

	// Numeric precision
	Precision int `json:"precision"`
	Scale     int `json:"scale"`
	Digits    int `json:"digits"`

	// Reference metadata
	ReferenceTo             []string `json:"referenceTo"`
	RelationshipName        string   `json:"relationshipName"`
	RelationshipOrder       *int     `json:"relationshipOrder"`
	CascadeDelete           bool     `json:"cascadeDelete"`
	RestrictedDelete        bool     `json:"restrictedDelete"`
	WriteRequiresMasterRead bool     `json:"writeRequiresMasterRead"`

	// Picklist metadata
	PicklistValues     []PicklistValue `json:"picklistValues"`
	RestrictedPicklist bool            `json:"restrictedPicklist"`
	ControllerName     string          `json:"controllerName"`
	DependentPicklist  bool            `json:"dependentPicklist"`

	// Formula / default / autonumber
	CalculatedFormula   string `json:"calculatedFormula"`
	DefaultValue        any    `json:"defaultValue"`
	DefaultValueFormula string `json:"defaultValueFormula"`
	AutoNumber          bool   `json:"autoNumber"`

	// Security / audit
	Encrypted     bool `json:"encrypted"`
	CaseSensitive bool `json:"caseSensitive"`
	HTMLFormatted bool `json:"htmlFormatted"`
	NameField     bool `json:"nameField"`

	// SOQL capabilities
	Createable   bool `json:"createable"`
	Updateable   bool `json:"updateable"`
	Filterable   bool `json:"filterable"`
	Sortable     bool `json:"sortable"`
	Groupable    bool `json:"groupable"`
	Aggregatable bool `json:"aggregatable"`
}

// Field implements the query.Row contract so the chip engine can
// filter the Schema field list by predicate (same mechanism that
// powers the /objects sObject chips). Boolean derived flags mirror the
// FLAGS column + the field-kind chips: required = !nillable; the
// reference sub-kinds (lookup vs master-detail) derive from the
// relationship cascade/required-on-parent semantics.
func (f Field) Field(name string) (any, bool) {
	switch name {
	case "Name", "QualifiedApiName", "DeveloperName":
		return f.Name, true
	case "Label", "MasterLabel":
		return f.Label, true
	case "Type":
		return f.Type, true
	case "IsCustom":
		return f.Custom, true
	case "IsRequired":
		return !f.Nillable, true
	case "IsUnique":
		return f.Unique, true
	case "IsExternalId":
		return f.ExternalID, true
	case "IsEncrypted":
		return f.Encrypted, true
	case "IsAutoNumber":
		return f.AutoNumber, true
	case "IsNameField":
		return f.NameField, true
	case "IsCalculated", "IsFormula":
		return f.CalculatedFormula != "", true
	case "IsDependentPicklist":
		return f.DependentPicklist, true
	case "IsPicklist":
		return f.Type == "picklist" || f.Type == "multipicklist", true
	case "IsReference":
		return f.Type == "reference" || len(f.ReferenceTo) > 0, true
	case "IsMasterDetail":
		// Master-detail = reference whose child write requires read on
		// the parent (the defining MD trait) OR a cascade/restrict
		// delete rule on the relationship.
		return (f.Type == "reference" || len(f.ReferenceTo) > 0) &&
			(f.WriteRequiresMasterRead || f.CascadeDelete || f.RestrictedDelete), true
	case "IsLookup":
		// Lookup = a reference that is NOT master-detail.
		isRef := f.Type == "reference" || len(f.ReferenceTo) > 0
		isMD := f.WriteRequiresMasterRead || f.CascadeDelete || f.RestrictedDelete
		return isRef && !isMD, true
	}
	return nil, false
}

// PicklistValue is one option on a picklist field.
type PicklistValue struct {
	Label        string `json:"label"`
	Value        string `json:"value"`
	Active       bool   `json:"active"`
	DefaultValue bool   `json:"defaultValue"`
}

// SObjectDescribe is a trimmed subset of `sf sobject describe` output.
type SObjectDescribe struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	LabelPlural string `json:"labelPlural"`
	Custom      bool   `json:"custom"`
	KeyPrefix   string `json:"keyPrefix"`
	Queryable   bool   `json:"queryable"`
	Creatable   bool   `json:"createable"`
	Updatable   bool   `json:"updateable"`
	Deletable   bool   `json:"deletable"`
	// MruEnabled — whether Salesforce tracks "most recently used" /
	// recently-viewed for this object. False objects (e.g.
	// BatchProcessJobDefinition, many setup entities) have no
	// LastViewedDate field, so querying RecentlyViewed-style SOQL
	// against them throws INVALID_FIELD. Gate the Recently-Viewed
	// chip on this.
	MruEnabled         bool                `json:"mruEnabled"`
	Fields             []Field             `json:"fields"`
	ChildRelationships []ChildRelationship `json:"childRelationships"`
}

// ChildRelationship describes one inverse-lookup entry: another
// sObject's field that points AT this object. Surfaced on the
// record detail "RELATED" panel + used by the child-count helper
// to drive (SELECT COUNT() FROM <RelationshipName>) subqueries.
type ChildRelationship struct {
	// RelationshipName is the SOQL-friendly name (e.g. "Opportunities"
	// for Account → Opportunity). Used directly in subquery SOQL.
	// Empty when SF didn't generate one (rare; mostly internal/system
	// relationships).
	RelationshipName string `json:"relationshipName"`
	// ChildSObject is the API name of the child (e.g. "Opportunity").
	ChildSObject string `json:"childSObject"`
	// Field is the lookup/master-detail field on the child that
	// references the parent (e.g. "AccountId").
	Field string `json:"field"`
	// CascadeDelete + RestrictedDelete mirror the standard
	// describe flags so callers can render badges (master-detail
	// cascades, restricted-delete badges).
	CascadeDelete    bool `json:"cascadeDelete"`
	RestrictedDelete bool `json:"restrictedDelete"`
	// DeprecatedAndHidden filters out childRelationships SF
	// reports but tells us not to surface. UI should skip these
	// in the standard view.
	DeprecatedAndHidden bool `json:"deprecatedAndHidden"`
}

type describeWrapper struct {
	Status int             `json:"status"`
	Result SObjectDescribe `json:"result"`
}

// Describe returns the metadata for a single sObject. Read-only.
//
// Fast path: REST-direct to /sobjects/<name>/describe. Falls back to
// `sf sobject describe` if REST bootstrap isn't available.
//
// Concurrent Describe(target, X) calls coalesce: the first call fires
// the REST request, subsequent callers block on its channel until it
// completes and receive the same result. This eliminates a class of
// duplicate-describe bugs where two Resources (EnsureDescribe +
// EnsureCustomObjectBaseline) independently call Describe in the
// same drill-in batch.
func Describe(target, sobjectName string) (SObjectDescribe, error) {
	key := target + "|" + sobjectName
	describeFlightMu.Lock()
	if call, ok := describeFlight[key]; ok {
		describeFlightMu.Unlock()
		<-call.done
		return call.result, call.err
	}
	call := &describeCall{done: make(chan struct{})}
	describeFlight[key] = call
	describeFlightMu.Unlock()

	call.result, call.err = doDescribe(target, sobjectName)
	close(call.done)

	describeFlightMu.Lock()
	delete(describeFlight, key)
	describeFlightMu.Unlock()

	return call.result, call.err
}

// doDescribe is the un-coalesced REST/CLI fetch. Always issues a
// network call. The singleflight wrapper in Describe is the only
// caller.
func doDescribe(target, sobjectName string) (SObjectDescribe, error) {
	if c, err := RESTClient(target); err == nil {
		return c.DescribeREST(sobjectName)
	}
	out, err := runSF("sobject", "describe", "-s", sobjectName, "-o", target, "--json")
	if err != nil {
		return SObjectDescribe{}, err
	}
	var parsed describeWrapper
	if err := json.Unmarshal(out, &parsed); err != nil {
		return SObjectDescribe{}, err
	}
	return parsed.Result, nil
}
