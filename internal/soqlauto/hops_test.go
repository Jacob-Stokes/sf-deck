package soqlauto

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// mockDescribeStore is a tiny in-memory describe source used by hops
// + engine tests.
type mockDescribeStore struct {
	loaded  map[string]sf.SObjectDescribe
	missing map[string]bool
	asked   []string
}

func newMockDescribeStore() *mockDescribeStore {
	return &mockDescribeStore{
		loaded:  map[string]sf.SObjectDescribe{},
		missing: map[string]bool{},
	}
}

func (m *mockDescribeStore) add(d sf.SObjectDescribe) {
	m.loaded[d.Name] = d
}

func (m *mockDescribeStore) ref(name string) DescribeRef {
	if d, ok := m.loaded[name]; ok {
		c := d
		return DescribeRef{Status: StatusLoaded, Describe: &c}
	}
	if m.missing[name] {
		return DescribeRef{Status: StatusLoading}
	}
	return DescribeRef{Status: StatusUnknown}
}

func (m *mockDescribeStore) ensure(name string) {
	m.asked = append(m.asked, name)
}

func (m *mockDescribeStore) snapshot(query string, cursor int) Snapshot {
	return Snapshot{
		Query:          query,
		CursorPos:      cursor,
		SelEnd:         cursor,
		Describes:      m.ref,
		EnsureDescribe: m.ensure,
	}
}

// describe helpers ----------------------------------------------------

func sd(name string, fields ...sf.Field) sf.SObjectDescribe {
	return sf.SObjectDescribe{Name: name, Label: name, Fields: fields}
}

func refField(apiName, relName string, targets ...string) sf.Field {
	return sf.Field{
		Name:             apiName,
		Label:            apiName,
		Type:             "reference",
		RelationshipName: relName,
		ReferenceTo:      targets,
	}
}

func plainField(apiName, kind string) sf.Field {
	return sf.Field{Name: apiName, Label: apiName, Type: kind}
}

// tests ---------------------------------------------------------------

func TestWalkHopsSingleHop(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account",
		plainField("Name", "string"),
		refField("OwnerId", "Owner", "User"),
	))
	store.add(sd("User", plainField("Email", "email")))

	snap := store.snapshot("", 0)
	got, loading := WalkHops(snap, "Account", []string{"Owner"})
	if len(got) != 1 || got[0].Name != "User" {
		t.Fatalf("got %v, want [User]", describeNames(got))
	}
	if len(loading) != 0 {
		t.Fatalf("loading = %v, want none", loading)
	}
}

func TestWalkHopsTwoHops(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account", refField("OwnerId", "Owner", "User")))
	store.add(sd("User",
		refField("ManagerId", "Manager", "User"),
		plainField("Email", "email"),
	))

	snap := store.snapshot("", 0)
	got, loading := WalkHops(snap, "Account", []string{"Owner", "Manager"})
	if len(got) != 1 || got[0].Name != "User" {
		t.Fatalf("got %v, want [User]", describeNames(got))
	}
	if len(loading) != 0 {
		t.Fatalf("loading = %v", loading)
	}
}

func TestWalkHopsPolymorphic(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Task", refField("WhoId", "Who", "Contact", "Lead")))
	store.add(sd("Contact", plainField("Email", "email")))
	store.add(sd("Lead", plainField("Company", "string")))

	snap := store.snapshot("", 0)
	got, _ := WalkHops(snap, "Task", []string{"Who"})
	names := describeNames(got)
	if len(names) != 2 {
		t.Fatalf("got %v, want 2 describes", names)
	}
	have := map[string]bool{names[0]: true, names[1]: true}
	if !have["Contact"] || !have["Lead"] {
		t.Fatalf("got %v, want [Contact Lead]", names)
	}
}

func TestWalkHopsMissingDescribeEmitsLoading(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account", refField("OwnerId", "Owner", "User")))
	// User describe is NOT loaded.

	snap := store.snapshot("", 0)
	got, loading := WalkHops(snap, "Account", []string{"Owner"})
	if len(got) != 0 {
		t.Fatalf("got %v, want empty (dead end)", describeNames(got))
	}
	if len(loading) != 1 || loading[0] != "User" {
		t.Fatalf("loading = %v, want [User]", loading)
	}
	if len(store.asked) != 1 || store.asked[0] != "User" {
		t.Fatalf("asked = %v, want [User]", store.asked)
	}
}

func TestWalkHopsRootMissing(t *testing.T) {
	store := newMockDescribeStore()
	snap := store.snapshot("", 0)
	got, loading := WalkHops(snap, "Account", nil)
	if got != nil || len(loading) != 1 || loading[0] != "Account" {
		t.Fatalf("got=%v loading=%v", describeNames(got), loading)
	}
}

func TestWalkHopsDeadEnd(t *testing.T) {
	store := newMockDescribeStore()
	store.add(sd("Account", plainField("Name", "string")))

	snap := store.snapshot("", 0)
	got, loading := WalkHops(snap, "Account", []string{"NoSuchRel"})
	if len(got) != 0 || len(loading) != 0 {
		t.Fatalf("got=%v loading=%v", describeNames(got), loading)
	}
}

func describeNames(ds []sf.SObjectDescribe) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}
