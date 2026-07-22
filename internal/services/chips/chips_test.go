package chips

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func newStubSettings(seed ...settings.ChipConfig) *settings.Settings {
	s := &settings.Settings{}
	for _, c := range seed {
		s.UpsertChip(c)
	}
	return s
}

// counter helper builds a Persister that bumps a counter so we can
// assert Save is called exactly when we expect.
func counter() (func() error, *int) {
	n := 0
	return func() error { n++; return nil }, &n
}

func TestList_FiltersByDomain(t *testing.T) {
	st := newStubSettings(
		settings.ChipConfig{ID: "a", Domain: "records", Label: "A"},
		settings.ChipConfig{ID: "b", Domain: "objects", Label: "B"},
		settings.ChipConfig{ID: "c", Domain: "records", Label: "C"},
	)
	all, err := List(st, "")
	if err != nil {
		t.Fatalf("List(\"\"): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}
	recs, err := List(st, "records")
	if err != nil {
		t.Fatalf("List(records): %v", err)
	}
	if len(recs) != 2 {
		t.Errorf("len(records) = %d, want 2", len(recs))
	}
	for _, c := range recs {
		if c.Domain != "records" {
			t.Errorf("got domain %q in records filter", c.Domain)
		}
	}
}

func TestList_StableSort(t *testing.T) {
	st := newStubSettings(
		settings.ChipConfig{ID: "z", Domain: "records", Label: "Zeta"},
		settings.ChipConfig{ID: "a", Domain: "records", Label: "Alpha"},
		settings.ChipConfig{ID: "m", Domain: "objects", Label: "Mid"},
	)
	got, err := List(st, "")
	if err != nil {
		t.Fatal(err)
	}
	// Stable order: domain then label then id.
	want := []string{"Mid", "Alpha", "Zeta"}
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	for i, w := range want {
		if got[i].Label != w {
			t.Errorf("got[%d].Label = %q, want %q", i, got[i].Label, w)
		}
	}
}

func TestList_InvalidDomain(t *testing.T) {
	if _, err := List(newStubSettings(), "bogus"); err == nil {
		t.Fatal("expected error on bogus domain")
	}
}

func TestShow_Found(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "renewals", Domain: "records", Label: "Renewals", Scope: "Account",
	})
	got, err := Show(st, "records", "renewals")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if got.Label != "Renewals" || got.Scope != "Account" {
		t.Errorf("got %+v", got)
	}
}

func TestShow_NotFound(t *testing.T) {
	_, err := Show(newStubSettings(), "records", "missing")
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T, want ErrNotFound: %v", err, err)
	}
	if nf.ID != "missing" || nf.Domain != "records" {
		t.Errorf("ErrNotFound = %+v", nf)
	}
}

func TestCreate_HappyPath(t *testing.T) {
	st := newStubSettings()
	save, n := counter()
	res, err := Create(st, CreateInput{
		ID: "renewals", Domain: "records", Label: "Renewals",
		Scope: "Account", Limit: 50, Columns: []string{"Id", "Name"},
	}, save)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false, want true")
	}
	if *n != 1 {
		t.Errorf("save called %d times, want 1", *n)
	}
	if got := st.Chips(); len(got) != 1 {
		t.Fatalf("after create, len(chips) = %d", len(got))
	}
}

func TestCreate_WithClausesPersistsAdvancedQuery(t *testing.T) {
	st := newStubSettings()
	save, n := counter()
	res, err := Create(st, CreateInput{
		ID: "requests", Domain: "records", Label: "Urgent requests",
		Scope: "Request__c", Columns: []string{"Id", "Name", "Status__c"},
		Clauses: "WHERE Status__c = 'Open' AND Priority__c = 'High' ORDER BY CreatedDate DESC LIMIT 25",
	}, save)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false, want true")
	}
	if *n != 1 {
		t.Errorf("save called %d times, want 1", *n)
	}
	if !strings.Contains(res.Chip.Clauses, "WHERE") ||
		!strings.Contains(res.Chip.Clauses, "ORDER BY CreatedDate DESC") ||
		!strings.Contains(res.Chip.Clauses, "LIMIT 25") {
		t.Errorf("Clauses = %q", res.Chip.Clauses)
	}
	got := st.Chips()
	if len(got) != 1 {
		t.Fatalf("len(chips) = %d, want 1", len(got))
	}
	if got[0].Query.Where == nil {
		t.Fatal("Query.Where is nil")
	}
	if len(got[0].Query.OrderBy) != 1 || got[0].Query.OrderBy[0].Field != "CreatedDate" {
		t.Errorf("OrderBy = %+v", got[0].Query.OrderBy)
	}
	if got[0].Query.Limit != 25 {
		t.Errorf("Limit = %d, want 25", got[0].Query.Limit)
	}
	if !stringsEqual(got[0].Query.Columns, []string{"Id", "Name", "Status__c"}) {
		t.Errorf("Columns = %v", got[0].Query.Columns)
	}
}

func TestCreate_WithBadClausesRejected(t *testing.T) {
	st := newStubSettings()
	save, n := counter()
	_, err := Create(st, CreateInput{
		ID: "bad", Domain: "records", Label: "Bad", Scope: "Account",
		Clauses: "WHERE Name =",
	}, save)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if *n != 0 {
		t.Errorf("save called on parse failure: n = %d", *n)
	}
}

