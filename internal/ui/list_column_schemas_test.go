package ui

import (
	"reflect"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func TestApexClassColumnSchemaDefaultsAndCells(t *testing.T) {
	resolved := mustResolveColumns(apexClassColumnSchema())
	var got []string
	for _, col := range resolved.ListColumns() {
		got = append(got, col.Name)
	}
	want := []string{"Name", "Status", "Valid", "Api", "Size", "Modified", "ModifiedBy", "Marks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}

	row := sf.ApexClassRow{
		Name:               "RequestController",
		Status:             "Active",
		IsValid:            true,
		ApiVersion:         61,
		LengthNoComments:   42,
		LastModifiedDate:   "2026-05-18T10:00:00.000+0000",
		LastModifiedByName: "Sam Admin",
	}
	if got := resolved.Defs[3].Cell(row); got != "v61.0" {
		t.Fatalf("Api cell = %q, want v61.0", got)
	}
	if got := resolved.Defs[4].Cell(row); got != "42" {
		t.Fatalf("Size cell = %q, want 42", got)
	}
	// Large char counts compact — a ~990k-CHAR class (e.g. MetadataService)
	// renders "992.5K", not the raw figure that used to masquerade as lines.
	if got := resolved.Defs[4].Cell(sf.ApexClassRow{LengthNoComments: 992501}); got != "992.5K" {
		t.Fatalf("Size cell (large) = %q, want 992.5K", got)
	}
	if got := resolved.Defs[5].Cell(row); got != "2026-05-18 10:00" {
		t.Fatalf("Modified cell = %q, want 2026-05-18 10:00", got)
	}
	if got := resolved.Defs[6].Cell(row); got != "Sam Admin" {
		t.Fatalf("ModifiedBy cell = %q, want Sam Admin", got)
	}
}

// TestNumericColumnsSortByRawValue guards the class of bug where a
// column whose cell renders a human label (Apex "1.5K" size, "v62.0"
// API, flow "v9") sorted lexically off that label instead of the raw
// number — so "992" ordered below "1.5K" and flow v10 below v2. Each
// column's SortKey must produce strings that compare in true numeric
// order. See resolvedSortCellByID.
func TestNumericColumnsSortByRawValue(t *testing.T) {
	// Apex SIZE: 847 chars must sort BELOW a 3.2K (3200) class even
	// though "847" > "3.2K" lexically.
	apex := mustResolveColumns(apexClassColumnSchema())
	small := sortCellForRow(t, apex, "Size", sf.ApexClassRow{LengthNoComments: 847})
	big := sortCellForRow(t, apex, "Size", sf.ApexClassRow{LengthNoComments: 3200})
	if !(small < big) {
		t.Fatalf("apex Size sort: 847 (%q) should sort below 3200 (%q)", small, big)
	}

	// Apex API: v9.0 must sort below v62.0.
	lo := sortCellForRow(t, apex, "Api", sf.ApexClassRow{ApiVersion: 9})
	hi := sortCellForRow(t, apex, "Api", sf.ApexClassRow{ApiVersion: 62})
	if !(lo < hi) {
		t.Fatalf("apex Api sort: v9 (%q) should sort below v62 (%q)", lo, hi)
	}

	// Flow VERSION: active v2 must sort below active v10 (the user's
	// report: "does 1-9 and 9-1 regardless of other numbers").
	flow := mustResolveColumns(flowColumnSchema())
	v2 := sortCellForRow(t, flow, "Version", sf.Flow{ActiveVersionNum: 2})
	v10 := sortCellForRow(t, flow, "Version", sf.Flow{ActiveVersionNum: 10})
	if !(v2 < v10) {
		t.Fatalf("flow Version sort: v2 (%q) should sort below v10 (%q)", v2, v10)
	}
	// No active version → falls back to latest for sorting.
	vLatest := sortCellForRow(t, flow, "Version", sf.Flow{LatestVersionNum: 5})
	vNone := sortCellForRow(t, flow, "Version", sf.Flow{})
	if !(vNone < vLatest) {
		t.Fatalf("flow Version sort: no-version (%q) should sort below v5 (%q)", vNone, vLatest)
	}
}

// sortCellForRow resolves the named column's SortCell for one row.
func sortCellForRow[T any](t *testing.T, resolved tablemodel.Resolved[T], name string, row T) string {
	t.Helper()
	for _, def := range resolved.Defs {
		if def.ID == name {
			return def.SortCell(row)
		}
	}
	t.Fatalf("column %q not found", name)
	return ""
}

func TestComponentColumnSchemasDefaults(t *testing.T) {
	lwc := mustResolveColumns(lwcColumnSchema()).ListColumns()
	if got, want := namesOfListColumns(lwc), []string{"Name", "Label", "Exposed", "Api", "Modified", "ModifiedBy", "Marks"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lwc columns = %v, want %v", got, want)
	}

	aura := mustResolveColumns(auraColumnSchema()).ListColumns()
	if got, want := namesOfListColumns(aura), []string{"Name", "Label", "Api", "Modified", "ModifiedBy", "Marks"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("aura columns = %v, want %v", got, want)
	}
}

func TestStandardAuditColumnsSitBeforeMarks(t *testing.T) {
	check := func(name string, cols []uilayout.ListColumn) {
		got := namesOfListColumns(cols)
		mod := testColumnIndex(got, "Modified")
		modBy := testColumnIndex(got, "ModifiedBy")
		marks := testColumnIndex(got, "Marks")
		if mod < 0 || modBy < 0 {
			t.Fatalf("%s columns = %v; missing audit columns", name, got)
		}
		if marks >= 0 && !(mod < marks && modBy < marks) {
			t.Fatalf("%s columns = %v; audit columns should be before Marks", name, got)
		}
		if modBy != mod+1 {
			t.Fatalf("%s columns = %v; ModifiedBy should immediately follow Modified", name, got)
		}
	}

	check("sobject", mustResolveColumns(sobjectColumnSchema()).ListColumns())
	check("flow", mustResolveColumns(flowColumnSchema()).ListColumns())
	check("apexClass", mustResolveColumns(apexClassColumnSchema()).ListColumns())
	check("trigger", mustResolveColumns(apexTriggerColumnSchema()).ListColumns())
	check("lwc", mustResolveColumns(lwcColumnSchema()).ListColumns())
	check("aura", mustResolveColumns(auraColumnSchema()).ListColumns())
	check("permset", mustResolveColumns(permSetColumnSchema()).ListColumns())
	check("psg", mustResolveColumns(psgColumnSchema()).ListColumns())
	check("profile", mustResolveColumns(profileColumnSchema()).ListColumns())
}

// TestFlagsColumnsAreUnsortable guards the "can't sort on flags column"
// behaviour: every FLAGS column across the list schemas is a composite
// glyph strip with no meaningful lex order, so it must carry
// Unsortable=true (handleColSort flashes a hint and refuses). Non-FLAGS
// columns must stay sortable. If someone adds a new FLAGS column without
// the flag, this catches it.
func TestFlagsColumnsAreUnsortable(t *testing.T) {
	check := func(name string, cols []uilayout.ListColumn) {
		sawFlags := false
		for _, c := range cols {
			isFlags := c.Header == "FLAGS"
			if isFlags {
				sawFlags = true
				if !c.Unsortable {
					t.Errorf("%s: FLAGS column %q is sortable, want Unsortable", name, c.Name)
				}
			} else if c.Unsortable {
				t.Errorf("%s: non-FLAGS column %q is Unsortable unexpectedly", name, c.Name)
			}
		}
		if !sawFlags {
			t.Errorf("%s: no FLAGS column found — test target moved?", name)
		}
	}

	check("sobject", mustResolveColumns(sobjectColumnSchema()).ListColumns())
	check("field", mustResolveColumns(fieldColumnSchema()).ListColumns())
	check("flow", mustResolveColumns(flowColumnSchema()).ListColumns())
	check("apexClass", mustResolveColumns(apexClassColumnSchema()).ListColumns())
	check("trigger", mustResolveColumns(apexTriggerColumnSchema()).ListColumns())
	check("lwc", mustResolveColumns(lwcColumnSchema()).ListColumns())
	check("aura", mustResolveColumns(auraColumnSchema()).ListColumns())
}

func namesOfListColumns(cols []uilayout.ListColumn) []string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		out = append(out, col.Name)
	}
	return out
}

func testColumnIndex(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
	return -1
}
