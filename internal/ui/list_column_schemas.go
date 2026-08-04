package ui

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func sobjectColumnSchema() tablemodel.Schema[sf.SObject] {
	return tablemodel.Schema[sf.SObject]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.SObject]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 18, Ideal: 32},
				Render: func(s sf.SObject) string { return s.Name },
			},
			"Label": {
				Header: "LABEL",
				Width:  tablemodel.Width{Min: 16, Ideal: 36},
				Render: func(s sf.SObject) string { return s.Label },
			},
			"Modified":   modifiedDateColumnDef[sf.SObject](func(s sf.SObject) string { return s.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.SObject](func(s sf.SObject) string { return s.LastModifiedByName }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 14, Max: 18},
			},
		},
	}
}

// fieldColumnSchema drives the Schema subtab's field list (the
// /objects/<X>/Schema browser). FLAGS is a regular column rendering the
// fixed-slot icon strip (R/U/X/A/B) rather than the RowMark-badge model
// the other surfaces use — fields want the scannable fixed-position
// layout. Name's custom=cyan tint is applied via the surface Recolor
// hook (the schema can't see Custom from the bare Render value).
func fieldColumnSchema() tablemodel.Schema[sf.Field] {
	return tablemodel.Schema[sf.Field]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Type", "Flags", "Detail"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.Field]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 16, Ideal: 30},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(f sf.Field) string { return f.Name },
			},
			"Label": {
				Header: "LABEL",
				Width:  tablemodel.Width{Min: 12, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(f sf.Field) string { return dashIfEmpty(f.Label) },
			},
			"Type": {
				Header: "TYPE",
				Width:  tablemodel.Width{Min: 10, Ideal: 16},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: fieldTypeDisplay,
			},
			"Flags": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 9, Ideal: 9, Max: 9},
				Render:     func(f sf.Field) string { return fieldFlagsIcons(f) },
			},
			"Detail": {
				Header: "DETAIL",
				Width:  tablemodel.Width{Min: 12, Ideal: 40},
				Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
				Render: fieldDetailDisplay,
			},
		},
	}
}

func flowColumnSchema() tablemodel.Schema[sf.Flow] {
	return tablemodel.Schema[sf.Flow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Type", "Status", "Version", "Label", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.Flow]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(f sf.Flow) string { return f.DeveloperName },
			},
			"Type": {
				Header: "TYPE",
				Width:  tablemodel.Width{Min: 8, Ideal: 12},
				Style:  lipgloss.NewStyle().Foreground(theme.Cyan),
				Render: func(f sf.Flow) string { return shortenProcessType(f.ProcessType) },
			},
			"Status": {
				Header: "STATUS",
				Width:  tablemodel.Width{Min: 8, Ideal: 12},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(f sf.Flow) string { return f.Status },
			},
			"Version": {
				Header: "VERSION",
				// Cell is "v3" / "—" normally, or "v3 (v4)" when the
				// newest version is a later draft than the active one.
				// Ideal fits the bracketed form; Min still fits the
				// header + a bare "v3" when space is tight.
				Width:  tablemodel.Width{Min: 5, Ideal: 9},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: flowVersionCell,
				// Sort by the effective version number (active, or
				// latest when there's no active version), zero-padded
				// so it orders numerically — the rendered "v3 (v4)"
				// label would sort lexically ("v10" before "v2").
				SortKey: func(f sf.Flow) string { return fmt.Sprintf("%06d", flowSortVersion(f)) },
			},
			"Label": {
				Header: "LABEL",
				Width:  tablemodel.Width{Min: 16, Ideal: 32},
				Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
				Render: func(f sf.Flow) string {
					if f.MasterLabel != "" {
						return f.MasterLabel
					}
					return dashIfEmpty(f.Description)
				},
			},
			"Modified":   modifiedDateColumnDef[sf.Flow](func(f sf.Flow) string { return f.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.Flow](func(f sf.Flow) string { return f.LastModifiedBy }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 16, Max: 22},
			},
		},
	}
}

