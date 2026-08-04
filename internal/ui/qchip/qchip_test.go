package qchip

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/query"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// mapRow is a tiny query.Row impl backed by a map. Used by every test
// that wants to evaluate a chip without dragging in the sf package.
type mapRow map[string]any

func (m mapRow) Field(name string) (any, bool) {
	v, ok := m[name]
	return v, ok
}

// ---- Registry ---------------------------------------------------------

func TestRegistryChipsForScope(t *testing.T) {
	universal := Chip{ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn}
	accountOnly := Chip{ID: "open", Label: "Open", Scope: "Account", Origin: OriginBuiltIn}
	caseOnly := Chip{ID: "active", Label: "Active", Scope: "Case", Origin: OriginBuiltIn}

	r := NewRegistry("records", []Chip{universal, accountOnly, caseOnly})

	got := r.ChipsFor("Account")
	if len(got) != 2 {
		t.Fatalf("Account scope: want 2 chips (universal + Account), got %d (%v)", len(got), chipIDs(got))
	}
	if got[0].ID != "all" || got[1].ID != "open" {
		t.Fatalf("Account scope: wrong ids %v", chipIDs(got))
	}

	got = r.ChipsFor("Case")
	if len(got) != 2 || got[1].ID != "active" {
		t.Fatalf("Case scope: %v", chipIDs(got))
	}

	got = r.ChipsFor("Lead")
	if len(got) != 1 || got[0].ID != "all" {
		t.Fatalf("Lead scope: only universal should apply, got %v", chipIDs(got))
	}
}

func TestRegistryFindByID(t *testing.T) {
	r := NewRegistry("objects", []Chip{
		{ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn},
	})
	r.SetUser([]Chip{
		{ID: "mine", Label: "Mine", Scope: "*", Origin: OriginUser},
	})

	if c, ok := r.FindByID("all"); !ok || c.Origin != OriginBuiltIn {
		t.Fatalf("FindByID(all): %#v ok=%v", c, ok)
	}
	if c, ok := r.FindByID("mine"); !ok || c.Origin != OriginUser {
		t.Fatalf("FindByID(mine): %#v ok=%v", c, ok)
	}
	if _, ok := r.FindByID("nope"); ok {
		t.Fatal("FindByID should return ok=false for unknown ids")
	}
}

func TestRegistryLoadFromSettings(t *testing.T) {
	s := &settings.Settings{}
	s.SetChips([]settings.ChipConfig{
		{ID: "user-records", Domain: "records", Label: "User R"},
		{ID: "user-objects", Domain: "objects", Label: "User O"},
		{ID: "user-flows", Domain: "flows", Label: "User F"},
	})

	rRecords := NewRegistry("records", nil)
	rRecords.LoadFromSettings(s)
	if user := rRecords.User(); len(user) != 1 || user[0].ID != "user-records" {
		t.Fatalf("records registry should load only its domain: %v", chipIDs(user))
	}

	rObjects := NewRegistry("objects", nil)
	rObjects.LoadFromSettings(s)
	if user := rObjects.User(); len(user) != 1 || user[0].ID != "user-objects" {
		t.Fatalf("objects registry should load only its domain: %v", chipIDs(user))
	}
}

func TestRegistryPersistUserPreservesOtherDomains(t *testing.T) {
	s := &settings.Settings{}
	s.SetChips([]settings.ChipConfig{
		// Pre-existing flow chip — persisting records should leave this alone.
		{ID: "active-flow", Domain: "flows", Label: "Active"},
	})
	rRecords := NewRegistry("records", nil)
	rRecords.SetUser([]Chip{
		{ID: "open-cases", Label: "Open Cases", Scope: "Case", Origin: OriginUser},
	})
	rRecords.PersistUser(s)

	chips := s.Chips()
	if len(chips) != 2 {
		t.Fatalf("expected 2 chips after persist (1 records + 1 flows), got %d: %#v", len(chips), chips)
	}
	var hasFlow, hasRecords bool
	for _, c := range chips {
		if c.Domain == "flows" && c.ID == "active-flow" {
			hasFlow = true
		}
		if c.Domain == "records" && c.ID == "open-cases" {
			hasRecords = true
		}
	}
	if !hasFlow {
		t.Error("PersistUser dropped the existing flow chip")
	}
	if !hasRecords {
		t.Error("PersistUser didn't write the new records chip")
	}
}

