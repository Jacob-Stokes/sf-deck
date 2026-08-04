package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestPicklistEditor_CycleAndCommit(t *testing.T) {
	f := sf.Field{
		Type: "picklist", Updateable: true, Nillable: true,
		PicklistValues: []sf.PicklistValue{
			{Value: "Prospect", Label: "Prospect", Active: true},
			{Value: "Customer", Label: "Customer", Active: true},
			{Value: "Lost", Label: "Lost", Active: true},
			{Value: "Archived", Label: "Archived", Active: false}, // inactive — should be filtered
		},
	}
	e := &picklistEditor{}
	state := e.Init(f, "Prospect")
	if state.Cursor != 0 {
		t.Errorf("init cursor: %d", state.Cursor)
	}
	e.HandleKey(&state, fakeKey("down"))
	if state.Raw != "Customer" {
		t.Errorf("after down: %q", state.Raw)
	}
	e.HandleKey(&state, fakeKey("down"))
	if state.Raw != "Lost" {
		t.Errorf("after second down: %q", state.Raw)
	}
	e.HandleKey(&state, fakeKey("down"))
	// Clamps at the last active value — wrap would burn through the
	// list on a trackpad burst, so down at the edge is a no-op.
	if state.Raw != "Lost" {
		t.Errorf("after clamp at bottom: %q", state.Raw)
	}
	mode, val, _ := e.Commit(&state)
	if mode != CommitValue || val != "Lost" {
		t.Errorf("commit: mode=%v val=%v", mode, val)
	}
}

func TestPicklistEditor_FiltersInactive(t *testing.T) {
	f := sf.Field{
		PicklistValues: []sf.PicklistValue{
			{Value: "A", Active: true},
			{Value: "B", Active: false},
			{Value: "C", Active: true},
		},
	}
	got := activePicklistValues(f)
	if len(got) != 2 {
		t.Fatalf("want 2 active, got %d", len(got))
	}
	if got[0].Value != "A" || got[1].Value != "C" {
		t.Errorf("filter dropped wrong values: %+v", got)
	}
}

func TestMultipicklist_ToggleSelections(t *testing.T) {
	f := sf.Field{
		Type: "multipicklist", Updateable: true, Nillable: true,
		PicklistValues: []sf.PicklistValue{
			{Value: "Red", Active: true},
			{Value: "Green", Active: true},
			{Value: "Blue", Active: true},
		},
	}
	e := &multipicklistEditor{}
	state := e.Init(f, "Red;Blue")
	if len(state.Selected) != 2 || state.Selected[0] != "Red" || state.Selected[1] != "Blue" {
		t.Errorf("init split wrong: %v", state.Selected)
	}
	// cursor=0 (Red), toggle removes it.
	e.HandleKey(&state, fakeKey("space"))
	if len(state.Selected) != 1 || state.Selected[0] != "Blue" {
		t.Errorf("after toggle off: %v", state.Selected)
	}
	// move to Green, toggle on.
	e.HandleKey(&state, fakeKey("down"))
	e.HandleKey(&state, fakeKey("space"))
	if !containsString(state.Selected, "Green") {
		t.Errorf("Green not added: %v", state.Selected)
	}
	mode, val, _ := e.Commit(&state)
	if mode != CommitValue {
		t.Fatalf("commit mode=%v", mode)
	}
	// Order depends on toggle order; just check both expected
	// values are present.
	s := val.(string)
	if !contains(s, "Blue") || !contains(s, "Green") {
		t.Errorf("commit value missing entries: %q", s)
	}
}

func TestMultipicklist_EmptyNillableCommitsNull(t *testing.T) {
	f := sf.Field{Type: "multipicklist", Updateable: true, Nillable: true,
		PicklistValues: []sf.PicklistValue{{Value: "X", Active: true}},
	}
	e := &multipicklistEditor{}
	state := e.Init(f, "")
	mode, val, _ := e.Commit(&state)
	if mode != CommitNull {
		t.Errorf("empty nillable should commit null, got %v", mode)
	}
	if val != nil {
		t.Errorf("commit null but val=%v", val)
	}
}

// W2: when the stored value isn't among the ACTIVE options (admin
// deactivated it after the record was set), Init must leave Cursor=-1 so
// the cell renders the real stored value (Raw) — not mask it with
// options[0], which would PATCH a value two away on a single ↑/↓.
func TestPicklist_InactiveStoredValueNotMasked(t *testing.T) {
	f := sf.Field{Type: "picklist", Updateable: true,
		PicklistValues: []sf.PicklistValue{
			{Value: "Open", Active: true},
			{Value: "Closed", Active: true},
		},
	}
	e := &picklistEditor{}
	// Stored value "Legacy" is no longer an active option.
	state := e.Init(f, "Legacy")
	if state.Cursor != -1 {
		t.Errorf("inactive stored value should leave Cursor=-1, got %d (would mask with options[0])", state.Cursor)
	}
	if state.Raw != "Legacy" {
		t.Errorf("Raw should preserve the stored value, got %q", state.Raw)
	}
	// A no-touch commit must write back the real stored value, not option 0.
	mode, val, _ := e.Commit(&state)
	if mode != CommitValue || val != "Legacy" {
		t.Errorf("no-touch commit should keep %q, got mode=%v val=%v", "Legacy", mode, val)
	}
}

// An ACTIVE stored value still resolves to its option index.
func TestPicklist_ActiveStoredValueSelected(t *testing.T) {
	f := sf.Field{Type: "picklist", Updateable: true,
		PicklistValues: []sf.PicklistValue{
			{Value: "Open", Active: true},
			{Value: "Closed", Active: true},
		},
	}
	e := &picklistEditor{}
	state := e.Init(f, "Closed")
	if state.Cursor != 1 {
		t.Errorf("active stored value 'Closed' should select index 1, got %d", state.Cursor)
	}
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
