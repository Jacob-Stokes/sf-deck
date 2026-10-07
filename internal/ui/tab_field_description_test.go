package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// The "description" row on the field-detail page: custom fields fetch it
// lazily via Tooling (loading → value/dash); standard fields never have
// an editable Setup description so they always read "—".
func TestFieldDescriptionDisplay(t *testing.T) {
	custom := sf.Field{Custom: true}
	std := sf.Field{Custom: false}

	cases := []struct {
		name       string
		f          sf.Field
		desc       string
		descLoaded bool
		want       string
	}{
		{"standard field never fetches", std, "", false, "—"},
		{"custom in-flight shows loading", custom, "", false, "…  (loading)"},
		{"custom loaded with value", custom, "Pre-withdrawal stage of the application.", true, "Pre-withdrawal stage of the application."},
		{"custom loaded but empty shows dash", custom, "", true, "—"},
	}
	for _, c := range cases {
		if got := fieldDescriptionDisplay(c.f, c.desc, c.descLoaded); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The cache lookup distinguishes "fetched-empty" from "not fetched".
func TestFieldDescriptionCache(t *testing.T) {
	d := &orgData{}
	if _, loaded := fieldDescriptionCache(d, "Acc", "F__c"); loaded {
		t.Fatal("nil cache should report not-loaded")
	}
	d.FieldDescriptions = map[string]string{"Acc.F__c": ""}
	if v, loaded := fieldDescriptionCache(d, "Acc", "F__c"); !loaded || v != "" {
		t.Fatalf("fetched-empty should report loaded with empty value; got %q loaded=%v", v, loaded)
	}
	d.FieldDescriptions["Acc.G__c"] = "hello"
	if v, loaded := fieldDescriptionCache(d, "Acc", "G__c"); !loaded || v != "hello" {
		t.Fatalf("cached value not returned; got %q loaded=%v", v, loaded)
	}
}
