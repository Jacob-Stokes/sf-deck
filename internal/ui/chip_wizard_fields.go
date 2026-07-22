package ui

// Per-domain field catalogues for the chip wizard. Adding a new
// surface (perm sets, profiles…) is one new function here plus one
// case in wizardFieldsFor.
//
// Each field maps to a single CompareNode op in the AST. Picking the
// right Op per row is what makes simple mode round-trip cleanly with
// advanced mode — populateFromCompareNodes matches on (Field, Op).
//
// Records is special: the chip targets a specific sObject so the
// catalogue is *describe-driven* rather than static. The Model's
// cached describe is consulted to surface the right fields with the
// right types. Other domains have a fixed shape.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// wizardFieldsFor returns the catalogue of editable rows for a
// wizard invocation. For records the catalogue is sObject-specific;
// scope carries the sObject API name (Account / Case / Custom__c).
// For other domains scope is ignored.
//
// Every domain's catalogue gets a trailing Limit row so users can
// pin a per-chip row cap. The Field name "$limit" is a sentinel —
// buildSimpleQuery routes it into Query.Limit instead of building
// a CompareNode.
func (m Model) wizardFieldsFor(d chipDomain, scope string) []cwField {
	var base []cwField
	for _, def := range chipDomainDefs() {
		if def.Domain == d && def.WizardFields != nil {
			base = def.WizardFields(m, scope)
			break
		}
	}
	if base == nil {
		return nil
	}
	hint := "space to switch default ↔ custom · custom + blank = no limit"
	return append(base, cwField{
		Field: chipLimitSentinel,
		Label: "Limit",
		Hint:  hint,
		Op:    query.OpEq,
		Kind:  cwLimit,
	})
}

// chipLimitSentinel is the cwField.Field value buildSimpleQuery
// recognises as "this row's value goes into Query.Limit, not into
// the WHERE clause." Picked to avoid colliding with any real SF
// field name; the leading "$" mirrors the substitution-token style
// already used for $userId.
const chipLimitSentinel = "$limit"

// objectFields — what the user can filter /objects on. Names match
// EntityDefinition columns the SObject row impl exposes.
func objectFields() []cwField {
	return []cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains",
			Hint: "case-insensitive substring against API name", Kind: cwText},
		{Field: "Label", Op: query.OpContains, Label: "Label contains",
			Hint: "case-insensitive substring against the user-facing label", Kind: cwText},
		{Field: "Name", Op: query.OpStartsWith, Label: "API name prefix",
			Hint: `e.g. "FSL__" for managed-package objects`, Kind: cwText},
		{Field: "Name", Op: query.OpEndsWith, Label: "API name suffix",
			Hint: `"__c" for custom, "__e" for platform events`, Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace",
			Hint: "package namespace prefix (e.g. skuid, FSL)", Kind: cwText},
		{Field: "DeploymentStatus", Op: query.OpEq, Label: "Deployment status",
			Hint: "Deployed / InDevelopment", Kind: cwText},
		{Field: "KeyPrefix", Op: query.OpEq, Label: "Key prefix",
			Hint: "3-char id prefix, e.g. 001 for Account", Kind: cwText},
		{Field: "LastModifiedDate", Op: query.OpGT, Label: "Modified after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastModifiedDate", Op: query.OpLT, Label: "Modified before",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "IsCustom", Op: query.OpEq, Label: "Custom only",
			Hint: "yes = __c objects, no = standard, any = no constraint", Kind: cwTri},
		{Field: "IsApexTriggerable", Op: query.OpEq, Label: "Apex triggerable",
			Hint: "EntityDefinition.IsApexTriggerable", Kind: cwTri},
		{Field: "IsWorkflowEnabled", Op: query.OpEq, Label: "Workflow enabled",
			Hint: "EntityDefinition.IsWorkflowEnabled", Kind: cwTri},
	}
}

