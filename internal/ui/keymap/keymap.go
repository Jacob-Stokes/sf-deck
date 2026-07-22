package keymap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Keymap is the single source of truth for every global keybinding in
// the TUI. Every case in the input-handling code looks up its key here
// rather than hard-coding a string, so:
//
//  1. Changing a binding is one edit in one file.
//  2. Users can override bindings in a TOML file without recompiling.
//  3. Status-bar / help text reads its labels from this struct, so the
//     displayed hints never go out of sync with the actual bindings.
//
// Each field holds a slice of strings — a binding can have multiple keys
// that all fire it (e.g. MoveUp defaults to both `k` and `up`).
//
// Key string syntax matches Bubble Tea's tea.KeyMsg.String():
//
//	single letters        — "a", "A", "?"
//	named keys            — "enter", "esc", "tab", "backspace", "up",
//	                         "down", "left", "right", "space"
//	modifiers             — "ctrl+o", "ctrl+c", "ctrl+u"
//
// The `toml:` tags are the on-disk config field names. They use
// snake_case for readability in user-edited TOML.
type Keymap struct {
	// Process / panes
	Quit                 []string `toml:"quit"`
	FocusOrgs            []string `toml:"focus_orgs"`      // jumps to + selects the Orgs utility
	FocusBookmarks       []string `toml:"focus_bookmarks"` // jumps to + selects the Bookmarks utility
	TogglePane           []string `toml:"toggle_pane"`
	Back                 []string `toml:"back"` // esc in non-nested views
	ToggleSidebar        []string `toml:"toggle_sidebar"`
	ToggleSidebarStacked []string `toml:"toggle_sidebar_stacked"` // ctrl+\ — switches sidebar between right-of-main and below-main (2/3 main, 1/3 sidebar)
	ToggleLeft           []string `toml:"toggle_left"`            // left-rail widget pane
	Help                 []string `toml:"help"`                   // opens the per-view info modal
	InspectPanel         []string `toml:"inspect_panel"`          // opens the full sidebar/context info in a modal (escape hatch when the panel truncates)

	// List nav
	MoveUp        []string `toml:"move_up"`
	MoveDown      []string `toml:"move_down"`
	JumpUp        []string `toml:"jump_up"`   // ctrl+up / K — finer than page, coarser than single-step
	JumpDown      []string `toml:"jump_down"` // ctrl+down / J
	PageUp        []string `toml:"page_up"`
	PageDown      []string `toml:"page_down"`
	GoTop         []string `toml:"go_top"`
	GoBottom      []string `toml:"go_bottom"`
	Drill         []string `toml:"drill"` // enter on a list row
	Refresh       []string `toml:"refresh"`
	GlobalRefresh []string `toml:"global_refresh"` // re-fetch all loaded data for active org

	// Tab switching (1..9 by default, mapping to the TabsForNumbers
	// list in tab.go — Tab1 is index 0 of that slice).
	Tab1 []string `toml:"tab_1"`
	Tab2 []string `toml:"tab_2"`
	Tab3 []string `toml:"tab_3"`
	Tab4 []string `toml:"tab_4"`
	Tab5 []string `toml:"tab_5"`
	Tab6 []string `toml:"tab_6"`
	Tab7 []string `toml:"tab_7"`
	Tab8 []string `toml:"tab_8"`
	Tab9 []string `toml:"tab_9"`
	Tab0 []string `toml:"tab_0"`

	// Subtab switching (shift+1..9). When the active tab has multiple
	// subtabs, the number row "shifts down" to address them: top-tab
	// pills lose their leading number, subtab pills gain it. shift+N
	// jumps directly to subtab index N-1.
	//
	// Two binding shapes are accepted: the typed shifted character on
	// a US layout ("!" / "@" / "#" / "$" / "%" / "^" / "&" / "*" /
	// "(") AND the modifier-aware form ("shift+1" .. "shift+9") for
	// terminals with the kitty / iTerm modify-other-keys protocols.
	Subtab1 []string `toml:"subtab_1"`
	Subtab2 []string `toml:"subtab_2"`
	Subtab3 []string `toml:"subtab_3"`
	Subtab4 []string `toml:"subtab_4"`
	Subtab5 []string `toml:"subtab_5"`
	Subtab6 []string `toml:"subtab_6"`
	Subtab7 []string `toml:"subtab_7"`
	Subtab8 []string `toml:"subtab_8"`
	Subtab9 []string `toml:"subtab_9"`
	// Subtab0 opens the subtab overflow modal — same shape as the
	// top-level "0 More…" tab. Only meaningful when the active tab
	// has overflow subtabs; otherwise the keypress is a no-op.
	Subtab0 []string `toml:"subtab_0"`

	// Open / yank
	OpenDefault []string `toml:"open_default"`
	OpenMenu    []string `toml:"open_menu"`
	YankDefault []string `toml:"yank_default"`
	YankMenu    []string `toml:"yank_menu"`

	// Search
	SearchStart      []string `toml:"search_start"`
	SearchClear      []string `toml:"search_clear"`
	GlobalSearch     []string `toml:"global_search"`      // cross-cutting modal search: sobjects, fields, flows, …
	SearchToggleMode []string `toml:"search_toggle_mode"` // inside the global-search modal: toggle between metadata + records modes

	// FLS grid.
	FLSToggleRead []string `toml:"fls_toggle_read"` // toggle Read on cursored field
	FLSToggleEdit []string `toml:"fls_toggle_edit"` // toggle Edit (implies Read)

	// Object permissions grid (TabPermParentDetail + SubtabParentObjects).
	ObjPermRead      []string `toml:"obj_perm_read"`       // toggle Read
	ObjPermCreate    []string `toml:"obj_perm_create"`     // toggle Create
	ObjPermEdit      []string `toml:"obj_perm_edit"`       // toggle Edit
	ObjPermDelete    []string `toml:"obj_perm_delete"`     // toggle Delete
	ObjPermViewAll   []string `toml:"obj_perm_view_all"`   // toggle ViewAllRecords
	ObjPermModifyAll []string `toml:"obj_perm_modify_all"` // toggle ModifyAllRecords

	// System permissions grid (TabPermParentDetail + SubtabParentSystem).
	SysPermToggle []string `toml:"sys_perm_toggle"` // toggle the selected system perm

	// View-chip navigation on two-zone tabs (Objects, Records).
	PrevView        []string `toml:"prev_view"`         // cycle backward through chips
	NextView        []string `toml:"next_view"`         // cycle forward through chips
	ToggleDashboard []string `toml:"toggle_dashboard"`  // collapse/expand dashboard
	ToggleQueryLine []string `toml:"toggle_query_line"` // hide/show the SOQL query line on records surfaces

	// Global settings modal (= to open).
	OpenSettings []string `toml:"open_settings"`

	// Debug panel showing the ring-buffer of recent API calls.
	OpenAPILog []string `toml:"open_api_log"`

	// Downloads overlay listing in-flight + recent exports.
	OpenDownloads []string `toml:"open_downloads"`

	// Toggle the chip-strip mode (sf-deck chips ↔ Salesforce list
	// views). Active in any records-shaped tab.
	//
	// Field is still named LensModeToggle for back-compat with the
	// original (lens-vocabulary) keybinding code. The TOML key is
	// "chip_mode_toggle" going forward; the legacy "lens_mode_toggle"
	// key still loads (LegacyLensModeToggle below) and the merge
	// pass copies it into the canonical slot.
	LensModeToggle       []string `toml:"chip_mode_toggle"`
	LegacyLensModeToggle []string `toml:"lens_mode_toggle,omitempty"`

	// Open the chip manager modal (list / new / edit / delete /
	// import-from-Salesforce). Same rename as ChipModeToggle.
	OpenLensManager       []string `toml:"open_chip_manager"`
	LegacyOpenLensManager []string `toml:"open_lens_manager,omitempty"`

	// Open the "+ N more…" chip overflow modal — non-favourite
	// chips for the current surface. Picks land on the strip as
	// the transient slot; press F to pin the picked chip to the
	// favourites group, or pick another to replace.
	OpenChipOverflow []string `toml:"open_chip_overflow"`

	// Toggle favourite on the chip currently active on the strip
	// (the cursored chip on /objects + /flows; ListViewCur on
	// records). Promotes a transient chip to a favourite, or
	// demotes a non-locked favourite back to the overflow modal.
	// LockedFavourite chips (Recent on records, All on objects /
	// flows) silently refuse the toggle.
	ToggleChipFavourite []string `toml:"toggle_chip_favourite"`

	// Subtab navigation (when a drilled-in tab has multiple subtabs).
	PrevSubtab []string `toml:"prev_subtab"` // cycle to previous subtab
	NextSubtab []string `toml:"next_subtab"` // cycle to next subtab

	// Per-tab extras
	FilterCycle       []string `toml:"filter_cycle"` // sobjects: manageable/all/custom — LEGACY, now handled via PrevView/NextView
	SOQLEdit          []string `toml:"soql_edit"`
	SOQLToggleTooling []string `toml:"soql_toggle_tooling"`
	SOQLToggleBulk    []string `toml:"soql_toggle_bulk"` // ctrl+b — route this run through Bulk API 2.0 (1 job per query vs ~1 call per 2000 rows on REST)
	SOQLSave          []string `toml:"soql_save"`        // S — save / update current editor query
	SOQLSaveAs        []string `toml:"soql_save_as"`     // ctrl+n — always save as new
	SOQLDelete        []string `toml:"soql_delete"`      // D — delete cursored Library query
	SOQLDuplicate     []string `toml:"soql_duplicate"`   // c — duplicate cursored saved query
	SOQLRename        []string `toml:"soql_rename"`      // R — rename cursored saved query
	SOQLExport        []string `toml:"soql_export"`      // x — export current results to xlsx/csv/json
	RecordsExport     []string `toml:"records_export"`   // x — export the active records-list chip's rows to xlsx/csv/json
	SOQLYankCell      []string `toml:"soql_yank_cell"`   // y — yank cursored cell (results pane)
	SOQLYankRow       []string `toml:"soql_yank_row"`    // Y — yank cursored row as TSV (results pane)
	SOQLYankColumn    []string `toml:"soql_yank_column"` // ctrl+y — yank column as ('id1','id2',…) IN-clause (results pane)

	// /record — inline field edit.
	RecordEditField     []string `toml:"record_edit_field"`      // e — enter edit mode on the cursored field
	RecordEditSave      []string `toml:"record_edit_save"`       // ctrl+s — PATCH all dirty fields
	RecordEditCancelAll []string `toml:"record_edit_cancel_all"` // ctrl+X — discard every dirty edit

	// /exec — anonymous Apex.
	ExecEdit           []string `toml:"exec_edit"`            // e — focus the editor textarea
	ExecExternalEditor []string `toml:"exec_external_editor"` // ctrl+e — open $EDITOR with the current body
	ExecToggleLog      []string `toml:"exec_toggle_log"`      // ctrl+d — toggle debug-log capture for the next run
	ExecSave           []string `toml:"exec_save"`            // S — save / update current snippet
	ExecSaveAs         []string `toml:"exec_save_as"`         // ctrl+n — always save as new
	ExecDelete         []string `toml:"exec_delete"`          // D — delete cursored saved snippet
	ExecDuplicate      []string `toml:"exec_duplicate"`       // c — duplicate cursored saved snippet
	ExecRename         []string `toml:"exec_rename"`          // R — rename cursored saved snippet

	// Reports-specific.
	ReportExport []string `toml:"report_export"` // x — export with saved post-processors (unified with dev-project export)

	// Dev-projects.
	NewProject         []string `toml:"new_project"`          // n — new dev project on /dev-projects
	EditProject        []string `toml:"edit_project"`         // e — rename / edit description on /dev-projects
	DeleteProject      []string `toml:"delete_project"`       // d — delete; refuses when project has items (cascade with shift+D)
	DeleteProjectForce []string `toml:"delete_project_force"` // shift+D — force-delete, cascades items
	CollectItem        []string `toml:"collect_item"`         // K — quick-collect cursored item to the loaded project (toggle); picker fallback when none loaded
	CollectItemPick    []string `toml:"collect_item_pick"`    // ctrl+k — collect cursored item, always opening the picker to choose a project
	OpenDevProjects    []string `toml:"open_dev_projects"`    // - (minus) — open the master dev-projects list. Right-rail nav pill.
	OpenTags           []string `toml:"open_tags"`            // ` — open the tag manager. Right-rail nav pill. (# belongs to subtab_3: US shift+3.)
	ExportProject      []string `toml:"export_project"`       // x — create a bundle from the cursored / drilled-in dev project (full sfdx project + retrieve from org) OR export the item list as csv / xlsx / json
	OpenBundles        []string `toml:"open_bundles"`         // b — open the /bundles list for the drilled-in dev project (sfdx project directories tied to it)
	ToggleProjectScope []string `toml:"toggle_project_scope"` // \ — on /dev-project-detail, toggle "this org / all orgs" view of the project's items
	LoadOrgProject     []string `toml:"load_org_project"`     // _ — context-aware: on /dev-projects toggles load/unload for the active org. Anywhere else jumps to the loaded project's detail (and shows as a right-rail pill when a project is loaded). The toml key is unchanged for back-compat with existing user keymap files; semantics are now "load shortcut" rather than "load org-project".
	ToggleProjectMode  []string `toml:"toggle_project_mode"`  // p (on /reports) — toggle the synthetic project-pin in the breadcrumb strip

	// List-table column controls — apply to records / SOQL / reports
	// run / Salesforce list-view results (every surface that uses the
	// shared list-table primitive). All no-op on non-list-table surfaces.
	//
	// We use < > for resize (visually "smaller / bigger") and , . for
	// column scroll (video-player convention: step back / step forward).
	// [ ] are reserved for subtab cycling, so they can't be used here.
	EditCurrentView []string `toml:"edit_current_view"` // e (in column-mode on a view surface) — open the wizard for the active view
	ColShrink       []string `toml:"col_shrink"`        // < — narrow current column
	ColGrow         []string `toml:"col_grow"`          // > — widen current column
	ColSnapMin      []string `toml:"col_snap_min"`      // { — snap column to its header-only width
	ColSnapMax      []string `toml:"col_snap_max"`      // } — snap column to its widest visible cell (fit-to-content)
	ColResetWidths  []string `toml:"col_reset_widths"`  // W — clear user-pinned widths for the active table
	ColScrollL      []string `toml:"col_scroll_l"`      // ← / h / , — scroll columns left + advance column cursor
	ColScrollR      []string `toml:"col_scroll_r"`      // → / l / . — scroll columns right + advance column cursor
	ColSort         []string `toml:"col_sort"`          // s — sort by cursored column (asc → desc → off)
	ColSortClear    []string `toml:"col_sort_clear"`    // S — clear any active sort
	ZenMode         []string `toml:"zen_mode"`          // z — fullscreen toggle for list-table views
	Paginate        []string `toml:"paginate"`          // P — toggle paginated mode (pgup/pgdown advance pages instead of scrolling)
	Tag             []string `toml:"tag"`               // t — open tag picker for the cursored item
	TagAll          []string `toml:"tag_all"`           // T — open tag picker for EVERY visible row (bulk)
	TagColumn       []string `toml:"tag_column"`        // ctrl+t — toggle the tag-dot gutter on/off across all lists
	ProjectColumn   []string `toml:"project_column"`    // ctrl+p — toggle the project-membership gutter on/off across all lists
	FlagColumn      []string `toml:"flag_column"`       // ctrl+g — cycle the FLAGS column (full / letter / hidden)

	// Command palette — fuzzy-find modal over every Tab + every
	// command in the registry. Bound to ; and ctrl+k by default;
	// the registry pattern means new commands get reachable
	// through the palette automatically.
	CommandPalette []string `toml:"command_palette"`

	// Bundles list (tab=bundles).
	BundleOpen     []string `toml:"bundle_open"`     // o — open bundle directory
	BundleUnlink   []string `toml:"bundle_unlink"`   // d — unlink bundle from project (leaves dir on disk)
	BundleRetrieve []string `toml:"bundle_retrieve"` // r — sf project retrieve into the bundle dir
	BundleDeploy   []string `toml:"bundle_deploy"`   // D — sf project deploy from the bundle dir

	// Bundle detail (tab=bundle-detail).
	BundleYankPath    []string `toml:"bundle_yank_path"`    // y — yank bundle path
	BundleValidate    []string `toml:"bundle_validate"`     // v — validate-only deploy
	BundleRefreshDiff []string `toml:"bundle_refresh_diff"` // R — force re-fetch the cached diff

	// Flow detail (tab=flow-detail — the versions view). Metadata
	// writes, gated by the org's safety level.
	FlowRename        []string `toml:"flow_rename"`         // e — rename the flow's display label
	FlowVersionDelete []string `toml:"flow_version_delete"` // D — delete the cursored inactive version

	// Downloads modal + /home Downloads subtab (modal=downloads, tab=home-downloads).
	DownloadOpen     []string `toml:"download_open"`      // o — open the exported file
	DownloadReveal   []string `toml:"download_reveal"`    // r — reveal in Finder
	DownloadYankPath []string `toml:"download_yank_path"` // y — yank file path
	DownloadRemove   []string `toml:"download_remove"`    // d — remove from history (Done/Failed only)

	// Theme picker (modal=theme-picker).
	ThemePickerFavourite []string `toml:"theme_picker_favourite"` // f / F — toggle favourite
	ThemePickerClear     []string `toml:"theme_picker_clear"`     // C — clear search

	// Cache settings (modal=cache-settings).
	CacheResetTTL []string `toml:"cache_reset_ttl"` // r — reset cursored TTL to default

	// Chip wizard internals (modal=chip-wizard).
	ChipWizardLookup []string `toml:"chip_wizard_lookup"` // ctrl+l — open lookup
	ChipWizardDelete []string `toml:"chip_wizard_delete"` // ctrl+x — delete current row
	ChipWizardSave   []string `toml:"chip_wizard_save"`   // ctrl+s — save the chip
	ChipWizardMode   []string `toml:"chip_wizard_mode"`   // ctrl+t — toggle row mode

	// Orgs panel (focus=orgs). The rail itself owns only fold/expand
	// + the modal trigger; every edit action lives in the org-manage
	// modal so the narrow rail stays a quick-nav surface.
	OrgGroupToggle []string `toml:"org_group_toggle"` // space — collapse/expand cursored group
	OrgManageOpen  []string `toml:"org_manage_open"`  // ctrl+e — open the org-management modal

	// Org-management modal (modal=org-manage). All edit actions live
	// here so the keybindings are visible alongside a roomy view of
	// the grouped org tree.
	OrgAddOrg           []string `toml:"org_add_org"`            // A — start the add-org auth flow
	OrgGroupCreate      []string `toml:"org_group_create"`       // n — new group
	OrgGroupRename      []string `toml:"org_group_rename"`       // R — rename cursored group
	OrgGroupDelete      []string `toml:"org_group_delete"`       // x — delete cursored group
	OrgGroupReorderUp   []string `toml:"org_group_reorder_up"`   // [ — move cursored group up
	OrgGroupReorderDn   []string `toml:"org_group_reorder_down"` // ] — move cursored group down
	OrgMoveUp           []string `toml:"org_move_up"`            // < — move cursored org up
	OrgMoveDown         []string `toml:"org_move_down"`          // > — move cursored org down
	OrgMoveToGroup      []string `toml:"org_move_to_group"`      // g — move-to-group picker
	OrgDisconnect       []string `toml:"org_disconnect"`         // D — sf org logout
	OrgReauth           []string `toml:"org_reauth"`             // L — sf org login web (same alias, re-auth)
	OrgSetDefault       []string `toml:"org_set_default"`        // * — sf config set target-org
	OrgSetDefaultDevHub []string `toml:"org_set_default_devhub"` // ^ — sf config set target-dev-hub
	OrgPinStartup       []string `toml:"org_pin_startup"`        // P — pin sf-deck-level startup org
	OrgCycleSafety      []string `toml:"org_cycle_safety"`       // s — cycle safety level on cursor org
	OrgSetAlias         []string `toml:"org_set_alias"`          // = — sf alias set
	OrgUnsetAlias       []string `toml:"org_unset_alias"`        // - — sf alias unset
}

