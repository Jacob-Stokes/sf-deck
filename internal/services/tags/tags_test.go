package tags

import (
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// newTestStore opens a devproject store under a temp HOME so tests
// never touch ~/.sf-deck. devproject.Open() resolves the path via
// os.UserHomeDir; t.Setenv shadows that for the lifetime of the
// subtest.
func newTestStore(t *testing.T) *devproject.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := devproject.Open()
	if err != nil {
		t.Fatalf("devproject.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestList_EmptyStoreReturnsEmptySlice(t *testing.T) {
	s := newTestStore(t)
	got, err := List(s, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestCreate_HappyPath(t *testing.T) {
	s := newTestStore(t)
	res, err := Create(s, CreateInput{Name: "renewals", Color: "blue", Icon: "💚"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false, want true")
	}
	if res.Tag.ID == 0 {
		t.Errorf("ID = 0, want non-zero")
	}
	if res.Tag.Name != "renewals" {
		t.Errorf("Name = %q", res.Tag.Name)
	}
}

func TestCreate_RejectsEmptyName(t *testing.T) {
	s := newTestStore(t)
	_, err := Create(s, CreateInput{Name: "  "})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreate_DuplicateRejected(t *testing.T) {
	s := newTestStore(t)
	if _, err := Create(s, CreateInput{Name: "x"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := Create(s, CreateInput{Name: "X"}) // case-insensitive collision
	var dup ErrAlreadyExists
	if !errors.As(err, &dup) {
		t.Fatalf("err type = %T, want ErrAlreadyExists: %v", err, err)
	}
}

func TestShow_ByIDAndByName(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "alpha"})
	b, _ := Create(s, CreateInput{Name: "beta"})

	got, err := Show(s, a.Tag.ID, "")
	if err != nil || got.Name != "alpha" {
		t.Errorf("Show by id: got=%+v err=%v", got, err)
	}
	got, err = Show(s, 0, "beta")
	if err != nil || got.ID != b.Tag.ID {
		t.Errorf("Show by name: got=%+v err=%v", got, err)
	}
}

func TestShow_MissingArguments(t *testing.T) {
	s := newTestStore(t)
	if _, err := Show(s, 0, ""); err == nil {
		t.Error("expected error for missing both")
	}
	if _, err := Show(s, 1, "x"); err == nil {
		t.Error("expected error for both set")
	}
}

func TestShow_NotFoundReturnsTyped(t *testing.T) {
	s := newTestStore(t)
	_, err := Show(s, 0, "missing")
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if nf.Name != "missing" {
		t.Errorf("Name = %q", nf.Name)
	}
}

func TestUpdate_PartialAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "old", Color: "blue"})

	// Rename — Changed=true.
	newName := "new"
	res, err := Update(s, created.Tag.ID, UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update rename: %v", err)
	}
	if !res.Changed || res.Tag.Name != "new" {
		t.Errorf("rename res = %+v", res)
	}

	// Re-set same name — Changed=false.
	same := "new"
	res, err = Update(s, created.Tag.ID, UpdateInput{Name: &same})
	if err != nil {
		t.Fatalf("Update idempotent: %v", err)
	}
	if res.Changed {
		t.Error("idempotent update reported Changed=true")
	}
}

func TestUpdate_CollisionRejected(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "alpha"})
	_, _ = Create(s, CreateInput{Name: "beta"})

	newName := "beta"
	_, err := Update(s, a.Tag.ID, UpdateInput{Name: &newName})
	var dup ErrAlreadyExists
	if !errors.As(err, &dup) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestUpdate_RejectsNoFields(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	if _, err := Update(s, a.Tag.ID, UpdateInput{}); err == nil {
		t.Error("expected error for no update fields")
	}
}

func TestDelete_RemovesAndReturnsSnapshot(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	res, err := Delete(s, a.Tag.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !res.Changed || res.Tag.Name != "x" {
		t.Errorf("Delete res = %+v", res)
	}
	// Verify gone.
	if _, err := Show(s, a.Tag.ID, ""); !errors.As(err, new(ErrNotFound)) {
		t.Errorf("post-delete Show err = %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := Delete(s, 99); !errors.As(err, new(ErrNotFound)) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestApply_NewBindingChangedTrue(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	res, err := Apply(s, a.Tag.ID, "record", "001abc", "dev@example.com")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
}

func TestApply_IdempotentSecondCall(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	_, _ = Apply(s, a.Tag.ID, "record", "001abc", "dev@x")
	res, err := Apply(s, a.Tag.ID, "record", "001abc", "dev@x")
	if err != nil {
		t.Fatalf("Apply idempotent: %v", err)
	}
	if res.Changed {
		t.Error("re-Apply reported Changed=true")
	}
}

func TestApply_BadKind(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	_, err := Apply(s, a.Tag.ID, "bogus", "ref", "")
	var bk ErrInvalidKind
	if !errors.As(err, &bk) {
		t.Fatalf("err = %v, want ErrInvalidKind", err)
	}
}

func TestRemove_ChangedReflectsPresence(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	// Remove from empty — Changed=false.
	res, err := Remove(s, a.Tag.ID, "record", "ref", "")
	if err != nil {
		t.Fatalf("Remove from empty: %v", err)
	}
	if res.Changed {
		t.Error("removing absent reported Changed=true")
	}

	// Apply then remove — Changed=true.
	_, _ = Apply(s, a.Tag.ID, "record", "ref", "")
	res, err = Remove(s, a.Tag.ID, "record", "ref", "")
	if err != nil {
		t.Fatalf("Remove after apply: %v", err)
	}
	if !res.Changed {
		t.Error("remove-present reported Changed=false")
	}
}

func TestSet_ReplacesAndDetectsNoOp(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "alpha"})
	b, _ := Create(s, CreateInput{Name: "beta"})

	// First set: empty → [a, b].
	res, err := Set(s, "record", "ref", "", []int64{a.Tag.ID, b.Tag.ID})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !res.Changed {
		t.Error("initial set Changed=false")
	}

	// Same set again — Changed=false.
	res, err = Set(s, "record", "ref", "", []int64{a.Tag.ID, b.Tag.ID})
	if err != nil {
		t.Fatalf("Set idempotent: %v", err)
	}
	if res.Changed {
		t.Error("idempotent Set Changed=true")
	}

	// Shrink to just [a] — Changed=true.
	res, err = Set(s, "record", "ref", "", []int64{a.Tag.ID})
	if err != nil {
		t.Fatalf("Set shrink: %v", err)
	}
	if !res.Changed {
		t.Error("shrink Set Changed=false")
	}

	// Verify final state via TagsFor.
	tags, err := TagsFor(s, "record", "ref", "")
	if err != nil {
		t.Fatalf("TagsFor: %v", err)
	}
	if len(tags) != 1 || tags[0].ID != a.Tag.ID {
		t.Errorf("final tags = %+v", tags)
	}
}

func TestItemsWithTag(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "x"})
	_, _ = Apply(s, a.Tag.ID, "record", "001A", "dev@x")
	_, _ = Apply(s, a.Tag.ID, "record", "001B", "dev@x")
	_, _ = Apply(s, a.Tag.ID, "record", "001C", "other@x")

	all, err := ItemsWithTag(s, a.Tag.ID, "")
	if err != nil {
		t.Fatalf("ItemsWithTag all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}
	devOnly, err := ItemsWithTag(s, a.Tag.ID, "dev@x")
	if err != nil {
		t.Fatalf("ItemsWithTag dev: %v", err)
	}
	if len(devOnly) != 2 {
		t.Errorf("len(dev) = %d, want 2", len(devOnly))
	}
}

func TestItemsWithTag_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := ItemsWithTag(s, 99, "")
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if nf.ID != 99 {
		t.Errorf("ErrNotFound.ID = %d, want 99", nf.ID)
	}
}

func TestList_UsageOnlyFilter(t *testing.T) {
	s := newTestStore(t)
	a, _ := Create(s, CreateInput{Name: "used"})
	_, _ = Create(s, CreateInput{Name: "unused"})
	_, _ = Apply(s, a.Tag.ID, "record", "001A", "")

	all, _ := List(s, false)
	if len(all) != 2 {
		t.Errorf("List(all) = %d, want 2", len(all))
	}
	used, _ := List(s, true)
	if len(used) != 1 || used[0].Name != "used" {
		t.Errorf("List(usage-only) = %+v", used)
	}
}

func TestKnownKinds_Sorted(t *testing.T) {
	got := KnownKinds()
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("KnownKinds not sorted: %v", got)
			break
		}
	}
	if len(got) == 0 {
		t.Error("KnownKinds empty")
	}
}
