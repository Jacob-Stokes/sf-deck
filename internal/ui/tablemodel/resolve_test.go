package tablemodel

import (
	"errors"
	"testing"
)

func TestResolveUsesDefaultsAndRequiredFields(t *testing.T) {
	schema := Schema[string]{
		DefaultColumns: func(scope string) []string { return []string{"Name"} },
		RequiredFields: func(scope string) []string {
			return []string{"Id"}
		},
		Columns: map[string]ColumnDef[string]{
			"Name": {FetchFields: []string{"Name"}, Render: func(s string) string { return s }},
		},
	}

	got, err := Resolve(schema, nil, "Account")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Defs) != 1 || got.Defs[0].ID != "Name" {
		t.Fatalf("Defs = %#v, want Name", got.Defs)
	}
	if fields := got.FetchFields(); len(fields) != 2 || fields[0] != "Id" || fields[1] != "Name" {
		t.Fatalf("FetchFields() = %#v, want Id,Name", fields)
	}
}

func TestResolveRejectsUnknownColumn(t *testing.T) {
	_, err := Resolve(Schema[string]{}, []string{"Bogus"}, "")
	if !errors.Is(err, ErrUnknownColumn) {
		t.Fatalf("Resolve err = %v, want ErrUnknownColumn", err)
	}
}

func TestResolvedCellAndSortCell(t *testing.T) {
	rows := []string{"b"}
	res := Resolved[string]{Defs: []ColumnDef[string]{
		{
			ID:      "Name",
			Render:  func(s string) string { return "render:" + s },
			SortKey: func(s string) string { return "sort:" + s },
		},
	}}

	if got := res.Cell(rows)(0, 0); got != "render:b" {
		t.Fatalf("Cell() = %q", got)
	}
	if got := res.SortCell(rows)(0, 0); got != "sort:b" {
		t.Fatalf("SortCell() = %q", got)
	}
}