func apexClassColumnSchema() tablemodel.Schema[sf.ApexClassRow] {
	return tablemodel.Schema[sf.ApexClassRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Status", "Valid", "Api", "Size", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.ApexClassRow]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 18, Ideal: 32},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(a sf.ApexClassRow) string { return a.Name },
			},
			"Status": {
				Header: "STATUS",
				Width:  tablemodel.Width{Min: 8, Ideal: 10},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(a sf.ApexClassRow) string { return a.Status },
			},
			"Valid": {
				Header: "VALID",
				Width:  tablemodel.Width{Min: 6, Ideal: 6},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(a sf.ApexClassRow) string {
					if a.IsValid {
						return "yes"
					}
					return "no"
				},
			},
			"Api": {
				Header:  "API",
				Width:   tablemodel.Width{Min: 6, Ideal: 6},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(a sf.ApexClassRow) string { return apiVersionCell(a.ApiVersion) },
				SortKey: func(a sf.ApexClassRow) string { return fmt.Sprintf("%08.1f", a.ApiVersion) },
			},
			// SIZE, not LINES: the source is ApexClass.LengthWithoutComments,
			// which is the CHARACTER count of the body minus comments — not
			// a line count. Labelling it "LINES" made every class look
			// 30-50x too big (a ~14k-line class reports ~990k). Rendered as
			// a compact char count ("3.2K", "992K").
			"Size": {
				Header: "SIZE",
				Width:  tablemodel.Width{Min: 7, Ideal: 7},
				Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
				// Sort by the raw char count, zero-padded so the string
				// compare orders numerically ("847" must sort below
				// "3.2K"/3200, which the rendered form wouldn't).
				SortKey: func(a sf.ApexClassRow) string {
					return fmt.Sprintf("%09d", a.LengthNoComments)
				},
				Render: func(a sf.ApexClassRow) string {
					if a.LengthNoComments > 0 {
						return compactChars(a.LengthNoComments)
					}
					return ""
				},
			},
			"Modified": {
				Header:  "MODIFIED",
				Width:   tablemodel.Width{Min: 14, Ideal: 16},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(a sf.ApexClassRow) string { return prettyDate(a.LastModifiedDate) },
				SortKey: func(a sf.ApexClassRow) string { return a.LastModifiedDate },
			},
			"ModifiedBy": modifiedByColumnDef[sf.ApexClassRow](func(a sf.ApexClassRow) string { return a.LastModifiedByName }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 16, Max: 24},
			},
		},
	}
}

func apexTriggerColumnSchema() tablemodel.Schema[sf.TriggerRow] {
	return tablemodel.Schema[sf.TriggerRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Trigger", "SObject", "Status", "Events", "Valid", "Api", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.TriggerRow]{
			"Trigger": {
				Header: "TRIGGER",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(t sf.TriggerRow) string { return t.Name },
			},
			"SObject": {
				Header: "SOBJECT",
				Width:  tablemodel.Width{Min: 16, Ideal: 20},
				Style:  lipgloss.NewStyle().Foreground(theme.Cyan),
				Render: func(t sf.TriggerRow) string { return t.Table },
			},
			"Status": {
				Header: "STATUS",
				Width:  tablemodel.Width{Min: 8, Ideal: 10},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(t sf.TriggerRow) string { return dashIfEmpty(t.Status) },
			},
			"Events": {
				Header: "EVENTS",
				Width:  tablemodel.Width{Min: 16, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
				Render: func(t sf.TriggerRow) string { return dashIfEmpty(t.Events) },
			},
			"Valid": {
				Header: "VALID",
				Width:  tablemodel.Width{Min: 6, Ideal: 6},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(t sf.TriggerRow) string {
					if t.Valid {
						return "yes"
					}
					return "no"
				},
			},
			"Api": {
				Header:  "API",
				Width:   tablemodel.Width{Min: 6, Ideal: 6},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(t sf.TriggerRow) string { return apiVersionCell(t.ApiVer) },
				SortKey: func(t sf.TriggerRow) string { return fmt.Sprintf("%08.1f", t.ApiVer) },
			},
			"Modified":   modifiedDateColumnDef[sf.TriggerRow](func(t sf.TriggerRow) string { return t.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.TriggerRow](func(t sf.TriggerRow) string { return t.LastModifiedByName }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 16, Max: 24},
			},
		},
	}
}

func lwcColumnSchema() tablemodel.Schema[sf.LWCBundle] {
	return tablemodel.Schema[sf.LWCBundle]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Exposed", "Api", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.LWCBundle]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(b sf.LWCBundle) string { return b.DeveloperName },
			},
			"Label": {
				Header: "LABEL",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(b sf.LWCBundle) string { return bundleLabelCell(b.DeveloperName, b.MasterLabel) },
			},
			"Exposed": {
				Header: "EXPOSED",
				Width:  tablemodel.Width{Min: 8, Ideal: 8},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(b sf.LWCBundle) string {
					if b.IsExposed {
						return "yes"
					}
					return "no"
				},
			},
			"Api": {
				Header:  "API",
				Width:   tablemodel.Width{Min: 6, Ideal: 6},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(b sf.LWCBundle) string { return apiVersionCell(b.ApiVersion) },
				SortKey: func(b sf.LWCBundle) string { return fmt.Sprintf("%08.1f", b.ApiVersion) },
			},
			"Modified": {
				Header:  "MODIFIED",
				Width:   tablemodel.Width{Min: 14, Ideal: 16},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(b sf.LWCBundle) string { return prettyDate(b.LastModifiedDate) },
				SortKey: func(b sf.LWCBundle) string { return b.LastModifiedDate },
			},
			"ModifiedBy": modifiedByColumnDef[sf.LWCBundle](func(b sf.LWCBundle) string { return b.LastModifiedByName }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 16, Max: 22},
			},
		},
	}
}