// flowFields — Flow object columns.
func flowFields() []cwField {
	return []cwField{
		{Field: "DeveloperName", Op: query.OpContains, Label: "Name contains",
			Hint: "case-insensitive substring against DeveloperName", Kind: cwText},
		{Field: "MasterLabel", Op: query.OpContains, Label: "Label contains",
			Hint: "case-insensitive substring against MasterLabel", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains",
			Hint: "case-insensitive substring against the flow description", Kind: cwText},
		{Field: "DeveloperName", Op: query.OpStartsWith, Label: "Name prefix",
			Hint: `e.g. "FSL_" for managed-package flows`, Kind: cwText},
		{Field: "DeveloperName", Op: query.OpEndsWith, Label: "Name suffix", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace",
			Hint: "package namespace prefix", Kind: cwText},
		{Field: "Status", Op: query.OpEq, Label: "Status",
			Hint: "Active / Draft / Obsolete / Inactive / InvalidDraft", Kind: cwText},
		{Field: "ProcessType", Op: query.OpEq, Label: "Process type",
			Hint: "Flow / AutoLaunchedFlow / InvocableProcess / Workflow / CustomEvent", Kind: cwText},
		{Field: "ApiVersion", Op: query.OpGTE, Label: "API version ≥", Kind: cwInt},
		{Field: "ApiVersion", Op: query.OpLTE, Label: "API version ≤", Kind: cwInt},
		{Field: "LastModifiedDate", Op: query.OpGT, Label: "Modified after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastModifiedDate", Op: query.OpLT, Label: "Modified before",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastModifiedBy", Op: query.OpContains, Label: "Modified by",
			Hint: "case-insensitive substring against LastModifiedBy.Name", Kind: cwText},
		{Field: "CreatedDate", Op: query.OpGT, Label: "Created after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "CreatedDate", Op: query.OpLT, Label: "Created before",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "CreatedBy", Op: query.OpContains, Label: "Created by",
			Hint: "case-insensitive substring against CreatedBy.Name", Kind: cwText},
	}
}

// recordFields builds the wizard catalogue for a records-shaped chip
// scoped to a specific sObject. Reads the cached describe to surface
// fields that actually exist on the target.
//
// Catalogue shape:
//
//  1. Audit-trail fields, always shown when present (OwnerId,
//     CreatedDate, LastModifiedDate, CreatedById, LastModifiedById).
//     These are what 90% of "filter records" queries hit.
//
//  2. Name + any externalId / unique / nameField the describe flags.
//     These are the natural identity columns the user cares about.
//
//  3. A handful of additional filterable fields, capped to keep the
//     wizard navigable. The advanced-SOQL editor is the right tool
//     for queries that need more.
//
// When the describe isn't cached yet (rare — the user has to drill
// into an sObject before V opens here), fall back to the static
// short list.
func (m Model) recordFields(sobject string) []cwField {
	desc, ok := m.cachedDescribe(sobject)
	if !ok {
		return staticRecordFields()
	}
	return fieldsFromDescribe(desc)
}

// cachedDescribe pulls the active org's describe for the given
// sObject if it's been loaded. Lazily-loaded resource — when the
// user is on the Records subtab a describe is always already in
// flight, so this hits typically.
func (m Model) cachedDescribe(sobject string) (sf.SObjectDescribe, bool) {
	if len(m.orgs) == 0 || sobject == "" || sobject == "*" {
		return sf.SObjectDescribe{}, false
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return sf.SObjectDescribe{}, false
	}
	res, ok := d.Describes[sobject]
	if !ok || res.FetchedAt().IsZero() {
		return sf.SObjectDescribe{}, false
	}
	return res.Value(), true
}

// staticRecordFields is the fallback shown when the describe isn't
// cached yet. Same shape recordFields used to return before this
// became describe-driven.
func staticRecordFields() []cwField {
	return []cwField{
		{Field: "OwnerId", Op: query.OpEq, Label: "Owner",
			Hint: "Salesforce Id, or $userId for the current user", Kind: cwText},
		{Field: "CreatedDate", Op: query.OpDateLiteral, Label: "Created",
			Hint: "TODAY / THIS_WEEK / LAST_N_DAYS:30 / etc.", Kind: cwText},
		{Field: "LastModifiedDate", Op: query.OpDateLiteral, Label: "Modified",
			Hint: "TODAY / THIS_WEEK / LAST_N_DAYS:30 / etc.", Kind: cwText},
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
	}
}

