package ui

// /meta list surfaces: the Browse type catalogue + per-type component
// list, and the six rich subtabs. All spec-derived; no chips (search
// via / covers filtering needs at these list sizes).

import (
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// --- Browse: type catalogue -----------------------------------------------

func metaTypesColumnSchema() tablemodel.Schema[sf.MetadataTypeInfo] {
	return tablemodel.Schema[sf.MetadataTypeInfo]{
		DefaultColumns: func(scope string) []string {
			return []string{"Type", "Dir", "Folders", "Children"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.MetadataTypeInfo]{
			"Type": textColumnDef[sf.MetadataTypeInfo]("TYPE", tablemodel.Width{Min: 24, Ideal: 36}, func(t sf.MetadataTypeInfo) string { return t.XMLName }),
			"Dir":  textColumnDef[sf.MetadataTypeInfo]("DIR", tablemodel.Width{Min: 14, Ideal: 24}, func(t sf.MetadataTypeInfo) string { return t.DirectoryName }),
			"Folders": textColumnDef[sf.MetadataTypeInfo]("FOLDERS", tablemodel.Width{Min: 7, Ideal: 8}, func(t sf.MetadataTypeInfo) string {
				if t.InFolder {
					return "yes"
				}
				return ""
			}),
			"Children": textColumnDef[sf.MetadataTypeInfo]("CHILDREN", tablemodel.Width{Min: 8, Ideal: 10}, func(t sf.MetadataTypeInfo) string {
				if len(t.ChildXMLNames) == 0 {
					return ""
				}
				return fmt.Sprintf("%d", len(t.ChildXMLNames))
			}),
		},
	}
}

var metaTypesTableSpec = ListViewTableSpec[sf.MetadataTypeInfo]{
	Schema:   metaTypesColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.MetadataTypeInfo] { return &d.MetaTypesList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.MetaTypesTableState },
	Title: func(m Model, d *orgData, items []sf.MetadataTypeInfo) string {
		return standardListTitle("METADATA TYPES", d.MetaTypesList.Len(), &d.MetaTypes)
	},
	ResErr: func(d *orgData) error { return d.MetaTypes.Err() },
	Empty:  "  no metadata types (describe failed?)",
}

var metaTypesListSurface = listSurfaceFromSpec(metaTypesTableSpec)

// --- Browse: drilled type's components --------------------------------------

func metaTypeItemsColumnSchema() tablemodel.Schema[sf.MetadataItem] {
	return tablemodel.Schema[sf.MetadataItem]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Namespace", "Modified"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.MetadataItem]{
			"Name":      textColumnDef[sf.MetadataItem]("NAME", tablemodel.Width{Min: 30, Ideal: 48}, func(i sf.MetadataItem) string { return i.FullName }),
			"Namespace": textColumnDef[sf.MetadataItem]("NAMESPACE", tablemodel.Width{Min: 10, Ideal: 16}, func(i sf.MetadataItem) string { return i.NamespacePrefix }),
			"Modified":  modifiedDateColumnDef[sf.MetadataItem](func(i sf.MetadataItem) string { return i.LastModifiedDate }),
		},
	}
}

var metaTypeItemsTableSpec = ListViewTableSpec[sf.MetadataItem]{
	Schema:   metaTypeItemsColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.MetadataItem] { return &d.MetaTypeItemList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.MetaTypeItemsTableState },
	ResErr: func(d *orgData) error {
		if r, ok := d.MetaTypeItems[d.MetaTypeCur]; ok && r != nil {
			return r.Err()
		}
		return nil
	},
	Title: func(m Model, d *orgData, items []sf.MetadataItem) string {
		title := d.MetaTypeCur
		if title == "" {
			title = "COMPONENTS"
		}
		suffix := ""
		if r, ok := d.MetaTypeItems[d.MetaTypeCur]; ok && r != nil {
			suffix = " · " + humanAge(r.FetchedAt()) + stateSuffix(r.Busy(), r.Err())
		}
		return title + " · " + fmt.Sprintf("%d", d.MetaTypeItemList.Len()) + suffix
	},
	Empty: "  no components of this type in the org",
}

var metaTypeItemsListSurface = listSurfaceFromSpec(metaTypeItemsTableSpec)

// --- Custom Labels -----------------------------------------------------------