func auraColumnSchema() tablemodel.Schema[sf.AuraBundle] {
	return tablemodel.Schema[sf.AuraBundle]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Api", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.AuraBundle]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(b sf.AuraBundle) string { return b.DeveloperName },
			},
			"Label": {
				Header: "LABEL",
				Width:  tablemodel.Width{Min: 18, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(b sf.AuraBundle) string { return bundleLabelCell(b.DeveloperName, b.MasterLabel) },
			},
			"Api": {
				Header:  "API",
				Width:   tablemodel.Width{Min: 6, Ideal: 6},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(b sf.AuraBundle) string { return apiVersionCell(b.ApiVersion) },
				SortKey: func(b sf.AuraBundle) string { return fmt.Sprintf("%08.1f", b.ApiVersion) },
			},
			"Modified": {
				Header:  "MODIFIED",
				Width:   tablemodel.Width{Min: 14, Ideal: 16},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(b sf.AuraBundle) string { return prettyDate(b.LastModifiedDate) },
				SortKey: func(b sf.AuraBundle) string { return b.LastModifiedDate },
			},
			"ModifiedBy": modifiedByColumnDef[sf.AuraBundle](func(b sf.AuraBundle) string { return b.LastModifiedByName }),
			"Marks": {
				Header:     "FLAGS",
				Unsortable: true, // composite glyph strip — see ColumnDef.Unsortable
				Width:      tablemodel.Width{Min: 8, Ideal: 14, Max: 18},
			},
		},
	}
}

func apiVersionCell(v float64) string {
	if v > 0 {
		return fmt.Sprintf("v%.1f", v)
	}
	return ""
}

func bundleLabelCell(name, label string) string {
	if label == name {
		return ""
	}
	return dashIfEmpty(label)
}

func mustResolveColumns[T any](schema tablemodel.Schema[T]) tablemodel.Resolved[T] {
	resolved, err := tablemodel.Resolve(schema, nil, "*")
	if err != nil {
		return tablemodel.Resolved[T]{}
	}
	return resolved
}

func schemaListColumns[T any](schema tablemodel.Schema[T]) []uilayout.ListColumn {
	return mustResolveColumns(schema).ListColumns()
}

func resolvedCellByID[T any](resolved tablemodel.Resolved[T], row T, id string) string {
	for _, def := range resolved.Defs {
		if def.ID == id {
			return def.Cell(row)
		}
	}
	return ""
}

// resolvedSortCellByID mirrors resolvedCellByID but returns the
// column's SortCell — the SortKey value when the column defines one,
// falling back to the rendered cell. Column sorting MUST use this,
// not resolvedCellByID: a column like Apex "SIZE" renders "1.5K" but
// sorts on a zero-padded raw char count, so ordering off the rendered
// label sorts lexically ("1.5K" < "992" < "3.2K") instead of by size.
func resolvedSortCellByID[T any](resolved tablemodel.Resolved[T], row T, id string) string {
	for _, def := range resolved.Defs {
		if def.ID == id {
			return def.SortCell(row)
		}
	}
	return ""
}

func resolvedCellForListColumn[T any](
	resolved tablemodel.Resolved[T],
	items []T,
	cols []uilayout.ListColumn,
	row, col int,
) string {
	if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
		return ""
	}
	return resolvedCellByID(resolved, items[row], cols[col].Name)
}

// resolvedSortCellForListColumn is the sort-path counterpart to
// resolvedCellForListColumn — resolves via SortKey so column sorts
// order by the raw value, not the rendered label. See
// resolvedSortCellByID.
func resolvedSortCellForListColumn[T any](
	resolved tablemodel.Resolved[T],
	items []T,
	cols []uilayout.ListColumn,
	row, col int,
) string {
	if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
		return ""
	}
	return resolvedSortCellByID(resolved, items[row], cols[col].Name)
}

