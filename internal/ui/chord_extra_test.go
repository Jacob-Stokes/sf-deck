package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// TestNewListChordsRegistered pins the expanded chord set: the three new
// semantic sorts (b/t/z), the reset-view (x), and the two yank chords
// (y/i). Sorts + yanks are view-contingent (non-nil Available); the
// number-row nav chords stay always-available.
func TestNewListChordsRegistered(t *testing.T) {
	viewContingent := map[string]bool{"s": true, "c": true, "n": true, "l": true, "b": true, "t": true, "z": true, "x": true, "y": true, "i": true, "a": true, "r": true}
	seen := map[string]bool{}
	for _, ch := range chordRegistry() {
		seen[ch.Letter] = true
		if viewContingent[ch.Letter] && ch.Available == nil {
			t.Errorf("q-%s should be view-contingent (non-nil Available)", ch.Letter)
		}
	}
	for letter := range viewContingent {
		if !seen[letter] {
			t.Errorf("view-contingent chord q-%s missing from registry", letter)
		}
	}
}

// TestClearRecentChordAvailability: q-r clears sf-deck's LOCAL recent
// log, so it must only offer itself on views that display that log —
// a bare model (no orgs, no chip surface) must not advertise it.
func TestClearRecentChordAvailability(t *testing.T) {
	var spec *chordSpec
	for _, ch := range chordRegistry() {
		if ch.Letter == "r" {
			c := ch
			spec = &c
			break
		}
	}
	if spec == nil {
		t.Fatal("q-r chord not registered")
	}
	if spec.Available == nil {
		t.Fatal("q-r must be view-contingent")
	}
	if spec.Available(Model{}) {
		t.Error("q-r should be unavailable with no orgs / no recent-displaying view")
	}
}

// TestYankableListColumns drops the flags/gutter + blank columns so the
// yanked table/ids carry only real data columns.
func TestYankableListColumns(t *testing.T) {
	cols := []uilayout.ListColumn{
		{Name: "Name"},
		{Name: "Marks"}, // flags gutter — excluded
		{Name: ""},      // blank — excluded
		{Name: "Id"},
	}
	idx := yankableListColumns(cols)
	if len(idx) != 2 || idx[0] != 0 || idx[1] != 3 {
		t.Errorf("yankableListColumns = %v, want [0 3] (Name, Id)", idx)
	}
}
