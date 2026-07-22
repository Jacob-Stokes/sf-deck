package keymap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/keymap"
)

func TestKeymapDefaultsReachable(t *testing.T) {
	km := keymap.DefaultKeymap()
	if !keymap.Matches("o", km.OpenDefault) {
		t.Error("default open_default should accept 'o'")
	}
	if !keymap.Matches("ctrl+o", km.OpenMenu) {
		t.Error("default open_menu should accept 'ctrl+o'")
	}
	if !keymap.Matches("'", km.FocusOrgs) {
		t.Error("default focus_orgs should accept apostrophe")
	}
}

// TestCollectKeysSplit locks in the two distinct collect gestures:
// K quick-collects to the loaded project (the easy, frequent one on the
// easy, no-modifier key), ctrl+k opens the pick-a-project chooser. The
// keys must not overlap (else one gesture would fire both handlers).
func TestCollectKeysSplit(t *testing.T) {
	km := keymap.DefaultKeymap()

	if !keymap.Matches("K", km.CollectItem) {
		t.Error("collect_item should accept K")
	}
	if keymap.Matches("ctrl+k", km.CollectItem) {
		t.Error("collect_item must NOT accept ctrl+k — that's the picker")
	}
	if !keymap.Matches("ctrl+k", km.CollectItemPick) {
		t.Error("collect_item_pick should accept ctrl+k")
	}
	if keymap.Matches("K", km.CollectItemPick) {
		t.Error("collect_item_pick must NOT accept K")
	}
}

func TestKeymapOverrideMerges(t *testing.T) {
	// Write a temp config file; override only open_default + quit.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "keybindings.toml")
	body := `open_default = ["b"]
quit = ["Q"]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Point the loader at tmp by swapping HOME.
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })
	os.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".sf-deck"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(tmp, ".sf-deck", "keybindings.toml")); err != nil {
		t.Fatal(err)
	}

	km, warn := keymap.LoadKeymap()
	if warn != "" {
		t.Errorf("unexpected warning: %s", warn)
	}
	if !keymap.Matches("b", km.OpenDefault) {
		t.Errorf("override should have rebound open_default to 'b', got %v", km.OpenDefault)
	}
	if keymap.Matches("o", km.OpenDefault) {
		t.Errorf("overridden open_default should no longer include 'o', got %v", km.OpenDefault)
	}
	if !keymap.Matches("Q", km.Quit) {
		t.Errorf("override should have rebound quit to 'Q', got %v", km.Quit)
	}
	// Unmodified fields stay default.
	if !keymap.Matches("ctrl+o", km.OpenMenu) {
		t.Errorf("untouched open_menu should stay default, got %v", km.OpenMenu)
	}
}

func TestKeymapMalformedFileWarns(t *testing.T) {
	tmp := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })
	os.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".sf-deck"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "this is {{{ not valid toml"
	if err := os.WriteFile(filepath.Join(tmp, ".sf-deck", "keybindings.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	km, warn := keymap.LoadKeymap()
	if warn == "" {
		t.Error("expected a parse warning for malformed TOML")
	}
	if !strings.Contains(warn, "parse") {
		t.Errorf("warning should mention parse: %q", warn)
	}
	// Defaults should still be usable.
	if !keymap.Matches("o", km.OpenDefault) {
		t.Error("on parse failure defaults should remain")
	}
}
