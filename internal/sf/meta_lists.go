package sf

// /meta rich-subtab list fetches. Each is one SOQL (tooling or
// regular) over the long-tail metadata sObjects — verified against
// the phd sandbox 2026-06-12. The generic Browse subtab covers
// everything else via DescribeMetadataTypes + ListMetadata.

// --- Custom Labels (tooling ExternalString) ------------------------------

type CustomLabelRow struct {
	ID              string
	Name            string
	MasterLabel     string
	Value           string
	Category        string
	Language        string
	IsProtected     bool
	NamespacePrefix string
}

func (r CustomLabelRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Name", "DeveloperName":
		return r.Name, true
	case "MasterLabel", "Label":
		return r.MasterLabel, true
	case "Value":
		return r.Value, true
	case "Category":
		return r.Category, true
	case "Language":
		return r.Language, true
	case "IsProtected", "Protected":
		return r.IsProtected, true
	case "NamespacePrefix", "Namespace":
		return r.NamespacePrefix, true
	}
	return nil, false
}

func (r CustomLabelRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "setup", Label: "Custom Labels (Setup)",
			Path: "/lightning/setup/ExternalStrings/home"},
	}
}

// YankTargets exposes the label's API name (Name) / MasterLabel / its
// text Value / Id. The Value is often the reason you're here, so it's
// offered explicitly alongside the identity fields.
func (r CustomLabelRow) YankTargets() []YankTarget {
	ts := nameLabelIDYankTargets(r.Name, r.MasterLabel, r.ID)
	if r.Value != "" {
		ts = append(ts, YankTarget{ID: "value", Label: "Value", Value: r.Value, Shortcut: "v"})
	}
	return ts
}

func ListCustomLabels(target string) ([]CustomLabelRow, error) {
	return queryRows(target,
		"SELECT Id, Name, MasterLabel, Value, Category, Language, IsProtected, "+
			"NamespacePrefix FROM ExternalString ORDER BY Name",
		true, mapCustomLabelRow)
}

func mapCustomLabelRow(r map[string]any) CustomLabelRow {
	row := CustomLabelRow{
		ID:              asString(r["Id"]),
		Name:            asString(r["Name"]),
		MasterLabel:     asString(r["MasterLabel"]),
		Value:           asString(r["Value"]),
		Category:        asString(r["Category"]),
		Language:        asString(r["Language"]),
		NamespacePrefix: asString(r["NamespacePrefix"]),
	}
	if b, ok := r["IsProtected"].(bool); ok {
		row.IsProtected = b
	}
	return row
}

// --- Custom Metadata types + Custom Settings (EntityDefinition) ----------

// MetaEntityRow is one EntityDefinition row — used for both the
// Custom Metadata (…__mdt) and Custom Settings subtabs; Kind
// disambiguates ("cmt" / "setting") and picks the Setup target.
type MetaEntityRow struct {
	QualifiedApiName string
	Label            string
	KeyPrefix        string
	NamespacePrefix  string
	Kind             string
}

func (r MetaEntityRow) Field(name string) (any, bool) {
	switch name {
	case "QualifiedApiName", "Name", "ApiName":
		return r.QualifiedApiName, true
	case "Label", "MasterLabel":
		return r.Label, true
	case "KeyPrefix":
		return r.KeyPrefix, true
	case "NamespacePrefix", "Namespace":
		return r.NamespacePrefix, true
	}
	return nil, false
}

func (r MetaEntityRow) Targets() []OpenTarget {
	if r.Kind == "setting" {
		return []OpenTarget{
			{ID: "setup", Label: "Custom Settings (Setup)",
				Path: "/lightning/setup/CustomSettings/home"},
		}
	}
	t := []OpenTarget{
		{ID: "setup", Label: "Custom Metadata Types (Setup)",
			Path: "/lightning/setup/CustomMetadata/home"},
	}
	if r.KeyPrefix != "" {
		// Classic records-list page wrapped in Lightning chrome —
		// jumps straight to THIS type's records.
		t = append([]OpenTarget{{ID: "records", Label: "Records",
			Path: "/lightning/setup/CustomMetadata/page?address=%2F" + r.KeyPrefix}}, t...)
	}
	return t
}

