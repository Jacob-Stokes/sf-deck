package keymap

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestRegistryMatchesStruct enforces the contract: every Command in
// the registry must have a matching []string field on the Keymap
// struct, and every []string field on the struct (other than the
// LegacyXxx back-compat fields) must have a matching Command.
//
// If this fails the SSOT is broken — DefaultKeymap, mergeKeymap and
// DumpTOML all walk the registry, so a struct field without a
// registry entry is silently never populated, and a registry entry
// pointing at a missing struct field is silently skipped.
func TestRegistryMatchesStruct(t *testing.T) {
	km := Keymap{}
	v := reflect.ValueOf(km)
	tp := v.Type()

	registryByField := map[string]bool{}
	for _, c := range Commands {
		registryByField[c.FieldName] = true
	}

	structFields := map[string]bool{}
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if f.Type.Kind() != reflect.Slice {
			continue
		}
		// Legacy* fields are intentionally not in the registry —
		// they're the back-compat landing pad for renamed TOML keys.
		if strings.HasPrefix(f.Name, "Legacy") {
			continue
		}
		structFields[f.Name] = true
	}

	for field := range structFields {
		if !registryByField[field] {
			t.Errorf("Keymap struct field %q has no Command in commands.go", field)
		}
	}
	for field := range registryByField {
		if !structFields[field] {
			t.Errorf("Command FieldName %q has no matching field on the Keymap struct", field)
		}
	}
}

// knownConflicts is the allowlist of pre-existing key conflicts —
// inherited from before the registry SSOT existed. Each entry is a
// "shadowed | global_command_id | shadowed_command_id | key" tuple
// or "context | a_id | b_id | key" for within-context.
//
// New conflicts MUST NOT be added to this list — fix them by picking
// a different key, splitting the When context, or reordering the
// dispatcher. The list shrinks over time as we fix the legacy ones.
var knownConflicts = map[string]string{
	// global ↔ context shadows that the dispatcher has historically
	// just lived with. Each is a real bug the test is now surfacing
	// honestly; we'll fix them opportunistically.
	"shadow|refresh|obj_perm_read|r":           "perm grid uses different dispatch order",
	"shadow|refresh|fls_toggle_read|r":         "FLS grid uses different dispatch order",
	"shadow|yank_default|soql_yank_cell|y":     "SOQL Editor handler runs before global yank",
	"shadow|yank_menu|soql_yank_column|ctrl+y": "same — SOQL ctrl+y handler runs first",
	// Lifted-from-hardcoded keys. Each context handler runs BEFORE
	// the global one — the dispatcher checks tab/modal context first.
	"shadow|open_default|bundle_open|o":               "tab=bundles handler runs before global open",
	"shadow|open_default|download_open|o":             "downloads handler runs before global open",
	"shadow|yank_default|bundle_yank_path|y":          "bundle-detail handler runs before global yank",
	"shadow|yank_default|download_yank_path|y":        "downloads handler runs before global yank",
	"shadow|refresh|cache_reset_ttl|r":                "cache-settings modal handler runs before global refresh",
	"shadow|global_refresh|search_toggle_mode|ctrl+r": "global-search modal (overlay) handler runs before global ctrl+r; toggle works in-modal, global refresh elsewhere",
	"shadow|tag_column|chip_wizard_mode|ctrl+t":       "chip-wizard modal handler runs before global tag_column",
	"shadow|refresh|bundle_retrieve|r":                "tab=bundles handler runs before global refresh",
	"shadow|refresh|download_reveal|r":                "downloads handler runs before global refresh",
	"shadow|open_tags|subtab_3|#":                     "subtab_3 only fires on subtabbed tabs; # opens tag manager elsewhere",
	// Org Manager modal owns its own dispatcher — runs in handleKey
	// BEFORE the global switch when m.orgManageModal != nil. The
	// keys below only shadow when the modal isn't open, in which
	// case the global handler is the right one.
	"shadow|go_top|org_move_to_group|g":          "modal handler consumes g before global go_top",
	"shadow|prev_view|org_group_reorder_up|[":    "modal handler consumes [ before global prev_view",
	"shadow|next_view|org_group_reorder_down|]":  "modal handler consumes ] before global next_view",
	"shadow|open_dev_projects|org_unset_alias|-": "modal handler consumes - before global open_dev_projects",
	"shadow|search_clear|theme_picker_clear|C":   "theme picker modal handler runs before global search_clear",
	// OpenSettings moved off ctrl+, (not encodeable without CSI-u) onto
	// ctrl+s. Three scoped ctrl+s handlers were already in place; each
	// runs before the global OpenSettings case in the dispatcher.
	"shadow|open_settings|chip_wizard_save|ctrl+s": "chip-wizard modal overlay runs before the global switch",
	"shadow|open_settings|record_edit_save|ctrl+s": "tab=record handler at update_keys.go:658 fires before the OpenSettings case",
}

