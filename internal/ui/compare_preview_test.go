package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

// lines builds a diff.Line slice from a compact op string: 'e' equal,
// 'd' delete, 'i' insert. e.g. "eedee" → equal,equal,delete,equal,equal.
func lines(ops string) []diff.Line {
	out := make([]diff.Line, len(ops))
	for i, c := range ops {
		switch c {
		case 'e':
			out[i].Op = diff.OpEqual
		case 'd':
			out[i].Op = diff.OpDelete
		case 'i':
			out[i].Op = diff.OpInsert
		}
	}
	return out
}

func TestFirstDiffLine(t *testing.T) {
	cases := []struct {
		ops  string
		want int
	}{
		{"eeee", 0},  // all equal → top
		{"eedee", 2}, // first diff at index 2
		{"dee", 0},   // diff at very top
		{"", 0},      // empty
	}
	for _, c := range cases {
		if got := firstDiffLine(lines(c.ops)); got != c.want {
			t.Errorf("firstDiffLine(%q) = %d, want %d", c.ops, got, c.want)
		}
	}
}

func TestNextDiffHunk(t *testing.T) {
	// hunks at index 2 (d) and 5 (i): "eedeie" → e e d e i e
	ls := lines("eedeie")
	// from start of first hunk (2) → next hunk start (4, the 'i').
	if got := nextDiffHunk(ls, 2); got != 4 {
		t.Errorf("nextDiffHunk from 2 = %d, want 4", got)
	}
	// from the last hunk → stays put (no further hunk).
	if got := nextDiffHunk(ls, 4); got != 4 {
		t.Errorf("nextDiffHunk from last = %d, want 4 (stay)", got)
	}
	// a multi-line hunk: "edddee" → hunk at 1-3, next none.
	ls2 := lines("edddee")
	if got := nextDiffHunk(ls2, 1); got != 1 {
		t.Errorf("nextDiffHunk single multi-line hunk = %d, want 1 (stay)", got)
	}
}

func TestPrevDiffHunk(t *testing.T) {
	ls := lines("eedeie") // hunks at 2 and 4
	// from the second hunk (4) → previous hunk start (2).
	if got := prevDiffHunk(ls, 4); got != 2 {
		t.Errorf("prevDiffHunk from 4 = %d, want 2", got)
	}
	// from the first hunk → stays put (no earlier hunk).
	if got := prevDiffHunk(ls, 2); got != 2 {
		t.Errorf("prevDiffHunk from first = %d, want 2 (stay)", got)
	}
	// multi-line hunk start is returned, not its middle: "edddee".
	ls2 := lines("edddee") // hunk spans 1..3
	if got := prevDiffHunk(ls2, 5); got != 1 {
		t.Errorf("prevDiffHunk to multi-line hunk = %d, want 1 (hunk start)", got)
	}
}
