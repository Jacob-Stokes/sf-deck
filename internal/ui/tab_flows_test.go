package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestFlowVersionCell(t *testing.T) {
	cases := []struct {
		name string
		f    sf.Flow
		want string
	}{
		{"active only", sf.Flow{ActiveVersionNum: 3, LatestVersionNum: 3}, "v3"},
		{"active, no latest set", sf.Flow{ActiveVersionNum: 3}, "v3"},
		// Edge case: active v3, a newer draft v4 → bracketed.
		{"newer draft", sf.Flow{ActiveVersionNum: 3, LatestVersionNum: 4}, "v3 (v4)"},
		// No active version (never activated) → show the latest number bare.
		{"draft only", sf.Flow{ActiveVersionNum: 0, LatestVersionNum: 2}, "v2"},
		{"none", sf.Flow{}, "—"},
		// Guard: latest somehow behind active (shouldn't happen) → no brackets.
		{"latest behind active", sf.Flow{ActiveVersionNum: 5, LatestVersionNum: 4}, "v5"},
	}
	for _, c := range cases {
		if got := flowVersionCell(c.f); got != c.want {
			t.Errorf("%s: flowVersionCell = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFlowLatestStatusWord(t *testing.T) {
	cases := map[string]string{
		"Draft":        "draft",
		"Obsolete":     "obsolete",
		"InvalidDraft": "invalid draft",
		"Active":       "active",
		"":             "unactivated",
		"Something":    "something", // unknown → lowercased
	}
	for status, want := range cases {
		got := flowLatestStatusWord(sf.Flow{LatestVersionStatus: status})
		if got != want {
			t.Errorf("status %q: got %q, want %q", status, got, want)
		}
	}
}

func TestFlowVersionMismatch(t *testing.T) {
	if !flowVersionMismatch(sf.Flow{ActiveVersionNum: 3, LatestVersionNum: 4}) {
		t.Error("active v3 + latest v4 should be a mismatch")
	}
	if flowVersionMismatch(sf.Flow{ActiveVersionNum: 3, LatestVersionNum: 3}) {
		t.Error("active == latest is not a mismatch")
	}
	if flowVersionMismatch(sf.Flow{ActiveVersionNum: 0, LatestVersionNum: 2}) {
		t.Error("no active version → no mismatch annotation (nothing to compare against)")
	}
}
