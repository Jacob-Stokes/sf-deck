package sf

// Aura = legacy Lightning Component framework. Tooling exposes
// bundles as AuraDefinitionBundle + per-file AuraDefinition rows
// (.cmp / .js / .css / .design / .svg / .auradoc / .evt / .tokens /
// the controller and helper).
//
// Same shape as LWC; we surface them on the same /components tab via
// a chip toggle so the user can swap between "LWC" and "Aura" without
// changing surfaces.

import (
	"encoding/json"
	"fmt"
)

// AuraBundle is one row in the org's Aura list.
type AuraBundle struct {
	ID                 string
	DeveloperName      string
	MasterLabel        string
	Description        string
	NamespacePrefix    string
	ApiVersion         float64
	CreatedDate        string
	CreatedByName      string
	LastModifiedDate   string
	LastModifiedByName string
}

// AuraResource is one file inside a bundle. Salesforce uses DefType +
// Format to discriminate (e.g. DefType=COMPONENT/Format=XML for the
// .cmp; DefType=CONTROLLER/Format=JS for the controller). We surface
// FilePath when available + always Source.
type AuraResource struct {
	ID      string
	DefType string // "APPLICATION" / "COMPONENT" / "CONTROLLER" / "HELPER" / ...
	Format  string // "XML" / "JS" / "CSS" / ...
	Source  string
}

// ListAuraBundles returns every AuraDefinitionBundle in the org,
// including namespaced (managed-package) bundles. Chip filters in
// the UI handle the "managed vs unmanaged" cut.
func ListAuraBundles(target string) ([]AuraBundle, error) {
	return queryRows(target,
		"SELECT Id, DeveloperName, MasterLabel, Description, "+
			"NamespacePrefix, ApiVersion, "+
			"CreatedDate, CreatedBy.Name, LastModifiedDate, LastModifiedBy.Name "+
			"FROM AuraDefinitionBundle "+
			"ORDER BY DeveloperName",
		true, mapAuraBundle)
}

func mapAuraBundle(r map[string]any) AuraBundle {
	row := AuraBundle{
		ID:                 asString(r["Id"]),
		DeveloperName:      asString(r["DeveloperName"]),
		MasterLabel:        asString(r["MasterLabel"]),
		Description:        asString(r["Description"]),
		NamespacePrefix:    asString(r["NamespacePrefix"]),
		CreatedDate:        asString(r["CreatedDate"]),
		CreatedByName:      relationName(r, "CreatedBy"),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
	if v, ok := r["ApiVersion"].(float64); ok {
		row.ApiVersion = v
	}
	return row
}

// AuraBundleDetail bundles every resource (file) under one Aura bundle.
type AuraBundleDetail struct {
	Bundle    AuraBundle
	Resources []AuraResource
}

// GetAuraBundle fetches a bundle's resources via Tooling.
func GetAuraBundle(target, bundleID string) (AuraBundleDetail, error) {
	c, err := RESTClient(target)
	if err != nil {
		return AuraBundleDetail{}, err
	}
	bundlePath := c.ToolingPath("sobjects/AuraDefinitionBundle/" + bundleID)
	body, err := c.get(bundlePath, nil)
	if err != nil {
		return AuraBundleDetail{}, upgradeToSFError(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(body, &rec); err != nil {
		return AuraBundleDetail{}, err
	}
	det := AuraBundleDetail{
		Bundle: AuraBundle{
			ID:                 asString(rec["Id"]),
			DeveloperName:      asString(rec["DeveloperName"]),
			MasterLabel:        asString(rec["MasterLabel"]),
			Description:        asString(rec["Description"]),
			NamespacePrefix:    asString(rec["NamespacePrefix"]),
			CreatedDate:        asString(rec["CreatedDate"]),
			CreatedByName:      relationName(rec, "CreatedBy"),
			LastModifiedDate:   asString(rec["LastModifiedDate"]),
			LastModifiedByName: relationName(rec, "LastModifiedBy"),
		},
	}
	if v, ok := rec["ApiVersion"].(float64); ok {
		det.Bundle.ApiVersion = v
	}

	soql := fmt.Sprintf(
		"SELECT Id, DefType, Format, Source "+
			"FROM AuraDefinition "+
			"WHERE AuraDefinitionBundleId = '%s' "+
			"ORDER BY DefType",
		sqlEscape(bundleID))
	q, err := c.QueryREST(soql, true)
	if err != nil {
		return det, nil
	}
	for _, r := range q.Records {
		det.Resources = append(det.Resources, AuraResource{
			ID:      asString(r["Id"]),
			DefType: asString(r["DefType"]),
			Format:  asString(r["Format"]),
			Source:  asString(r["Source"]),
		})
	}
	return det, nil
}

// Field implements query.Row for chip predicates.
func (a AuraBundle) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return a.ID, true
	case "DeveloperName", "Name":
		return a.DeveloperName, true
	case "MasterLabel", "Label":
		return a.MasterLabel, true
	case "Description":
		return a.Description, true
	case "NamespacePrefix", "Namespace":
		return a.NamespacePrefix, true
	case "ApiVersion", "APIVersion":
		return a.ApiVersion, true
	case "LastModifiedDate":
		return a.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return a.LastModifiedByName, true
	case "CreatedDate":
		return a.CreatedDate, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName":
		return a.CreatedByName, true
	}
	return nil, false
}

// Targets implements sf.Openable. No direct Setup URL for Aura
// bundles in Lightning; classic /<id> redirect resolves to the
// editor when present.
func (a AuraBundle) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "library", Label: "Lightning Component Library",
			AbsoluteURL: "https://developer.salesforce.com/docs/component-library/overview/components"},
	}
	if a.ID != "" {
		t = append([]OpenTarget{{
			ID:    "view",
			Label: "Bundle (classic redirect)",
			Path:  fmt.Sprintf("/%s", a.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the bundle DeveloperName / MasterLabel / Id.
func (a AuraBundle) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(a.DeveloperName, a.MasterLabel, a.ID)
}
