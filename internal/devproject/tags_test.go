package devproject

import (
	"errors"
	"path/filepath"
	"testing"
)

// openTestStore opens a fresh store under a t.TempDir() so tests
// never collide with the user's real ~/.sf-deck data. The package's
// real Open() goes via $HOME — bypass it by opening a sql.DB
// directly with the schema applied.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := openAt(path)
	if err != nil {
		t.Fatalf("openAt: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestTags_CreateAndList(t *testing.T) {
	s := openTestStore(t)
	a, err := s.CreateTag("cleanup-q2", "blue", "")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if a.ID == 0 {
		t.Fatalf("CreateTag returned id=0")
	}
	if a.Name != "cleanup-q2" || a.Color != "blue" {
		t.Errorf("CreateTag fields wrong: %+v", a)
	}
	b, err := s.CreateTag("tech-debt", "red", "🔧")
	if err != nil {
		t.Fatalf("CreateTag b: %v", err)
	}
	if b.Icon != "🔧" {
		t.Errorf("icon round-trip lost: got %q", b.Icon)
	}

	tags, err := s.ListTags()
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("ListTags: got %d, want 2", len(tags))
	}
	// Sorted by name (case-insensitive); "cleanup-q2" < "tech-debt"
	if tags[0].Name != "cleanup-q2" || tags[1].Name != "tech-debt" {
		t.Errorf("ListTags wrong order: %v", tags)
	}
}

func TestTags_CreateDuplicateName(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateTag("dup", "", ""); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := s.CreateTag("DUP", "", "") // case-insensitive collision
	if !errors.Is(err, ErrTagExists) {
		t.Errorf("expected ErrTagExists, got %v", err)
	}
}

func TestTags_UpdateAndRename(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTag("old-name", "blue", "")
	if err := s.UpdateTag(a.ID, "new-name", "purple", "✨"); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
	tags, _ := s.ListTags()
	if tags[0].Name != "new-name" || tags[0].Color != "purple" || tags[0].Icon != "✨" {
		t.Errorf("Update didn't stick: %+v", tags[0])
	}

	// Update to a name that collides with another tag → ErrTagExists.
	b, _ := s.CreateTag("other", "", "")
	if err := s.UpdateTag(a.ID, "other", "", ""); !errors.Is(err, ErrTagExists) {
		t.Errorf("expected ErrTagExists on rename collision, got %v", err)
	}
	_ = b
}

func TestTags_UpdateMissingTag(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpdateTag(99999, "ghost", "", ""); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound, got %v", err)
	}
}

