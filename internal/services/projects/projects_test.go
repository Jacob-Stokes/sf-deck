package projects

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

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

func TestList_EmptyStore(t *testing.T) {
	s := newTestStore(t)
	got, err := List(s)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestCreate_HappyPath(t *testing.T) {
	s := newTestStore(t)
	res, err := Create(s, CreateInput{Name: "Q2 cleanup", Description: "post-release"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if res.Project.ID == "" {
		t.Errorf("ID empty")
	}
	if res.Project.Name != "Q2 cleanup" {
		t.Errorf("Name = %q", res.Project.Name)
	}
	if res.Project.CreatedAt.IsZero() || res.Project.TouchedAt.IsZero() {
		t.Errorf("timestamps unset: %+v", res.Project)
	}
}

func TestCreate_WithExplicitID(t *testing.T) {
	s := newTestStore(t)
	res, err := Create(s, CreateInput{ID: "dp-fixed-1", Name: "Fixed"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Project.ID != "dp-fixed-1" {
		t.Errorf("ID = %q, want dp-fixed-1", res.Project.ID)
	}
}

func TestCreate_RejectsEmptyName(t *testing.T) {
	s := newTestStore(t)
	if _, err := Create(s, CreateInput{Name: "  "}); err == nil {
		t.Error("expected validation error")
	}
}

func TestShow_ByIDAndByName(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "Q2"})

	byID, err := Show(s, created.Project.ID, "")
	if err != nil {
		t.Fatalf("Show id: %v", err)
	}
	if byID.Name != "Q2" {
		t.Errorf("byID.Name = %q", byID.Name)
	}
	byName, err := Show(s, "", "q2") // case-insensitive
	if err != nil {
		t.Fatalf("Show name: %v", err)
	}
	if byName.ID != created.Project.ID {
		t.Errorf("byName.ID = %q", byName.ID)
	}
}

func TestShow_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := Show(s, "missing", "")
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T: %v", err, err)
	}
}

func TestShow_MissingArguments(t *testing.T) {
	s := newTestStore(t)
	if _, err := Show(s, "", ""); err == nil {
		t.Error("expected error")
	}
	if _, err := Show(s, "x", "y"); err == nil {
		t.Error("expected error")
	}
}

func TestUpdate_PartialAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "Old", Description: "before"})

	newName := "New"
	res, err := Update(s, created.Project.ID, UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !res.Changed || res.Project.Name != "New" {
		t.Errorf("first update: %+v", res)
	}
	if res.Project.Description != "before" {
		t.Errorf("Description should be untouched: %q", res.Project.Description)
	}

	same := "New"
	res, err = Update(s, created.Project.ID, UpdateInput{Name: &same})
	if err != nil {
		t.Fatalf("idempotent Update: %v", err)
	}
	if res.Changed {
		t.Error("idempotent reported Changed=true")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s := newTestStore(t)
	v := "x"
	_, err := Update(s, "missing", UpdateInput{Name: &v})
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T", err)
	}
}

func TestUpdate_RejectsNoFields(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	if _, err := Update(s, created.Project.ID, UpdateInput{}); err == nil {
		t.Error("expected error")
	}
}

func TestDelete_EmptyProject(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})

	res, err := Delete(s, created.Project.ID, false)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if _, err := Show(s, created.Project.ID, ""); !errors.As(err, new(ErrNotFound)) {
		t.Errorf("post-delete show: %v", err)
	}
}

func TestDelete_NonEmptyRequiresForce(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID,
		Kind:      "record",
		Ref:       "001A",
		OrgUser:   "dev@x",
	})

	// Without force — ErrNotEmpty.
	_, err := Delete(s, created.Project.ID, false)
	var ne ErrNotEmpty
	if !errors.As(err, &ne) {
		t.Fatalf("err type = %T (%v), want ErrNotEmpty", err, err)
	}
	if ne.Items != 1 {
		t.Errorf("ne.Items = %d", ne.Items)
	}

	// With force — deletes.
	res, err := Delete(s, created.Project.ID, true)
	if err != nil {
		t.Fatalf("force Delete: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := Delete(s, "missing", false)
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T", err)
	}
}

func TestAddItem_HappyPath(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})

	res, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID,
		Kind:      "field",
		Ref:       "Account.MyField__c",
		OrgUser:   "dev@x",
		Name:      "My Field",
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if res.Item.Ref != "Account.MyField__c" {
		t.Errorf("Ref = %q", res.Item.Ref)
	}
}