func TestCreate_DuplicateRejected(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "X",
	})
	save, n := counter()
	_, err := Create(st, CreateInput{
		ID: "x", Domain: "records", Label: "Different",
	}, save)
	var dup ErrAlreadyExists
	if !errors.As(err, &dup) {
		t.Fatalf("err type = %T, want ErrAlreadyExists: %v", err, err)
	}
	if *n != 0 {
		t.Errorf("save should not be called on duplicate; n = %d", *n)
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		in   CreateInput
	}{
		{"missing id", CreateInput{Domain: "records", Label: "X"}},
		{"bad domain", CreateInput{ID: "a", Domain: "weird", Label: "X"}},
		{"missing label", CreateInput{ID: "a", Domain: "records"}},
		{"id with whitespace", CreateInput{ID: "a b", Domain: "records", Label: "X"}},
		{"negative limit", CreateInput{ID: "a", Domain: "records", Label: "X", Limit: -1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			save, n := counter()
			if _, err := Create(newStubSettings(), c.in, save); err == nil {
				t.Errorf("expected error")
			}
			if *n != 0 {
				t.Errorf("save called on validation failure: n = %d", *n)
			}
		})
	}
}

func TestUpdate_PartialFields(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "Old", Scope: "Account",
	})
	save, n := counter()
	newLabel := "New"
	res, err := Update(st, "records", "x", UpdateInput{Label: &newLabel}, save)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false, want true")
	}
	if res.Chip.Label != "New" {
		t.Errorf("Label = %q, want New", res.Chip.Label)
	}
	if res.Chip.Scope != "Account" {
		t.Errorf("Scope should be untouched, got %q", res.Chip.Scope)
	}
	if *n != 1 {
		t.Errorf("save called %d times", *n)
	}
}

func TestUpdate_IdempotentNoOp(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "Same",
	})
	save, n := counter()
	sameLabel := "Same"
	res, err := Update(st, "records", "x", UpdateInput{Label: &sameLabel}, save)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Changed {
		t.Error("Changed = true, want false (no-op)")
	}
	if *n != 0 {
		t.Errorf("save should not run on no-op; n = %d", *n)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	save, _ := counter()
	v := "x"
	_, err := Update(newStubSettings(), "records", "missing", UpdateInput{Label: &v}, save)
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T, want ErrNotFound: %v", err, err)
	}
}

func TestUpdate_ColumnsReplaced(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "X",
		Query: settings.ChipQueryYAML{Columns: []string{"A", "B"}},
	})
	save, _ := counter()
	newCols := []string{"X", "Y", "Z"}
	res, err := Update(st, "records", "x", UpdateInput{Columns: &newCols}, save)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if got := res.Chip.Columns; len(got) != 3 || got[0] != "X" {
		t.Errorf("Columns = %v", got)
	}
}

func TestUpdate_ClausesReplaceQueryClauses(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "X",
		Query: settings.ChipQueryYAML{Columns: []string{"Id", "Name"}, Limit: 10},
	})
	save, n := counter()
	clauses := "WHERE Active__c = true ORDER BY CreatedDate DESC"
	res, err := Update(st, "records", "x", UpdateInput{Clauses: &clauses}, save)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if *n != 1 {
		t.Errorf("save called %d times, want 1", *n)
	}
	if !strings.Contains(res.Chip.Clauses, "WHERE Active__c = true") ||
		!strings.Contains(res.Chip.Clauses, "ORDER BY CreatedDate DESC") {
		t.Errorf("Clauses = %q", res.Chip.Clauses)
	}
	got := st.Chips()[0]
	if got.Query.Limit != 0 {
		t.Errorf("Limit = %d, want cleared to 0", got.Query.Limit)
	}
	if !stringsEqual(got.Query.Columns, []string{"Id", "Name"}) {
		t.Errorf("Columns = %v", got.Query.Columns)
	}
}

func TestDelete_RemovesAndReportsChanged(t *testing.T) {
	st := newStubSettings(
		settings.ChipConfig{ID: "x", Domain: "records", Label: "X"},
		settings.ChipConfig{ID: "y", Domain: "records", Label: "Y"},
	)
	save, n := counter()
	res, err := Delete(st, "records", "x", save)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !res.Changed {
		t.Error("Changed = false")
	}
	if res.Chip.ID != "x" {
		t.Errorf("returned chip id = %q, want x", res.Chip.ID)
	}
	if len(st.Chips()) != 1 {
		t.Errorf("after delete, len = %d, want 1", len(st.Chips()))
	}
	if *n != 1 {
		t.Errorf("save called %d times", *n)
	}
}

func TestDelete_NotFound(t *testing.T) {
	save, n := counter()
	_, err := Delete(newStubSettings(), "records", "missing", save)
	var nf ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err type = %T, want ErrNotFound: %v", err, err)
	}
	if *n != 0 {
		t.Errorf("save called on missing delete; n = %d", *n)
	}
}

func TestFavourite_TogglesAndIsIdempotent(t *testing.T) {
	st := newStubSettings(settings.ChipConfig{
		ID: "x", Domain: "records", Label: "X", Favourite: false,
	})
	save, n := counter()

	// First toggle on — should mutate + save.
	res, err := Favourite(st, "records", "x", true, save)
	if err != nil {
		t.Fatalf("Favourite on: %v", err)
	}
	if !res.Changed || !res.Chip.Favourite {
		t.Errorf("first toggle res = %+v", res)
	}
	if *n != 1 {
		t.Errorf("save n = %d, want 1", *n)
	}

	// Re-favourite — no-op, no save.
	res, err = Favourite(st, "records", "x", true, save)
	if err != nil {
		t.Fatalf("Favourite idempotent: %v", err)
	}
	if res.Changed {
		t.Error("idempotent re-favourite reported Changed=true")
	}
	if *n != 1 {
		t.Errorf("save n = %d, want still 1", *n)
	}
}
