package ui

// keymap.go — thin wrappers forwarding to the keymap sub-package.
// All keymap logic now lives in internal/ui/keymap/keymap.go.
// These package-level aliases allow existing callers in internal/ui/
// and external callers (cmd/sf-deck/main.go) to keep using the ui
// package's exported names without any changes.

import "github.com/Jacob-Stokes/sf-deck/internal/ui/keymap"

// Keymap is a package-level alias for keymap.Keymap.
type Keymap = keymap.Keymap

// Keys is THE single global keymap, mutated in place by the
// keybindings modal and assigned at startup by main.go after
// LoadKeymap merges the user's TOML overrides on top of defaults.
// Dispatchers read it via the package-internal `matches(key,
// Keys.X)` shape; the keybindings modal calls Keys.SetByID +
// Keys.SaveTOML to edit + persist.
//
// There used to be a parallel `keymap.Keys` global in the keymap
// sub-package; that has been removed because it caused a real bug
// — main.go assigned only ui.Keys, dispatch read ui.Keys, but the
// modal mutated keymap.Keys, so edits were invisible to dispatch
// AND SaveTOML clobbered the user's file with defaults+1-edit.
var Keys = keymap.DefaultKeymap()

// LoadKeymap reads the user's keybindings file and returns a merged Keymap.
func LoadKeymap() (Keymap, string) { return keymap.LoadKeymap() }

// matches is the package-internal key-matching helper.
func matches(key string, slots []string) bool { return keymap.Matches(key, slots) }

// firstPretty returns a display-friendly label for the first binding.
func firstPretty(slots []string) string { return keymap.FirstPretty(slots) }