// devProjectItemColumnSchema drives the /dev-projects detail Items
// table. Unified schema covering all 19 ItemKinds via kind-aware
// renderers so a flow row, a field row, and a saved-SOQL row can
// share one sortable table without per-kind layout hacks.
//
// Default columns: Kind, Name, Reference, Type, Added. Origin /
// Managed / Notes are hidden by default but addable via chip
// --columns when the user wants them. The five defaults are the
// minimum that lets a user identify any item in the project at a
// glance.
func devProjectItemColumnSchema() tablemodel.Schema[devproject.Item] {
	return tablemodel.Schema[devproject.Item]{
		DefaultColumns: func(scope string) []string {
			return []string{"Kind", "Name", "Reference", "Type", "Added"}
		},
		Columns: map[string]tablemodel.ColumnDef[devproject.Item]{
			"Kind": {
				Header: "KIND",
				Width:  tablemodel.Width{Min: 8, Ideal: 12},
				Style:  lipgloss.NewStyle().Foreground(theme.Cyan),
				Render: func(it devproject.Item) string { return devProjectKindLabel(it.Kind) },
			},
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 20, Ideal: 36},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(it devproject.Item) string {
					name := it.Name
					if name == "" {
						name = it.Ref
					}
					if it.Managed() {
						name = "[ns] " + name
					}
					return name
				},
			},
			"Reference": {
				Header: "REFERENCE",
				Width:  tablemodel.Width{Min: 16, Ideal: 24},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(it devproject.Item) string { return it.Ref },
			},
			"Type": {
				Header: "TYPE",
				Width:  tablemodel.Width{Min: 12, Ideal: 20},
				Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
				Render: func(it devproject.Item) string {
					if it.Type != "" {
						return it.Type
					}
					return "—"
				},
			},
			"Added":   devProjectAddedColumnDef(),
			"Org":     devProjectOrgColumnDef(),
			"Notes":   devProjectNotesColumnDef(),
			"Managed": devProjectManagedColumnDef(),
		},
	}
}

func devProjectAddedColumnDef() tablemodel.ColumnDef[devproject.Item] {
	return tablemodel.ColumnDef[devproject.Item]{
		Header: "ADDED",
		Width:  tablemodel.Width{Min: 10, Ideal: 14},
		Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
		Render: func(it devproject.Item) string {
			if it.AddedAt.IsZero() {
				return "—"
			}
			return humanTimeAgo(it.AddedAt)
		},
	}
}

func devProjectOrgColumnDef() tablemodel.ColumnDef[devproject.Item] {
	return tablemodel.ColumnDef[devproject.Item]{
		Header: "ORG",
		Width:  tablemodel.Width{Min: 12, Ideal: 24},
		Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
		Render: func(it devproject.Item) string {
			if it.OrgUser == "" {
				return "—" // org-agnostic kinds (soql_query, apex_snippet)
			}
			return it.OrgUser
		},
	}
}

func devProjectNotesColumnDef() tablemodel.ColumnDef[devproject.Item] {
	return tablemodel.ColumnDef[devproject.Item]{
		Header: "NOTES",
		Width:  tablemodel.Width{Min: 8, Ideal: 24},
		Style:  lipgloss.NewStyle().Foreground(theme.FgDim),
		Render: func(it devproject.Item) string {
			if it.Notes == "" {
				return ""
			}
			return it.Notes
		},
	}
}

func devProjectManagedColumnDef() tablemodel.ColumnDef[devproject.Item] {
	return tablemodel.ColumnDef[devproject.Item]{
		Header: "NS",
		Width:  tablemodel.Width{Min: 4, Ideal: 12},
		Style:  lipgloss.NewStyle().Foreground(theme.Yellow),
		Render: func(it devproject.Item) string { return it.Namespace },
	}
}

// devProjectKindLabel renders the user-visible kind label. Mirrors
// the labels already in use on the kind-filter chip strip so the
// column and the chip read the same way.
func devProjectKindLabel(k devproject.ItemKind) string {
	switch k {
	case devproject.KindSObject:
		return "Object"
	case devproject.KindField:
		return "Field"
	case devproject.KindFlow:
		return "Flow"
	case devproject.KindFlowVersion:
		return "Flow ver"
	case devproject.KindRecord:
		return "Record"
	case devproject.KindApexClass:
		return "Apex"
	case devproject.KindApexTrigger:
		return "Trigger"
	case devproject.KindReport:
		return "Report"
	case devproject.KindPermissionSet:
		return "Permset"
	case devproject.KindPermissionSetGroup:
		return "PSG"
	case devproject.KindProfile:
		return "Profile"
	case devproject.KindValidationRule:
		return "Val rule"
	case devproject.KindRecordType:
		return "Rec type"
	case devproject.KindLWC:
		return "LWC"
	case devproject.KindAura:
		return "Aura"
	case devproject.KindQueue:
		return "Queue"
	case devproject.KindPublicGroup:
		return "Group"
	case devproject.KindSOQLQuery:
		return "SOQL"
	case devproject.KindApexSnippet:
		return "Snippet"
	}
	return string(k)
}
