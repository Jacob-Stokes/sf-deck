package tablemodel

import "github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"

type Resolved[T any] struct {
	Defs           []ColumnDef[T]
	RequiredFields []string
}

func Resolve[T any](schema Schema[T], ids []string, scope string) (Resolved[T], error) {
	if len(ids) == 0 {
		ids = schema.Defaults(scope)
	}
	res := Resolved[T]{
		Defs:           make([]ColumnDef[T], 0, len(ids)),
		RequiredFields: schema.Required(scope),
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		def, ok := schema.Column(id)
		if !ok {
			return Resolved[T]{}, UnknownColumnError{ID: id}
		}
		if def.ID == "" {
			def.ID = id
		}
		if !def.Hidden {
			res.Defs = append(res.Defs, def)
		}
		res.RequiredFields = appendUnique(res.RequiredFields, def.FetchFields...)
	}
	return res, nil
}

func (r Resolved[T]) ListColumns() []uilayout.ListColumn {
	out := make([]uilayout.ListColumn, 0, len(r.Defs))
	for _, def := range r.Defs {
		out = append(out, def.ListColumn())
	}
	return out
}

func (r Resolved[T]) Cell(rows []T) func(row, col int) string {
	return func(row, col int) string {
		if row < 0 || row >= len(rows) || col < 0 || col >= len(r.Defs) {
			return ""
		}
		return r.Defs[col].Cell(rows[row])
	}
}

func (r Resolved[T]) SortCell(rows []T) func(row, col int) string {
	return func(row, col int) string {
		if row < 0 || row >= len(rows) || col < 0 || col >= len(r.Defs) {
			return ""
		}
		return r.Defs[col].SortCell(rows[row])
	}
}

func (r Resolved[T]) FetchFields() []string {
	return append([]string(nil), r.RequiredFields...)
}

func appendUnique(dst []string, vals ...string) []string {
	seen := make(map[string]bool, len(dst)+len(vals))
	out := dst[:0]
	for _, v := range dst {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, v := range vals {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