// Note: there is intentionally NO package-level `Keys` global here.
// The single source of truth is `ui.Keys` in
// internal/ui/keymap.go — main.go assigns the loaded keymap to that
// variable at startup, the dispatcher reads it, and the keybindings
// modal mutates it in place. Having a second `keymap.Keys` global
// here previously caused a real bug: the modal mutated a copy that
// dispatch never saw, AND its SaveTOML clobbered the user's file
// with the defaults + their one edit.

// Matches reports whether `key` (usually tea.KeyMsg.String()) is bound
// to any of the slots.
func Matches(key string, slots []string) bool {
	for _, s := range slots {
		if s == key {
			return true
		}
	}
	return false
}

// First returns the first binding in a slot, for use in status-bar /
// help text. "" if the slot is empty (user deleted all bindings).
func First(slots []string) string {
	if len(slots) == 0 {
		return ""
	}
	return slots[0]
}

// FirstPretty is like First() but with a few display-friendly
// substitutions so the status bar doesn't render "enter" or "tab" as
// word-strings when a symbol is more compact.
func FirstPretty(slots []string) string {
	s := First(slots)
	switch s {
	case "enter":
		return "↵"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "space":
		return "␣"
	case "ctrl+o":
		return "^o"
	case "ctrl+y":
		return "^y"
	case "ctrl+c":
		return "^c"
	case "ctrl+u":
		return "^u"
	case "ctrl+w":
		return "^w"
	case "ctrl+r":
		return "^r"
	case "ctrl+\\":
		return "^\\"
	case "tab":
		return "⇥"
	case "shift+tab":
		return "⇧⇥"
	}
	return s
}

