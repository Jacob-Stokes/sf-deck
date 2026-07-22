package devproject

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Pure functions in collect.go. No store, no FS, no org contact.

// ----- LabelForItem ----------------------------------------------

func TestLabelForItem_PrefersName(t *testing.T) {
	got := LabelForItem(Item{Name: "Account.Phone", Kind: KindField, Ref: "Account.Phone"})
	if got != "Account.Phone" {
		t.Errorf("got %q", got)
	}
}

func TestLabelForItem_FallsBackToTypeAndRef(t *testing.T) {
	got := LabelForItem(Item{Type: "Account", Kind: KindField, Ref: "Phone"})
	if got != "Account Phone" {
		t.Errorf("got %q, want 'Account Phone'", got)
	}
}

func TestLabelForItem_FallsBackToKindAndRef(t *testing.T) {
	got := LabelForItem(Item{Kind: KindFlow, Ref: "MyFlow"})
	if got != "flow MyFlow" {
		t.Errorf("got %q, want 'flow MyFlow'", got)
	}
}

// ----- SupportedKinds --------------------------------------------

func TestSupportedKinds_IncludesAllExpected(t *testing.T) {
	kinds := SupportedKinds()
	seen := map[ItemKind]bool{}
	for _, k := range kinds {
		seen[k] = true
	}
	required := []ItemKind{
		KindSObject, KindField, KindFlow, KindFlowVersion,
		KindRecord, KindApexClass, KindReport,
		KindPermissionSet, KindPermissionSetGroup, KindProfile,
		KindValidationRule, KindRecordType, KindApexTrigger,
		KindLWC, KindAura, KindQueue, KindPublicGroup,
	}
	for _, k := range required {
		if !seen[k] {
			t.Errorf("SupportedKinds missing %v", k)
		}
	}
}

// ----- sObjectFromRecord -----------------------------------------

func TestSObjectFromRecord_ExtractsType(t *testing.T) {
	rec := map[string]any{
		"attributes": map[string]any{"type": "Account"},
		"Id":         "001x",
	}
	got, ok := sObjectFromRecord(rec)
	if !ok || got != "Account" {
		t.Errorf("got (%q, %v), want (Account, true)", got, ok)
	}
}

func TestSObjectFromRecord_MissingAttributes(t *testing.T) {
	if _, ok := sObjectFromRecord(map[string]any{"Id": "001x"}); ok {
		t.Error("expected ok=false for missing attributes")
	}
}

func TestSObjectFromRecord_EmptyType(t *testing.T) {
	rec := map[string]any{
		"attributes": map[string]any{"type": ""},
	}
	if _, ok := sObjectFromRecord(rec); ok {
		t.Error("expected ok=false for empty type")
	}
}

// ----- recordDisplayName -----------------------------------------

func TestRecordDisplayName_PrefersName(t *testing.T) {
	rec := map[string]any{"Name": "Acme Co", "Id": "001x"}
	if got := recordDisplayName(rec); got != "Acme Co" {
		t.Errorf("got %q", got)
	}
}

func TestRecordDisplayName_TriesAllFallbacks(t *testing.T) {
	cases := []struct {
		rec  map[string]any
		want string
	}{
		{map[string]any{"Subject": "Question", "Id": "500x"}, "Question"},
		{map[string]any{"CaseNumber": "00001", "Id": "500x"}, "00001"},
		{map[string]any{"DeveloperName": "My_Flow", "Id": "300x"}, "My_Flow"},
		{map[string]any{"Title": "Doc title", "Id": "06Ax"}, "Doc title"},
	}
	for _, c := range cases {
		if got := recordDisplayName(c.rec); got != c.want {
			t.Errorf("rec %v: got %q, want %q", c.rec, got, c.want)
		}
	}
}

func TestRecordDisplayName_FallsBackToID(t *testing.T) {
	if got := recordDisplayName(map[string]any{"Id": "001x"}); got != "001x" {
		t.Errorf("got %q", got)
	}
}

