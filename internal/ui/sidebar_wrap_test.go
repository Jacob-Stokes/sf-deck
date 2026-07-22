package ui

// Pins the sidebar text plumbing after the 2026-06-13 field report:
// multi-line blocks were collapsing to one truncated line because
// sideDim ansi.Truncate'd the whole string as a single line, and
// wrap() byte-sliced mid-rune with no word awareness.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrap_WordBoundariesAndIndent(t *testing.T) {
	got := wrap("Prevents a Graduate Application Contact from being reparented", 20)
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected multiple wrapped lines, got %q", got)
	}
	for i, l := range lines {
		if i > 0 && !strings.HasPrefix(l, "  ") {
			t.Errorf("continuation line %d missing hanging indent: %q", i, l)
		}
		if ansi.StringWidth(strings.TrimPrefix(l, "  ")) > 20 {
			t.Errorf("line %d exceeds width: %q", i, l)
		}
		if strings.Contains(l, "Gradu") && !strings.Contains(l, "Graduate") {
			t.Errorf("word split mid-token: %q", l)
		}
	}
}

func TestWrap_RuneSafeOnLongTokens(t *testing.T) {
	// A long unbroken token of multi-byte runes must hard-break
	// without splitting a rune (the old byte-slicer corrupted these).
	in := strings.Repeat("é", 50)
	got := wrap(in, 20)
	if strings.Contains(got, "�") {
		t.Fatalf("replacement char in output: %q", got)
	}
	total := 0
	for _, l := range strings.Split(got, "\n") {
		total += strings.Count(l, "é")
	}
	if total != 50 {
		t.Fatalf("lost runes: %d of 50 survived", total)
	}
}

func TestWrap_PreservesParagraphs(t *testing.T) {
	got := wrap("first para\nsecond para", 40)
	if !strings.Contains(got, "first para") || !strings.Contains(got, "second para") {
		t.Fatalf("paragraph lost: %q", got)
	}
	if len(strings.Split(got, "\n")) != 2 {
		t.Fatalf("expected 2 lines, got %q", got)
	}
}

func TestSideDim_TruncatesPerLineNotWholeBlock(t *testing.T) {
	in := "  " + wrap("Prevents a Graduate Application Contact from being reparented to another Account record", 30)
	out := sideDim(in, 34)
	plain := ansi.Strip(out)
	outLines := strings.Split(plain, "\n")
	inLines := strings.Split(in, "\n")
	if len(outLines) != len(inLines) {
		t.Fatalf("line count changed: %d in, %d out — multi-line block collapsed", len(inLines), len(outLines))
	}
	// The old bug: everything past line one vanished into "…".
	if !strings.Contains(plain, "reparented") {
		t.Fatalf("tail of wrapped text lost: %q", plain)
	}
}