func TestTags_ApplyAndQuery(t *testing.T) {
	s := openTestStore(t)
	tag, _ := s.CreateTag("merge-fragile", "red", "")

	// Apply same tag to fields across two orgs — bindings are
	// per-(item, org) so both should land.
	if err := s.ApplyTag(tag.ID, KindField, "Account.Phone", "alice@dev"); err != nil {
		t.Fatalf("Apply 1: %v", err)
	}
	if err := s.ApplyTag(tag.ID, KindField, "Account.Phone", "alice@uat"); err != nil {
		t.Fatalf("Apply 2: %v", err)
	}
	// Re-applying is idempotent.
	if err := s.ApplyTag(tag.ID, KindField, "Account.Phone", "alice@dev"); err != nil {
		t.Fatalf("Apply idempotent: %v", err)
	}

	// TagsFor returns just this org's binding.
	tags, err := s.TagsFor(KindField, "Account.Phone", "alice@dev")
	if err != nil {
		t.Fatalf("TagsFor: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "merge-fragile" {
		t.Errorf("TagsFor: got %v, want [merge-fragile]", tags)
	}

	// ItemsWithTag scoped to one org returns 1; cross-org returns 2.
	bs, _ := s.ItemsWithTag(tag.ID, "alice@dev")
	if len(bs) != 1 {
		t.Errorf("ItemsWithTag scoped: got %d, want 1", len(bs))
	}
	bs, _ = s.ItemsWithTag(tag.ID, "")
	if len(bs) != 2 {
		t.Errorf("ItemsWithTag cross-org: got %d, want 2", len(bs))
	}
}

func TestTags_RemoveAndDeleteCascade(t *testing.T) {
	s := openTestStore(t)
	tag, _ := s.CreateTag("temporary", "", "")
	_ = s.ApplyTag(tag.ID, KindFlow, "0F1abc", "alice@dev")
	_ = s.ApplyTag(tag.ID, KindFlow, "0F1xyz", "alice@dev")

	// RemoveTag drops one binding; the other survives.
	if err := s.RemoveTag(tag.ID, KindFlow, "0F1abc", "alice@dev"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	bs, _ := s.ItemsWithTag(tag.ID, "")
	if len(bs) != 1 {
		t.Errorf("after RemoveTag: got %d, want 1", len(bs))
	}

	// DeleteTag cascades — the second binding should also be gone.
	if err := s.DeleteTag(tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	bs, _ = s.ItemsWithTag(tag.ID, "")
	if len(bs) != 0 {
		t.Errorf("after DeleteTag cascade: got %d, want 0", len(bs))
	}
	tags, _ := s.ListTags()
	if len(tags) != 0 {
		t.Errorf("DeleteTag left tag in list: %v", tags)
	}
}

func TestTags_SetTagsFor(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTag("a", "", "")
	b, _ := s.CreateTag("b", "", "")
	c, _ := s.CreateTag("c", "", "")

	// Initial set: {a, b}.
	if err := s.SetTagsFor(KindField, "Account.Phone", "alice@dev",
		[]int64{a.ID, b.ID}); err != nil {
		t.Fatalf("SetTagsFor 1: %v", err)
	}
	tags, _ := s.TagsFor(KindField, "Account.Phone", "alice@dev")
	if len(tags) != 2 {
		t.Errorf("after first set: got %d, want 2", len(tags))
	}

	// Replace with {b, c} — a should be removed, c added.
	if err := s.SetTagsFor(KindField, "Account.Phone", "alice@dev",
		[]int64{b.ID, c.ID}); err != nil {
		t.Fatalf("SetTagsFor 2: %v", err)
	}
	tags, _ = s.TagsFor(KindField, "Account.Phone", "alice@dev")
	if len(tags) != 2 {
		t.Fatalf("after second set: got %d, want 2", len(tags))
	}
	names := []string{tags[0].Name, tags[1].Name}
	if names[0] != "b" || names[1] != "c" {
		t.Errorf("after replace: got %v, want [b c]", names)
	}

	// Empty set clears all bindings on the item.
	if err := s.SetTagsFor(KindField, "Account.Phone", "alice@dev", nil); err != nil {
		t.Fatalf("SetTagsFor empty: %v", err)
	}
	tags, _ = s.TagsFor(KindField, "Account.Phone", "alice@dev")
	if len(tags) != 0 {
		t.Errorf("after clear: got %v, want []", tags)
	}
}

func TestTags_FindByName(t *testing.T) {
	s := openTestStore(t)
	created, _ := s.CreateTag("findme", "", "")

	got, ok, err := s.FindTagByName("FINDME") // case-insensitive
	if err != nil {
		t.Fatalf("FindTagByName: %v", err)
	}
	if !ok || got.ID != created.ID {
		t.Errorf("FindTagByName: got (%v, %v), want id=%d", got, ok, created.ID)
	}
	_, ok, _ = s.FindTagByName("ghost")
	if ok {
		t.Errorf("FindTagByName ghost: expected ok=false")
	}
}

func TestTags_ListWithUsage(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTag("popular", "", "")
	b, _ := s.CreateTag("unused", "", "")
	_ = s.ApplyTag(a.ID, KindFlow, "0F1", "alice@dev")
	_ = s.ApplyTag(a.ID, KindFlow, "0F2", "alice@dev")
	_ = s.ApplyTag(a.ID, KindField, "Account.Phone", "alice@dev")

	usage, err := s.ListTagsWithUsage()
	if err != nil {
		t.Fatalf("ListTagsWithUsage: %v", err)
	}
	if len(usage) != 2 {
		t.Fatalf("got %d, want 2", len(usage))
	}
	byName := map[string]int{}
	for _, u := range usage {
		byName[u.Name] = u.Count
	}
	if byName["popular"] != 3 {
		t.Errorf("popular count: got %d, want 3", byName["popular"])
	}
	if byName["unused"] != 0 {
		t.Errorf("unused count: got %d, want 0", byName["unused"])
	}
	_ = b
}

func TestTags_BulkLookup(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTag("a", "", "")
	b, _ := s.CreateTag("b", "", "")
	_ = s.ApplyTag(a.ID, KindField, "Account.Phone", "alice@dev")
	_ = s.ApplyTag(b.ID, KindField, "Account.Phone", "alice@dev")
	_ = s.ApplyTag(a.ID, KindField, "Account.Email", "alice@dev")
	// Untagged item: Account.Fax (no bindings).

	keys := []TagLookupKey{
		{Kind: KindField, Ref: "Account.Phone"},
		{Kind: KindField, Ref: "Account.Email"},
		{Kind: KindField, Ref: "Account.Fax"},
	}
	out, err := s.TagsForItems("alice@dev", keys)
	if err != nil {
		t.Fatalf("TagsForItems: %v", err)
	}
	if len(out["field:Account.Phone"]) != 2 {
		t.Errorf("Phone: got %d, want 2", len(out["field:Account.Phone"]))
	}
	if len(out["field:Account.Email"]) != 1 {
		t.Errorf("Email: got %d, want 1", len(out["field:Account.Email"]))
	}
	if _, ok := out["field:Account.Fax"]; ok {
		t.Errorf("Fax should not appear in lookup map (untagged)")
	}
}

func TestTags_BulkApplyRemoveTags(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTag("bulk-a", "", "")
	b, _ := s.CreateTag("bulk-b", "", "")
	refs := []string{"Account", "Contact", "Lead"}

	// Lead already carries b — a bulk add of a must leave it intact.
	if err := s.ApplyTag(b.ID, KindSObject, "Lead", "alice@dev"); err != nil {
		t.Fatal(err)
	}
	if err := s.BulkApplyRemoveTags(KindSObject, "alice@dev", refs, []int64{a.ID}, nil); err != nil {
		t.Fatal(err)
	}
	keys := []TagLookupKey{
		{Kind: KindSObject, Ref: "Account"},
		{Kind: KindSObject, Ref: "Contact"},
		{Kind: KindSObject, Ref: "Lead"},
	}
	got, err := s.TagsForItems("alice@dev", keys)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		tags := got[string(KindSObject)+":"+ref]
		if len(tags) == 0 || tags[0].ID != a.ID && tags[len(tags)-1].ID != a.ID {
			t.Fatalf("%s: bulk-applied tag missing: %+v", ref, tags)
		}
	}
	if tags := got["sobject:Lead"]; len(tags) != 2 {
		t.Fatalf("Lead should keep its pre-existing tag plus the bulk one, got %+v", tags)
	}

	// Bulk remove a from all three; Lead keeps b.
	if err := s.BulkApplyRemoveTags(KindSObject, "alice@dev", refs, nil, []int64{a.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.TagsForItems("alice@dev", keys)
	if tags := got["sobject:Account"]; len(tags) != 0 {
		t.Fatalf("Account should have no tags after bulk remove, got %+v", tags)
	}
	if tags := got["sobject:Lead"]; len(tags) != 1 || tags[0].ID != b.ID {
		t.Fatalf("Lead should keep only its own tag, got %+v", tags)
	}

	// Re-applying an existing binding is a no-op, not an error.
	if err := s.BulkApplyRemoveTags(KindSObject, "alice@dev", []string{"Lead"}, []int64{b.ID}, nil); err != nil {
		t.Fatalf("idempotent re-apply errored: %v", err)
	}
}
