package sf

// Profile listing for the /perms tab.
//
// Important quirk: in Salesforce, every Profile is backed by an
// implicit PermissionSet. ObjectPermissions, FieldPermissions, and
// system-permission boolean fields live on the implicit PermissionSet
// — not on the Profile itself. So when an admin "edits FLS on the
// System Admin profile" they're actually writing FieldPermissions
// rows whose ParentId = the implicit PermissionSet's Id.
//
// We resolve that implicit-permset Id at list time (one extra query)
// and cache it on the Profile struct so downstream callers don't
// each have to re-resolve.

import "fmt"

// Profile is one Salesforce Profile with its implicit-permset Id
// already resolved.
type Profile struct {
	ID                 string
	Name               string
	Description        string
	UserType           string // "Standard", "PowerPartner", "Guest", etc.
	UserLicenseID      string
	UserLicenseName    string // UserLicense.Name
	PermissionSetID    string // the implicit PermissionSet's Id — use this for FLS/ObjectPerm/SystemPerm writes
	CreatedDate        string
	LastModifiedDate   string
	LastModifiedByName string
}

// ListProfiles returns every Profile in the org, with each Profile's
// implicit-permset Id and user-license name resolved. All three
// queries (profiles, licenses, implicit permsets) are bundled into
// one /composite request — one round-trip instead of three.
func ListProfiles(target string) ([]Profile, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	profilesSOQL := "SELECT Id, Name, Description, UserType, UserLicenseId, " +
		"CreatedDate, LastModifiedDate, LastModifiedBy.Name " +
		"FROM Profile " +
		"ORDER BY Name"
	licensesSOQL := "SELECT Id, Name FROM UserLicense"
	implicitSOQL := "SELECT Id, ProfileId FROM PermissionSet WHERE IsOwnedByProfile = true"

	responses, err := c.Composite([]CompositeRequest{
		{Method: "GET", URL: c.QueryURL(profilesSOQL, false), ReferenceID: "profiles"},
		{Method: "GET", URL: c.QueryURL(licensesSOQL, false), ReferenceID: "licenses"},
		{Method: "GET", URL: c.QueryURL(implicitSOQL, false), ReferenceID: "implicit"},
	}, false)
	if err != nil {
		return nil, err
	}
	results, subErrs := CompositeQueryResults(responses)
	// Profiles is the only hard requirement; license/implicit errors
	// downgrade gracefully to "missing data" rather than failing the
	// whole call.
	if err, ok := subErrs["profiles"]; ok {
		return nil, err
	}
	profilesQ, ok := results["profiles"]
	if !ok {
		return nil, fmt.Errorf("composite: missing profiles subresponse")
	}

	profiles := make([]Profile, 0, len(profilesQ.Records))
	for _, r := range profilesQ.Records {
		p := Profile{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			Description:   asString(r["Description"]),
			UserType:      asString(r["UserType"]),
			UserLicenseID: asString(r["UserLicenseId"]),
		}
		p.CreatedDate = asString(r["CreatedDate"])
		p.LastModifiedDate = asString(r["LastModifiedDate"])
		p.LastModifiedByName = relationName(r, "LastModifiedBy")
		profiles = append(profiles, p)
	}

	if licensesQ, ok := results["licenses"]; ok {
		byID := make(map[string]string, len(licensesQ.Records))
		for _, r := range licensesQ.Records {
			id := asString(r["Id"])
			name := asString(r["Name"])
			if id != "" && name != "" {
				byID[id] = name
			}
		}
		for i := range profiles {
			if name := byID[profiles[i].UserLicenseID]; name != "" {
				profiles[i].UserLicenseName = name
			}
		}
	}

	if implicitQ, ok := results["implicit"]; ok {
		byProfile := make(map[string]string, len(implicitQ.Records))
		for _, r := range implicitQ.Records {
			pid := asString(r["ProfileId"])
			psid := asString(r["Id"])
			if pid != "" && psid != "" {
				byProfile[pid] = psid
			}
		}
		for i := range profiles {
			profiles[i].PermissionSetID = byProfile[profiles[i].ID]
		}
	}
	return profiles, nil
}

// Targets — profiles open into the Setup Profile detail page. Note
// that "enhanced profiles" is the default now; the legacy page is
// still reachable via the address= query.
// Field implements query.Row for chip predicates.
func (p Profile) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return p.ID, true
	case "Name":
		return p.Name, true
	case "Description":
		return p.Description, true
	case "UserType":
		return p.UserType, true
	case "License", "UserLicense", "UserLicenseName":
		return p.UserLicenseName, true
	case "CreatedDate":
		return p.CreatedDate, true
	case "LastModifiedDate":
		return p.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return p.LastModifiedByName, true
	}
	return nil, false
}

func (p Profile) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "All Profiles",
			Path: "/lightning/setup/EnhancedProfiles/home"},
	}
	if p.ID != "" {
		t = append([]OpenTarget{{
			ID: "detail", Label: "Profile — Detail",
			Path: fmt.Sprintf("/lightning/setup/EnhancedProfiles/page?address=%%2F%s", p.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the profile Name / Id. Profiles have no separate
// API-vs-label split, so the Name doubles as the API name row.
func (p Profile) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(p.Name, "", p.ID)
}
