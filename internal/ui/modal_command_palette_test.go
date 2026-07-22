package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
)

func TestPaletteFuzzyMatch(t *testing.T) {
	cases := []struct {
		label, q string
		want     int // expected score band (>= floor)
	}{
		{"/soql", "/soql", 1000},      // exact
		{"/soql", "/so", 800},         // prefix
		{"/soql Saved", "saved", 600}, // contains
		{"/soql Saved", "ssvd", 400},  // subsequence
		{"/objects", "xyz", 0},        // no match
	}
	for _, tc := range cases {
		got := palettematchScore(strings.ToLower(tc.label), strings.ToLower(tc.q))
		if tc.want == 0 && got != 0 {
			t.Errorf("palettematchScore(%q, %q) = %d, want 0", tc.label, tc.q, got)
		}
		if tc.want > 0 && got < tc.want {
			t.Errorf("palettematchScore(%q, %q) = %d, want >= %d", tc.label, tc.q, got, tc.want)
		}
	}
}

func TestBuildPaletteEntriesIncludesNumberedTabs(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer c.Close()
	m := New(c)
	entries := buildPaletteEntries(m)
	// Must include the 9 number-row tabs.
	want := []string{"/home", "/soql", "/objects", "/flows", "/apex", "/components", "/perms", "/reports", "/meta"}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Label] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("palette missing tab entry: %q", w)
		}
	}
}

func TestBuildPaletteEntriesIncludesSubtabs(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	m := New(c)
	entries := buildPaletteEntries(m)
	// SOQL has Editor + Saved + History subtabs.
	wantSubstrings := []string{"/soql Saved", "/soql History"}
	for _, want := range wantSubstrings {
		found := false
		for _, e := range entries {
			if e.Label == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("palette missing subtab entry: %q", want)
		}
	}
}

func TestApplyPaletteFilterEmptyQuery(t *testing.T) {
	cp := &commandPaletteState{
		Entries: []paletteEntry{
			{Label: "/home"},
			{Label: "/soql"},
			{Label: "/objects"},
		},
	}
	cp.applyPaletteFilter()
	if len(cp.Filtered) != 3 {
		t.Errorf("empty query should keep all entries, got %d", len(cp.Filtered))
	}
}

func TestApplyPaletteFilterRanks(t *testing.T) {
	cp := &commandPaletteState{
		Query: "soql",
		Entries: []paletteEntry{
			{Label: "/home"},
			{Label: "/soql"},
			{Label: "/soql Saved"},
			{Label: "/objects"},
		},
	}
	cp.applyPaletteFilter()
	if len(cp.Filtered) != 2 {
		t.Errorf("query 'soql' should match 2 entries, got %d: %+v", len(cp.Filtered), cp.Filtered)
	}
	// Exact match wins ordering.
	if cp.Filtered[0].Label != "/soql" {
		t.Errorf("exact match should rank first, got %q", cp.Filtered[0].Label)
	}
}

func TestApplyPaletteFilterNoMatch(t *testing.T) {
	cp := &commandPaletteState{
		Query: "xyzzy",
		Entries: []paletteEntry{
			{Label: "/home"},
			{Label: "/soql"},
		},
	}
	cp.applyPaletteFilter()
	if len(cp.Filtered) != 0 {
		t.Errorf("nonsense query should match 0 entries, got %d", len(cp.Filtered))
	}
}
