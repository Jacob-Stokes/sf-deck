package settings

import "testing"

func TestChordSortModifiedDesc(t *testing.T) {
	// Default (unset) is NEWEST-FIRST (true) — opposite of DefaultSort.
	s := &Settings{}
	if !s.ChordSortModifiedDesc() {
		t.Error("unset chord sort should default to desc (newest first)")
	}
	// Explicit asc flips it.
	s.SetChordSortModified("asc")
	if s.ChordSortModifiedDesc() {
		t.Error("asc should give oldest-first (false)")
	}
	// Explicit desc.
	s.SetChordSortModified("desc")
	if !s.ChordSortModifiedDesc() {
		t.Error("desc should give newest-first (true)")
	}
	// nil-safe: defaults to desc.
	var n *Settings
	if !n.ChordSortModifiedDesc() {
		t.Error("nil should default to desc")
	}
}