// fieldsFromDescribe converts a describe payload into wizard rows.
// Returns every filterable field — the wizard's viewport scrolls
// through long lists. Order:
//
//  1. Priority audit/identity fields (Name, OwnerId, Created*,
//     LastModified*) — what most queries reach for first.
//  2. Other identity-shaped fields the describe flags (NameField /
//     ExternalID / Unique).
//  3. Custom fields (__c suffix) — what users filter on most often
//     after the audit columns on bespoke objects.
//  4. Remaining standard filterable fields — Industry, Type, etc.
//
// Fields are deduplicated by Name. Anything that isn't filterable
// (formulas SF won't let you filter on, blob fields, …) drops out.
// Unsupported simple-mode types (location, address, base64) drop
// too; users reach those via Advanced SOQL.
func fieldsFromDescribe(desc sf.SObjectDescribe) []cwField {
	byName := make(map[string]sf.Field, len(desc.Fields))
	for _, f := range desc.Fields {
		byName[f.Name] = f
	}

	priorityNames := []string{
		"Name",
		"OwnerId",
		"CreatedDate",
		"LastModifiedDate",
		"CreatedById",
		"LastModifiedById",
	}

	var rows []cwField
	seen := map[string]bool{}
	add := func(f sf.Field) {
		if seen[f.Name] || !f.Filterable {
			return
		}
		if row, ok := wizardRowForField(f); ok {
			rows = append(rows, row)
			seen[f.Name] = true
		}
	}

	// 1. Priority audit/identity in the canonical order.
	for _, name := range priorityNames {
		if f, ok := byName[name]; ok {
			add(f)
		}
	}

	// 2. Identity-shaped extras (NameField / ExternalID / Unique).
	for _, f := range desc.Fields {
		if f.NameField || f.ExternalID || f.Unique {
			add(f)
		}
	}

	// 3. Custom fields — usually the user's actual filtering targets
	//    on bespoke objects.
	for _, f := range desc.Fields {
		if f.Custom {
			add(f)
		}
	}

	// 4. Remaining standard fields (Industry, Type, IsDeleted, …).
	for _, f := range desc.Fields {
		add(f)
	}
	return rows
}

// wizardRowForField maps a describe Field to a wizard cwField row.
// Returns ok=false for types simple mode can't usefully edit (blob,
// address, location etc.) — callers skip these and the user can
// reach for advanced SOQL if needed.
func wizardRowForField(f sf.Field) (cwField, bool) {
	label := f.Label
	if label == "" {
		label = f.Name
	}
	hint := f.Name
	if f.Type != "" {
		hint = f.Name + " · " + f.Type
	}
	switch f.Type {
	case "string", "textarea", "email", "phone", "url":
		return cwField{
			Field: f.Name, Op: query.OpContains, Label: label + " contains",
			Hint: hint, Kind: cwText,
		}, true
	case "picklist", "multipicklist", "combobox":
		hint = picklistHint(f)
		return cwField{
			Field: f.Name, Op: query.OpEq, Label: label,
			Hint: hint, Kind: cwText,
		}, true
	case "boolean":
		return cwField{
			Field: f.Name, Op: query.OpEq, Label: label,
			Hint: hint, Kind: cwTri,
		}, true
	case "int", "long", "double", "currency", "percent":
		return cwField{
			Field: f.Name, Op: query.OpEq, Label: label,
			Hint: hint, Kind: cwInt,
		}, true
	case "date", "datetime", "time":
		return cwField{
			Field: f.Name, Op: query.OpDateLiteral, Label: label,
			Hint: "TODAY / THIS_WEEK / LAST_N_DAYS:30 / ISO date", Kind: cwText,
		}, true
	case "id", "reference":
		// Reference / lookup fields. Hint mentions $userId for the
		// common OwnerId / CreatedById case.
		ref := hint
		if f.Name == "OwnerId" || f.Name == "CreatedById" || f.Name == "LastModifiedById" {
			ref = "Salesforce Id, or $userId for the current user"
		}
		return cwField{
			Field: f.Name, Op: query.OpEq, Label: label,
			Hint: ref, Kind: cwText,
		}, true
	}
	// Unknown / unsupported type (blob, address, location, …).
	// Skip — advanced SOQL is the escape hatch.
	return cwField{}, false
}

