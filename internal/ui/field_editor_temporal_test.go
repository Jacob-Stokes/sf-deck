package ui

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestDateEditor_AcceptsCommonFormats(t *testing.T) {
	e := &dateEditor{}
	f := sf.Field{Type: "date", Updateable: true, Nillable: true}
	cases := []struct {
		in   string
		want string
	}{
		{"2026-05-14", "2026-05-14"},
		{"2026/05/14", "2026-05-14"}, // year-first slash is unambiguous
	}
	for _, tc := range cases {
		state := EditState{Field: f, Raw: tc.in}
		mode, val, err := e.Commit(&state)
		if err != nil {
			t.Errorf("commit %q: %v", tc.in, err)
			continue
		}
		if mode != CommitValue {
			t.Errorf("commit %q: mode=%v", tc.in, mode)
			continue
		}
		if val != tc.want {
			t.Errorf("commit %q: got %v want %v", tc.in, val, tc.want)
		}
	}
}

// W4: ambiguous DD/MM-vs-MM/DD slash dates must be REJECTED, not
// silently parsed as MM/DD (which stored the wrong date for half the
// audience). Only year-first slash and ISO are accepted.
func TestDateEditor_RejectsAmbiguousSlashDates(t *testing.T) {
	e := &dateEditor{}
	f := sf.Field{Type: "date", Updateable: true, Nillable: true}
	for _, in := range []string{"05/14/2026", "14/05/2026", "03/04/2026", "1/2/2026"} {
		state := EditState{Field: f, Raw: in}
		mode, _, err := e.Commit(&state)
		if mode != CommitNone || err == nil {
			t.Errorf("ambiguous slash date %q should be rejected, got mode=%v err=%v", in, mode, err)
		}
	}
}

func TestDateEditor_RejectsGarbage(t *testing.T) {
	e := &dateEditor{}
	f := sf.Field{Type: "date", Updateable: true, Nillable: true}
	state := EditState{Field: f, Raw: "not a date"}
	mode, _, err := e.Commit(&state)
	if mode != CommitNone || err == nil {
		t.Errorf("garbage should refuse, got mode=%v err=%v", mode, err)
	}
	if state.Error == "" {
		t.Errorf("error should be surfaced on state")
	}
}

func TestDatetimeEditor_NormalisesToUTC(t *testing.T) {
	e := &datetimeEditor{}
	f := sf.Field{Type: "datetime", Updateable: true, Nillable: true}
	state := EditState{Field: f, Raw: "2026-05-14T12:00:00+02:00"}
	mode, val, err := e.Commit(&state)
	if err != nil {
		t.Fatal(err)
	}
	if mode != CommitValue {
		t.Fatalf("expected commit, got mode=%v", mode)
	}
	// +02:00 noon → 10:00 UTC.
	want := "2026-05-14T10:00:00.000Z"
	if val != want {
		t.Errorf("got %v want %v", val, want)
	}
}

// W3: a zone-less datetime is the user's LOCAL wall-clock and must be
// converted from time.Local to UTC — not blindly stamped with Z (which
// silently wrote the wrong instant for anyone not on UTC).
func TestDatetimeEditor_ZonelessIsLocalNotUTC(t *testing.T) {
	e := &datetimeEditor{}
	f := sf.Field{Type: "datetime", Updateable: true, Nillable: true}
	state := EditState{Field: f, Raw: "2026-05-14 09:00:00"}
	mode, val, err := e.Commit(&state)
	if err != nil || mode != CommitValue {
		t.Fatalf("commit zoneless datetime: mode=%v err=%v", mode, err)
	}
	// Compute the expected UTC string from the same local instant, so the
	// test is correct in any timezone (including CI's UTC, where it equals
	// the input).
	want := time.Date(2026, 5, 14, 9, 0, 0, 0, time.Local).UTC().Format("2006-01-02T15:04:05.000Z")
	if val != want {
		t.Errorf("zoneless local 09:00 → got %v, want %v (local→UTC)", val, want)
	}
}

func TestBooleanEditor_SpaceToggles(t *testing.T) {
	e := &booleanEditor{}
	f := sf.Field{Type: "boolean", Updateable: true}
	state := e.Init(f, false)
	if state.Raw != "false" {
		t.Errorf("init false: %q", state.Raw)
	}
	e.HandleKey(&state, fakeKey("space"))
	if state.Raw != "true" {
		t.Errorf("after space: %q", state.Raw)
	}
	e.HandleKey(&state, fakeKey("space"))
	if state.Raw != "false" {
		t.Errorf("after second space: %q", state.Raw)
	}
	_, val, _ := e.Commit(&state)
	if val != false {
		t.Errorf("commit false should be bool false, got %v", val)
	}
}

func TestBooleanEditor_LetterTAndF(t *testing.T) {
	e := &booleanEditor{}
	f := sf.Field{Type: "boolean", Updateable: true}
	state := e.Init(f, false)
	e.HandleKey(&state, fakeKey("t"))
	if state.Raw != "true" {
		t.Errorf("t → %q", state.Raw)
	}
	e.HandleKey(&state, fakeKey("f"))
	if state.Raw != "false" {
		t.Errorf("f → %q", state.Raw)
	}
}
