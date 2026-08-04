package devproject

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSavedQueries_CreateAndGet(t *testing.T) {
	s := openTestStore(t)
	q, err := s.CreateSavedQuery("Top accounts", "Recent big accounts",
		"SELECT Id, Name FROM Account ORDER BY CreatedDate DESC LIMIT 50")
	if err != nil {
		t.Fatalf("CreateSavedQuery: %v", err)
	}
	if !strings.HasPrefix(q.ID, "sq_") {
		t.Errorf("ID prefix wrong: %q", q.ID)
	}
	if q.CreatedAt.IsZero() || q.UpdatedAt.IsZero() {
		t.Errorf("timestamps zero: %+v", q)
	}

	got, err := s.GetSavedQuery(q.ID)
	if err != nil {
		t.Fatalf("GetSavedQuery: %v", err)
	}
	if got.Name != q.Name || got.Body != q.Body || got.Description != q.Description {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, q)
	}
}

func TestSavedQueries_RejectsEmpty(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateSavedQuery("  ", "", "SELECT Id FROM Account"); !errors.Is(err, ErrSavedQueryEmpty) {
		t.Errorf("blank name should ErrSavedQueryEmpty; got %v", err)
	}
	if _, err := s.CreateSavedQuery("name", "", "  "); !errors.Is(err, ErrSavedQueryEmpty) {
		t.Errorf("blank body should ErrSavedQueryEmpty; got %v", err)
	}
}

func TestSavedQueries_GetMissing(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.GetSavedQuery("sq_nope"); !errors.Is(err, ErrSavedQueryNotFound) {
		t.Errorf("missing id: got %v want ErrSavedQueryNotFound", err)
	}
}

func TestSavedQueries_Update(t *testing.T) {
	s := openTestStore(t)
	q, _ := s.CreateSavedQuery("v1", "", "SELECT Id FROM Account")
	// nudge updated_at apart so order is unambiguous
	time.Sleep(1100 * time.Millisecond)
	if err := s.UpdateSavedQuery(q.ID, "v2", "now with more rows", "SELECT Id, Name FROM Account"); err != nil {
		t.Fatalf("UpdateSavedQuery: %v", err)
	}
	got, _ := s.GetSavedQuery(q.ID)
	if got.Name != "v2" || got.Description != "now with more rows" || !strings.Contains(got.Body, "Name") {
		t.Errorf("update didn't take: %+v", got)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt should advance: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestSavedQueries_UpdateMissing(t *testing.T) {
	s := openTestStore(t)
	if err := s.UpdateSavedQuery("sq_ghost", "x", "", "SELECT Id FROM Account"); !errors.Is(err, ErrSavedQueryNotFound) {
		t.Errorf("ghost update: got %v want ErrSavedQueryNotFound", err)
	}
}

func TestSavedQueries_ListOrdersByUpdated(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateSavedQuery("A", "", "SELECT Id FROM Account")
	time.Sleep(1100 * time.Millisecond)
	b, _ := s.CreateSavedQuery("B", "", "SELECT Id FROM Account")
	list, err := s.ListSavedQueries()
	if err != nil {
		t.Fatalf("ListSavedQueries: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2", len(list))
	}
	if list[0].ID != b.ID || list[1].ID != a.ID {
		t.Errorf("ordering wrong: got %v %v want %v %v", list[0].ID, list[1].ID, b.ID, a.ID)
	}
}

func TestSavedQueries_DeleteCascadesTagsAndPins(t *testing.T) {
	s := openTestStore(t)
	q, _ := s.CreateSavedQuery("Q", "", "SELECT Id FROM Account")
	tag, _ := s.CreateTag("favourites", "", "")
	if err := s.ApplyTag(tag.ID, KindSOQLQuery, q.ID, ""); err != nil {
		t.Fatalf("ApplyTag: %v", err)
	}
	// pin to a project
	dp := DevProject{ID: "dp_x", Name: "X", CreatedAt: time.Now(), TouchedAt: time.Now()}
	if err := s.CreateDevProject(dp); err != nil {
		t.Fatalf("CreateDevProject: %v", err)
	}
	if _, err := s.AddItem(Item{
		DevProjectID: dp.ID, OrgUser: "", Kind: KindSOQLQuery, Ref: q.ID,
		Name: "Q", AddedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := s.DeleteSavedQuery(q.ID); err != nil {
		t.Fatalf("DeleteSavedQuery: %v", err)
	}
	// Verify cascades.
	tagged, _ := s.TagsFor(KindSOQLQuery, q.ID, "")
	if len(tagged) != 0 {
		t.Errorf("tag binding survived delete: %v", tagged)
	}
	items, _ := s.ListItems(dp.ID, "")
	for _, it := range items {
		if it.Kind == KindSOQLQuery && it.Ref == q.ID {
			t.Errorf("project pin survived delete: %+v", it)
		}
	}
	if _, err := s.GetSavedQuery(q.ID); !errors.Is(err, ErrSavedQueryNotFound) {
		t.Errorf("query still resolvable after delete")
	}
}

func TestSavedQueries_TouchBumpsUpdated(t *testing.T) {
	s := openTestStore(t)
	q, _ := s.CreateSavedQuery("X", "", "SELECT Id FROM Account")
	original := q.UpdatedAt
	time.Sleep(1100 * time.Millisecond)
	if err := s.TouchSavedQuery(q.ID); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _ := s.GetSavedQuery(q.ID)
	if !got.UpdatedAt.After(original) {
		t.Errorf("Touch didn't advance UpdatedAt")
	}
}

func TestSOQLHistory_LogAndList(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.LogSOQLHistory("u@org.test", "SELECT Id FROM Account", 120, 50, ""); err != nil {
		t.Fatalf("LogSOQLHistory: %v", err)
	}
	if _, err := s.LogSOQLHistory("u@org.test", "SELECT Id FROM Contact", 80, 30, ""); err != nil {
		t.Fatalf("LogSOQLHistory: %v", err)
	}
	if _, err := s.LogSOQLHistory("other@org.test", "SELECT Id FROM Account", 50, 10, ""); err != nil {
		t.Fatalf("LogSOQLHistory: %v", err)
	}

	rows, err := s.ListSOQLHistory("u@org.test", 10)
	if err != nil {
		t.Fatalf("ListSOQLHistory: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 rows for u@org.test, got %d", len(rows))
	}
	// Newest first.
	if !strings.Contains(rows[0].Body, "Contact") {
		t.Errorf("ordering wrong: %v", rows[0])
	}

	all, _ := s.ListSOQLHistory("", 0)
	if len(all) != 3 {
		t.Errorf("cross-org history: got %d want 3", len(all))
	}
}

func TestSOQLHistory_TrimKeepsLatestPerOrg(t *testing.T) {
	s := openTestStore(t)
	for i := 0; i < 5; i++ {
		s.LogSOQLHistory("a@x", "q a "+string(rune('A'+i)), 0, 0, "")
	}
	for i := 0; i < 3; i++ {
		s.LogSOQLHistory("b@x", "q b "+string(rune('A'+i)), 0, 0, "")
	}
	if err := s.TrimSOQLHistory(2); err != nil {
		t.Fatalf("Trim: %v", err)
	}
	a, _ := s.ListSOQLHistory("a@x", 0)
	if len(a) != 2 {
		t.Errorf("a@x: got %d want 2", len(a))
	}
	b, _ := s.ListSOQLHistory("b@x", 0)
	if len(b) != 2 {
		t.Errorf("b@x: got %d want 2", len(b))
	}
}