// picklistHint formats the first few picklist values into a
// comma-joined hint so the user knows what to type. Caps the list
// to keep the hint short.
func picklistHint(f sf.Field) string {
	if len(f.PicklistValues) == 0 {
		return f.Name + " · picklist"
	}
	const max = 5
	vals := make([]string, 0, max)
	for _, pv := range f.PicklistValues {
		if !pv.Active {
			continue
		}
		vals = append(vals, pv.Value)
		if len(vals) >= max {
			break
		}
	}
	out := strings.Join(vals, " / ")
	if len(f.PicklistValues) > len(vals) {
		out += " / …"
	}
	return out
}

// modifiedFields is the Modified after/before/by triple shared by
// every org-metadata catalogue whose row exposes LastModifiedDate +
// LastModifiedBy.Name.
func modifiedFields() []cwField {
	return []cwField{
		{Field: "LastModifiedDate", Op: query.OpGT, Label: "Modified after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastModifiedDate", Op: query.OpLT, Label: "Modified before",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastModifiedBy", Op: query.OpContains, Label: "Modified by",
			Hint: "case-insensitive substring against LastModifiedBy.Name", Kind: cwText},
	}
}

// apexClassFields — ApexClassRow columns (/apex Classes subtab).
func apexClassFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Name", Op: query.OpStartsWith, Label: "Name prefix", Kind: cwText},
		{Field: "Name", Op: query.OpEndsWith, Label: "Name suffix",
			Hint: `e.g. "Test" to catch test classes by convention`, Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace",
			Hint: "package namespace prefix", Kind: cwText},
		{Field: "Status", Op: query.OpEq, Label: "Status",
			Hint: "Active / Deleted", Kind: cwText},
		{Field: "IsValid", Op: query.OpEq, Label: "Valid",
			Hint: "compiles against current metadata", Kind: cwTri},
		{Field: "ApiVersion", Op: query.OpGTE, Label: "API version ≥", Kind: cwInt},
		{Field: "ApiVersion", Op: query.OpLTE, Label: "API version ≤", Kind: cwInt},
		{Field: "LengthWithoutComments", Op: query.OpGTE, Label: "Lines ≥",
			Hint: "LengthWithoutComments", Kind: cwInt},
	}, modifiedFields()...)
}

// apexTriggerFields — TriggerRow columns (/apex Triggers subtab).
func apexTriggerFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Table", Op: query.OpEq, Label: "sObject",
			Hint: "parent sObject API name, e.g. Account", Kind: cwText},
		{Field: "Events", Op: query.OpContains, Label: "Events contain",
			Hint: `e.g. "before insert"`, Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
		{Field: "Status", Op: query.OpEq, Label: "Status",
			Hint: "Active / Inactive", Kind: cwText},
		{Field: "IsValid", Op: query.OpEq, Label: "Valid", Kind: cwTri},
		{Field: "ApiVersion", Op: query.OpGTE, Label: "API version ≥", Kind: cwInt},
		{Field: "ApiVersion", Op: query.OpLTE, Label: "API version ≤", Kind: cwInt},
	}, modifiedFields()...)
}

// userFields — UserRow columns (/users).
func userFields() []cwField {
	return []cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Username", Op: query.OpContains, Label: "Username contains", Kind: cwText},
		{Field: "Profile", Op: query.OpContains, Label: "Profile contains",
			Hint: "case-insensitive substring against Profile.Name", Kind: cwText},
		{Field: "Role", Op: query.OpContains, Label: "Role contains",
			Hint: "case-insensitive substring against UserRole.Name", Kind: cwText},
		{Field: "IsActive", Op: query.OpEq, Label: "Active", Kind: cwTri},
		{Field: "LastLoginDate", Op: query.OpGT, Label: "Last login after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "LastLoginDate", Op: query.OpLT, Label: "Last login before",
			Hint: "ISO date or datetime", Kind: cwDate},
	}
}

