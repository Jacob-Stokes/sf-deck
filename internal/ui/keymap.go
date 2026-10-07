package ui

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
// A single global avoids divergence between dispatch, modal edits, and
// SaveTOML persistence.
var Keys = keymap.DefaultKeymap()

// LoadKeymap reads the user's keybindings file and returns a merged Keymap.
func LoadKeymap() (Keymap, string) { return keymap.LoadKeymap() }

func matches(key string, slots []string) bool { return keymap.Matches(key, slots) }

func firstPretty(slots []string) string { return keymap.FirstPretty(slots) }