// SetByID updates one command's keys on the receiver, looked up via
// the Commands registry. Returns an error when the ID doesn't match
// a registered command. Used by the keybindings settings page to
// apply user edits.
//
// SetByID does not validate against conflicts — the caller is
// responsible for running the conflict check before saving. The
// settings modal does that at apply time so users see warnings
// before they commit.
func (km *Keymap) SetByID(id string, keys []string) error {
	c := CommandByID(id)
	if c == nil {
		return fmt.Errorf("keymap: unknown command %q", id)
	}
	v := reflect.ValueOf(km).Elem()
	f := v.FieldByName(c.FieldName)
	if !f.IsValid() || !f.CanSet() {
		return fmt.Errorf("keymap: field %q not settable", c.FieldName)
	}
	out := append([]string(nil), keys...)
	f.Set(reflect.ValueOf(out))
	return nil
}

// KeysByID returns the active keys for a command. Returns nil when
// the ID isn't registered (caller treats nil as "unbound").
func (km Keymap) KeysByID(id string) []string {
	c := CommandByID(id)
	if c == nil {
		return nil
	}
	v := reflect.ValueOf(km)
	f := v.FieldByName(c.FieldName)
	if !f.IsValid() || f.Kind() != reflect.Slice {
		return nil
	}
	return f.Interface().([]string)
}

