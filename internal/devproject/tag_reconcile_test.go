package devproject

import "testing"

// seed a tag + bind it to an item.
func seedTagBinding(t *testing.T, s *Store, tagName string, kind ItemKind, ref, org string) int64 {
	t.Helper()
	tag, ok, err := s.FindTagByName(tagName)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		tag, err = s.CreateTag(tagName, "blue", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ApplyTag(tag.ID, kind, ref, org); err != nil {
		t.Fatalf("apply %s->%s: %v", tagName, ref, err)
	}
	return tag.ID
}

func boundRefs(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	items, err := s.ListBoundItems()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, b := range items {
		out[string(b.Kind)+"|"+b.Ref+"|"+b.OrgUser] = true
	}
	return out
}

func TestListBoundItems(t *testing.T) {
	s := openTestStore(t)
	seedTagBinding(t, s, "sprint", KindFlow, "300ABC", "alice")
	seedTagBinding(t, s, "review", KindFlow, "300ABC", "alice") // same item, 2nd tag → still one distinct item
	seedTagBinding(t, s, "sprint", KindApexClass, "01pX", "alice")

	got := boundRefs(t, s)
	if len(got) != 2 {
		t.Fatalf("distinct bound items = %d, want 2 (%v)", len(got), got)
	}
	if !got["flow|300ABC|alice"] || !got["apex_class|01pX|alice"] {
		t.Errorf("unexpected bound set: %v", got)
	}
}

// TestReconcileTagBindings_DeleteStale removes all tags on a deleted item.
func TestReconcileTagBindings_DeleteStale(t *testing.T) {
	s := openTestStore(t)
	seedTagBinding(t, s, "a", KindFlow, "300GONE", "alice")
	seedTagBinding(t, s, "b", KindFlow, "300GONE", "alice") // 2 tags on the stale item
	seedTagBinding(t, s, "a", KindFlow, "300LIVE", "alice")

	removed, merged, err := s.ReconcileTagBindings(
		[]TagBindingDelete{{Kind: KindFlow, Ref: "300GONE", OrgUser: "alice"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 || merged != 0 {
		t.Fatalf("removed=%d merged=%d, want 2/0 (both tags on the stale item)", removed, merged)
	}
	refs := boundRefs(t, s)
	if refs["flow|300GONE|alice"] {
		t.Error("stale item's bindings should be gone")
	}
	if !refs["flow|300LIVE|alice"] {
		t.Error("live item's binding must survive")
	}
}

// TestReconcileTagBindings_RewriteMerges: re-key a DeveloperName-ref
// binding to the DefinitionId, merging tag sets (INSERT OR IGNORE).
func TestReconcileTagBindings_RewriteMerges(t *testing.T) {
	s := openTestStore(t)
	// item tagged under DeveloperName with tag "x"
	seedTagBinding(t, s, "x", KindFlow, "Summer_School_Reinstate", "alice")
	// same logical flow already tagged under DefinitionId with tags "x" (dup) and "y"
	seedTagBinding(t, s, "x", KindFlow, "300CANON", "alice")
	seedTagBinding(t, s, "y", KindFlow, "300CANON", "alice")

	_, merged, err := s.ReconcileTagBindings(nil,
		[]TagBindingRewrite{{Kind: KindFlow, OrgUser: "alice",
			FromRef: "Summer_School_Reinstate", ToRef: "300CANON"}})
	if err != nil {
		t.Fatal(err)
	}
	if merged != 1 {
		t.Fatalf("merged=%d, want 1 (the source DeveloperName row)", merged)
	}
	// DeveloperName ref gone; canonical keeps its two distinct tags (x, y)
	// — the duplicate "x" from the source was ignored on insert.
	refs := boundRefs(t, s)
	if refs["flow|Summer_School_Reinstate|alice"] {
		t.Error("DeveloperName-ref binding should be gone")
	}
	tags, err := s.TagsFor(KindFlow, "300CANON", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Errorf("canonical item should have exactly 2 tags (x,y), got %d", len(tags))
	}
}

func TestReconcileTagBindings_Noop(t *testing.T) {
	s := openTestStore(t)
	before := s.Generation()
	r, m, err := s.ReconcileTagBindings(nil, nil)
	if err != nil || r != 0 || m != 0 {
		t.Fatalf("noop: r=%d m=%d err=%v", r, m, err)
	}
	if s.Generation() != before {
		t.Error("empty tag reconcile must not touch() the store")
	}
}
