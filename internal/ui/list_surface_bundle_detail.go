package ui

// /bundle component-preview list surface.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

type bundleDetailRow struct {
	Action    string // "To retrieve" / "To deploy" / "Delete" / "Conflict" / "Ignored"
	Kind      string // ManifestPreviewItem.Type — e.g. "Flow", "ApexClass", "CustomField"
	Member    string // ManifestPreviewItem.FullName — e.g. "Account.Phone"
	Path      string // ManifestPreviewItem.Path — relative to bundle dir; "" when remote-only
	Namespace string // managed-package prefix; empty for non-managed
}

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

func bundleDetailListCols() []uilayout.ListColumn {
	return mustResolveColumns(bundleDetailColumnSchema()).ListColumns()
}

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
