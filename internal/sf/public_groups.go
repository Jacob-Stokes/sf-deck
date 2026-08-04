package sf

// Public Groups — Group records where Type='Regular'. Salesforce
// reuses the Group sObject for queues, public groups, role groups,
// and the implicit groups behind a sharing rule. We only surface
// the user-defined "Regular" public groups here.

type PublicGroupRow struct {
	ID                 string
	Name               string
	DeveloperName      string
	DoesIncludeBosses  bool
	Members            int
	LastModifiedDate   string
	LastModifiedByName string
}

func (g PublicGroupRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return g.ID, true
	case "Name":
		return g.Name, true
	case "DeveloperName":
		return g.DeveloperName, true
	case "DoesIncludeBosses":
		return g.DoesIncludeBosses, true
	case "Members":
		return g.Members, true
	case "LastModifiedDate":
		return g.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return g.LastModifiedByName, true
	}
	return nil, false
}

func (g PublicGroupRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "Public Groups (Setup)",
			Path: "/lightning/setup/PublicGroups/home"},
	}
	if g.ID != "" {
		t = append([]OpenTarget{{
			ID:    "view",
			Label: "Group (classic redirect)",
			Path:  "/" + g.ID,
		}}, t...)
	}
	return t
}

// YankTargets exposes the group DeveloperName (API) / Name (label) / Id.
func (g PublicGroupRow) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(g.DeveloperName, g.Name, g.ID)
}

// ListPublicGroups returns every non-queue, non-role Group in the
// org. Type='Regular' filter excludes the implicit groups Salesforce
// creates for queues / role hierarchies / sharing rules.
func ListPublicGroups(target string) ([]PublicGroupRow, error) {
	return queryRows(target,
		"SELECT Id, Name, DeveloperName, DoesIncludeBosses, LastModifiedDate, LastModifiedBy.Name FROM Group "+
			"WHERE Type='Regular' ORDER BY Name",
		false, mapPublicGroupRow)
}

func mapPublicGroupRow(r map[string]any) PublicGroupRow {
	row := PublicGroupRow{
		ID:                 asString(r["Id"]),
		Name:               asString(r["Name"]),
		DeveloperName:      asString(r["DeveloperName"]),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
	if b, ok := r["DoesIncludeBosses"].(bool); ok {
		row.DoesIncludeBosses = b
	}
	return row
}
