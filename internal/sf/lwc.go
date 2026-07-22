package sf

// LWC = Lightning Web Component. Tooling exposes them as
// LightningComponentBundle records (one bundle per developer-named
// component) plus per-resource LightningComponentResource rows for
// each file inside the bundle (.html / .js / .css / .xml).
//
// We list the bundles for /lwc and offer drill-in to the bundle's
// resources for /lwc/<bundle>. Aura is a different entity
// (AuraDefinitionBundle) — supported separately if/when needed; the
// list here is LWC-only.

import (
	"encoding/json"
	"fmt"
)

// LWCBundle is one row in the org's LWC list. Drill in to fetch
// resources via GetLWCBundle.
type LWCBundle struct {
	ID                 string
	DeveloperName      string
	MasterLabel        string
	Description        string
	NamespacePrefix    string
	ApiVersion         float64
	IsExposed          bool
	CreatedDate        string
	CreatedByName      string
	LastModifiedDate   string
	LastModifiedByName string
}

// LWCResource is one file inside a bundle.
type LWCResource struct {
	ID       string
	FilePath string // "myComponent/myComponent.js"
	Format   string // "js" / "html" / "css" / "xml" / "svg" / "ts"
	Source   string
}

// ListLWCBundles returns every LightningComponentBundle in the org,
// including namespaced (managed-package) bundles. Chip filters in
// the UI handle the "managed vs unmanaged" cut.
func ListLWCBundles(target string) ([]LWCBundle, error) {
	return queryRows(target,
		"SELECT Id, DeveloperName, MasterLabel, Description, "+
			"NamespacePrefix, ApiVersion, IsExposed, "+
			"CreatedDate, CreatedBy.Name, LastModifiedDate, LastModifiedBy.Name "+
			"FROM LightningComponentBundle "+
			"ORDER BY DeveloperName",
		true, mapLWCBundle)
}

func mapLWCBundle(r map[string]any) LWCBundle {
	row := LWCBundle{
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
	if b, ok := r["IsExposed"].(bool); ok {
		row.IsExposed = b
	}
	return row
}

// LWCBundleDetail bundles every resource (file) under one LWC bundle.
type LWCBundleDetail struct {
	Bundle    LWCBundle
	Resources []LWCResource
}

// GetLWCBundle fetches a bundle's resources. Two queries:
//  1. The bundle row itself (refreshes the metadata captured in the
//     list call — useful when the user drills in stale).
//  2. LightningComponentResource rows for that bundle, with Source.
//
// Source can be large (~1-100KB per file). We pull it eagerly because
// the user is drilling specifically to read it; lazy fetch per-file
// would chain N+1 round-trips for what's already a single-bundle view.
func GetLWCBundle(target, bundleID string) (LWCBundleDetail, error) {
	c, err := RESTClient(target)
	if err != nil {
		return LWCBundleDetail{}, err
	}
	bundlePath := c.ToolingPath("sobjects/LightningComponentBundle/" + bundleID)
	body, err := c.get(bundlePath, nil)
	if err != nil {
		return LWCBundleDetail{}, upgradeToSFError(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(body, &rec); err != nil {
		return LWCBundleDetail{}, err
	}
	det := LWCBundleDetail{
		Bundle: LWCBundle{
			ID:                 asString(rec["Id"]),
			DeveloperName:      asString(rec["DeveloperName"]),
			MasterLabel:        asString(rec["MasterLabel"]),
			Description:        asString(rec["Description"]),
			NamespacePrefix:    asString(rec["NamespacePrefix"]),
			CreatedDate:        asString(rec["CreatedDate"]),
			LastModifiedDate:   asString(rec["LastModifiedDate"]),
			LastModifiedByName: relationName(rec, "LastModifiedBy"),
		},
	}
	if v, ok := rec["ApiVersion"].(float64); ok {
		det.Bundle.ApiVersion = v
	}
	if b, ok := rec["IsExposed"].(bool); ok {
		det.Bundle.IsExposed = b
	}

	soql := fmt.Sprintf(
		"SELECT Id, FilePath, Format, Source "+
			"FROM LightningComponentResource "+
			"WHERE LightningComponentBundleId = '%s' "+
			"ORDER BY FilePath",
		sqlEscape(bundleID))
	q, err := c.QueryREST(soql, true)
	if err != nil {
		return det, nil // bundle row is still useful even if resources fail
	}
	for _, r := range q.Records {
		det.Resources = append(det.Resources, LWCResource{
			ID:       asString(r["Id"]),
			FilePath: asString(r["FilePath"]),
			Format:   asString(r["Format"]),
			Source:   asString(r["Source"]),
		})
	}
	return det, nil
}

// Field implements query.Row for chip predicates.
func (l LWCBundle) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return l.ID, true
	case "DeveloperName", "Name":
		return l.DeveloperName, true
	case "MasterLabel", "Label":
		return l.MasterLabel, true
	case "Description":
		return l.Description, true
	case "NamespacePrefix", "Namespace":
		return l.NamespacePrefix, true
	case "IsExposed", "Exposed":
		return l.IsExposed, true
	case "ApiVersion", "APIVersion":
		return l.ApiVersion, true
	case "LastModifiedDate":
		return l.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return l.LastModifiedByName, true
	case "CreatedDate":
		return l.CreatedDate, true
	case "CreatedBy", "CreatedBy.Name", "CreatedByName":
		return l.CreatedByName, true
	}
	return nil, false
}

// Targets implements sf.Openable. LWC bundles aren't directly
// reachable in Lightning Setup; we fall back to the LWC reference
// page (where users can search) and provide a generic "/<id>"
// classic-redirect that still resolves bundles by Id.
func (l LWCBundle) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "library", Label: "Lightning Web Component Library",
			AbsoluteURL: "https://developer.salesforce.com/docs/component-library/overview/components"},
	}
	if l.ID != "" {
		t = append([]OpenTarget{{
			ID:    "view",
			Label: "Bundle (classic redirect)",
			Path:  fmt.Sprintf("/%s", l.ID),
		}}, t...)
	}
	return t
}

// YankTargets exposes the bundle DeveloperName / MasterLabel / Id.
func (l LWCBundle) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(l.DeveloperName, l.MasterLabel, l.ID)
}
