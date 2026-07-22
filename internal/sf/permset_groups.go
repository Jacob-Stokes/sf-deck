package sf

// Permission Set Group listing + component resolution for the /perms
// tab. A PSG's effective permissions are the union of its component
// PermissionSets; for v1 we surface the component list only (no
// merged-perm view yet).

import "fmt"

// PermissionSetGroup is one PSG record.
type PermissionSetGroup struct {
	ID                 string
	DeveloperName      string
	MasterLabel        string
	Description        string
	Status             string // "Updated", "Outdated", "Failed", "Updating"
	NamespacePrefix    string
	CreatedDate        string
	LastModifiedDate   string
	LastModifiedByName string
}

// ListPermissionSetGroups returns every PSG in the org, sorted by label.
func ListPermissionSetGroups(target string) ([]PermissionSetGroup, error) {
	return queryRows(target,
		"SELECT Id, DeveloperName, MasterLabel, Description, Status, "+
			"NamespacePrefix, CreatedDate, LastModifiedDate, LastModifiedBy.Name "+
			"FROM PermissionSetGroup "+
			"ORDER BY MasterLabel",
		false, mapPermissionSetGroup)
}

func mapPermissionSetGroup(r map[string]any) PermissionSetGroup {
	g := PermissionSetGroup{
		ID:              asString(r["Id"]),
		DeveloperName:   asString(r["DeveloperName"]),
		MasterLabel:     asString(r["MasterLabel"]),
		Description:     asString(r["Description"]),
		Status:          asString(r["Status"]),
		NamespacePrefix: asString(r["NamespacePrefix"]),
	}
	g.CreatedDate = asString(r["CreatedDate"])
	g.LastModifiedDate = asString(r["LastModifiedDate"])
	g.LastModifiedByName = relationName(r, "LastModifiedBy")
	return g
}

// PSGComponent is one PermissionSet membership in a PSG.
type PSGComponent struct {
	ID                   string // PermissionSetGroupComponent.Id
	PermissionSetGroupID string
	PermissionSetID      string
	PermissionSetName    string
	PermissionSetLabel   string
}

// PermSetGroupComponents returns the component permsets of a PSG.
func PermSetGroupComponents(target, psgID string) ([]PSGComponent, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, PermissionSetGroupId, PermissionSetId, "+
				"PermissionSet.Name, PermissionSet.Label "+
				"FROM PermissionSetGroupComponent "+
				"WHERE PermissionSetGroupId = '%s'",
			sqlEscape(psgID)),
		false, mapPSGComponent)
}

func mapPSGComponent(r map[string]any) PSGComponent {
	comp := PSGComponent{
		ID:                   asString(r["Id"]),
		PermissionSetGroupID: asString(r["PermissionSetGroupId"]),
		PermissionSetID:      asString(r["PermissionSetId"]),
	}
	if ps, ok := r["PermissionSet"].(map[string]any); ok {
		comp.PermissionSetName = asString(ps["Name"])
		comp.PermissionSetLabel = asString(ps["Label"])
	}
	return comp
}

// Targets — PSGs open into the Setup PSG overview page.
// Field implements query.Row for chip predicates.
func (g PermissionSetGroup) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return g.ID, true
	case "DeveloperName", "Name":
		return g.DeveloperName, true
	case "MasterLabel", "Label":
		return g.MasterLabel, true
	case "Description":
		return g.Description, true
	case "Status":
		return g.Status, true
	case "Namespace", "NamespacePrefix":
		return g.NamespacePrefix, true
	case "CreatedDate":
		return g.CreatedDate, true
	case "LastModifiedDate":
		return g.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return g.LastModifiedByName, true
	}
	return nil, false
}

func (g PermissionSetGroup) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "All Permission Set Groups",
			Path: "/lightning/setup/PermSetGroups/home"},
	}
	if g.ID != "" {
		t = append([]OpenTarget{{
			ID: "overview", Label: "Permission Set Group — Overview",
			Path: fmt.Sprintf("/lightning/setup/PermSetGroups/page?address=%%2F%s", g.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the PSG DeveloperName (API) / MasterLabel / Id.
func (g PermissionSetGroup) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(g.DeveloperName, g.MasterLabel, g.ID)
}
