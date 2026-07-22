package sf

// Dashboards — list of saved Lightning dashboards via SOQL on the
// Dashboard sObject. Listed for browsing + open-in-Lightning; sf-deck
// doesn't attempt to render dashboard components in a terminal.

// DashboardRow is one dashboard in the org list.
type DashboardRow struct {
	ID                 string
	Title              string
	DeveloperName      string
	FolderName         string
	Type               string // running-user mode: SpecifiedUser / LoggedInUser
	NamespacePrefix    string
	Description        string
	LastModifiedDate   string
	LastModifiedByName string
}

// Field implements query.Row for chip predicates.
func (d DashboardRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return d.ID, true
	case "Title", "Name":
		return d.Title, true
	case "DeveloperName":
		return d.DeveloperName, true
	case "Folder", "FolderName":
		return d.FolderName, true
	case "Type":
		return d.Type, true
	case "Namespace", "NamespacePrefix":
		return d.NamespacePrefix, true
	case "Description":
		return d.Description, true
	case "LastModifiedDate":
		return d.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return d.LastModifiedByName, true
	}
	return nil, false
}

// Targets implements Openable — o opens the dashboard in Lightning.
func (d DashboardRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "dashboard", Label: "Dashboard · " + d.Title,
			Path: "/lightning/r/Dashboard/" + d.ID + "/view"},
		{ID: "dashboard_list", Label: "All Dashboards",
			Path: "/lightning/o/Dashboard/home"},
	}
}

// ListDashboards returns every dashboard visible to the user.
func ListDashboards(target string) ([]DashboardRow, error) {
	return queryRows(target,
		"SELECT Id, Title, DeveloperName, FolderName, Type, NamespacePrefix, "+
			"Description, LastModifiedDate, LastModifiedBy.Name "+
			"FROM Dashboard ORDER BY Title",
		false, mapDashboardRow)
}

func mapDashboardRow(r map[string]any) DashboardRow {
	return DashboardRow{
		ID:                 asString(r["Id"]),
		Title:              asString(r["Title"]),
		DeveloperName:      asString(r["DeveloperName"]),
		FolderName:         asString(r["FolderName"]),
		Type:               asString(r["Type"]),
		NamespacePrefix:    asString(r["NamespacePrefix"]),
		Description:        asString(r["Description"]),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
}