// YankTargets exposes the entity's QualifiedApiName / Label. There's no
// stable Id on EntityDefinition rows, so the key prefix is offered
// instead when present.
func (r MetaEntityRow) YankTargets() []YankTarget {
	ts := nameLabelIDYankTargets(r.QualifiedApiName, r.Label, "")
	if r.KeyPrefix != "" {
		ts = append(ts, YankTarget{ID: "prefix", Label: "Key prefix", Value: r.KeyPrefix})
	}
	return ts
}

func ListCustomMetadataTypes(target string) ([]MetaEntityRow, error) {
	return listEntityDefs(target,
		"WHERE QualifiedApiName LIKE '%__mdt'", "cmt")
}

func ListCustomSettings(target string) ([]MetaEntityRow, error) {
	return listEntityDefs(target,
		"WHERE IsCustomSetting = true", "setting")
}

func listEntityDefs(target, where, kind string) ([]MetaEntityRow, error) {
	return queryRows(target,
		"SELECT QualifiedApiName, Label, KeyPrefix, NamespacePrefix "+
			"FROM EntityDefinition "+where+" ORDER BY QualifiedApiName",
		false, func(r map[string]any) MetaEntityRow {
			return MetaEntityRow{
				QualifiedApiName: asString(r["QualifiedApiName"]),
				Label:            asString(r["Label"]),
				KeyPrefix:        asString(r["KeyPrefix"]),
				NamespacePrefix:  asString(r["NamespacePrefix"]),
				Kind:             kind,
			}
		})
}

// --- Static Resources -----------------------------------------------------

type StaticResourceRow struct {
	ID                 string
	Name               string
	ContentType        string
	BodyLength         int
	CacheControl       string
	Description        string
	NamespacePrefix    string
	LastModifiedDate   string
	LastModifiedByName string
}

func (r StaticResourceRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Name":
		return r.Name, true
	case "ContentType":
		return r.ContentType, true
	case "BodyLength", "Size":
		return r.BodyLength, true
	case "CacheControl":
		return r.CacheControl, true
	case "Description":
		return r.Description, true
	case "NamespacePrefix", "Namespace":
		return r.NamespacePrefix, true
	case "LastModifiedDate":
		return r.LastModifiedDate, true
	case "LastModifiedBy", "LastModifiedBy.Name", "LastModifiedByName":
		return r.LastModifiedByName, true
	}
	return nil, false
}

func (r StaticResourceRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "setup", Label: "Static Resources (Setup)",
			Path: "/lightning/setup/StaticResources/home"},
	}
	if r.ID != "" {
		t = append([]OpenTarget{{ID: "detail", Label: "Resource detail",
			Path: "/lightning/setup/StaticResources/page?address=%2F" + r.ID}}, t...)
	}
	return t
}

// YankTargets exposes the static resource Name / Id. No separate label.
func (r StaticResourceRow) YankTargets() []YankTarget {
	return nameLabelIDYankTargets(r.Name, "", r.ID)
}

func ListStaticResources(target string) ([]StaticResourceRow, error) {
	return queryRows(target,
		"SELECT Id, Name, ContentType, BodyLength, CacheControl, Description, "+
			"NamespacePrefix, LastModifiedDate, LastModifiedBy.Name "+
			"FROM StaticResource ORDER BY Name",
		false, mapStaticResourceRow)
}

func mapStaticResourceRow(r map[string]any) StaticResourceRow {
	return StaticResourceRow{
		ID:                 asString(r["Id"]),
		Name:               asString(r["Name"]),
		ContentType:        asString(r["ContentType"]),
		BodyLength:         asInt(r["BodyLength"]),
		CacheControl:       asString(r["CacheControl"]),
		Description:        asString(r["Description"]),
		NamespacePrefix:    asString(r["NamespacePrefix"]),
		LastModifiedDate:   asString(r["LastModifiedDate"]),
		LastModifiedByName: relationName(r, "LastModifiedBy"),
	}
}

// --- Named Credentials (tooling) ------------------------------------------

type NamedCredentialRow struct {
	ID              string
	DeveloperName   string
	MasterLabel     string
	Endpoint        string
	PrincipalType   string
	NamespacePrefix string
}