// lwcFields — LWCBundle columns (/components LWC subtab).
func lwcFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains",
			Hint: "case-insensitive substring against DeveloperName", Kind: cwText},
		{Field: "Label", Op: query.OpContains, Label: "Label contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
		{Field: "IsExposed", Op: query.OpEq, Label: "Exposed",
			Hint: "available to App Builder targets", Kind: cwTri},
		{Field: "ApiVersion", Op: query.OpGTE, Label: "API version ≥", Kind: cwInt},
		{Field: "ApiVersion", Op: query.OpLTE, Label: "API version ≤", Kind: cwInt},
	}, modifiedFields()...)
}

// auraFields — AuraBundle columns (/components Aura subtab).
func auraFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains",
			Hint: "case-insensitive substring against DeveloperName", Kind: cwText},
		{Field: "Label", Op: query.OpContains, Label: "Label contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
		{Field: "ApiVersion", Op: query.OpGTE, Label: "API version ≥", Kind: cwInt},
		{Field: "ApiVersion", Op: query.OpLTE, Label: "API version ≤", Kind: cwInt},
	}, modifiedFields()...)
}

// permSetFields — PermissionSet columns (/perms Permission Sets).
func permSetFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Label", Op: query.OpContains, Label: "Label contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
		{Field: "Type", Op: query.OpEq, Label: "Type",
			Hint: "Regular / Group / Session etc.", Kind: cwText},
		{Field: "License", Op: query.OpContains, Label: "License contains", Kind: cwText},
		{Field: "IsCustom", Op: query.OpEq, Label: "Custom", Kind: cwTri},
	}, modifiedFields()...)
}

// psgFields — PermissionSetGroup columns (/perms PSGs).
func psgFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains",
			Hint: "case-insensitive substring against DeveloperName", Kind: cwText},
		{Field: "Label", Op: query.OpContains, Label: "Label contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "Status", Op: query.OpEq, Label: "Status",
			Hint: "Updated / Outdated / Updating / Failed", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
	}, modifiedFields()...)
}

// profileFields — Profile columns (/perms Profiles).
func profileFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "UserType", Op: query.OpEq, Label: "User type",
			Hint: "Standard / Guest / PowerPartner etc.", Kind: cwText},
		{Field: "License", Op: query.OpContains, Label: "License contains",
			Hint: "case-insensitive substring against UserLicense.Name", Kind: cwText},
	}, modifiedFields()...)
}

// queueFields — QueueRow columns (/perms Queues).
func queueFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "DeveloperName", Op: query.OpContains, Label: "Dev name contains", Kind: cwText},
		{Field: "Email", Op: query.OpContains, Label: "Email contains", Kind: cwText},
		{Field: "SObjects", Op: query.OpContains, Label: "sObjects contain",
			Hint: "queue-enabled object API name, e.g. Case", Kind: cwText},
	}, modifiedFields()...)
}

// publicGroupFields — PublicGroupRow columns (/perms Public Groups).
func publicGroupFields() []cwField {
	return append([]cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "DeveloperName", Op: query.OpContains, Label: "Dev name contains", Kind: cwText},
		{Field: "DoesIncludeBosses", Op: query.OpEq, Label: "Includes bosses",
			Hint: "grant access using hierarchies", Kind: cwTri},
	}, modifiedFields()...)
}

// savedQueryFields — devproject.SavedQuery columns (/soql Saved).
func savedQueryFields() []cwField {
	return []cwField{
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
		{Field: "Body", Op: query.OpContains, Label: "Query contains",
			Hint: `matches the SOQL text, e.g. "Account"`, Kind: cwText},
		{Field: "HasDescription", Op: query.OpEq, Label: "Has description", Kind: cwTri},
		{Field: "UpdatedAt", Op: query.OpGT, Label: "Updated after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "UpdatedAt", Op: query.OpLT, Label: "Updated before",
			Hint: "ISO date or datetime", Kind: cwDate},
	}
}

