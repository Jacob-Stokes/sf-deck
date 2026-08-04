package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// ---- wizardRowForField mapping per Salesforce field type -----------

func TestWizardRowForFieldTypes(t *testing.T) {
	cases := []struct {
		name        string
		field       sf.Field
		wantOk      bool
		wantOp      query.Op
		wantKind    cwKind
		wantInLabel string // substring expected somewhere in the label
	}{
		{"string", sf.Field{Name: "Name", Label: "Name", Type: "string", Filterable: true},
			true, query.OpContains, cwText, "Name contains"},
		{"textarea", sf.Field{Name: "Description", Type: "textarea", Filterable: true},
			true, query.OpContains, cwText, "Description"},
		{"email", sf.Field{Name: "Email", Type: "email", Filterable: true},
			true, query.OpContains, cwText, ""},
		{"picklist", sf.Field{Name: "Stage", Type: "picklist", Filterable: true,
			PicklistValues: []sf.PicklistValue{
				{Value: "Open", Active: true},
				{Value: "Closed", Active: true},
			},
		}, true, query.OpEq, cwText, ""},
		{"boolean", sf.Field{Name: "IsActive", Type: "boolean", Filterable: true},
			true, query.OpEq, cwTri, ""},
		{"int", sf.Field{Name: "Count", Type: "int", Filterable: true},
			true, query.OpEq, cwInt, ""},
		{"double", sf.Field{Name: "Score", Type: "double", Filterable: true},
			true, query.OpEq, cwInt, ""},
		{"currency", sf.Field{Name: "Amount", Type: "currency", Filterable: true},
			true, query.OpEq, cwInt, ""},
		{"date", sf.Field{Name: "CloseDate", Type: "date", Filterable: true},
			true, query.OpDateLiteral, cwText, ""},
		{"datetime", sf.Field{Name: "LastModifiedDate", Type: "datetime", Filterable: true},
			true, query.OpDateLiteral, cwText, ""},
		{"id", sf.Field{Name: "Id", Type: "id", Filterable: true},
			true, query.OpEq, cwText, ""},
		{"reference", sf.Field{Name: "OwnerId", Type: "reference", Filterable: true},
			true, query.OpEq, cwText, ""},
		// Unsupported types skip the catalogue.
		{"blob", sf.Field{Name: "Body", Type: "base64", Filterable: true},
			false, "", 0, ""},
		{"location", sf.Field{Name: "ShippingAddress", Type: "location", Filterable: true},
			false, "", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, ok := wizardRowForField(tc.field)
			if ok != tc.wantOk {
				t.Fatalf("ok mismatch: got %v want %v", ok, tc.wantOk)
			}
			if !ok {
				return
			}
			if row.Op != tc.wantOp {
				t.Errorf("Op: got %s want %s", row.Op, tc.wantOp)
			}
			if row.Kind != tc.wantKind {
				t.Errorf("Kind: got %d want %d", row.Kind, tc.wantKind)
			}
			if tc.wantInLabel != "" && !contains(row.Label, tc.wantInLabel) {
				t.Errorf("Label %q should contain %q", row.Label, tc.wantInLabel)
			}
		})
	}
}

func TestWizardRowForFieldOwnerHintsAtUserId(t *testing.T) {
	row, ok := wizardRowForField(sf.Field{
		Name: "OwnerId", Type: "reference", Filterable: true,
	})
	if !ok {
		t.Fatal("OwnerId should produce a row")
	}
	if !contains(row.Hint, "$userId") {
		t.Fatalf("OwnerId hint should mention $userId, got %q", row.Hint)
	}
}

func TestWizardRowForFieldUsesDescribeLabel(t *testing.T) {
	row, ok := wizardRowForField(sf.Field{
		Name: "Industry__c", Label: "Industry", Type: "picklist", Filterable: true,
	})
	if !ok {
		t.Fatal("custom picklist should produce a row")
	}
	if row.Label != "Industry" {
		t.Errorf("expected label 'Industry', got %q", row.Label)
	}
}

func TestWizardRowForFieldFallsBackToNameWhenLabelMissing(t *testing.T) {
	row, ok := wizardRowForField(sf.Field{
		Name: "Custom_Field__c", Type: "string", Filterable: true,
	})
	if !ok {
		t.Fatal("expected row")
	}
	if !contains(row.Label, "Custom_Field__c") {
		t.Fatalf("label should fall back to Name, got %q", row.Label)
	}
}

