package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestTextEditor_StringRoundTrip(t *testing.T) {
	e := &textEditor{kind: textKindString}
	f := sf.Field{Name: "Name", Type: "string", Updateable: true, Nillable: true}
	state := e.Init(f, "Acme Corp")
	if state.Raw != "Acme Corp" {
		t.Errorf("Init lost value: %q", state.Raw)
	}
	// Edit: append " Inc"
	for _, r := range " Inc" {
		consumed, _ := e.HandleKey(&state, fakeKey(string(r)))
		if !consumed {
			t.Errorf("HandleKey didn't consume %q", string(r))
		}
	}
	mode, val, err := e.Commit(&state)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if mode != CommitValue {
		t.Errorf("expected CommitValue, got %v", mode)
	}
	if val != "Acme Corp Inc" {
		t.Errorf("commit value wrong: %q", val)
	}
}

func TestTextEditor_IntRejectsNonNumeric(t *testing.T) {
	e := &textEditor{kind: textKindInt}
	f := sf.Field{Name: "Count", Type: "int", Updateable: true, Nillable: true}
	state := e.Init(f, 42.0)
	if state.Raw != "42" {
		t.Errorf("Init lost int value: %q", state.Raw)
	}
	// Replace 42 with "abc"
	for i := 0; i < 2; i++ {
		e.HandleKey(&state, fakeKey("backspace"))
	}
	for _, r := range "abc" {
		e.HandleKey(&state, fakeKey(string(r)))
	}
	mode, _, err := e.Commit(&state)
	if mode != CommitNone {
		t.Errorf("expected CommitNone, got %v", mode)
	}
	if err == nil {
		t.Errorf("expected commit error")
	}
	if state.Error == "" {
		t.Errorf("expected EditState.Error to be set")
	}
}

func TestTextEditor_NillableEmptyCommitsNull(t *testing.T) {
	e := &textEditor{kind: textKindString}
	f := sf.Field{Name: "Phone", Type: "string", Updateable: true, Nillable: true}
	state := e.Init(f, "555-1212")
	// Wipe.
	for state.Raw != "" {
		e.HandleKey(&state, fakeKey("backspace"))
	}
	mode, val, err := e.Commit(&state)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if mode != CommitNull {
		t.Errorf("expected CommitNull, got %v", mode)
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
}

func TestTextEditor_NonNillableEmptyRejects(t *testing.T) {
	e := &textEditor{kind: textKindString}
	f := sf.Field{Name: "Subject", Type: "string", Updateable: true, Nillable: false}
	state := e.Init(f, "")
	mode, _, err := e.Commit(&state)
	if mode != CommitNone {
		t.Errorf("non-nillable empty should not commit, got %v", mode)
	}
	if err == nil {
		t.Errorf("expected required-field error")
	}
}

func TestReadOnlyEditor_RefusesEverything(t *testing.T) {
	e := &readOnlyEditor{reason: "test"}
	f := sf.Field{Name: "CreatedDate", Type: "datetime", Updateable: false}
	if e.CanEdit(f) {
		t.Errorf("read-only editor should never permit edit")
	}
	state := e.Init(f, nil)
	mode, _, err := e.Commit(&state)
	if mode != CommitNone || err == nil {
		t.Errorf("read-only commit should refuse, got mode=%v err=%v", mode, err)
	}
}

func TestRegistry_StringResolves(t *testing.T) {
	if got := resolveFieldEditor(sf.Field{Type: "string"}); got == nil {
		t.Errorf("string editor not registered")
	}
	if got := resolveFieldEditor(sf.Field{Type: "unknown_type_xyz"}); got != nil {
		t.Errorf("unknown type should resolve to nil, got %T", got)
	}
}

// fakeKey builds a minimal tea.KeyMsg whose String() returns the
// requested key string. Sufficient for the editors which only
// inspect msg.String().
func fakeKey(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	switch s {
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Text: s}
}
