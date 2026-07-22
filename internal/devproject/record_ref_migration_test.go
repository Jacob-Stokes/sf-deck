package devproject

import (
	"path/filepath"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Legacy KindRecord items stored the bare record Id (sObject only in
// Type), while every tag/project lookup keys by "<sObject>:<Id>" — so
// collected records never showed a PROJECTS pill. The migration
// rewrites legacy refs on open; FromOpenable now emits the canonical
// shape so new collects match immediately.
func TestMigrateRecordItemRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devprojects.db")
	s, err := OpenPath(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.CreateDevProject(DevProject{ID: "p1", Name: "P"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Simulate a pre-fix item: bare Id ref, sObject in Type.
	if _, err := s.AddItem(Item{
		DevProjectID: "p1", OrgUser: "u@x", Kind: KindRecord,
		Ref: "m16UE000002vC4vYAE", Type: "Person_Account_Merge_Object__mdt",
		Name: "Address - Parent Account",
	}); err != nil {
		t.Fatalf("add legacy: %v", err)
	}
	// And its canonical twin (as if re-collected post-fix) to prove the
	// migration is PK-collision safe.
	if _, err := s.AddItem(Item{
		DevProjectID: "p1", OrgUser: "u@x", Kind: KindRecord,
		Ref: "Person_Account_Merge_Object__mdt:m16UE000002vC4wYAE", Type: "Person_Account_Merge_Object__mdt",
	}); err != nil {
		t.Fatalf("add canonical: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen — openAt runs the migration.
	s2, err := OpenPath(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	items, err := s2.ListItems("p1", "u@x")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	for _, it := range items {
		sobj, id := "", it.Ref
		if i := indexByte(it.Ref, ':'); i >= 0 {
			sobj, id = it.Ref[:i], it.Ref[i+1:]
		}
		if sobj != "Person_Account_Merge_Object__mdt" || id == "" {
			t.Errorf("ref not migrated to canonical shape: %q", it.Ref)
		}
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// FromOpenable must emit the canonical "<sObject>:<Id>" ref for record
// rows (it used to return the bare Id — the source of the orphan refs)
// and refuse rows with no sObject rather than storing an unmatchable ref.
func TestFromOpenableRecordRefShape(t *testing.T) {
	kind, ref, typ, _, ok := FromOpenable(sf.RecordRef{Record: map[string]any{
		"Id":         "001XX000003DHPh",
		"Name":       "Acme",
		"attributes": map[string]any{"type": "Account"},
	}})
	if !ok || kind != KindRecord {
		t.Fatalf("expected record collect, got ok=%v kind=%v", ok, kind)
	}
	if ref != "Account:001XX000003DHPh" {
		t.Errorf("ref = %q, want canonical Account:001XX000003DHPh", ref)
	}
	if typ != "Account" {
		t.Errorf("typ = %q, want Account", typ)
	}

	// No attributes.type → refuse (can't build a canonical ref).
	if _, _, _, _, ok := FromOpenable(sf.RecordRef{Record: map[string]any{
		"Id": "001XX000003DHPh",
	}}); ok {
		t.Error("record with no sObject must not be collectable")
	}
}