func TestAddItem_PopulatesAddedAt(t *testing.T) {
	// Regression: devproject.AddItem takes Item by value, so the
	// AddedAt timestamp it assigns when zero doesn't propagate back.
	// The service must stamp AddedAt itself before passing.
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	res, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001A",
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if res.Item.AddedAt.IsZero() {
		t.Errorf("AddedAt is zero, want populated")
	}
}

func TestAddItem_Idempotent(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})

	in := AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001A", OrgUser: "d@x",
	}
	_, _ = AddItem(s, in)
	res, err := AddItem(s, in)
	if err != nil {
		t.Fatalf("AddItem 2: %v", err)
	}
	if res.Changed {
		t.Error("re-add reported Changed=true")
	}
}

func TestAddItem_BadKind(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "weird", Ref: "x",
	})
	var bk ErrInvalidKind
	if !errors.As(err, &bk) {
		t.Fatalf("err type = %T", err)
	}
}

func TestAddItem_ProjectMissing(t *testing.T) {
	s := newTestStore(t)
	_, err := AddItem(s, AddItemInput{
		ProjectID: "missing", Kind: "record", Ref: "x",
	})
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T", err)
	}
}

// TestAddItem_RejectsFlowViewId pins the validator that caught a
// real-world mis-collection: the agent stored FlowDefinitionView.Id
// (3dd...) but /flows matches on FlowDefinition.Id (300...). Reject
// at add-time with the hint that points at DurableId.
func TestAddItem_RejectsFlowViewId(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, OrgUser: "u",
		Kind: "flow", Ref: "3ddUE00000TJAFxYAP",
	})
	var br ErrInvalidRef
	if !errors.As(err, &br) {
		t.Fatalf("err type = %T (%v)", err, err)
	}
	if br.Kind != "flow" {
		t.Errorf("Kind = %q, want flow", br.Kind)
	}
	if !strings.Contains(br.Hint, "FlowDefinition.Id") {
		t.Errorf("Hint missing FlowDefinition.Id pointer: %q", br.Hint)
	}
}

// TestAddItem_AcceptsCorrectFlowId guards against an over-eager
// validator. The right shape (300...) must pass through.
func TestAddItem_AcceptsCorrectFlowId(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, OrgUser: "u",
		Kind: "flow", Ref: "300UE00000TJAFxYAP",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestAddItem_RejectsBareFieldRef pins the field-ref shape — must
// be <SObject>.<Field>, not a bare Id or DeveloperName.
func TestAddItem_RejectsBareFieldRef(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, err := AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, OrgUser: "u",
		Kind: "field", Ref: "Phone",
	})
	var br ErrInvalidRef
	if !errors.As(err, &br) {
		t.Fatalf("err type = %T", err)
	}
}

func TestRemoveItem_ChangedReflectsPresence(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})

	// Remove from empty — Changed=false (no row).
	res, err := RemoveItem(s, created.Project.ID, "d@x", "record", "001A")
	if err != nil {
		t.Fatalf("Remove empty: %v", err)
	}
	if res.Changed {
		t.Error("remove-absent Changed=true")
	}

	// Add then remove — Changed=true.
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001A", OrgUser: "d@x",
	})
	res, err = RemoveItem(s, created.Project.ID, "d@x", "record", "001A")
	if err != nil {
		t.Fatalf("Remove present: %v", err)
	}
	if !res.Changed {
		t.Error("remove-present Changed=false")
	}
}

func TestListItems_FilterByOrg(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001A", OrgUser: "d@x",
	})
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001B", OrgUser: "p@x",
	})

	all, err := ListItems(s, created.Project.ID, "")
	if err != nil {
		t.Fatalf("ListItems all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("len(all) = %d, want 2", len(all))
	}
	dev, err := ListItems(s, created.Project.ID, "d@x")
	if err != nil {
		t.Fatalf("ListItems dev: %v", err)
	}
	if len(dev) != 1 || dev[0].Ref != "001A" {
		t.Errorf("dev = %+v", dev)
	}
}

func TestList_PopulatesItemCount(t *testing.T) {
	s := newTestStore(t)
	created, _ := Create(s, CreateInput{Name: "X"})
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001A",
	})
	_, _ = AddItem(s, AddItemInput{
		ProjectID: created.Project.ID, Kind: "record", Ref: "001B",
	})

	list, err := List(s)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].ItemCount != 2 {
		t.Errorf("ItemCount = %d, want 2", list[0].ItemCount)
	}
}
