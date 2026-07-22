package ui

// /bundle drill list surface. Replaces the static text dump in
// tab_bundle_detail.go's body with a sortable list table over the
// preview's components.
//
// Row = one component in the bundle's retrieve / deploy preview.
// Action = "To retrieve" / "To deploy" / "Delete" / "Conflict" /
// "Ignored" — sourced from which slice of the ManifestPreview the
// item came from. Kind / Member mirror the preview's Type /
// FullName.
//
// Storage lives on Model.bundleDetail* (not orgData) because
// bundles are org-independent. The table state resolver
// (listTableBundleDetail) is wired via TabSpec.ListTable rather
// than the listSurface registry so the closures can reach Model
// directly.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// bundleDetailRow is one selectable row in the bundle detail
// list. Action drives the leftmost column + the recolor pass
// (green = retrieve, blue = deploy, red = delete, yellow =
// conflict, muted = ignored).
type bundleDetailRow struct {
	Action    string // "To retrieve" / "To deploy" / "Delete" / "Conflict" / "Ignored"
	Kind      string // ManifestPreviewItem.Type — e.g. "Flow", "ApexClass", "CustomField"
	Member    string // ManifestPreviewItem.FullName — e.g. "Account.Phone"
	Path      string // ManifestPreviewItem.Path — relative to bundle dir; "" when remote-only
	Namespace string // managed-package prefix; empty for non-managed
}

// bundleDetailColumnSchema is the canonical column spec for the
// list. Action / Kind / Member is the natural reading order;
// sortable so users can group by action ("show me everything I'd
// retrieve") or by kind ("show me all the flows").
func bundleDetailColumnSchema() tablemodel.Schema[bundleDetailRow] {
	return tablemodel.Schema[bundleDetailRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Action", "Kind", "Member"}
		},
		Columns: map[string]tablemodel.ColumnDef[bundleDetailRow]{
			"Action": {
				Header: "ACTION",
				Width:  tablemodel.Width{Min: 10, Ideal: 14},
				Render: func(r bundleDetailRow) string { return r.Action },
			},
			"Kind": {
				Header: "KIND",
				Width:  tablemodel.Width{Min: 12, Ideal: 24},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(r bundleDetailRow) string { return r.Kind },
			},
			"Member": {
				Header: "MEMBER",
				Width:  tablemodel.Width{Min: 20, Ideal: 40},
				Render: func(r bundleDetailRow) string {
					if r.Namespace != "" {
						return r.Namespace + "__" + r.Member
					}
					return r.Member
				},
			},
		},
	}
}

// bundleDetailListCols is the published column list. Same shape
// the renderer uses + the TabSpec.ListTable resolver returns.
func bundleDetailListCols() []uilayout.ListColumn {
	return mustResolveColumns(bundleDetailColumnSchema()).ListColumns()
}

// listTableBundleDetail is the TabSpec.ListTable resolver. Hands
// back the persistent list-table state + canonical column defs so
// the c (column-mode) / s (sort) / [ ] (resize) gestures wire
// onto this surface for free. Branches on view mode so the FILES
// view gets its own table-state + columns instead of sharing the
// components surface's.
func listTableBundleDetail(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	if m == nil {
		return nil, nil
	}
	if m.bundleDetailView == bundleViewFiles {
		return &m.bundleFilesTable, bundleFileListCols()
	}
	return &m.bundleDetailTable, bundleDetailListCols()
}

// bundleDetailRowsFromPreview flattens a bundlePreview into the
// row schema. Action labels mirror Salesforce's own terminology
// from `sf project retrieve preview` (toRetrieve, toDelete,
// conflicts, ignored) and `sf project deploy preview` (toDeploy).
//
// Order: retrieve → deploy → conflicts → delete → ignored. Puts
// the actionable rows first; ignored sits at the bottom because
// it's noise most of the time but useful for diagnosing "why isn't
// this picked up."
func bundleDetailRowsFromPreview(p bundlePreview) []bundleDetailRow {
	if p.Err != nil {
		return nil
	}
	var rows []bundleDetailRow
	pushItems := func(items []sf.ManifestPreviewItem, action string) {
		for _, it := range items {
			rows = append(rows, bundleDetailRow{
				Action:    action,
				Kind:      it.Type,
				Member:    it.FullName,
				Path:      it.Path,
				Namespace: it.Namespace,
			})
		}
	}
	pushItems(p.Retrieve.ToRetrieve, "To retrieve")
	pushItems(p.Deploy.ToDeploy, "To deploy")
	// Conflict / delete / ignored come from either preview — the
	// shapes are identical and a row only appears in one slice. We
	// prefer the retrieve preview because that's the canonical view
	// in the source-tracked path; the fallback path leaves Deploy
	// empty.
	pushItems(p.Retrieve.Conflicts, "Conflict")
	pushItems(p.Retrieve.ToDelete, "Delete (local)")
	pushItems(p.Retrieve.Ignored, "Ignored")
	return rows
}

// recolorBundleDetailRow tints the Action cell based on the row's
// action — green retrieve / blue deploy / yellow conflict / red
// delete / muted ignored. Returns the style derived from the row;
// applies only to col 0 (Action), leaves the rest untouched.
func recolorBundleDetailRow(r bundleDetailRow, col int, base lipgloss.Style) lipgloss.Style {
	if col != 0 {
		return base
	}
	switch r.Action {
	case "To retrieve":
		return base.Foreground(theme.Green)
	case "To deploy":
		return base.Foreground(theme.Blue)
	case "Conflict":
		return base.Foreground(theme.Yellow)
	case "Delete (local)":
		return base.Foreground(theme.Red)
	case "Ignored":
		return base.Foreground(theme.Muted)
	}
	return base
}
