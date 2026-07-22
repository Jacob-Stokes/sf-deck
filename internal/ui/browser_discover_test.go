package ui

import "testing"

func TestBrowserChoiceIDRoundTrip(t *testing.T) {
	for _, name := range []string{"", "Firefox", "Google Chrome", "Arc"} {
		id := browserChoiceID(name)
		got, ok := parseBrowserChoiceID(id)
		if !ok {
			t.Fatalf("parseBrowserChoiceID(%q) not recognised", id)
		}
		if got != name {
			t.Fatalf("round-trip: got %q want %q", got, name)
		}
	}
	// A non-browser id must not parse as one.
	if _, ok := parseBrowserChoiceID("view"); ok {
		t.Fatal("plain target id wrongly parsed as browser choice")
	}
}

func TestDiscoverBrowsersNeverEmpty(t *testing.T) {
	// Whatever the host, discovery must return at least one choice so
	// the picker is never empty (falls back to the full known list).
	got := discoverBrowsers()
	if len(got) == 0 {
		t.Fatal("discoverBrowsers returned nothing; picker would be empty")
	}
}

func TestBrowserChoiceOptionsLeadWithDefault(t *testing.T) {
	opts, _ := browserChoiceOptions("")
	if len(opts) == 0 || opts[0].Value != "" {
		t.Fatal("first browser option should be the empty (system default) value")
	}
	// current-selection cursor points at the matching row.
	if all := discoverBrowsers(); len(all) > 0 {
		_, cursor := browserChoiceOptions(all[0])
		if cursor != 1 {
			t.Fatalf("cursor for first discovered browser = %d, want 1", cursor)
		}
	}
}

func TestBrowserPrivateFlag(t *testing.T) {
	cases := map[string]struct {
		flag string
		ok   bool
	}{
		"Google Chrome":  {"--incognito", true},
		"Brave Browser":  {"--incognito", true},
		"Microsoft Edge": {"--inprivate", true},
		"Firefox":        {"--private-window", true},
		"Safari":         {"", false}, // no CLI private mode
		"Arc":            {"", false},
		"Unknown":        {"", false},
	}
	for name, want := range cases {
		flag, ok := browserPrivateFlag(name)
		if ok != want.ok || flag != want.flag {
			t.Errorf("browserPrivateFlag(%q) = (%q, %v), want (%q, %v)",
				name, flag, ok, want.flag, want.ok)
		}
	}
}