func customLabelColumnSchema() tablemodel.Schema[sf.CustomLabelRow] {
	return tablemodel.Schema[sf.CustomLabelRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Value", "Category", "Language"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.CustomLabelRow]{
			"Name":     textColumnDef[sf.CustomLabelRow]("NAME", tablemodel.Width{Min: 20, Ideal: 32}, func(r sf.CustomLabelRow) string { return r.Name }),
			"Value":    textColumnDef[sf.CustomLabelRow]("VALUE", tablemodel.Width{Min: 24, Ideal: 48}, func(r sf.CustomLabelRow) string { return r.Value }),
			"Category": textColumnDef[sf.CustomLabelRow]("CATEGORY", tablemodel.Width{Min: 10, Ideal: 16}, func(r sf.CustomLabelRow) string { return r.Category }),
			"Language": textColumnDef[sf.CustomLabelRow]("LANG", tablemodel.Width{Min: 6, Ideal: 8}, func(r sf.CustomLabelRow) string { return r.Language }),
			"Protected": textColumnDef[sf.CustomLabelRow]("PROT", tablemodel.Width{Min: 5, Ideal: 6}, func(r sf.CustomLabelRow) string {
				if r.IsProtected {
					return "yes"
				}
				return ""
			}),
			"Namespace": textColumnDef[sf.CustomLabelRow]("NAMESPACE", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.CustomLabelRow) string { return r.NamespacePrefix }),
		},
	}
}

var customLabelsTableSpec = ListViewTableSpec[sf.CustomLabelRow]{
	Schema:   customLabelColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.CustomLabelRow] { return &d.CustomLabelList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.CustomLabelsTableState },
	Title: func(m Model, d *orgData, items []sf.CustomLabelRow) string {
		return standardListTitle("CUSTOM LABELS", d.CustomLabelList.Len(), &d.CustomLabels)
	},
	ResErr: func(d *orgData) error { return d.CustomLabels.Err() },
	Empty:  "  no custom labels in this org",
}

var customLabelsListSurface = listSurfaceFromSpec(customLabelsTableSpec)

// --- Custom Metadata types + Custom Settings (shared schema) ----------------

func metaEntityColumnSchema() tablemodel.Schema[sf.MetaEntityRow] {
	return tablemodel.Schema[sf.MetaEntityRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"ApiName", "Label", "Namespace"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.MetaEntityRow]{
			"ApiName":   textColumnDef[sf.MetaEntityRow]("API NAME", tablemodel.Width{Min: 26, Ideal: 44}, func(r sf.MetaEntityRow) string { return r.QualifiedApiName }),
			"Label":     textColumnDef[sf.MetaEntityRow]("LABEL", tablemodel.Width{Min: 18, Ideal: 30}, func(r sf.MetaEntityRow) string { return r.Label }),
			"Namespace": textColumnDef[sf.MetaEntityRow]("NAMESPACE", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.MetaEntityRow) string { return r.NamespacePrefix }),
			"KeyPrefix": textColumnDef[sf.MetaEntityRow]("PREFIX", tablemodel.Width{Min: 6, Ideal: 8}, func(r sf.MetaEntityRow) string { return r.KeyPrefix }),
		},
	}
}

var cmtTableSpec = ListViewTableSpec[sf.MetaEntityRow]{
	Schema:   metaEntityColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.MetaEntityRow] { return &d.CMTList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.CMTTableState },
	Title: func(m Model, d *orgData, items []sf.MetaEntityRow) string {
		return standardListTitle("CUSTOM METADATA TYPES", d.CMTList.Len(), &d.CMTTypes)
	},
	ResErr: func(d *orgData) error { return d.CMTTypes.Err() },
	Empty:  "  no custom metadata types in this org",
}

var cmtListSurface = listSurfaceFromSpec(cmtTableSpec)

var customSettingsTableSpec = ListViewTableSpec[sf.MetaEntityRow]{
	Schema:   metaEntityColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.MetaEntityRow] { return &d.CustomSettingList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.CustomSettingsTableState },
	Title: func(m Model, d *orgData, items []sf.MetaEntityRow) string {
		return standardListTitle("CUSTOM SETTINGS", d.CustomSettingList.Len(), &d.CustomSettings)
	},
	ResErr: func(d *orgData) error { return d.CustomSettings.Err() },
	Empty:  "  no custom settings in this org",
}

var customSettingsListSurface = listSurfaceFromSpec(customSettingsTableSpec)

// --- Static Resources --------------------------------------------------------

func staticResourceColumnSchema() tablemodel.Schema[sf.StaticResourceRow] {
	return tablemodel.Schema[sf.StaticResourceRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "ContentType", "Size", "Modified", "ModifiedBy"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.StaticResourceRow]{
			"Name":        textColumnDef[sf.StaticResourceRow]("NAME", tablemodel.Width{Min: 22, Ideal: 34}, func(r sf.StaticResourceRow) string { return r.Name }),
			"ContentType": textColumnDef[sf.StaticResourceRow]("TYPE", tablemodel.Width{Min: 14, Ideal: 26}, func(r sf.StaticResourceRow) string { return r.ContentType }),
			"Size": withSortKey(
				textColumnDef[sf.StaticResourceRow]("SIZE", tablemodel.Width{Min: 8, Ideal: 10}, func(r sf.StaticResourceRow) string { return humanBytes(r.BodyLength) }),
				func(r sf.StaticResourceRow) string { return fmt.Sprintf("%012d", r.BodyLength) }),
			"Cache":      textColumnDef[sf.StaticResourceRow]("CACHE", tablemodel.Width{Min: 7, Ideal: 8}, func(r sf.StaticResourceRow) string { return r.CacheControl }),
			"Namespace":  textColumnDef[sf.StaticResourceRow]("NAMESPACE", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.StaticResourceRow) string { return r.NamespacePrefix }),
			"Modified":   modifiedDateColumnDef[sf.StaticResourceRow](func(r sf.StaticResourceRow) string { return r.LastModifiedDate }),
			"ModifiedBy": textColumnDef[sf.StaticResourceRow]("BY", tablemodel.Width{Min: 12, Ideal: 20}, func(r sf.StaticResourceRow) string { return r.LastModifiedByName }),
		},
	}
}

