package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func kp(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	return tea.KeyPressMsg{Text: s}
}

func TestChordEnterAndCancel(t *testing.T) {
	m := Model{}
	m, _ = m.enterChordMode()
	if !m.chordActive {
		t.Fatal("q should enter chord mode")
	}
	// q again cancels.
	m, _ = m.handleChordKey(kp("q"))
	if m.chordActive {
		t.Error("q in chord mode should cancel (exit)")
	}
	// esc cancels too.
	m, _ = m.enterChordMode()
	m, _ = m.handleChordKey(kp("esc"))
	if m.chordActive {
		t.Error("esc should cancel chord mode")
	}
}

func TestChordUnboundLetterExits(t *testing.T) {
	m := Model{}
	m, _ = m.enterChordMode()
	m, _ = m.handleChordKey(kp("z")) // no q-z chord
	if m.chordActive {
		t.Error("unbound letter must exit chord mode (no fall-through)")
	}
}

func TestChordRegistryWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range chordRegistry() {
		if c.Letter == "" || c.Label == "" || c.Do == nil {
			t.Errorf("chord %q malformed", c.Letter)
		}
		if seen[c.Letter] {
			t.Errorf("duplicate chord letter %q", c.Letter)
		}
		seen[c.Letter] = true
	}
}

func TestSemanticSortChordsPresent(t *testing.T) {
	want := map[string]bool{"s": false, "c": false, "l": false}
	for _, c := range chordRegistry() {
		if _, ok := want[c.Letter]; ok {
			want[c.Letter] = true
		}
	}
	for letter, found := range want {
		if !found {
			t.Errorf("semantic-sort chord q-%s missing", letter)
		}
	}
}
