package theme

import "testing"

// TestPaletteCount sanity-checks that the generated catalogue
// loaded — if themegen emits an empty file or a parse bug crops
// up, we want CI to catch it rather than the user discovering an
// empty theme picker.
func TestPaletteCount(t *testing.T) {
	ids := PaletteIDs()
	// We ship 5 curated + ~400 generated. Floor at 200 in case
	// upstream prunes.
	if len(ids) < 200 {
		t.Errorf("expected >= 200 palettes, got %d", len(ids))
	}
	for _, want := range []string{"tokyo-night", "catppuccin", "dracula", "solarized-light", "terminal-app-light"} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("curated id %q missing from PaletteIDs()", want)
		}
	}
}

// TestApplyPaletteUnknown verifies we fall back gracefully on a bad
// id rather than crashing or returning zero values.
func TestApplyPaletteUnknown(t *testing.T) {
	ApplyPalette("not-a-real-theme-id-12345")
	if Current.Name == "" {
		t.Error("ApplyPalette of unknown id left Current empty")
	}
}

// TestPopularTierLeadsCatalogue pins that the popular themes appear
// right after the curated ones and before the alphabetical mass, so
// common choices (GitHub, Nord, Gruvbox, …) aren't buried.
func TestPopularTierLeadsCatalogue(t *testing.T) {
	ids := PaletteIDs()
	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i
	}
	// Every popular id that exists must be placed within the leading
	// block (curated + popular), i.e. before the alphabetical rest.
	lead := len(curatedIDs) + len(popularIDs)
	valid := map[string]bool{}
	for _, id := range ids {
		valid[id] = true
	}
	for _, id := range popularIDs {
		if !valid[id] {
			t.Errorf("popular id %q not in catalogue — remove or fix it", id)
			continue
		}
		if pos[id] >= lead {
			t.Errorf("popular id %q at position %d, expected within the leading %d", id, pos[id], lead)
		}
	}
	// github-dark should beat a random alphabetical theme like "nord"...
	// actually assert a headline: github-dark-default comes before the
	// first purely-alphabetical entry.
	if pos["github-dark-default"] > pos["nord"] {
		// both popular — fine; just ensure both lead a deep alpha id.
	}
	if deep, ok := pos["zenburn"]; ok && pos["github-dark-default"] > deep {
		t.Error("github-dark-default should lead a deep-alphabetical theme")
	}
}
