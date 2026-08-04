package keymap

// Command registry — the single source of truth for every keybinding
// the TUI exposes.
//
// Every binding has historically lived in three or four places:
// the Keymap struct field, the DefaultKeymap() default, the
// mergeKeymap() user-override clause, and the serialize() table
// entry. Adding a key meant editing all four; forgetting one
// silently broke either the default, the override, or the
// disk-persistence round-trip.
//
// commands is the new authoritative list. Each entry carries
// everything any caller needs:
//
//   ID:          stable disk-persistent identifier (also TOML key)
//   FieldName:   the matching Keymap struct field, used by reflection-
//                free helpers below to map ID → []string
//   Label:       human-readable name shown in the palette + settings
//   Category:    grouping for the settings page (Process, List nav,
//                SOQL, etc.)
//   When:        context predicate — "global" (always available),
//                "tab=soql" (only when on /soql), "col_mode" (only
//                in list-table column mode), etc. Used by conflict
//                detection: two commands binding the same key in
//                "global" collide; two commands in different When
//                contexts don't.
//   Default:     out-of-the-box key bindings (Bubble Tea string form)
//
// DefaultKeymap, mergeKeymap, and serialize all walk this slice.
// New keys: append one entry. Conflict detection (TestNoConflicts)
// runs over the slice on every test invocation.
//
// The Keymap struct still exists because dispatch sites read
// Keys.SOQLSave-style fields. The struct + this registry must
// stay in sync via FieldName; TestRegistryMatchesStruct enforces
// that every Command has a matching Keymap field and vice-versa.
//
// ── Modifier convention ──────────────────────────────────────
// New bindings should follow this convention. Departures need a
// defensible reason (terminal portability, established muscle
// memory, etc.) called out in a comment on the entry.
//
//   bare key (x)        primary action for the surface
//   capital (X)         "more / harder / opposite" of bare X
//   ctrl+x              global / structural / system action
//   shift+x             reserved — capital is the canonical "more X"
//   symbol (, . [ ] < >) navigation / column / structural verbs
//   alt+x               reserved for future per-surface escape hatch
//
// Examples that follow the convention:
//   d  delete project          → D  force-delete (harder)
//   s  sort column              → S  clear sort (opposite)
//   o  open in Lightning        → ctrl+o open menu (structural)
//   y  yank URL                 → Y yank row (wider)
//
// Examples that depart with reason:
//   subtab_3 binds "#" alongside "shift+3" because some terminals
//   don't deliver shift+digit reliably — terminal portability.

// Command is one entry in the registry.
type Command struct {
	ID        string
	FieldName string // matches the Keymap struct field
	Label     string
	Category  string
	When      string // "global", "tab=foo", "col_mode", etc.
	Default   []string
	// LegacyTOMLKey is the previous on-disk name when the command's ID
	// changed (e.g. lens_mode_toggle → chip_mode_toggle). Empty for
	// commands without a rename history. Loaders honor both keys.
	LegacyTOMLKey string
}

