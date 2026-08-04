package soqlauto

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestSuggestFieldsBasic(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		sf.Field{Name: "Name", Label: "Account Name", Type: "string", Filterable: true, Sortable: true, Groupable: true},
		sf.Field{Name: "Industry", Label: "Industry", Type: "picklist", Filterable: true, Sortable: true, Groupable: true,
			PicklistValues: []sf.PicklistValue{{Label: "Tech", Value: "Tech", Active: true}, {Label: "Health", Value: "Health", Active: true}}},
		sf.Field{Name: "OwnerId", Label: "Owner", Type: "reference", RelationshipName: "Owner", ReferenceTo: []string{"User"}, Filterable: true},
	))
	store.add(sd("User", sf.Field{Name: "Email", Type: "email", Filterable: true}))

	snap := store.snapshot("SELECT Na FROM Account", len("SELECT Na"))
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	if len(got) == 0 || got[0].Display != "Name" {
		t.Fatalf("expected Name first, got %+v", got)
	}
}

func TestSuggestRelationshipDotEntries(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		sf.Field{Name: "OwnerId", Type: "reference", RelationshipName: "Owner", ReferenceTo: []string{"User"}, Filterable: true},
	))
	store.add(sd("User", sf.Field{Name: "Email", Type: "email"}))

	snap := store.snapshot("SELECT Own FROM Account", len("SELECT Own"))
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	foundRel := false
	for _, s := range got {
		if s.Kind == KindRelationship && s.Value == "Owner." {
			foundRel = true
		}
	}
	if !foundRel {
		t.Fatalf("expected 'Owner.' relationship suggestion, got %+v", suggestionValues(got))
	}
}

func TestSuggestHopsResolveTargetFields(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		sf.Field{Name: "OwnerId", Type: "reference", RelationshipName: "Owner", ReferenceTo: []string{"User"}},
	))
	store.add(sd("User", sf.Field{Name: "Email", Type: "email"}, sf.Field{Name: "ManagerId", Type: "reference", RelationshipName: "Manager", ReferenceTo: []string{"User"}}))

	q := "SELECT Owner. FROM Account"
	cur := strings.Index(q, ".") + 1
	snap := store.snapshot(q, cur)
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	names := suggestionValues(got)
	if !contains(names, "Email") || !contains(names, "Manager.") {
		t.Fatalf("expected Email + Manager., got %v", names)
	}
}

func TestSuggestValuesPicklist(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		sf.Field{Name: "Industry", Type: "picklist", Filterable: true,
			PicklistValues: []sf.PicklistValue{{Label: "Tech", Value: "Tech", Active: true}, {Label: "Healthcare", Value: "Healthcare", Active: true}}},
	))

	q := "SELECT Id FROM Account WHERE Industry = '"
	snap := store.snapshot(q, len(q))
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	names := suggestionValues(got)
	if !contains(names, "'Tech'") || !contains(names, "'Healthcare'") {
		t.Fatalf("expected picklist values, got %v", names)
	}
}

func TestSuggestValuesDateLiterals(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		sf.Field{Name: "CreatedDate", Type: "datetime", Filterable: true, Nillable: true},
	))

	q := "SELECT Id FROM Account WHERE CreatedDate = "
	snap := store.snapshot(q, len(q))
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	names := suggestionValues(got)
	if !contains(names, "TODAY") || !contains(names, "LAST_N_DAYS:1") || !contains(names, "null") {
		t.Fatalf("expected date literals + null, got %v", names)
	}
}

func TestSuggestSObjectsAfterFrom(t *testing.T) {
	store := newMockDescribeStore()
	snap := store.snapshot("SELECT Id FROM Acc", len("SELECT Id FROM Acc"))
	snap.SObjects = []string{"Account", "AccountContactRelation", "Asset", "Contact"}
	cls := Classify(snap)
	got := Suggest(snap, &cls)
	names := suggestionValues(got)
	if !contains(names, "Account") || !contains(names, "AccountContactRelation") {
		t.Fatalf("expected Account-prefixed objects, got %v", names)
	}
	if contains(names, "Contact") {
		t.Fatalf("Contact should rank 0 for token 'Acc', got %v", names)
	}
}

func TestRankBuckets(t *testing.T) {
	if Rank("Name", "Name") != 4 {
		t.Error("exact match should be 4")
	}
	if Rank("Account", "Acc") != 3 {
		t.Error("startsWith should be 3")
	}
	if Rank("Custom_Active__c", "Active") != 2 {
		t.Error("__c-suffix contains-match should be 2")
	}
	if Rank("LastModifiedDate", "Modified") != 1 {
		t.Error("plain substring should be 1")
	}
	if Rank("Foo", "Bar") != 0 {
		t.Error("no match should be 0")
	}
}

// helpers -------------------------------------------------------------

func suggestionValues(ss []Suggestion) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Value
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