// TestNoConflictsWithinContext asserts that no two commands in the
// same When context bind the same key, AND no global command
// shadows a context-specific command. Pre-existing conflicts are
// allowlisted in knownConflicts — new conflicts fail the test.
//
// Two commands binding the same key in DIFFERENT contexts is fine
// when the dispatch path knows which context wins (e.g. `e` is
// SOQL edit on /soql, FLS toggle Edit on the FLS grid — handled
// by tab-aware case clauses).
func TestNoConflictsWithinContext(t *testing.T) {
	byContext := map[string][]Command{}
	for _, c := range Commands {
		byContext[c.When] = append(byContext[c.When], c)
	}

	// Phase 1: within-context conflicts. These are unambiguous bugs
	// — two commands bound to the same key in the same context
	// can't both fire.
	for ctx, cmds := range byContext {
		seen := map[string]string{}
		for _, c := range cmds {
			for _, k := range c.Default {
				if other, exists := seen[k]; exists {
					key := ctx + "|" + other + "|" + c.ID + "|" + k
					altKey := ctx + "|" + c.ID + "|" + other + "|" + k
					if _, allow := knownConflicts[key]; allow {
						continue
					}
					if _, allow := knownConflicts[altKey]; allow {
						continue
					}
					t.Errorf("conflict in context %q: key %q bound by both %q and %q (add to knownConflicts only if intentional)",
						ctx, k, other, c.ID)
				}
				seen[k] = c.ID
			}
		}
	}

	// Phase 2: global keys that shadow context-specific bindings.
	globals := byContext["global"]
	for _, g := range globals {
		for _, k := range g.Default {
			for ctx, cmds := range byContext {
				if ctx == "global" {
					continue
				}
				for _, c := range cmds {
					for _, ck := range c.Default {
						if ck != k {
							continue
						}
						key := "shadow|" + g.ID + "|" + c.ID + "|" + k
						if _, allow := knownConflicts[key]; allow {
							continue
						}
						t.Errorf("global command %q (key %q) shadows %q in context %q — global key wins, %q never fires",
							g.ID, k, c.ID, ctx, c.ID)
					}
				}
			}
		}
	}
}

// TestEveryCategoryNonEmpty ensures every Category appears with at
// least one command. Empty categories would just clutter the
// settings page; this guards against typo'd category names.
func TestEveryCategoryNonEmpty(t *testing.T) {
	seen := map[string]int{}
	for _, c := range Commands {
		seen[c.Category]++
	}
	if len(seen) == 0 {
		t.Fatal("no categories in registry")
	}
	// Order them for stable output.
	cats := make([]string, 0, len(seen))
	for k := range seen {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	t.Logf("%d categories with %d commands total", len(cats), len(Commands))
	for _, c := range cats {
		if seen[c] == 0 {
			t.Errorf("category %q has zero commands", c)
		}
	}
}

// TestUniqueIDs ensures no two commands share an ID. IDs are stable
// disk identifiers (TOML keys); duplicates would silently lose
// settings on save.
func TestUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands {
		if seen[c.ID] {
			t.Errorf("duplicate ID %q in registry", c.ID)
		}
		seen[c.ID] = true
	}
}

// TestUniqueFieldNames mirrors UniqueIDs for FieldName — two
// commands pointing at the same struct field would race on which
// Default wins via reflection.
func TestUniqueFieldNames(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands {
		if seen[c.FieldName] {
			t.Errorf("duplicate FieldName %q in registry", c.FieldName)
		}
		seen[c.FieldName] = true
	}
}
