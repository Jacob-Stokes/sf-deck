package chips

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

var ErrUnknownColumn = errors.New("unknown column")

// Column is the headless-facing catalogue entry for a chip display
// column. It intentionally describes stable IDs and metadata, not TUI
// widths/styles; those live in the UI table schema.
type Column struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"`
	Type       string `json:"type,omitempty"`
	Source     string `json:"source,omitempty"`
	Default    bool   `json:"default,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Custom     bool   `json:"custom,omitempty"`
	Filterable bool   `json:"filterable,omitempty"`
	Sortable   bool   `json:"sortable,omitempty"`
	Fetchable  bool   `json:"fetchable,omitempty"`
	Searchable bool   `json:"searchable,omitempty"`
	Exportable bool   `json:"exportable,omitempty"`
}

type ColumnCatalog struct {
	Domain         string   `json:"domain"`
	Scope          string   `json:"scope,omitempty"`
	DefaultColumns []string `json:"default_columns"`
	RequiredFields []string `json:"required_fetch_fields,omitempty"`
	Columns        []Column `json:"columns"`
}

type UnknownColumnError struct {
	Domain  string
	Scope   string
	Columns []string
}

func (e UnknownColumnError) Error() string {
	if e.Scope != "" {
		return fmt.Sprintf("%s for %s/%s: %s", ErrUnknownColumn, e.Domain, e.Scope, strings.Join(e.Columns, ", "))
	}
	return fmt.Sprintf("%s for %s: %s", ErrUnknownColumn, e.Domain, strings.Join(e.Columns, ", "))
}

func (e UnknownColumnError) Unwrap() error { return ErrUnknownColumn }

func StaticColumnCatalog(domain string) (ColumnCatalog, bool) {
	switch domain {
	case "objects":
		return catalogFromStatic(domain, "", []Column{
			{ID: "Name", Label: "Name", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Label", Label: "Label", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Marks", Label: "Flags", Type: "virtual", Source: "computed", Default: true, Sortable: true, Searchable: false, Exportable: false},
		}), true
	case "flows":
		return catalogFromStatic(domain, "", []Column{
			{ID: "Name", Label: "Name", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Type", Label: "Type", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Status", Label: "Status", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Version", Label: "Version", Type: "virtual", Source: "computed", Default: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Label", Label: "Label", Type: "string", Source: "static", Default: true, Filterable: true, Sortable: true, Searchable: true, Exportable: true},
			{ID: "Marks", Label: "Flags", Type: "virtual", Source: "computed", Default: true, Sortable: true, Searchable: false, Exportable: false},
		}), true
	}
	return ColumnCatalog{}, false
}

func RecordColumnCatalog(desc sf.SObjectDescribe) ColumnCatalog {
	defaults := recordDefaultColumns(desc)
	required := []string{"Id"}
	defaultSet := stringSet(defaults)
	cols := make([]Column, 0, len(desc.Fields))
	for _, f := range desc.Fields {
		cols = append(cols, Column{
			ID:         f.Name,
			Label:      labelOrName(f.Label, f.Name),
			Type:       f.Type,
			Source:     "describe",
			Default:    defaultSet[f.Name],
			Required:   f.Name == "Id",
			Custom:     f.Custom,
			Filterable: f.Filterable,
			Sortable:   f.Sortable,
			Fetchable:  true,
			Searchable: true,
			Exportable: true,
		})
	}
	sort.SliceStable(cols, func(i, j int) bool {
		if cols[i].Required != cols[j].Required {
			return cols[i].Required
		}
		if cols[i].Default != cols[j].Default {
			return cols[i].Default
		}
		return cols[i].ID < cols[j].ID
	})
	return ColumnCatalog{
		Domain:         "records",
		Scope:          desc.Name,
		DefaultColumns: defaults,
		RequiredFields: required,
		Columns:        cols,
	}
}

func ValidateColumns(catalog ColumnCatalog, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, c := range catalog.Columns {
		known[c.ID] = true
	}
	var unknown []string
	for _, id := range ids {
		if id == "" || known[id] {
			continue
		}
		unknown = append(unknown, id)
	}
	if len(unknown) > 0 {
		return UnknownColumnError{
			Domain:  catalog.Domain,
			Scope:   catalog.Scope,
			Columns: unknown,
		}
	}
	return nil
}

func FilterCatalog(catalog ColumnCatalog, contains string, customOnly bool) ColumnCatalog {
	needle := strings.ToLower(strings.TrimSpace(contains))
	if needle == "" && !customOnly {
		return catalog
	}
	out := catalog
	out.Columns = make([]Column, 0, len(catalog.Columns))
	for _, c := range catalog.Columns {
		if customOnly && !c.Custom {
			continue
		}
		if needle != "" &&
			!strings.Contains(strings.ToLower(c.ID), needle) &&
			!strings.Contains(strings.ToLower(c.Label), needle) {
			continue
		}
		out.Columns = append(out.Columns, c)
	}
	return out
}

func catalogFromStatic(domain, scope string, cols []Column) ColumnCatalog {
	defaults := make([]string, 0, len(cols))
	for i := range cols {
		if cols[i].Default {
			defaults = append(defaults, cols[i].ID)
		}
	}
	return ColumnCatalog{
		Domain:         domain,
		Scope:          scope,
		DefaultColumns: defaults,
		Columns:        cols,
	}
}

func recordDefaultColumns(desc sf.SObjectDescribe) []string {
	has := map[string]bool{}
	for _, f := range desc.Fields {
		has[f.Name] = true
	}
	out := []string{"Id"}
	if has["Name"] {
		out = append(out, "Name")
	}
	if has["LastModifiedDate"] {
		out = append(out, "LastModifiedDate")
	} else if has["CreatedDate"] {
		out = append(out, "CreatedDate")
	}
	return out
}

func labelOrName(label, name string) string {
	if strings.TrimSpace(label) != "" {
		return label
	}
	return name
}

func stringSet(vals []string) map[string]bool {
	out := make(map[string]bool, len(vals))
	for _, v := range vals {
		out[v] = true
	}
	return out
}
