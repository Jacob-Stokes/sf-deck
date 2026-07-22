package ui

import "testing"

func TestNextRotatingTagColor(t *testing.T) {
	// Consecutive tags get consecutive palette colours, wrapping.
	first := nextRotatingTagColor(0)
	second := nextRotatingTagColor(1)
	if first == second {
		t.Errorf("consecutive tags should differ: %q == %q", first, second)
	}
	if first != tagPalette[0] {
		t.Errorf("index 0 should be first palette colour, got %q", first)
	}
	// Wraps at palette length.
	if nextRotatingTagColor(len(tagPalette)) != tagPalette[0] {
		t.Error("should wrap to the first colour")
	}
	// Negative-safe.
	if nextRotatingTagColor(-3) != tagPalette[0] {
		t.Error("negative index should clamp to 0")
	}
}