// ---- fieldsFromDescribe ordering + capping --------------------------

func TestFieldsFromDescribePrioritisesAuditFields(t *testing.T) {
	desc := sf.SObjectDescribe{
		Name: "Account",
		Fields: []sf.Field{
			// Mix of fields including audit + a custom string.
			{Name: "Custom_Field__c", Type: "string", Filterable: true, Custom: true},
			{Name: "OwnerId", Type: "reference", Filterable: true},
			{Name: "Name", Type: "string", Filterable: true, NameField: true},
			{Name: "CreatedDate", Type: "datetime", Filterable: true},
			{Name: "LastModifiedDate", Type: "datetime", Filterable: true},
			{Name: "CreatedById", Type: "reference", Filterable: true},
			{Name: "LastModifiedById", Type: "reference", Filterable: true},
		},
	}
	rows := fieldsFromDescribe(desc)
	if len(rows) < 6 {
		t.Fatalf("expected at least 6 rows for Account, got %d", len(rows))
	}
	// First 6 must be the priority audit/identity fields, in order.
	want := []string{"Name", "OwnerId", "CreatedDate", "LastModifiedDate", "CreatedById", "LastModifiedById"}
	for i, w := range want {
		if rows[i].Field != w {
			t.Errorf("position %d: got %s want %s", i, rows[i].Field, w)
		}
	}
}

func TestFieldsFromDescribeSkipsNonFilterable(t *testing.T) {
	desc := sf.SObjectDescribe{
		Fields: []sf.Field{
			{Name: "ShouldSkip", Type: "string", Filterable: false},
			{Name: "Name", Type: "string", Filterable: true, NameField: true},
		},
	}
	rows := fieldsFromDescribe(desc)
	for _, r := range rows {
		if r.Field == "ShouldSkip" {
			t.Fatal("non-filterable field should be excluded from the catalogue")
		}
	}
}

func TestFieldsFromDescribeIncludesEveryFilterableField(t *testing.T) {
	// Priority + custom + standard fields. The wizard catalogue is
	// no longer capped — viewport scrolling handles long lists, so
	// every filterable field must show up.
	fields := []sf.Field{
		{Name: "OwnerId", Type: "reference", Filterable: true},
		{Name: "Name", Type: "string", Filterable: true, NameField: true},
		{Name: "CreatedDate", Type: "datetime", Filterable: true},
		{Name: "LastModifiedDate", Type: "datetime", Filterable: true},
		// Standard (non-custom) field — used to be excluded after
		// the priority slots; now should show up.
		{Name: "Industry", Type: "picklist", Filterable: true},
	}
	for i := 0; i < 20; i++ {
		fields = append(fields, sf.Field{
			Name: customName(i), Type: "string", Filterable: true, Custom: true,
		})
	}
	rows := fieldsFromDescribe(sf.SObjectDescribe{Fields: fields})
	// All 25 (4 priority + 1 standard + 20 custom) should appear.
	if len(rows) != 25 {
		t.Fatalf("expected every filterable field to show, got %d (want 25)", len(rows))
	}
	// Priority audit comes first.
	if rows[0].Field != "Name" {
		t.Fatalf("Name should be first, got %s", rows[0].Field)
	}
	// Industry is a standard field — must be present too.
	industryFound := false
	for _, r := range rows {
		if r.Field == "Industry" {
			industryFound = true
			break
		}
	}
	if !industryFound {
		t.Fatal("standard non-custom field should not be excluded from the catalogue")
	}
}

func TestFieldsFromDescribeDeduplicates(t *testing.T) {
	// Owner appears as a relationship + OwnerId as a lookup. The
	// catalogue keys on Field name so the relationship variant isn't
	// added twice (only one of them shows up since we iterate Fields
	// once and skip duplicates).
	desc := sf.SObjectDescribe{
		Fields: []sf.Field{
			{Name: "OwnerId", Type: "reference", Filterable: true},
			{Name: "OwnerId", Type: "reference", Filterable: true}, // dup
		},
	}
	rows := fieldsFromDescribe(desc)
	count := 0
	for _, r := range rows {
		if r.Field == "OwnerId" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("OwnerId should appear once in the catalogue, got %d", count)
	}
}

// ---- helpers ---------------------------------------------------------

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func customName(i int) string {
	// kebab-stable Custom_<n>__c
	digit := byte('0' + i%10)
	tens := byte('0' + (i/10)%10)
	return "Custom_" + string([]byte{tens, digit}) + "__c"
}
