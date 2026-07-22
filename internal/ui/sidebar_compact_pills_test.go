package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func TestCompactSidebarPillsPredicate(t *testing.T) {
	cases := []struct {
		stacked, forModal, want bool
	}{
		{stacked: false, forModal: false, want: false}, // beside → separate row
		{stacked: true, forModal: false, want: true},   // stacked → inline
		{stacked: true, forModal: true, want: false},   // modal → never compact
		{stacked: false, forModal: true, want: false},
	}
	for _, c := range cases {
		m := Model{}
		m.sidebarStacked = c.stacked
		m.sidebarForModal = c.forModal
		if got := m.compactSidebarPills(); got != c.want {
			t.Errorf("stacked=%v forModal=%v → %v, want %v", c.stacked, c.forModal, got, c.want)
		}
	}
}

func TestKVPanelPillsInlineVsSeparate(t *testing.T) {
	pills := []markPill{{Label: "managed: sf_devops", PillColor: theme.Yellow}}
	rows := []kv{{"id", "01q"}, {"status", "Active"}}

	// Stacked → pills inline on the title line (title + pills share line 0).
	stacked := Model{}
	stacked.sidebarStacked = true
	inlineOut := stacked.kvPanelPills(60, "MyTrigger", pills, rows)
	inlineFirst := strings.Split(inlineOut, "\n")[0]
	if !strings.Contains(inlineFirst, "MyTrigger") || !strings.Contains(inlineFirst, "managed") {
		t.Errorf("stacked: pills not inline on title line: %q", inlineFirst)
	}

	// Beside → pills on their own row (title line has no pill text).
	beside := Model{}
	besideOut := beside.kvPanelPills(60, "MyTrigger", pills, rows)
	besideLines := strings.Split(besideOut, "\n")
	if strings.Contains(besideLines[0], "managed") {
		t.Errorf("beside: pills should NOT be on the title line: %q", besideLines[0])
	}
	// And beside should be one line TALLER than inline (the separate pill row).
	if len(besideLines) <= len(strings.Split(inlineOut, "\n")) {
		t.Errorf("beside (%d lines) should be taller than inline (%d) — separate pill row",
			len(besideLines), len(strings.Split(inlineOut, "\n")))
	}
}

// TestJoinSidebarColumns covers the stacked-mode two-column layout for
// the TAGS + PROJECTS sidebar sections: side by side when both fit,
// vertical fallback (empty return) when a column is too narrow or a
// block is too wide to fit its half.
func TestJoinSidebarColumns(t *testing.T) {
	// Each block mimics sidebarTagSection output: "\n" + header + "\n  " + body.
	left := "\nTAGS\n  a b"
	right := "\nPROJECTS\n  proj"

	// Wide enough: both columns fit → joined, non-empty, and the two
	// headers share a line (side by side).
	got := joinSidebarColumns(left, right, 80)
	if got == "" {
		t.Fatal("wide inner: expected a joined two-column block, got vertical fallback")
	}
	firstContentLine := strings.Split(strings.TrimPrefix(got, "\n"), "\n")[0]
	if !strings.Contains(firstContentLine, "TAGS") || !strings.Contains(firstContentLine, "PROJECTS") {
		t.Errorf("headers not side by side on the first line: %q", firstContentLine)
	}

	// Too narrow to split (colW < 12) → vertical fallback.
	if got := joinSidebarColumns(left, right, 20); got != "" {
		t.Errorf("narrow inner: expected vertical fallback (\"\"), got %q", got)
	}

	// A block wider than its column → vertical fallback.
	wide := "\nTAGS\n  " + strings.Repeat("x", 60)
	if got := joinSidebarColumns(wide, right, 80); got != "" {
		t.Errorf("over-wide block: expected vertical fallback (\"\"), got %q", got)
	}
}