// soqlHistoryFields — devproject.SOQLHistoryEntry columns (/soql History).
func soqlHistoryFields() []cwField {
	return []cwField{
		{Field: "Body", Op: query.OpContains, Label: "Query contains",
			Hint: `matches the SOQL text, e.g. "Account"`, Kind: cwText},
		{Field: "HasError", Op: query.OpEq, Label: "Errored", Kind: cwTri},
		{Field: "RowCount", Op: query.OpGTE, Label: "Rows ≥", Kind: cwInt},
		{Field: "RowCount", Op: query.OpLTE, Label: "Rows ≤", Kind: cwInt},
		{Field: "DurationMs", Op: query.OpGTE, Label: "Duration ms ≥", Kind: cwInt},
		{Field: "ExecutedAt", Op: query.OpGT, Label: "Run after",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "ExecutedAt", Op: query.OpLT, Label: "Run before",
			Hint: "ISO date or datetime", Kind: cwDate},
		{Field: "Org", Op: query.OpContains, Label: "Org contains",
			Hint: "org username the query ran against", Kind: cwText},
	}
}

// recentFields — recent.Entry columns (/home Recent).
func recentFields() []cwField {
	return []cwField{
		{Field: "Kind", Op: query.OpEq, Label: "Kind",
			Hint: "record / sobject / flow / apexclass / report …", Kind: cwText},
		{Field: "Type", Op: query.OpContains, Label: "Type contains",
			Hint: "sObject API name for record visits", Kind: cwText},
		{Field: "Name", Op: query.OpContains, Label: "Name contains", Kind: cwText},
		{Field: "Origin", Op: query.OpEq, Label: "Origin",
			Hint: "where the visit happened", Kind: cwText},
	}
}

// dashboardFields — DashboardRow columns (/reports Dashboards).
// deployFields — DeployRow columns (/deploys).
func deployFields() []cwField {
	return []cwField{
		{Field: "Status", Op: query.OpEq, Label: "Status",
			Hint: "Succeeded / Failed / SucceededPartial / InProgress / Canceled", Kind: cwText},
		{Field: "CheckOnly", Op: query.OpEq, Label: "Validation only", Kind: cwTri},
		{Field: "CreatedBy", Op: query.OpContains, Label: "Deployed by contains", Kind: cwText},
		{Field: "ChangeSetName", Op: query.OpContains, Label: "Change set contains", Kind: cwText},
		{Field: "TestLevel", Op: query.OpEq, Label: "Test level",
			Hint: "NoTestRun / RunLocalTests / RunAllTestsInOrg / RunSpecifiedTests", Kind: cwText},
		{Field: "ComponentErrors", Op: query.OpGT, Label: "Component errors over", Kind: cwInt},
		{Field: "TestErrors", Op: query.OpGT, Label: "Test errors over", Kind: cwInt},
	}
}

func dashboardFields() []cwField {
	return append([]cwField{
		{Field: "Title", Op: query.OpContains, Label: "Title contains", Kind: cwText},
		{Field: "DeveloperName", Op: query.OpContains, Label: "Dev name contains", Kind: cwText},
		{Field: "Folder", Op: query.OpContains, Label: "Folder contains", Kind: cwText},
		{Field: "Type", Op: query.OpEq, Label: "Run-as mode",
			Hint: "LoggedInUser (viewer) / SpecifiedUser (fixed)", Kind: cwText},
		{Field: "Namespace", Op: query.OpEq, Label: "Namespace", Kind: cwText},
		{Field: "Description", Op: query.OpContains, Label: "Description contains", Kind: cwText},
	}, modifiedFields()...)
}

// reportTypeFields — ReportTypeRow columns (/reports Report Types).
func reportTypeFields() []cwField {
	return []cwField{
		{Field: "Label", Op: query.OpContains, Label: "Label contains", Kind: cwText},
		{Field: "Type", Op: query.OpContains, Label: "API name contains", Kind: cwText},
		{Field: "Category", Op: query.OpContains, Label: "Category contains", Kind: cwText},
		{Field: "Custom", Op: query.OpEq, Label: "Custom", Kind: cwTri},
		{Field: "SupportsJoined", Op: query.OpEq, Label: "Supports joined", Kind: cwTri},
	}
}