// Commands is the registry. Order is grouping by category for human
// readers — the dispatch / merge / serialize code doesn't care.
var Commands = []Command{
	// ---- Process / panes ----
	{ID: "quit", FieldName: "Quit", Label: "Quit", Category: "Process", When: "global", Default: []string{"ctrl+c"}},
	// Apostrophe (') is the most layout-stable single-press key:
	// home row, right pinkie, same physical position on every
	// Latin layout (US/UK/ISO/ANSI/European). Backtick was the
	// first pick but its position varies enough across layouts
	// that the binding ended up unfindable for some users.
	{ID: "focus_orgs", FieldName: "FocusOrgs", Label: "Focus orgs panel", Category: "Process", When: "global", Default: []string{"'"}},
	// FocusBookmarks intentionally has no default — `-` is bound to
	// open_dev_projects (which IS the bookmarks panel content) and
	// the bookmarks subtab focus is handled implicitly when that
	// command fires. Kept on the registry so users can rebind via
	// keybindings.toml if they want it back.
	{ID: "focus_bookmarks", FieldName: "FocusBookmarks", Label: "Focus bookmarks panel", Category: "Process", When: "global", Default: nil},
	{ID: "toggle_pane", FieldName: "TogglePane", Label: "Toggle pane focus", Category: "Process", When: "global", Default: nil},
	{ID: "back", FieldName: "Back", Label: "Back / cancel", Category: "Process", When: "global", Default: []string{"esc"}},
	{ID: "toggle_sidebar", FieldName: "ToggleSidebar", Label: "Toggle right sidebar", Category: "Process", When: "global", Default: []string{`\`}},
	{ID: "toggle_sidebar_stacked", FieldName: "ToggleSidebarStacked", Label: "Toggle stacked sidebar (below vs beside main)", Category: "Process", When: "global", Default: []string{"|"}},
	{ID: "toggle_left", FieldName: "ToggleLeft", Label: "Toggle left rail", Category: "Process", When: "global", Default: []string{`ctrl+\`}},
	{ID: "help", FieldName: "Help", Label: "Help / view info", Category: "Process", When: "global", Default: []string{"?"}},
	{ID: "inspect_panel", FieldName: "InspectPanel", Label: "Inspect — full sidebar info in a modal", Category: "Process", When: "global", Default: []string{"i"}},

	// ---- List nav ----
	{ID: "move_up", FieldName: "MoveUp", Label: "Move up", Category: "Navigation", When: "global", Default: []string{"k", "up"}},
	{ID: "move_down", FieldName: "MoveDown", Label: "Move down", Category: "Navigation", When: "global", Default: []string{"j", "down"}},
	{ID: "jump_up", FieldName: "JumpUp", Label: "Jump up (~5 rows)", Category: "Navigation", When: "global", Default: []string{"ctrl+up"}},
	{ID: "jump_down", FieldName: "JumpDown", Label: "Jump down (~5 rows)", Category: "Navigation", When: "global", Default: []string{"ctrl+down"}},
	{ID: "page_up", FieldName: "PageUp", Label: "Page up", Category: "Navigation", When: "global", Default: []string{"ctrl+u", "pgup"}},
	{ID: "page_down", FieldName: "PageDown", Label: "Page down", Category: "Navigation", When: "global", Default: []string{"ctrl+d", "pgdown"}},
	{ID: "go_top", FieldName: "GoTop", Label: "Go to top", Category: "Navigation", When: "global", Default: []string{"g", "home"}},
	{ID: "go_bottom", FieldName: "GoBottom", Label: "Go to bottom", Category: "Navigation", When: "global", Default: []string{"G", "end"}},
	{ID: "drill", FieldName: "Drill", Label: "Drill / activate", Category: "Navigation", When: "global", Default: []string{"enter"}},
	{ID: "refresh", FieldName: "Refresh", Label: "Refresh current view", Category: "Navigation", When: "global", Default: []string{"r"}},
	{ID: "global_refresh", FieldName: "GlobalRefresh", Label: "Refresh all loaded data (active org)", Category: "Navigation", When: "global", Default: []string{"ctrl+r"}},

	// ---- Tab switching ----
	{ID: "tab_1", FieldName: "Tab1", Label: "Tab 1", Category: "Tabs", When: "global", Default: []string{"1"}},
	{ID: "tab_2", FieldName: "Tab2", Label: "Tab 2", Category: "Tabs", When: "global", Default: []string{"2"}},
	{ID: "tab_3", FieldName: "Tab3", Label: "Tab 3", Category: "Tabs", When: "global", Default: []string{"3"}},
	{ID: "tab_4", FieldName: "Tab4", Label: "Tab 4", Category: "Tabs", When: "global", Default: []string{"4"}},
	{ID: "tab_5", FieldName: "Tab5", Label: "Tab 5", Category: "Tabs", When: "global", Default: []string{"5"}},
	{ID: "tab_6", FieldName: "Tab6", Label: "Tab 6", Category: "Tabs", When: "global", Default: []string{"6"}},
	{ID: "tab_7", FieldName: "Tab7", Label: "Tab 7", Category: "Tabs", When: "global", Default: []string{"7"}},
	{ID: "tab_8", FieldName: "Tab8", Label: "Tab 8", Category: "Tabs", When: "global", Default: []string{"8"}},
	// Slot 9 is the active overflow tab — last non-pinned tab the
	// user opened from the More modal or palette. Pressing 9 jumps
	// to it; the pill on the bar shows its name. No-op when no
	// overflow tab has been activated this session.
	{ID: "tab_9", FieldName: "Tab9", Label: "Active overflow tab", Category: "Tabs", When: "global", Default: []string{"9"}},
	// Slot 0 is the More modal trigger — always visible, opens a
	// picker listing every non-pinned tab. Sits last on the bar
	// to mirror the 0/menu convention.
	{ID: "tab_0", FieldName: "Tab0", Label: "More tabs (overflow modal)", Category: "Tabs", When: "global", Default: []string{"0"}},

	// ---- Subtab switching (shift+1..9) ----
	{ID: "subtab_1", FieldName: "Subtab1", Label: "Subtab 1", Category: "Tabs", When: "subtabbed", Default: []string{"!", "shift+1"}},
	{ID: "subtab_2", FieldName: "Subtab2", Label: "Subtab 2", Category: "Tabs", When: "subtabbed", Default: []string{"@", "\"", "shift+2"}},
	{ID: "subtab_3", FieldName: "Subtab3", Label: "Subtab 3", Category: "Tabs", When: "subtabbed", Default: []string{"#", "£", "shift+3"}},
	{ID: "subtab_4", FieldName: "Subtab4", Label: "Subtab 4", Category: "Tabs", When: "subtabbed", Default: []string{"$", "shift+4"}},
	{ID: "subtab_5", FieldName: "Subtab5", Label: "Subtab 5", Category: "Tabs", When: "subtabbed", Default: []string{"%", "shift+5"}},
	{ID: "subtab_6", FieldName: "Subtab6", Label: "Subtab 6", Category: "Tabs", When: "subtabbed", Default: []string{"^", "shift+6"}},
	{ID: "subtab_7", FieldName: "Subtab7", Label: "Subtab 7", Category: "Tabs", When: "subtabbed", Default: []string{"&", "shift+7"}},
	{ID: "subtab_8", FieldName: "Subtab8", Label: "Subtab 8", Category: "Tabs", When: "subtabbed", Default: []string{"*", "shift+8"}},
	{ID: "subtab_9", FieldName: "Subtab9", Label: "Subtab 9", Category: "Tabs", When: "subtabbed", Default: []string{"(", "shift+9"}},
	{ID: "subtab_0", FieldName: "Subtab0", Label: "Subtab overflow (More…)", Category: "Tabs", When: "subtabbed", Default: []string{")", "shift+0"}},
	{ID: "prev_subtab", FieldName: "PrevSubtab", Label: "Previous subtab", Category: "Tabs", When: "global", Default: []string{"shift+tab"}},
	{ID: "next_subtab", FieldName: "NextSubtab", Label: "Next subtab", Category: "Tabs", When: "global", Default: []string{"tab"}},

	// ---- Open / yank ----
	{ID: "open_default", FieldName: "OpenDefault", Label: "Open in Lightning", Category: "Open & Yank", When: "global", Default: []string{"o"}},
	{ID: "open_menu", FieldName: "OpenMenu", Label: "Open menu (multi-target)", Category: "Open & Yank", When: "global", Default: []string{"ctrl+o"}},
	{ID: "yank_default", FieldName: "YankDefault", Label: "Yank URL", Category: "Open & Yank", When: "global", Default: []string{"y"}},
	{ID: "yank_menu", FieldName: "YankMenu", Label: "Yank menu (multi-target)", Category: "Open & Yank", When: "global", Default: []string{"ctrl+y"}},

	// ---- Search ----
	{ID: "search_start", FieldName: "SearchStart", Label: "Start search", Category: "Search", When: "global", Default: []string{"/"}},
	// search_clear: `C` (capital C). Esc also cancels search in
	// most contexts, but having an explicit clear shortcut at any
	// drill level (matches the dispatcher comment in update_keys.go
	// referencing Keys.SearchClear) is muscle memory. Capital so it
	// doesn't collide with any bare-letter context bindings.
	{ID: "search_clear", FieldName: "SearchClear", Label: "Clear search", Category: "Search", When: "global", Default: []string{"C"}},
	{ID: "global_search", FieldName: "GlobalSearch", Label: "Global search modal", Category: "Search", When: "global", Default: []string{"ctrl+f"}},
	{ID: "search_toggle_mode", FieldName: "SearchToggleMode", Label: "Toggle metadata/records mode (inside global search)", Category: "Search", When: "modal", Default: []string{"ctrl+r"}},

	// ---- FLS grid ----
	{ID: "fls_toggle_read", FieldName: "FLSToggleRead", Label: "Toggle field-level Read", Category: "Permissions", When: "tab=fls", Default: []string{"r"}},
	{ID: "fls_toggle_edit", FieldName: "FLSToggleEdit", Label: "Toggle field-level Edit", Category: "Permissions", When: "tab=fls", Default: []string{"e"}},

	// ---- Object permissions grid ----
	{ID: "obj_perm_read", FieldName: "ObjPermRead", Label: "Toggle object Read", Category: "Permissions", When: "tab=perm-objects", Default: []string{"r"}},
	{ID: "obj_perm_create", FieldName: "ObjPermCreate", Label: "Toggle object Create", Category: "Permissions", When: "tab=perm-objects", Default: []string{"c"}},
	{ID: "obj_perm_edit", FieldName: "ObjPermEdit", Label: "Toggle object Edit", Category: "Permissions", When: "tab=perm-objects", Default: []string{"e"}},
	{ID: "obj_perm_delete", FieldName: "ObjPermDelete", Label: "Toggle object Delete", Category: "Permissions", When: "tab=perm-objects", Default: []string{"d"}},
	{ID: "obj_perm_view_all", FieldName: "ObjPermViewAll", Label: "Toggle ViewAllRecords", Category: "Permissions", When: "tab=perm-objects", Default: []string{"v"}},
	{ID: "obj_perm_modify_all", FieldName: "ObjPermModifyAll", Label: "Toggle ModifyAllRecords", Category: "Permissions", When: "tab=perm-objects", Default: []string{"m"}},
	{ID: "sys_perm_toggle", FieldName: "SysPermToggle", Label: "Toggle system permission", Category: "Permissions", When: "tab=perm-system", Default: []string{"space"}},

	// ---- View-chip nav ----
	{ID: "prev_view", FieldName: "PrevView", Label: "Previous chip", Category: "Chips", When: "global", Default: []string{"[", "shift+left"}},
	{ID: "next_view", FieldName: "NextView", Label: "Next chip", Category: "Chips", When: "global", Default: []string{"]", "shift+right"}},
	// Hides the chip filter strip + the "VIEWS" header above the
	// list table to give the table its full vertical height.
	// Useful on big lists once you've picked your chip — those 2-3
	// lines become 2-3 more rows. Toggle again to restore.
	{ID: "toggle_dashboard", FieldName: "ToggleDashboard", Label: "Hide chip strip (maximize list)", Category: "Chips", When: "tab=has-dashboard", Default: []string{"ctrl+="}},
	{ID: "toggle_query_line", FieldName: "ToggleQueryLine", Label: "Hide SOQL query line", Category: "Chips", When: "tab=records-shaped", Default: []string{"ctrl+-"}},

	// ---- Modals ----
	{ID: "open_settings", FieldName: "OpenSettings", Label: "Open settings", Category: "Modals", When: "global", Default: []string{"ctrl+s"}},
	{ID: "open_api_log", FieldName: "OpenAPILog", Label: "Open API call log", Category: "Modals", When: "global", Default: []string{"ctrl+a"}},
	{ID: "open_downloads", FieldName: "OpenDownloads", Label: "Open downloads", Category: "Modals", When: "global", Default: []string{"ctrl+j"}},

	// ---- Chip system ----
	{ID: "chip_mode_toggle", FieldName: "LensModeToggle", Label: "Toggle chip mode (sf-deck ↔ Salesforce list views)", Category: "Chips", When: "tab=records-shaped", Default: []string{"L"}, LegacyTOMLKey: "lens_mode_toggle"},
	{ID: "open_chip_manager", FieldName: "OpenLensManager", Label: "Open chip manager", Category: "Chips", When: "tab=chip-bearing", Default: []string{"V"}, LegacyTOMLKey: "open_lens_manager"},
	{ID: "open_chip_overflow", FieldName: "OpenChipOverflow", Label: "Open chip overflow", Category: "Chips", When: "tab=chip-bearing", Default: []string{"M"}},
	{ID: "toggle_chip_favourite", FieldName: "ToggleChipFavourite", Label: "Toggle chip favourite", Category: "Chips", When: "tab=chip-bearing", Default: []string{"F"}},

	// ---- SOQL ----
	{ID: "soql_edit", FieldName: "SOQLEdit", Label: "Edit query", Category: "SOQL", When: "tab=soql", Default: []string{"e"}},
	{ID: "soql_toggle_tooling", FieldName: "SOQLToggleTooling", Label: "Toggle Tooling API", Category: "SOQL", When: "tab=soql", Default: []string{"T"}},
	{ID: "soql_toggle_bulk", FieldName: "SOQLToggleBulk", Label: "Toggle Bulk API (1 call vs ~1/2k rows)", Category: "SOQL", When: "tab=soql", Default: []string{"ctrl+b"}},
	{ID: "soql_save", FieldName: "SOQLSave", Label: "Save / update query", Category: "SOQL", When: "tab=soql", Default: []string{"S"}},
	{ID: "soql_save_as", FieldName: "SOQLSaveAs", Label: "Save as new", Category: "SOQL", When: "tab=soql", Default: []string{"ctrl+n"}},
	{ID: "soql_delete", FieldName: "SOQLDelete", Label: "Delete saved query", Category: "SOQL", When: "tab=soql-saved", Default: []string{"D"}},
	{ID: "soql_duplicate", FieldName: "SOQLDuplicate", Label: "Duplicate saved query", Category: "SOQL", When: "tab=soql-saved", Default: []string{"c"}},
	{ID: "soql_rename", FieldName: "SOQLRename", Label: "Rename saved query", Category: "SOQL", When: "tab=soql-saved", Default: []string{"R"}},
	{ID: "soql_export", FieldName: "SOQLExport", Label: "Export results", Category: "SOQL", When: "tab=soql-editor", Default: []string{"ctrl+x"}},
	{ID: "records_export", FieldName: "RecordsExport", Label: "Export records", Category: "Records", When: "tab=records,object-detail,users", Default: []string{"ctrl+x"}},
	{ID: "soql_yank_cell", FieldName: "SOQLYankCell", Label: "Yank cursored cell", Category: "SOQL", When: "tab=soql-editor", Default: []string{"y"}},
	{ID: "soql_yank_row", FieldName: "SOQLYankRow", Label: "Yank cursored row (TSV)", Category: "SOQL", When: "tab=soql-editor", Default: []string{"Y"}},
	{ID: "soql_yank_column", FieldName: "SOQLYankColumn", Label: "Yank column as IN-clause", Category: "SOQL", When: "tab=soql-editor", Default: []string{"ctrl+y"}},

	// ---- Record edit (/record inline field edit) ----
	{ID: "record_edit_field", FieldName: "RecordEditField", Label: "Edit cursored field", Category: "Record", When: "tab=record", Default: []string{"e"}},
	{ID: "record_edit_save", FieldName: "RecordEditSave", Label: "Save dirty fields", Category: "Record", When: "tab=record", Default: []string{"ctrl+s"}},
	{ID: "record_edit_cancel_all", FieldName: "RecordEditCancelAll", Label: "Discard all dirty edits", Category: "Record", When: "tab=record", Default: []string{"ctrl+x"}},

	// ---- Exec (anonymous Apex) ----
	{ID: "exec_edit", FieldName: "ExecEdit", Label: "Edit snippet", Category: "Exec", When: "tab=exec", Default: []string{"e"}},
	{ID: "exec_external_editor", FieldName: "ExecExternalEditor", Label: "Open $EDITOR", Category: "Exec", When: "tab=exec", Default: []string{"ctrl+e"}},
	{ID: "exec_toggle_log", FieldName: "ExecToggleLog", Label: "Toggle debug-log capture", Category: "Exec", When: "tab=exec", Default: []string{"ctrl+l"}},
	{ID: "exec_save", FieldName: "ExecSave", Label: "Save / update snippet", Category: "Exec", When: "tab=exec", Default: []string{"S"}},
	{ID: "exec_save_as", FieldName: "ExecSaveAs", Label: "Save as new snippet", Category: "Exec", When: "tab=exec", Default: []string{"ctrl+n"}},
	{ID: "exec_delete", FieldName: "ExecDelete", Label: "Delete saved snippet", Category: "Exec", When: "tab=exec-saved", Default: []string{"D"}},
	{ID: "exec_duplicate", FieldName: "ExecDuplicate", Label: "Duplicate saved snippet", Category: "Exec", When: "tab=exec-saved", Default: []string{"c"}},
	{ID: "exec_rename", FieldName: "ExecRename", Label: "Rename saved snippet", Category: "Exec", When: "tab=exec-saved", Default: []string{"R"}},

	// ---- Reports ----
	{ID: "report_export", FieldName: "ReportExport", Label: "Export report", Category: "Reports", When: "tab=reports", Default: []string{"ctrl+x"}},

	// ---- Dev projects ----
	{ID: "new_project", FieldName: "NewProject", Label: "New dev project", Category: "Dev Projects", When: "tab=dev-projects", Default: []string{"n"}},
	{ID: "edit_project", FieldName: "EditProject", Label: "Edit / rename project", Category: "Dev Projects", When: "tab=dev-projects", Default: []string{"e"}},
	{ID: "delete_project", FieldName: "DeleteProject", Label: "Delete project", Category: "Dev Projects", When: "tab=dev-projects", Default: []string{"d"}},
	{ID: "delete_project_force", FieldName: "DeleteProjectForce", Label: "Force-delete project", Category: "Dev Projects", When: "tab=dev-projects", Default: []string{"D"}},
	{ID: "collect_item", FieldName: "CollectItem", Label: "Collect item to loaded project (toggle)", Category: "Dev Projects", When: "global", Default: []string{"K"}},
	{ID: "collect_item_pick", FieldName: "CollectItemPick", Label: "Collect item — pick project", Category: "Dev Projects", When: "global", Default: []string{"ctrl+k"}},
	{ID: "open_dev_projects", FieldName: "OpenDevProjects", Label: "Open dev-projects list", Category: "Dev Projects", When: "global", Default: []string{"-"}},
	// `#` opens the tag manager. Shadows subtab_3 on subtabbed tabs
	// (since `#` is shift+3 on US/UK layouts) — the dispatcher picks
	// subtab_3 in that context, leaving `#` free for the tag manager
	// everywhere else. The collision is allowlisted in
	// commands_test.go::knownConflicts.
	{ID: "open_tags", FieldName: "OpenTags", Label: "Open tag manager", Category: "Dev Projects", When: "global", Default: []string{"`"}},
	{ID: "export_project", FieldName: "ExportProject", Label: "Create bundle from project", Category: "Dev Projects", When: "tab=dev-projects", Default: []string{"x", "ctrl+x"}},
	{ID: "open_bundles", FieldName: "OpenBundles", Label: "Open bundles", Category: "Dev Projects", When: "tab=dev-project-detail", Default: []string{"b"}},
	{ID: "toggle_project_scope", FieldName: "ToggleProjectScope", Label: "Toggle project scope", Category: "Dev Projects", When: "tab=dev-project-detail", Default: []string{"O"}},
	// `_` is the visual prefix on the loaded-project pill ("_ <name>")
	// — bind the same character so muscle memory works. Context-
	// sensitive: on /dev-projects toggles load/unload of the cursored
	// project, anywhere else jumps to the loaded project's detail.
	{ID: "load_org_project", FieldName: "LoadOrgProject", Label: "Load / open loaded project", Category: "Dev Projects", When: "global", Default: []string{"_"}},
	{ID: "toggle_project_mode", FieldName: "ToggleProjectMode", Label: "Toggle project chip", Category: "Dev Projects", When: "tab=reports", Default: []string{"p"}},

	// ---- List-table column controls ----
	{ID: "edit_current_view", FieldName: "EditCurrentView", Label: "Edit current view", Category: "List Table", When: "list-table", Default: []string{"e"}},
	{ID: "col_shrink", FieldName: "ColShrink", Label: "Shrink column", Category: "List Table", When: "list-table", Default: []string{"<"}},
	{ID: "col_grow", FieldName: "ColGrow", Label: "Grow column", Category: "List Table", When: "list-table", Default: []string{">"}},
	{ID: "col_snap_min", FieldName: "ColSnapMin", Label: "Snap column to header", Category: "List Table", When: "list-table", Default: []string{"{"}},
	{ID: "col_snap_max", FieldName: "ColSnapMax", Label: "Snap column to widest cell", Category: "List Table", When: "list-table", Default: []string{"}"}},
	{ID: "col_reset_widths", FieldName: "ColResetWidths", Label: "Reset column widths", Category: "List Table", When: "list-table", Default: []string{"W"}},
	{ID: "col_scroll_l", FieldName: "ColScrollL", Label: "Scroll columns left", Category: "List Table", When: "list-table", Default: []string{"left", "h", ","}},
	{ID: "col_scroll_r", FieldName: "ColScrollR", Label: "Scroll columns right", Category: "List Table", When: "list-table", Default: []string{"right", "l", "."}},
	{ID: "col_sort", FieldName: "ColSort", Label: "Sort by cursored column", Category: "List Table", When: "list-table", Default: []string{"s"}},
	{ID: "col_sort_clear", FieldName: "ColSortClear", Label: "Clear sort", Category: "List Table", When: "list-table", Default: []string{"S"}},
	{ID: "zen_mode", FieldName: "ZenMode", Label: "Toggle zen mode", Category: "List Table", When: "list-table", Default: []string{"z"}},
	{ID: "paginate", FieldName: "Paginate", Label: "Toggle pagination", Category: "List Table", When: "list-table", Default: []string{"P"}},
	{ID: "tag", FieldName: "Tag", Label: "Tag picker", Category: "Tags", When: "global", Default: []string{"t"}},
	{ID: "tag_all", FieldName: "TagAll", Label: "Tag all visible rows", Category: "Tags", When: "list surfaces", Default: []string{"T"}},
	{ID: "tag_column", FieldName: "TagColumn", Label: "Toggle tag column", Category: "Tags", When: "global", Default: []string{"ctrl+t"}},
	{ID: "project_column", FieldName: "ProjectColumn", Label: "Toggle project column", Category: "Tags", When: "global", Default: []string{"ctrl+p"}},
	{ID: "flag_column", FieldName: "FlagColumn", Label: "Cycle flag column (full/letter/hidden)", Category: "Tags", When: "global", Default: []string{"ctrl+g"}},

	// ---- Filter cycle (legacy, still wired) ----
	{ID: "filter_cycle", FieldName: "FilterCycle", Label: "Cycle filter (legacy)", Category: "Legacy", When: "tab=objects", Default: []string{"f"}},

	// ---- Command palette + new commands (post-SSOT additions) ----
	// `;` is the primary; ctrl+k would collide with collect_item_pick.
	// Users can rebind to ctrl+shift+k or similar via keybindings.toml.
	{ID: "command_palette", FieldName: "CommandPalette", Label: "Open command menu", Category: "Process", When: "global", Default: []string{";"}},

	// ---- Bundles (lifted from tab_bundles.go / tab_bundle_detail.go hardcoded keys) ----
	{ID: "bundle_open", FieldName: "BundleOpen", Label: "Open bundle directory / file on disk", Category: "Bundles", When: "tab=bundles,bundle", Default: []string{"o"}},
	{ID: "bundle_unlink", FieldName: "BundleUnlink", Label: "Unlink bundle (leaves directory)", Category: "Bundles", When: "tab=bundles", Default: []string{"d"}},
	{ID: "bundle_retrieve", FieldName: "BundleRetrieve", Label: "Retrieve into bundle", Category: "Bundles", When: "tab=bundles", Default: []string{"r"}},
	{ID: "bundle_deploy", FieldName: "BundleDeploy", Label: "Deploy from bundle", Category: "Bundles", When: "tab=bundles", Default: []string{"D"}},
	{ID: "bundle_yank_path", FieldName: "BundleYankPath", Label: "Yank bundle path", Category: "Bundles", When: "tab=bundle-detail", Default: []string{"y"}},
	{ID: "bundle_validate", FieldName: "BundleValidate", Label: "Validate-only deploy", Category: "Bundles", When: "tab=bundle-detail", Default: []string{"v"}},
	{ID: "bundle_refresh_diff", FieldName: "BundleRefreshDiff", Label: "Force-refresh diff", Category: "Bundles", When: "tab=bundle-detail", Default: []string{"R"}},

	// ---- Flow detail (tab=flow-detail — versions view; metadata writes) ----
	{ID: "flow_rename", FieldName: "FlowRename", Label: "Rename flow label", Category: "Flows", When: "tab=flow-detail", Default: []string{"e"}},
	{ID: "flow_version_delete", FieldName: "FlowVersionDelete", Label: "Delete inactive flow version", Category: "Flows", When: "tab=flow-detail", Default: []string{"D"}},

	// ---- Downloads (lifted from modal_downloads.go / tab_home_downloads.go hardcoded keys) ----
	{ID: "download_open", FieldName: "DownloadOpen", Label: "Open exported file", Category: "Downloads", When: "downloads", Default: []string{"o"}},
	{ID: "download_reveal", FieldName: "DownloadReveal", Label: "Reveal in Finder", Category: "Downloads", When: "downloads", Default: []string{"r"}},
	{ID: "download_yank_path", FieldName: "DownloadYankPath", Label: "Yank file path", Category: "Downloads", When: "downloads", Default: []string{"y"}},
	{ID: "download_remove", FieldName: "DownloadRemove", Label: "Remove from history", Category: "Downloads", When: "downloads", Default: []string{"d"}},

	// ---- Theme picker (lifted from modal_theme_picker.go hardcoded keys) ----
	{ID: "theme_picker_favourite", FieldName: "ThemePickerFavourite", Label: "Toggle theme favourite", Category: "Theme Picker", When: "modal=theme-picker", Default: []string{"f", "F"}},
	{ID: "theme_picker_clear", FieldName: "ThemePickerClear", Label: "Clear search", Category: "Theme Picker", When: "modal=theme-picker", Default: []string{"C"}},

	// ---- Cache settings (lifted from modal_cache_settings.go hardcoded keys) ----
	{ID: "cache_reset_ttl", FieldName: "CacheResetTTL", Label: "Reset TTL to default", Category: "Cache Settings", When: "modal=cache-settings", Default: []string{"r"}},

	// ---- Chip wizard (lifted from chip_wizard.go hardcoded keys) ----
	{ID: "chip_wizard_lookup", FieldName: "ChipWizardLookup", Label: "Open lookup", Category: "Chip Wizard", When: "modal=chip-wizard", Default: []string{"ctrl+l"}},
	{ID: "chip_wizard_delete", FieldName: "ChipWizardDelete", Label: "Delete current row", Category: "Chip Wizard", When: "modal=chip-wizard", Default: []string{"ctrl+x"}},
	{ID: "chip_wizard_save", FieldName: "ChipWizardSave", Label: "Save chip", Category: "Chip Wizard", When: "modal=chip-wizard", Default: []string{"ctrl+s"}},
	{ID: "chip_wizard_mode", FieldName: "ChipWizardMode", Label: "Toggle row mode", Category: "Chip Wizard", When: "modal=chip-wizard", Default: []string{"ctrl+t"}},

	// ---- Orgs rail (focus=orgs) — only fold/expand + modal trigger ----
	{ID: "org_group_toggle", FieldName: "OrgGroupToggle", Label: "Toggle group expand/collapse", Category: "Orgs", When: "focus=orgs", Default: []string{" "}},
	{ID: "org_manage_open", FieldName: "OrgManageOpen", Label: "Open org-management modal", Category: "Orgs", When: "focus=orgs", Default: []string{"ctrl+e"}},

	// ---- Org-management modal (modal=org-manage) ----
	{ID: "org_add_org", FieldName: "OrgAddOrg", Label: "Add org", Category: "Org Manager", When: "modal=org-manage", Default: []string{"A"}},
	{ID: "org_group_create", FieldName: "OrgGroupCreate", Label: "New group", Category: "Org Manager", When: "modal=org-manage", Default: []string{"n"}},
	{ID: "org_group_rename", FieldName: "OrgGroupRename", Label: "Rename group", Category: "Org Manager", When: "modal=org-manage", Default: []string{"R"}},
	{ID: "org_group_delete", FieldName: "OrgGroupDelete", Label: "Delete group", Category: "Org Manager", When: "modal=org-manage", Default: []string{"x"}},
	{ID: "org_group_reorder_up", FieldName: "OrgGroupReorderUp", Label: "Move group up", Category: "Org Manager", When: "modal=org-manage", Default: []string{"["}},
	{ID: "org_group_reorder_down", FieldName: "OrgGroupReorderDn", Label: "Move group down", Category: "Org Manager", When: "modal=org-manage", Default: []string{"]"}},
	{ID: "org_move_up", FieldName: "OrgMoveUp", Label: "Move org up (within / across groups)", Category: "Org Manager", When: "modal=org-manage", Default: []string{"<"}},
	{ID: "org_move_down", FieldName: "OrgMoveDown", Label: "Move org down (within / across groups)", Category: "Org Manager", When: "modal=org-manage", Default: []string{">"}},
	{ID: "org_move_to_group", FieldName: "OrgMoveToGroup", Label: "Move org to group…", Category: "Org Manager", When: "modal=org-manage", Default: []string{"g"}},
	{ID: "org_disconnect", FieldName: "OrgDisconnect", Label: "Logout / disconnect org", Category: "Org Manager", When: "modal=org-manage", Default: []string{"D"}},
	{ID: "org_reauth", FieldName: "OrgReauth", Label: "Re-authenticate org (login web, same alias)", Category: "Org Manager", When: "modal=org-manage", Default: []string{"L"}},
	{ID: "org_set_default", FieldName: "OrgSetDefault", Label: "Set as sf CLI default org", Category: "Org Manager", When: "modal=org-manage", Default: []string{"*"}},
	{ID: "org_set_default_devhub", FieldName: "OrgSetDefaultDevHub", Label: "Set as default DevHub", Category: "Org Manager", When: "modal=org-manage", Default: []string{"^"}},
	{ID: "org_pin_startup", FieldName: "OrgPinStartup", Label: "Pin as sf-deck startup org", Category: "Org Manager", When: "modal=org-manage", Default: []string{"P"}},
	{ID: "org_cycle_safety", FieldName: "OrgCycleSafety", Label: "Cycle safety level (read_only → records → metadata → full → inherit)", Category: "Org Manager", When: "modal=org-manage", Default: []string{"s"}},
	{ID: "org_set_alias", FieldName: "OrgSetAlias", Label: "Rename alias", Category: "Org Manager", When: "modal=org-manage", Default: []string{"="}},
	{ID: "org_unset_alias", FieldName: "OrgUnsetAlias", Label: "Clear alias", Category: "Org Manager", When: "modal=org-manage", Default: []string{"-"}},
}

// CommandByID returns the command with the given ID. Returns nil
// when not found — callers should treat that as a programming error
// (every dispatch site references a known ID), not a runtime
// failure.
func CommandByID(id string) *Command {
	for i := range Commands {
		if Commands[i].ID == id {
			return &Commands[i]
		}
	}
	return nil
}