func TestRecordDisplayName_EmptyRecord(t *testing.T) {
	if got := recordDisplayName(map[string]any{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ----- displayLabelForSObject -----------------------------------

func TestDisplayLabelForSObject_LabelMatchesName(t *testing.T) {
	got := displayLabelForSObject(sf.SObject{Name: "Account", Label: "Account"})
	if got != "Account" {
		t.Errorf("got %q, want Account", got)
	}
}

func TestDisplayLabelForSObject_DistinctLabel(t *testing.T) {
	got := displayLabelForSObject(sf.SObject{Name: "Acc__c", Label: "Account Extra"})
	if !strings.Contains(got, "Acc__c") || !strings.Contains(got, "Account Extra") {
		t.Errorf("got %q; expected both name and label", got)
	}
}

func TestDisplayLabelForSObject_EmptyLabel(t *testing.T) {
	got := displayLabelForSObject(sf.SObject{Name: "Account"})
	if got != "Account" {
		t.Errorf("got %q, want Account", got)
	}
}

// ----- FromOpenable: a couple of representative kinds -------------

func TestFromOpenable_NilReturnsFalse(t *testing.T) {
	_, _, _, _, ok := FromOpenable(nil)
	if ok {
		t.Error("nil should return ok=false")
	}
}

func TestFromOpenable_SObject(t *testing.T) {
	kind, ref, typ, name, ok := FromOpenable(sf.SObject{Name: "Account", Label: "Account"})
	if !ok {
		t.Fatal("not ok")
	}
	if kind != KindSObject {
		t.Errorf("Kind = %v", kind)
	}
	if ref != "Account" {
		t.Errorf("Ref = %q", ref)
	}
	if typ != "" {
		t.Errorf("Type = %q, want empty", typ)
	}
	if name == "" {
		t.Error("Name should be set")
	}
}

func TestFromOpenable_FieldRef(t *testing.T) {
	kind, ref, typ, name, ok := FromOpenable(sf.FieldRef{
		SObjectName: "Account",
		Field:       sf.Field{Name: "Phone", Label: "Phone Number"},
	})
	if !ok {
		t.Fatal("not ok")
	}
	if kind != KindField {
		t.Errorf("Kind = %v", kind)
	}
	if ref != "Account.Phone" {
		t.Errorf("Ref = %q", ref)
	}
	if typ != "Account" {
		t.Errorf("Type = %q", typ)
	}
	if name != "Phone Number" {
		t.Errorf("Name = %q", name)
	}
}

func TestFromOpenable_FieldRefMissingNamesReject(t *testing.T) {
	_, _, _, _, ok := FromOpenable(sf.FieldRef{SObjectName: "Account"})
	if ok {
		t.Error("missing field name should reject")
	}
	_, _, _, _, ok = FromOpenable(sf.FieldRef{Field: sf.Field{Name: "Phone"}})
	if ok {
		t.Error("missing sobject should reject")
	}
}

func TestFromOpenable_FlowWithMissingDefIDRejects(t *testing.T) {
	_, _, _, _, ok := FromOpenable(sf.Flow{DeveloperName: "MyFlow"})
	if ok {
		t.Error("missing DefinitionID should reject")
	}
}

func TestFromOpenable_RecordRef(t *testing.T) {
	rr := sf.RecordRef{
		Record: map[string]any{
			"attributes": map[string]any{"type": "Account"},
			"Id":         "001x",
			"Name":       "Acme",
		},
	}
	kind, ref, typ, name, ok := FromOpenable(rr)
	if !ok {
		t.Fatal("not ok")
	}
	if kind != KindRecord {
		t.Errorf("Kind = %v", kind)
	}
	// Canonical "<sObject>:<Id>" — the shape every tag/project lookup
	// keys by. The old bare-Id ref meant collected records never
	// matched a PROJECTS-gutter lookup and showed no pill.
	if ref != "Account:001x" {
		t.Errorf("Ref = %q, want Account:001x", ref)
	}
	if typ != "Account" {
		t.Errorf("Type = %q", typ)
	}
	if name != "Acme" {
		t.Errorf("Name = %q", name)
	}
}

func TestFromOpenable_RecordRefMissingIDRejects(t *testing.T) {
	rr := sf.RecordRef{Record: map[string]any{"Name": "X"}}
	if _, _, _, _, ok := FromOpenable(rr); ok {
		t.Error("missing Id should reject")
	}
}
