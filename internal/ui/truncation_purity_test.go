package ui

// TestNoByteSliceTruncation is the ratchet for the mojibake class
// found in the 2026-06-13 truncation audit: `s[:n] + "…"` splits
// multi-byte runes mid-character and measures bytes, not display
// cells. All single-line truncation must go through ansi.Truncate
// (or uilayout.Truncate / dimLine / sideDim, which wrap it).
//
// Companion contract (enforced socially + by the helpers, not this
// test): HORIZONTAL truncation is for single-line contexts only —
// prose blocks wrap, and VERTICAL truncation happens at the pane's
// height budget with the ⚠ truncated indicator / scroll affordance.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var byteSliceTruncRE = regexp.MustCompile(`\[[^\[\]]*:[^\[\]]*\]\s*\+\s*"…"`)

func TestNoByteSliceTruncation(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if byteSliceTruncRE.MatchString(line) {
				t.Errorf("%s:%d byte-slice truncation (`[:n] + \"…\"`) — use ansi.Truncate (rune/width-safe)", name, i+1)
			}
		}
	}
}