// SaveTOML writes the receiver's effective keymap to ConfigPath().
// Creates the parent directory as needed. Returns an error on IO
// failure. Used by the keybindings settings page.
func (km Keymap) SaveTOML() error {
	path := ConfigPath()
	if path == "" {
		return fmt.Errorf("keymap: no config path (HOME unset?)")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("keymap: mkdir %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(km.DumpTOML()), 0o644)
}

// ConfigPath is the on-disk location the TOML config is read from.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sf-deck", "keybindings.toml")
}

// LoadKeymap reads the user's keybindings file (if any) and returns a
// merged Keymap: any slot declared in the file replaces the default;
// anything absent stays as-is. Also returns a non-fatal warning when
// the file exists but couldn't be parsed — the caller can surface that
// via the flash banner.
//
// If the file doesn't exist, LoadKeymap silently returns the defaults
// (no error, no warning).
func LoadKeymap() (Keymap, string) {
	km := DefaultKeymap()
	path := ConfigPath()
	if path == "" {
		return km, ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return km, ""
		}
		return km, fmt.Sprintf("keybindings: %v (defaults used)", err)
	}
	var override Keymap
	if _, err := toml.NewDecoder(bytes.NewReader(b)).Decode(&override); err != nil {
		return km, fmt.Sprintf("keybindings.toml parse error: %v", err)
	}
	mergeKeymap(&km, override)
	return km, ""
}

