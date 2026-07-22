package sf

// Permission Set listing for the /perms tab. Unlike FLSPickerEntry
// (which flattens profiles + permsets into one picker), PermissionSet
// here carries the full-fidelity metadata an admin actually wants to
// see in a dedicated permsets list: license, namespace, type,
// description, assignee count.

import "fmt"

// PermissionSet is one standalone Permission Set (NOT a profile's
// implicit permset — those are filtered out). For profiles see
// ListProfiles.
type PermissionSet struct {
	ID                 string
	Name               string // internal API name
	Label              string // user-facing label
	Description        string
	NamespacePrefix    string // "" for unpackaged
	LicenseID          string // PermissionSetLicenseId — "" for no license
	LicenseName        string // PermissionSetLicense.MasterLabel — resolved
	Type               string // "Regular", "Session", "Standard", "Group", "Custom"
	IsCustom           bool
	CreatedDate        string // ISO-8601, matches convention in deploys.go / orgstats.go
	LastModifiedDate   string
	LastModifiedByName string
}

// ListPermissionSets returns every standalone permission set in the
// org (IsOwnedByProfile = false). Sorted by Label.
func ListPermissionSets(target string) ([]PermissionSet, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	// Bundle the permsets + license-name lookup into a single
	// /composite call — one round-trip instead of two. Inline dotted
	// lookups (License.MasterLabel) fail in some orgs with "No such
	// column on entity 'Name'" because the relationship target's
	// default name-field set varies by api version / org, so we stick
	// with a two-query join resolved client-side.
	permsetsSOQL := "SELECT Id, Name, Label, Description, NamespacePrefix, " +
		"LicenseId, Type, IsCustom, " +
		"CreatedDate, LastModifiedDate, LastModifiedBy.Name " +
		"FROM PermissionSet " +
		"WHERE IsOwnedByProfile = false " +
		"ORDER BY Label"
	licensesSOQL := "SELECT Id, MasterLabel FROM PermissionSetLicense"

	responses, err := c.Composite([]CompositeRequest{
		{Method: "GET", URL: c.QueryURL(permsetsSOQL, false), ReferenceID: "permsets"},
		{Method: "GET", URL: c.QueryURL(licensesSOQL, false), ReferenceID: "licenses"},
	}, false)
	if err != nil {
		return nil, err
	}
	results, subErrs := CompositeQueryResults(responses)
	if err, ok := subErrs["permsets"]; ok {
		return nil, err
	}
	permsetsQ, ok := results["permsets"]
	if !ok {
		return nil, fmt.Errorf("composite: missing permsets subresponse")
	}

	out := make([]PermissionSet, 0, len(permsetsQ.Records))
	for _, r := range permsetsQ.Records {
		ps := PermissionSet{
			ID:              asString(r["Id"]),
			Name:            asString(r["Name"]),
			Label:           asString(r["Label"]),
			Description:     asString(r["Description"]),
			NamespacePrefix: asString(r["NamespacePrefix"]),
			LicenseID:       asString(r["LicenseId"]),
			Type:            asString(r["Type"]),
		}
		if b, ok := r["IsCustom"].(bool); ok {
			ps.IsCustom = b
		}
		ps.CreatedDate = asString(r["CreatedDate"])
		ps.LastModifiedDate = asString(r["LastModifiedDate"])
		ps.LastModifiedByName = relationName(r, "LastModifiedBy")
		out = append(out, ps)
	}

	if licensesQ, ok := results["licenses"]; ok {
		byID := make(map[string]string, len(licensesQ.Records))
		for _, r := range licensesQ.Records {
			id := asString(r["Id"])
			name := asString(r["MasterLabel"])
			if id != "" && name != "" {
				byID[id] = name
			}
		}
		for i := range out {
			if name := byID[out[i].LicenseID]; name != "" {
				out[i].LicenseName = name
			}
		}
	}
	return out, nil
}

// Targets — permission sets open into the Setup PermSet overview
// page. The enhanced-permset-builder URL is the current UI.
// Field implements query.Row for chip predicates.
func (p PermissionSet) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return p.ID, true
	case "Name":
		return p.Name, true
	case "Label":
		return p.Label, true
	case "Description":
		return p.Description, true
	case "Namespace", "NamespacePrefix":
		return p.NamespacePrefix, true
	case "Type":
		return p.Type, true
	case "IsCustom":
		return p.IsCustom, true
	case "License", "LicenseName":
		return p.LicenseName, true
	case "CreatedDate":
		return p.CreatedDate, true
	case "LastModifiedDate":
		return p.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return p.LastModifiedByName, true
	}
	return nil, false
}

func (p PermissionSet) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "All Permission Sets",
			Path: "/lightning/setup/PermSets/home"},
	}
	if p.ID != "" {
		t = append([]OpenTarget{{
			ID: "overview", Label: "Permission Set — Overview",
			Path: fmt.Sprintf("/lightning/setup/PermSets/page?address=%%2F%s", p.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the permset API name (Name) / Label / Id.
func (p PermissionSet) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(p.Name, p.Label, p.ID)
}
