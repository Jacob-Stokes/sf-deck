package ui

import (
	"strings"
	"testing"
)

func TestCompareSpinnerCycles(t *testing.T) {
	// Spinner advances and wraps with the frame.
	a := compareSpinner(0)
	b := compareSpinner(1)
	if a == b {
		t.Errorf("spinner frame 0 and 1 identical (%q) — no motion", a)
	}
	if compareSpinner(0) != compareSpinner(len(compareSpinnerFrames)) {
		t.Error("spinner should wrap modulo frame count")
	}
}

func TestCompareDots(t *testing.T) {
	for f, want := range map[int]string{0: "", 1: ".", 2: "..", 3: "...", 4: ""} {
		if got := compareDots(f); got != want {
			t.Errorf("compareDots(%d) = %q, want %q", f, got, want)
		}
	}
}

func TestCompareAnimatedBarFilledReflectsProgress(t *testing.T) {
	// 0 done → no full cells; all done → all full cells.
	none := compareAnimatedBar(0, 10, 20, 0)
	if strings.Count(none, "█") != 0 {
		t.Errorf("0%% bar has %d filled cells, want 0", strings.Count(none, "█"))
	}
	full := compareAnimatedBar(10, 10, 20, 0)
	if strings.Count(full, "█") != 20 {
		t.Errorf("100%% bar has %d filled cells, want 20", strings.Count(full, "█"))
	}
	// Shimmer moves with the frame in the unfilled track.
	b0 := compareAnimatedBar(2, 10, 20, 0)
	b1 := compareAnimatedBar(2, 10, 20, 1)
	if b0 == b1 {
		t.Error("animated bar identical across frames — shimmer not moving")
	}
}
