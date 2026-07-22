package ui

import (
	"reflect"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func TestCompactTabIDs(t *testing.T) {
	got := compactTabIDs([]string{"home", "", "flows", "", ""})
	want := []string{"home", "flows"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compactTabIDs = %v, want %v", got, want)
	}
	if got := compactTabIDs([]string{"", "", ""}); len(got) != 0 {
		t.Errorf("all-empty should compact to empty, got %v", got)
	}
}

func TestDefaultPinnedTabIDs(t *testing.T) {
	got := defaultPinnedTabIDs()
	want := []string{"home", "soql", "objects", "flows", "apex", "users", "perms", "system"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("defaultPinnedTabIDs = %v, want %v", got, want)
	}
}

// TestApplyTabBarSlot exercises the assign/move/clear/reset flow end to
// end through the settings round-trip. RebuildTabsForNumbers mutates a
// package-global cache, so we save and restore it around the test.
func TestApplyTabBarSlot(t *testing.T) {
	// Save()s in the apply path must NOT touch the real settings file.
	prevEph := settings.Ephemeral
	settings.Ephemeral = true
	defer func() { settings.Ephemeral = prevEph }()

	prev := TabsForNumbers()
	defer RebuildTabsForNumbers(tabsToIDs(prev))

	s := &settings.Settings{}
	m := &Model{modelServices: modelServices{settings: s}}

	// Start from the defaults so slot reads are deterministic.
	m.applyTabBarReset()
	if got := TabsForNumbers(); len(got) != 8 {
		t.Fatalf("after reset expected 8 tabs, got %d", len(got))
	}

	// Assign /reports to slot 0. It isn't a default pin, so this is a
	// genuine change; home (previously slot 0) should drop off.
	m.applyTabBarSlot(0, "reports")
	got := TabsForNumbers()
	if len(got) == 0 || got[0].String() != "reports" {
		t.Fatalf("slot 0 = %v, want reports at front", tabsToIDs(got))
	}

	// Assigning an already-present tab to a different slot MOVES it
	// (no duplicate). Put /reports (now at slot 0) into slot 3.
	m.applyTabBarSlot(3, "reports")
	got = TabsForNumbers()
	count := 0
	for _, tt := range got {
		if tt.String() == "reports" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("reports should appear exactly once after move, got %d in %v", count, tabsToIDs(got))
	}

	// Clearing a slot removes that tab and closes the gap.
	before := len(TabsForNumbers())
	m.applyTabBarSlot(0, "")
	if after := len(TabsForNumbers()); after != before-1 {
		t.Errorf("clearing a slot should shrink the bar by 1: before=%d after=%d", before, after)
	}
}

func tabsToIDs(tabs []Tab) []string {
	out := make([]string, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, t.String())
	}
	return out
}
