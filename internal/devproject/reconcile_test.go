package devproject

import (
	"testing"
	"time"
)

func seedItem(t *testing.T, s *Store, proj, org string, kind ItemKind, ref, name string) {
	t.Helper()
	if _, err := s.AddItem(Item{
		DevProjectID: proj, OrgUser: org, Kind: kind, Ref: ref, Name: name,
		AddedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed %s/%s: %v", kind, ref, err)
	}
}

func refsIn(t *testing.T, s *Store, proj string) map[string]string {
	t.Helper()
	items, err := s.ListItems(proj, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, it := range items {
		out[string(it.Kind)+"|"+it.Ref] = it.Name
	}
	return out
}

// TestApplyReconcile_RewriteToCanonical: a flow stored under its
// DeveloperName is re-keyed to its DefinitionId, name filled.
func TestApplyReconcile_RewriteToCanonical(t *testing.T) {
	s := openTestStore(t)
	s.CreateDevProject(DevProject{ID: "p", Name: "P", CreatedAt: time.Now(), TouchedAt: time.Now()})
	seedItem(t, s, "p", "alice", KindFlow, "Summer_School_Reinstate", "")

	merged, removed := mustReconcile(t, s,
		nil,
		[]ItemRewrite{{DevProjectID: "p", OrgUser: "alice", Kind: KindFlow,
			FromRef: "Summer_School_Reinstate", ToRef: "300XYZ", Name: "Summer_School_Reinstate"}},
	)
	if merged != 1 || removed != 0 {
		t.Fatalf("merged=%d removed=%d, want 1/0", merged, removed)
	}
	refs := refsIn(t, s, "p")
	if _, ok := refs["flow|Summer_School_Reinstate"]; ok {
		t.Error("DeveloperName-ref row should be gone")
	}
	if name := refs["flow|300XYZ"]; name != "Summer_School_Reinstate" {
		t.Errorf("canonical row missing/misnamed: %q", name)
	}
}

// TestApplyReconcile_MergeDuplicate: when BOTH the DeveloperName-ref and
// the DefinitionId-ref rows exist, the rewrite drops the source (the
// canonical target already there) — one row remains.
func TestApplyReconcile_MergeDuplicate(t *testing.T) {
	s := openTestStore(t)
	s.CreateDevProject(DevProject{ID: "p", Name: "P", CreatedAt: time.Now(), TouchedAt: time.Now()})
	seedItem(t, s, "p", "alice", KindFlow, "Summer_School_Reinstate", "")       // import row
	seedItem(t, s, "p", "alice", KindFlow, "300XYZ", "Summer_School_Reinstate") // collect row

	merged, removed := mustReconcile(t, s, nil,
		[]ItemRewrite{{DevProjectID: "p", OrgUser: "alice", Kind: KindFlow,
			FromRef: "Summer_School_Reinstate", ToRef: "300XYZ", Name: "Summer_School_Reinstate"}},
	)
	if merged != 1 || removed != 0 {
		t.Fatalf("merged=%d removed=%d, want 1/0", merged, removed)
	}
	items, _ := s.ListItems("p", "")
	if len(items) != 1 || items[0].Ref != "300XYZ" {
		t.Fatalf("expected one canonical row, got %+v", items)
	}
}

// TestApplyReconcile_Delete removes a confirmed-missing item.
func TestApplyReconcile_Delete(t *testing.T) {
	s := openTestStore(t)
	s.CreateDevProject(DevProject{ID: "p", Name: "P", CreatedAt: time.Now(), TouchedAt: time.Now()})
	seedItem(t, s, "p", "alice", KindApexClass, "01pGONE", "DeletedClass")
	seedItem(t, s, "p", "alice", KindApexClass, "01pKEEP", "LiveClass")

	_, removed := mustReconcile(t, s,
		[]ItemDelete{{DevProjectID: "p", OrgUser: "alice", Kind: KindApexClass, Ref: "01pGONE"}},
		nil,
	)
	if removed != 1 {
		t.Fatalf("removed=%d, want 1", removed)
	}
	refs := refsIn(t, s, "p")
	if _, ok := refs["apex_class|01pGONE"]; ok {
		t.Error("missing class should be deleted")
	}
	if _, ok := refs["apex_class|01pKEEP"]; !ok {
		t.Error("live class must survive")
	}
}

// TestApplyReconcile_NoopNoTouch: an empty plan doesn't bump the store
// generation (so it's cheap to call on every touch).
func TestApplyReconcile_NoopNoTouch(t *testing.T) {
	s := openTestStore(t)
	before := s.Generation()
	r, m, err := s.ApplyReconcile(nil, nil)
	if err != nil || r != 0 || m != 0 {
		t.Fatalf("noop reconcile: r=%d m=%d err=%v", r, m, err)
	}
	if s.Generation() != before {
		t.Error("empty reconcile must not touch() the store")
	}
}

func mustReconcile(t *testing.T, s *Store, dels []ItemDelete, rws []ItemRewrite) (merged, removed int) {
	t.Helper()
	removed, merged, err := s.ApplyReconcile(dels, rws)
	if err != nil {
		t.Fatalf("ApplyReconcile: %v", err)
	}
	return merged, removed
}