func (r NamedCredentialRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "DeveloperName", "Name":
		return r.DeveloperName, true
	case "MasterLabel", "Label":
		return r.MasterLabel, true
	case "Endpoint":
		return r.Endpoint, true
	case "PrincipalType":
		return r.PrincipalType, true
	case "NamespacePrefix", "Namespace":
		return r.NamespacePrefix, true
	}
	return nil, false
}

func (r NamedCredentialRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "setup", Label: "Named Credentials (Setup)",
			Path: "/lightning/setup/NamedCredential/home"},
	}
}

// YankTargets exposes the credential DeveloperName / MasterLabel / Id,
// plus the endpoint URL (the value most often copied for a cred).
func (r NamedCredentialRow) YankTargets() []YankTarget {
	ts := nameLabelIDYankTargets(r.DeveloperName, r.MasterLabel, r.ID)
	if r.Endpoint != "" {
		ts = append(ts, YankTarget{ID: "endpoint", Label: "Endpoint", Value: r.Endpoint, Shortcut: "e"})
	}
	return ts
}

func ListNamedCredentials(target string) ([]NamedCredentialRow, error) {
	return queryRows(target,
		"SELECT Id, DeveloperName, MasterLabel, Endpoint, PrincipalType, "+
			"NamespacePrefix FROM NamedCredential ORDER BY DeveloperName",
		true, mapNamedCredentialRow)
}

func mapNamedCredentialRow(r map[string]any) NamedCredentialRow {
	return NamedCredentialRow{
		ID:              asString(r["Id"]),
		DeveloperName:   asString(r["DeveloperName"]),
		MasterLabel:     asString(r["MasterLabel"]),
		Endpoint:        asString(r["Endpoint"]),
		PrincipalType:   asString(r["PrincipalType"]),
		NamespacePrefix: asString(r["NamespacePrefix"]),
	}
}

// --- Remote Site Settings (tooling RemoteProxy) ----------------------------

type RemoteSiteRow struct {
	ID          string
	SiteName    string
	EndpointUrl string
	IsActive    bool
	Description string
}

func (r RemoteSiteRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "SiteName", "Name":
		return r.SiteName, true
	case "EndpointUrl", "Endpoint":
		return r.EndpointUrl, true
	case "IsActive", "Active":
		return r.IsActive, true
	case "Description":
		return r.Description, true
	}
	return nil, false
}

func (r RemoteSiteRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "setup", Label: "Remote Site Settings (Setup)",
			Path: "/lightning/setup/SecurityRemoteProxy/home"},
	}
}

// YankTargets exposes the site name / Id, plus the endpoint URL (the
// value most often copied for a remote site).
func (r RemoteSiteRow) YankTargets() []YankTarget {
	ts := nameLabelIDYankTargets(r.SiteName, "", r.ID)
	if r.EndpointUrl != "" {
		ts = append(ts, YankTarget{ID: "endpoint", Label: "Endpoint", Value: r.EndpointUrl, Shortcut: "e"})
	}
	return ts
}

func ListRemoteSites(target string) ([]RemoteSiteRow, error) {
	return queryRows(target,
		"SELECT Id, SiteName, EndpointUrl, IsActive, Description "+
			"FROM RemoteProxy ORDER BY SiteName",
		true, mapRemoteSiteRow)
}

func mapRemoteSiteRow(r map[string]any) RemoteSiteRow {
	row := RemoteSiteRow{
		ID:          asString(r["Id"]),
		SiteName:    asString(r["SiteName"]),
		EndpointUrl: asString(r["EndpointUrl"]),
		Description: asString(r["Description"]),
	}
	if b, ok := r["IsActive"].(bool); ok {
		row.IsActive = b
	}
	return row
}

// --- Browse: per-type component listing -----------------------------------

// ListMetadataComponents lists every component of one metadata type
// via the SOAP listMetadata op (1 call). Used by the /meta Browse
// drill; the type catalogue itself comes from DescribeMetadataTypes.
func ListMetadataComponents(target, metaType string) ([]MetadataItem, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	byType, err := c.ListMetadata([]string{metaType})
	if err != nil {
		return nil, err
	}
	return byType[metaType], nil
}