var staticResourcesTableSpec = ListViewTableSpec[sf.StaticResourceRow]{
	Schema:   staticResourceColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.StaticResourceRow] { return &d.StaticResourceList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.StaticResourcesTableState },
	Title: func(m Model, d *orgData, items []sf.StaticResourceRow) string {
		return standardListTitle("STATIC RESOURCES", d.StaticResourceList.Len(), &d.StaticResources)
	},
	ResErr: func(d *orgData) error { return d.StaticResources.Err() },
	Empty:  "  no static resources in this org",
}

var staticResourcesListSurface = listSurfaceFromSpec(staticResourcesTableSpec)

// --- Named Credentials --------------------------------------------------------

func namedCredColumnSchema() tablemodel.Schema[sf.NamedCredentialRow] {
	return tablemodel.Schema[sf.NamedCredentialRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Endpoint", "Principal"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.NamedCredentialRow]{
			"Name":      textColumnDef[sf.NamedCredentialRow]("NAME", tablemodel.Width{Min: 20, Ideal: 30}, func(r sf.NamedCredentialRow) string { return r.DeveloperName }),
			"Label":     textColumnDef[sf.NamedCredentialRow]("LABEL", tablemodel.Width{Min: 16, Ideal: 26}, func(r sf.NamedCredentialRow) string { return r.MasterLabel }),
			"Endpoint":  textColumnDef[sf.NamedCredentialRow]("ENDPOINT", tablemodel.Width{Min: 24, Ideal: 44}, func(r sf.NamedCredentialRow) string { return r.Endpoint }),
			"Principal": textColumnDef[sf.NamedCredentialRow]("PRINCIPAL", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.NamedCredentialRow) string { return r.PrincipalType }),
			"Namespace": textColumnDef[sf.NamedCredentialRow]("NAMESPACE", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.NamedCredentialRow) string { return r.NamespacePrefix }),
		},
	}
}

var namedCredsTableSpec = ListViewTableSpec[sf.NamedCredentialRow]{
	Schema:   namedCredColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.NamedCredentialRow] { return &d.NamedCredList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.NamedCredsTableState },
	Title: func(m Model, d *orgData, items []sf.NamedCredentialRow) string {
		return standardListTitle("NAMED CREDENTIALS", d.NamedCredList.Len(), &d.NamedCreds)
	},
	ResErr: func(d *orgData) error { return d.NamedCreds.Err() },
	Empty:  "  no named credentials in this org",
}

var namedCredsListSurface = listSurfaceFromSpec(namedCredsTableSpec)

// --- Remote Sites --------------------------------------------------------------

func remoteSiteColumnSchema() tablemodel.Schema[sf.RemoteSiteRow] {
	return tablemodel.Schema[sf.RemoteSiteRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Endpoint", "Active", "Description"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.RemoteSiteRow]{
			"Name":     textColumnDef[sf.RemoteSiteRow]("NAME", tablemodel.Width{Min: 20, Ideal: 30}, func(r sf.RemoteSiteRow) string { return r.SiteName }),
			"Endpoint": textColumnDef[sf.RemoteSiteRow]("ENDPOINT", tablemodel.Width{Min: 26, Ideal: 46}, func(r sf.RemoteSiteRow) string { return r.EndpointUrl }),
			"Active": textColumnDef[sf.RemoteSiteRow]("ACTIVE", tablemodel.Width{Min: 6, Ideal: 8}, func(r sf.RemoteSiteRow) string {
				if r.IsActive {
					return "yes"
				}
				return "no"
			}),
			"Description": textColumnDef[sf.RemoteSiteRow]("DESCRIPTION", tablemodel.Width{Min: 16, Ideal: 32}, func(r sf.RemoteSiteRow) string { return r.Description }),
		},
	}
}

var remoteSitesTableSpec = ListViewTableSpec[sf.RemoteSiteRow]{
	Schema:   remoteSiteColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.RemoteSiteRow] { return &d.RemoteSiteList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.RemoteSitesTableState },
	Title: func(m Model, d *orgData, items []sf.RemoteSiteRow) string {
		return standardListTitle("REMOTE SITES", d.RemoteSiteList.Len(), &d.RemoteSites)
	},
	ResErr: func(d *orgData) error { return d.RemoteSites.Err() },
	Empty:  "  no remote site settings in this org",
}

var remoteSitesListSurface = listSurfaceFromSpec(remoteSitesTableSpec)
