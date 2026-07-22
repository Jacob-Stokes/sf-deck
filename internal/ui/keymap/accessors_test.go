package keymap

import "testing"

func TestFirstAndFirstPretty(t *testing.T) {
	if First(nil) != "" {
		t.Error("First(nil) should be empty")
	}
	if got := First([]string{"o", "ctrl+o"}); got != "o" {
		t.Errorf("First = %q, want o", got)
	}
	// FirstPretty substitutes symbol glyphs for a few word-keys.
	if got := FirstPretty([]string{"enter"}); got != "↵" {
		t.Errorf("FirstPretty(enter) = %q, want ↵", got)
	}
	if got := FirstPretty([]string{"up"}); got != "↑" {
		t.Errorf("FirstPretty(up) = %q, want ↑", got)
	}
	// A key with no substitution passes through unchanged.
	if got := FirstPretty([]string{"o"}); got != "o" {
		t.Errorf("FirstPretty(o) = %q, want o", got)
	}
}

func TestCommandByID(t *testing.T) {
	if CommandByID("collect_item") == nil {
		t.Error("collect_item should be a registered command")
	}
	// The split-collect command added recently.
	if CommandByID("collect_item_pick") == nil {
		t.Error("collect_item_pick should be registered")
	}
	if CommandByID("no_such_command_xyz") != nil {
		t.Error("unknown id should return nil")
	}
}

// TestSetByIDAndKeysByID round-trips the reflection-driven rebind path.
func TestSetByIDAndKeysByID(t *testing.T) {
	km := DefaultKeymap()

	// KeysByID returns the current binding.
	if len(km.KeysByID("drill")) == 0 {
		t.Error("drill should have default keys")
	}
	if km.KeysByID("no_such_id") != nil {
		t.Error("unknown id → nil keys")
	}

	// SetByID rebinds; KeysByID reflects it.
	if err := km.SetByID("collect_item", []string{"ctrl+k", "x"}); err != nil {
		t.Fatalf("SetByID: %v", err)
	}
	got := km.KeysByID("collect_item")
	if len(got) != 2 || got[0] != "ctrl+k" || got[1] != "x" {
		t.Errorf("after rebind, keys = %v, want [ctrl+k x]", got)
	}

	// SetByID on an unknown command errors.
	if err := km.SetByID("no_such_id", []string{"z"}); err == nil {
		t.Error("SetByID on unknown command should error")
	}
}