func formatStringSlice(xs []string) string {
	sort.Strings(append([]string(nil), xs...)) // stable display
	var parts []string
	for _, x := range xs {
		parts = append(parts, `"`+tomlEscape(x)+`"`)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// DefaultKeymap returns a fresh Keymap populated with the
// out-of-the-box bindings declared in the Commands registry.
//
// Single source of truth: the Commands slice in commands.go is the
// authoritative declaration of every keybinding. This function
// walks that slice, sets each Keymap field by reflection (via the
// FieldName mapping), and returns the populated struct. New
// commands appear in the keymap automatically — no edit needed
// here.
func DefaultKeymap() Keymap {
	var km Keymap
	v := reflect.ValueOf(&km).Elem()
	for _, c := range Commands {
		f := v.FieldByName(c.FieldName)
		if !f.IsValid() || !f.CanSet() {
			continue // FieldName mismatch — TestRegistryMatchesStruct catches this
		}
		// Always allocate a fresh slice so mutations on one Keymap
		// don't bleed into others (defensive — Default is shared
		// across all Keymap instances otherwise).
		out := append([]string(nil), c.Default...)
		f.Set(reflect.ValueOf(out))
	}
	return km
}

// mergeKeymap copies every non-nil slice from src into dst, walking
// the Commands registry so new fields are picked up automatically.
// Also honors LegacyTOMLKey: if a user's TOML still uses the old
// name, the value lands on the legacy field (kept on the struct for
// back-compat) and we fold it into the canonical field here.
func mergeKeymap(dst *Keymap, src Keymap) {
	dstV := reflect.ValueOf(dst).Elem()
	srcV := reflect.ValueOf(src)
	for _, c := range Commands {
		df := dstV.FieldByName(c.FieldName)
		sf := srcV.FieldByName(c.FieldName)
		if !df.IsValid() || !sf.IsValid() || !df.CanSet() {
			continue
		}
		// src.X != nil (a user-supplied override) replaces dst.X.
		if sf.Kind() == reflect.Slice && !sf.IsNil() {
			df.Set(sf)
		}
	}
	// Legacy TOML keys: read the legacy field (still on the struct)
	// and apply it to the canonical command's field if the canonical
	// field wasn't set in the user's TOML. Lets pre-rename configs
	// keep working without forcing a migration.
	for _, c := range Commands {
		if c.LegacyTOMLKey == "" {
			continue
		}
		// Build the legacy field name from the LegacyTOMLKey by
		// snake-case → PascalCase + "Legacy" prefix is the
		// convention used in the struct (LegacyLensModeToggle).
		// Defensive lookup: if no Legacy* field exists, skip.
		legacyField := "Legacy" + c.FieldName
		sf := srcV.FieldByName(legacyField)
		df := dstV.FieldByName(c.FieldName)
		if !sf.IsValid() || !df.IsValid() || !df.CanSet() {
			continue
		}
		if sf.Kind() == reflect.Slice && !sf.IsNil() {
			// Only overwrite if the canonical field wasn't already
			// set (canonical wins when both are present).
			canonV := srcV.FieldByName(c.FieldName)
			if !canonV.IsValid() || canonV.Kind() != reflect.Slice || canonV.IsNil() {
				df.Set(sf)
			}
		}
	}
}

// DumpTOML writes the effective keymap to w as a TOML fragment.
// Walks the Commands registry so output stays in registry order
// (grouped by category) and new commands appear automatically.
func (km Keymap) DumpTOML() string {
	var b bytes.Buffer
	fmt.Fprintln(&b, "# sf-deck keybindings")
	fmt.Fprintln(&b, "# Save as ~/.sf-deck/keybindings.toml to override.")
	fmt.Fprintln(&b, "# Any field you omit keeps its default.")
	fmt.Fprintln(&b, "# Key syntax: 'k', 'enter', 'tab', 'ctrl+o', etc.")
	fmt.Fprintln(&b)
	v := reflect.ValueOf(km)
	lastCategory := ""
	for _, c := range Commands {
		f := v.FieldByName(c.FieldName)
		if !f.IsValid() || f.Kind() != reflect.Slice {
			continue
		}
		if c.Category != lastCategory {
			fmt.Fprintf(&b, "\n# --- %s ---\n", c.Category)
			lastCategory = c.Category
		}
		slots := f.Interface().([]string)
		fmt.Fprintf(&b, "%s = %s\n", c.ID, formatStringSlice(slots))
	}
	return b.String()
}
