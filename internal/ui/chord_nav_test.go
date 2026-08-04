package ui

import (
	"strings"
	"testing"
)

// TestNumberRowChordsExist pins the view-independent number-row nav
// chords: q-<n> jumps to the Nth /home subtab (q-1 Landing, q-2 Recently
// Viewed, …). They must always be Available (no per-surface gate) since
// they're navigation.
func TestNumberRowChordsExist(t *testing.T) {
	want := map[string]string{
		"1": "Landing",
		"2": "Recently Viewed",
		"3": "Notifications",
		"4": "Limits",
		"5": "Licenses",
	}
	got := map[string]bool{}
	for _, c := range chordRegistry() {
		if label, ok := want[c.Letter]; ok {
			got[c.Letter] = true
			if c.Available != nil {
				t.Errorf("q-%s (%s) should be always-available (nil Available), navigation isn't view-contingent", c.Letter, label)
			}
			if c.Do == nil {
				t.Errorf("q-%s (%s) has no Do", c.Letter, label)
			}
		}
	}
	for letter, label := range want {
		if !got[letter] {
			t.Errorf("missing number-row chord q-%s (%s)", letter, label)
		}
	}
}

// TestLetterSortChordsUnchanged guards that repurposing the number row
// didn't disturb the view-contingent letter sorts (q-s/c/l —
// name-sort moved from n to l when q-n became the note chord).
func TestLetterSortChordsUnchanged(t *testing.T) {
	sorts := map[string]bool{"s": false, "c": false, "l": false}
	for _, c := range chordRegistry() {
		if _, ok := sorts[c.Letter]; ok {
			sorts[c.Letter] = true
			if c.Available == nil {
				t.Errorf("q-%s is a sort chord and should be view-contingent (non-nil Available)", c.Letter)
			}
		}
	}
	for letter, found := range sorts {
		if !found {
			t.Errorf("sort chord q-%s went missing", letter)
		}
	}
}

// TestChordListModal pins the q-q cheat-sheet: it lists every
// registered chord with its "q <key>" label, and marks view-contingent
// chords unavailable on a bare surface while nav chords stay available.
func TestChordListModal(t *testing.T) {
	m := Model{}
	modal := m.chordListModal()
	if modal.Title == "" {
		t.Fatal("chord modal has no title")
	}
	byKey := map[string]string{}
	for _, r := range modal.Rows {
		if len(r.Label) > 2 && r.Label[:2] == "q " {
			byKey[r.Label[2:]] = r.Body
		}
	}
	// Every registry chord must appear.
	for _, c := range chordRegistry() {
		if _, ok := byKey[c.Letter]; !ok {
			t.Errorf("chord q-%s missing from the q-q cheat-sheet", c.Letter)
		}
	}
	// Nav chords are always available → no "(n/a here)" marker.
	for _, letter := range []string{"1", "2", "3", "4", "5"} {
		if body := byKey[letter]; strings.Contains(body, "n/a") {
			t.Errorf("nav chord q-%s should be available everywhere, got %q", letter, body)
		}
	}
}
