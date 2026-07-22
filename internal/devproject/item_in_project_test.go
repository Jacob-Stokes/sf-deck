package devproject

import (
	"testing"
	"time"
)

func TestItemInProject(t *testing.T) {
	s := openTestStore(t)
	dp := DevProject{ID: "p1", Name: "Proj", CreatedAt: time.Now(), TouchedAt: time.Now()}
	if err := s.CreateDevProject(dp); err != nil {
		t.Fatal(err)
	}

	// Not present before adding.
	in, err := s.ItemInProject("p1", KindApexClass, "01p000", "alice@dev")
	if err != nil {
		t.Fatal(err)
	}
	if in {
		t.Fatal("item reported present before it was added")
	}

	if _, err := s.AddItem(Item{
		DevProjectID: "p1", OrgUser: "alice@dev",
		Kind: KindApexClass, Ref: "01p000", Name: "Foo",
	}); err != nil {
		t.Fatal(err)
	}

	// Present after adding.
	in, _ = s.ItemInProject("p1", KindApexClass, "01p000", "alice@dev")
	if !in {
		t.Fatal("item not reported present after add")
	}

	// Org-scoped: same item under a different org is NOT present.
	in, _ = s.ItemInProject("p1", KindApexClass, "01p000", "bob@dev")
	if in {
		t.Fatal("item wrongly reported present for a different org")
	}

	// Removing clears it (round-trips the toggle path).
	if err := s.RemoveItem("p1", "alice@dev", KindApexClass, "01p000"); err != nil {
		t.Fatal(err)
	}
	in, _ = s.ItemInProject("p1", KindApexClass, "01p000", "alice@dev")
	if in {
		t.Fatal("item still present after remove")
	}
}
