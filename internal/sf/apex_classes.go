package sf

// ApexClass list + drill helpers. Tooling-API read; mirrors the shape
// of triggers.go since both store Body as a top-level column.
//
// Read-only for now — write paths (deploy / save body) come later via
// the same UpdateToolingMetadata route triggers use.

import (
	"encoding/json"
	"fmt"
)

// ApexClassRow is one row in the org's apex-class list. Light enough
// for table rendering; drill into a row to fetch Body via GetApexClass.
type ApexClassRow struct {
	ID                 string
	Name               string
	NamespacePrefix    string
	Status             string // "Active" / "Deleted"
	IsValid            bool
	ApiVersion         float64
	LengthNoComments   int
	LastModifiedDate   string
	LastModifiedByName string
}

// ListApexClasses returns every ApexClass in the org. Includes
// managed-package classes — chip filters in the UI handle the
// "managed vs unmanaged" cut so admins keep visibility into
// package-installed code.
func ListApexClasses(target string) ([]ApexClassRow, error) {
	return queryRows(target,
		"SELECT Id, Name, NamespacePrefix, Status, IsValid, "+
			"ApiVersion, LengthWithoutComments, LastModifiedDate, LastModifiedBy.Name "+
			"FROM ApexClass ORDER BY Name",
		true, mapApexClassRow)
}

func mapApexClassRow(r map[string]any) ApexClassRow {
	row := ApexClassRow{
		ID:                 asString(r["Id"]),
		Name:               asString(r["Name"]),
		NamespacePrefix:    asString(r["NamespacePrefix"]),
		Status:             asString(r["Status"]),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
	if b, ok := r["IsValid"].(bool); ok {
		row.IsValid = b
	}
	if v, ok := r["ApiVersion"].(float64); ok {
		row.ApiVersion = v
	}
	if n, ok := r["LengthWithoutComments"].(float64); ok {
		row.LengthNoComments = int(n)
	}
	return row
}

// ApexClassDetail is the full body + column set for one class.
type ApexClassDetail struct {
	ID               string
	Name             string
	Status           string
	Body             string
	ApiVersion       float64
	IsValid          bool
	LengthNoComments int
	LastModifiedDate string
}

// GetApexClass fetches one ApexClass record — including Body — via
// Tooling. Body is a top-level column on Tooling's ApexClass entity,
// so no Metadata envelope is required for the read path.
func GetApexClass(target, id string) (ApexClassDetail, error) {
	c, err := RESTClient(target)
	if err != nil {
		return ApexClassDetail{}, err
	}
	path := c.ToolingPath("sobjects/ApexClass/" + id)
	body, err := c.get(path, nil)
	if err != nil {
		return ApexClassDetail{}, upgradeToSFError(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(body, &rec); err != nil {
		return ApexClassDetail{}, err
	}
	det := ApexClassDetail{
		ID:               asString(rec["Id"]),
		Name:             asString(rec["Name"]),
		Status:           asString(rec["Status"]),
		Body:             asString(rec["Body"]),
		LastModifiedDate: asString(rec["LastModifiedDate"]),
	}
	if b, ok := rec["IsValid"].(bool); ok {
		det.IsValid = b
	}
	if v, ok := rec["ApiVersion"].(float64); ok {
		det.ApiVersion = v
	}
	if n, ok := rec["LengthWithoutComments"].(float64); ok {
		det.LengthNoComments = int(n)
	}
	return det, nil
}

// Field implements query.Row so chip predicates can filter the list
// without round-tripping through reflection. Names mirror the
// SOQL columns we read.
func (a ApexClassRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return a.ID, true
	case "Name":
		return a.Name, true
	case "NamespacePrefix", "Namespace":
		return a.NamespacePrefix, true
	case "Status":
		return a.Status, true
	case "IsValid":
		return a.IsValid, true
	case "ApiVersion", "APIVersion":
		return a.ApiVersion, true
	case "LengthWithoutComments", "Lines":
		return a.LengthNoComments, true
	case "LastModifiedDate":
		return a.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return a.LastModifiedByName, true
	}
	return nil, false
}

// Targets implements sf.Openable so the cursor on /apex hands a
// resolvable Lightning URL set to the open-menu. Apex Setup classes
// list comes first (admins land here most often); class-specific URL
// uses the classic detail path which still works in Lightning.
func (a ApexClassRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "list", Label: "Apex Classes (Setup)",
			Path: "/lightning/setup/ApexClasses/home"},
	}
	if a.ID != "" {
		t = append([]OpenTarget{{
			ID:    "view",
			Label: "Class detail (classic)",
			Path:  fmt.Sprintf("/%s", a.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the class name / Id for copy. Apex classes have
// no separate label, so only the name + Id show (the name row is
// labelled "API name" for consistency with other components).
func (a ApexClassRow) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(a.Name, "", a.ID)
}