// ---- ApplyToSOQL ------------------------------------------------------

func TestApplyToSOQLEmitsValidSOQL(t *testing.T) {
	c := Chip{
		ID:    "active",
		Label: "Active",
		Query: query.Query{
			Where: query.Cmp("Status", query.OpEq, "Active"),
			OrderBy: []query.OrderBy{
				{Field: "LastModifiedDate", Direction: query.Descending},
			},
			Limit: 50,
		},
	}
	got := ApplyToSOQL(c, "Account", Substitutions{})
	want := "SELECT Id FROM Account WHERE Status = 'Active' ORDER BY LastModifiedDate DESC LIMIT 50"
	if got != want {
		t.Fatalf("ApplyToSOQL\nwant %q\ngot  %q", want, got)
	}
}

func TestApplyToSOQLSubstitutesUserID(t *testing.T) {
	c := Chip{
		ID: "mine",
		Query: query.Query{
			Where: query.Cmp("OwnerId", query.OpEq, "$userId"),
		},
	}
	got := ApplyToSOQL(c, "Case", Substitutions{UserID: "005000000000ABC"})
	if !strings.Contains(got, "OwnerId = '005000000000ABC'") {
		t.Fatalf("expected $userId substituted into SOQL, got %q", got)
	}
	if strings.Contains(got, "$userId") {
		t.Fatalf("$userId should not appear in emitted SOQL, got %q", got)
	}
}

func TestApplyToSOQLSubstitutesIntoInList(t *testing.T) {
	c := Chip{
		Query: query.Query{
			Where: query.Cmp("OwnerId", query.OpIn, []any{"$userId", "005other"}),
		},
	}
	got := ApplyToSOQL(c, "Case", Substitutions{UserID: "005me"})
	if !strings.Contains(got, "'005me'") {
		t.Fatalf("expected $userId resolved inside IN list, got %q", got)
	}
}

func TestApplyToSOQLDoesNotMutateInput(t *testing.T) {
	original := Chip{
		Query: query.Query{
			Where: query.Cmp("OwnerId", query.OpEq, "$userId"),
		},
	}
	clone := Chip{
		Query: query.Query{
			Where: query.Cmp("OwnerId", query.OpEq, "$userId"),
		},
	}
	_ = ApplyToSOQL(original, "Case", Substitutions{UserID: "005abc"})
	if !reflect.DeepEqual(original, clone) {
		t.Fatalf("ApplyToSOQL mutated the input chip")
	}
}

// ---- ApplyToRow -------------------------------------------------------

func TestApplyToRowEvaluatesPredicate(t *testing.T) {
	c := Chip{
		Query: query.Query{
			Where: query.Cmp("Status", query.OpEq, "Active"),
		},
	}
	if !ApplyToRow(c, mapRow{"Status": "Active"}, Substitutions{}) {
		t.Fatal("expected row to match Status=Active")
	}
	if ApplyToRow(c, mapRow{"Status": "Draft"}, Substitutions{}) {
		t.Fatal("expected Draft row to fail")
	}
}

func TestApplyToRowSubstitutesUserID(t *testing.T) {
	c := Chip{
		Query: query.Query{
			Where: query.Cmp("OwnerId", query.OpEq, "$userId"),
		},
	}
	if !ApplyToRow(c, mapRow{"OwnerId": "005me"}, Substitutions{UserID: "005me"}) {
		t.Fatal("expected $userId substitution to match the row's OwnerId")
	}
	if ApplyToRow(c, mapRow{"OwnerId": "005other"}, Substitutions{UserID: "005me"}) {
		t.Fatal("expected $userId substitution to reject a row with a different OwnerId")
	}
}

func TestApplyToRowEmptyChipMatchesEverything(t *testing.T) {
	c := Chip{Query: query.Query{}}
	if !ApplyToRow(c, mapRow{}, Substitutions{}) {
		t.Fatal("empty chip should match every row")
	}
}

// chipIDs is a tiny helper to keep failure messages readable.
func chipIDs(cs []Chip) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
