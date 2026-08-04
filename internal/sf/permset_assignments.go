package sf

// PermissionSetAssignment helpers — list users assigned to a given
// permission set (or a profile's implicit permset).

import "fmt"

// PermissionSetAssignment is one assignment row.
type PermissionSetAssignment struct {
	ID               string
	AssigneeID       string
	AssigneeName     string
	AssigneeUsername string
	AssigneeIsActive bool
	ParentID         string // PermissionSet Id
	ExpirationDate   string // ISO-8601 or ""
}

// ListAssignedUsers returns every PermissionSetAssignment for the given
// PermissionSet Id. Sorted by Assignee.Name.
func ListAssignedUsers(target, parentID string) ([]PermissionSetAssignment, error) {
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, AssigneeId, Assignee.Name, Assignee.Username, Assignee.IsActive, "+
				"PermissionSetId, ExpirationDate "+
				"FROM PermissionSetAssignment "+
				"WHERE PermissionSetId = '%s' "+
				"ORDER BY Assignee.Name",
			sqlEscape(parentID),
		),
		false, mapPermissionSetAssignment)
}

func mapPermissionSetAssignment(r map[string]any) PermissionSetAssignment {
	row := PermissionSetAssignment{
		ID:             asString(r["Id"]),
		AssigneeID:     asString(r["AssigneeId"]),
		ParentID:       asString(r["PermissionSetId"]),
		ExpirationDate: asString(r["ExpirationDate"]),
	}
	if a, ok := r["Assignee"].(map[string]any); ok {
		row.AssigneeName = asString(a["Name"])
		row.AssigneeUsername = asString(a["Username"])
		if b, ok := a["IsActive"].(bool); ok {
			row.AssigneeIsActive = b
		}
	}
	return row
}
