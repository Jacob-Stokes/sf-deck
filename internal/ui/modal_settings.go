package ui

// Settings modal — the "=" entry point. Today it's a single theme
// picker; the structure is intentionally wider than it needs to be
// so new setting categories (per-tab refresh cadence, dashboard
// defaults, …) slot in as additional choiceModal options here.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// openSettingsModal opens the top-level settings menu. Each entry
// drills into a specialised picker / editor for one category. New
// categories slot in here as additional choiceOption rows.
func (m *Model) openSettingsModal() tea.Cmd {
	currentTheme := "tokyo-night"
	if m.settings != nil {
		currentTheme = m.settings.Theme()
	}
	inspector := "(unset)"
	if m.settings != nil && m.settings.InspectorURL() != "" {
		inspector = m.settings.InspectorURL()
	}
	exportDir := "(default ~)"
	if m.settings != nil && m.settings.UI.ReportExportDir != "" {
		exportDir = m.settings.UI.ReportExportDir
	}
	startTab := settings.StartupStartTabFallback
	if m.settings != nil {
		startTab = m.settings.StartupStartTab()
	}
	// One entry per behaviour group; each drills into a submenu of the
	// individual knobs. Every setting in the app lives under exactly
	// one of these — no more "Misc" junk drawer.
	opts := []choiceOption{
		{Label: "Appearance", Hint: "theme: " + currentTheme + " · banner animation", Value: "appearance"},
		{Label: "Startup & defaults", Hint: "what the app opens with · start tab: " + startTab, Value: "startup"},
		{Label: "Navigation & input", Hint: "jump step, wheel scroll feel", Value: "input"},
		{Label: "Lists & limits", Hint: "default row counts for fetches + lists", Value: "limits"},
		{Label: "Search", Hint: "global-search ranking knobs", Value: "search"},
		{Label: "Layout & sizing", Hint: "pane + modal dimensions", Value: "layout"},
		{Label: "API & network", Hint: "timeouts, poll cadence, API version", Value: "api"},
		{Label: "Cache & refresh", Hint: "TTLs per resource type", Value: "cache"},
		{Label: "Export", Hint: "save dir: " + exportDir + " · history cap", Value: "export"},
		{Label: "Integrations", Hint: "Inspector URL: " + inspector + " · browser", Value: "integrations"},
		{Label: "Keybindings", Hint: "edit and save key bindings", Value: "keybindings"},
		{Label: "Debug", Hint: "developer/testing toggles (force welcome modal)", Value: "debug"},
	}
	state := choiceModalState{
		Title:   "Settings",
		Hint:    "Enter to drill in  ·  Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			// Pick threads through the typed-value channel —
			// no per-flow package globals. Update handles the
			// synthetic openSettingsSubmenuMsg by opening the
			// chosen submenu on the live Model.
			pick, _ := val.(string)
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: pick} }
		},
	}
	return m.openChoiceModal(state)
}

// openSettingsSubmenuMsg is the synthetic message that the settings
// meta-menu emits on Enter. Update handles it by opening the chosen
// submenu on the live Model — sidestepping the captured-pointer
// problem that would otherwise leave the submenu un-opened.
type openSettingsSubmenuMsg struct {
	pick string // "theme" | "inspector"
}

// openBrowserModal picks the macOS application used to open extension
// URLs. Required for moz-extension:// / chrome-extension:// because
// those schemes aren't globally routable — macOS needs an explicit
// -a hint. Blank = bare `open <url>` which works for https:// only.
func (m *Model) openBrowserModal() tea.Cmd {
	current := ""
	if m.settings != nil {
		current = m.settings.Browser()
	}
	// Auto-discovered from the machine (installed browsers only), with
	// "(system default)" first. A user can still type/paste any name
	// Launch Services knows about even if it's not in the list.
	opts, cursor := browserChoiceOptions(current)
	state := choiceModalState{
		Title:      "Browser (for extension URLs)",
		Hint:       "Enter to apply  ·  Esc to cancel",
		Options:    opts,
		Cursor:     cursor,
		SuccessMsg: "browser set",
		Save: func(val any) error {
			name, _ := val.(string)
			if m.settings == nil {
				return nil
			}
			m.settings.SetBrowser(name)
			return m.settings.Save()
		},
	}
	return m.openChoiceModal(state)
}

// openInspectorURLModal opens a single-line edit modal for the
// Salesforce Inspector Reloaded base URL. Empty-string submission
// clears the setting (Inspector targets disappear). Value is
// persisted to settings.toml on Enter.
func (m *Model) openInspectorURLModal() tea.Cmd {
	initial := ""
	if m.settings != nil {
		initial = m.settings.InspectorURL()
	}
	state := editModalState{
		Title: "Salesforce Inspector URL",
		Hint: "paste the extension's inspect.html URL · blank to clear · " +
			"e.g. moz-extension://<guid>/inspect.html",
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "inspector url saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			m.settings.SetInspectorURL(val)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// openInputModal opens the Navigation & input submenu — jump-step
// size plus the three wheel-scroll tunables.
func (m *Model) openInputModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	opts := []choiceOption{
		{Label: "Jump step (ctrl+arrow / J / K)", Hint: fmt.Sprintf("current: %d rows", s.JumpRows()), Value: "jump_rows"},
		{Label: "Wheel throttle: idle gap", Hint: fmt.Sprintf("current: %d ms · resets throttle when idle", s.WheelQuietGapMs()), Value: "wheel_quiet_gap"},
		{Label: "Wheel throttle: min interval", Hint: fmt.Sprintf("current: %d ms · gap between accepted ticks", s.WheelMinIntervalMs()), Value: "wheel_min_interval"},
		{Label: "Wheel: max rows per tick", Hint: fmt.Sprintf("current: %d rows · accumulator drain cap", s.WheelMaxStep()), Value: "wheel_max_step"},
		{Label: "Sort scope", Hint: "current: " + sortScopeLabel(s.SortPerView()), Value: "sort_per_view"},
		{Label: "Flow version: Enter behaviour", Hint: "current: " + flowVersionEnterLabel(s.FlowVersionEnterOpens()), Value: "flow_version_enter"},
	}
	return m.settingsSubmenu("Navigation & input", "input", opts)
}

// sortScopeLabel names the per-view-sort mode for the settings hint.
func sortScopeLabel(perView bool) string {
	if perView {
		return "per view (each view keeps its own sort)"
	}
	return "shared across views"
}

// flowVersionEnterLabel names the flow-version Enter behaviour for the
// settings hint.
func flowVersionEnterLabel(opens bool) string {
	if opens {
		return "open Flow Builder"
	}
	return "view definition (in-terminal)"
}

// openChipDefaultLimitModal — single-line edit modal for the
// shared chip-fetch row cap. Empty / 0 / non-integer → reset to
// settings.DefaultChipLimitFallback.
//
// Per-chip Limit on the chip's own Query AST overrides this — the
// default only applies when a chip declares no Limit of its own.
func (m *Model) openChipDefaultLimitModal() tea.Cmd {
	initial := ""
	if m.settings != nil {
		initial = strconv.Itoa(m.settings.DefaultChipLimit())
	}
	state := editModalState{
		Title:       "Chips: default row cap",
		Hint:        fmt.Sprintf("rows fetched per chip when the chip doesn't pin its own Limit · blank or 0 to reset to default (%d)", settings.DefaultChipLimitFallback),
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "chip cap saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			n, _ := strconv.Atoi(val)
			m.settings.SetDefaultChipLimit(n)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// openRecentLimitModal — single-line edit modal for the /recent
// display cap. Empty / 0 / non-integer → reset to default (50).
func (m *Model) openRecentLimitModal() tea.Cmd {
	initial := ""
	if m.settings != nil {
		initial = strconv.Itoa(m.settings.RecentLimit())
	}
	state := editModalState{
		Title:       "Recent: display cap",
		Hint:        "max rows shown on /recent · blank or 0 to reset to default (50)",
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "recent cap saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			n, _ := strconv.Atoi(val)
			m.settings.SetRecentLimit(n)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// openRecentExcludedSFTypesModal — free-text editor for the
// RecentlyViewed sObject-type exclusion list (the WHERE Type NOT IN
// content). One type per line; users add/remove raw API type names to
// hide builder internals / admin artifacts they don't want surfaced.
// Blank submission resets to the built-in defaults.
func (m *Model) openRecentExcludedSFTypesModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	initial := strings.Join(m.settings.RecentExcludedSFTypes(), "\n")
	state := editModalState{
		Title: "Recent: excluded SF types",
		Hint: "One sObject API type per line — hidden from Recently Viewed. " +
			"e.g. FlowRecordElement. Blank resets to defaults. Enter for a newline; ctrl+s saves.",
		InitialBody: initial,
		Multiline:   true,
		SuccessMsg:  "recent exclusion list saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			types := parseLinesList(val)
			if len(types) == 0 {
				// Blank → back to built-in defaults (clear the override).
				m.settings.SetRecentExcludedSFTypes(nil)
				m.settings.UI.Recent.UserSetExcludedSFTypes = false
			} else {
				m.settings.SetRecentExcludedSFTypes(types)
			}
			return m.settings.Save()
		},
		// Re-fetch the active org's RecentlyViewed so the new exclusion
		// list (which changes the SOQL) takes effect immediately.
		OnSuccess: func() tea.Cmd {
			if d := m.activeOrgData(); d != nil {
				return d.RecentlyViewed.Refresh(m.cache)
			}
			return nil
		},
	}
	return m.openEditModal(state)
}

// parseLinesList splits a textarea value into a trimmed, de-duped,
// comment/blank-stripped list — used by the SF-types editor.
func parseLinesList(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		v := strings.TrimSpace(line)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// openRecentExcludedKindsModal — toggleable per-kind exclude list.
// Each row shows a kind with [excluded] / [included] state; picking
// it flips the membership and re-opens the modal so the user can
// toggle several without reopening from the top each time.
func (m *Model) openRecentExcludedKindsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	// Every kind we track in /recent. Order = most-likely-to-toggle
	// first (the noise defaults), then the rest.
	allKinds := []struct {
		kind, label string
	}{
		{RecentKindListView, "List Views"},
		{RecentKindUser, "Users"},
		{RecentKindPublicGroup, "Public Groups"},
		{RecentKindPackage, "Installed Packages"},
		{RecentKindRecord, "Records"},
		{RecentKindReport, "Reports"},
		{RecentKindDashboard, "Dashboards"},
		{RecentKindFlow, "Flows"},
		{RecentKindApexClass, "Apex Classes"},
		{RecentKindLWC, "LWC Bundles"},
		{RecentKindAura, "Aura Bundles"},
		{RecentKindSObject, "sObjects"},
		{RecentKindField, "Fields"},
		{RecentKindPermSet, "Permission Sets"},
		{RecentKindPermSetGroup, "Permission Set Groups"},
		{RecentKindProfile, "Profiles"},
		{RecentKindQueue, "Queues"},
		{RecentKindDeploy, "Deploys"},
		{RecentKindApexLog, "Apex Logs"},
	}
	excluded := m.settings.RecentExcludedKinds()
	skip := make(map[string]bool, len(excluded))
	for _, k := range excluded {
		skip[k] = true
	}
	opts := make([]choiceOption, 0, len(allKinds)+1)
	for _, k := range allKinds {
		state := "included"
		if skip[k.kind] {
			state = "EXCLUDED"
		}
		opts = append(opts, choiceOption{
			Label: k.label,
			Hint:  state,
			Value: k.kind,
		})
	}
	opts = append(opts, choiceOption{Label: "Done", Cancel: true})
	st := choiceModalState{
		Title:      "Recent: excluded kinds",
		Hint:       "Enter to toggle a kind  ·  Esc / Done to close",
		Options:    opts,
		Cursor:     0,
		Searchable: true,
		Save: func(val any) error {
			kind, _ := val.(string)
			if kind == "" {
				return nil
			}
			cur := m.settings.RecentExcludedKinds()
			next := make([]string, 0, len(cur)+1)
			toggled := false
			for _, k := range cur {
				if k == kind {
					toggled = true
					continue
				}
				next = append(next, k)
			}
			if !toggled {
				next = append(next, kind)
			}
			m.settings.SetRecentExcludedKinds(next)
			return m.settings.Save()
		},
		// Re-open the modal after each toggle so the user can flip
		// several without reopening from the top each time.
		OnSuccessTyped: func(val any) tea.Cmd {
			return func() tea.Msg {
				return openSettingsSubmenuMsg{pick: "misc.recent_excluded_kinds"}
			}
		},
	}
	return m.openChoiceModal(st)
}

// openJumpRowsModal lets the user set the row count for the
// ctrl+arrow / J / K jump nav. Validation: must be a positive
// integer. Empty / 0 / non-integer input resets to the default (5).
func (m *Model) openJumpRowsModal() tea.Cmd {
	initial := ""
	if m.settings != nil {
		initial = strconv.Itoa(m.settings.JumpRows())
	}
	state := editModalState{
		Title:       "Jump step size",
		Hint:        "rows to move per ctrl+arrow / J / K · blank or 0 to reset to default (5)",
		InitialBody: initial,
		Multiline:   false,
		SuccessMsg:  "jump step saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			n, _ := strconv.Atoi(val)
			m.settings.SetJumpRows(n)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// numericEditModal is the shared "edit one number" pattern used by
// most experimental knobs. Caller supplies the title, hint, current
// value (for the input prefill), and a save closure that takes the
// parsed int.
//
// Empty / non-integer input passes 0 to save — accessors interpret
// that as "reset to default" by convention.
func (m *Model) numericEditModal(title, hint, successMsg string, current int, save func(int)) tea.Cmd {
	state := editModalState{
		Title:       title,
		Hint:        hint,
		InitialBody: strconv.Itoa(current),
		Multiline:   false,
		SuccessMsg:  successMsg,
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			n, _ := strconv.Atoi(val)
			save(n)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// floatEditModal mirrors numericEditModal for float-valued knobs
// (today: search project-membership boost). Empty / non-numeric
// passes 0 — accessor convention re-applies the default.
func (m *Model) floatEditModal(title, hint, successMsg string, current float64, save func(float64)) tea.Cmd {
	state := editModalState{
		Title:       title,
		Hint:        hint,
		InitialBody: strconv.FormatFloat(current, 'f', -1, 64),
		Multiline:   false,
		SuccessMsg:  successMsg,
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			f, _ := strconv.ParseFloat(val, 64)
			save(f)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// openWheelQuietGapModal — idle window before the wheel throttle
// resets. Bigger = more grouping; smaller = faster re-engagement.
func (m *Model) openWheelQuietGapModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Wheel throttle: idle gap",
		fmt.Sprintf("ms · blank or 0 = default (%d)", settings.DefaultWheelQuietGapMs),
		"wheel idle gap saved",
		m.settings.WheelQuietGapMs(),
		m.settings.SetWheelQuietGapMs,
	)
}

// openWheelMinIntervalModal — minimum spacing between accepted ticks
// in a single gesture.
func (m *Model) openWheelMinIntervalModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Wheel throttle: min interval",
		fmt.Sprintf("ms · blank or 0 = default (%d)", settings.DefaultWheelMinIntervalMs),
		"wheel min interval saved",
		m.settings.WheelMinIntervalMs(),
		m.settings.SetWheelMinIntervalMs,
	)
}

// openWheelMaxStepModal — per-accepted-tick cursor delta cap for
// continuous mode. The accumulator drains up to this many rows on
// each accept, so a fast trackpad flick translates faithfully into
// "advance N rows" rather than being capped at the throttle's
// accept rate. Paginated mode is unaffected — that path is one row
// per accept regardless.
func (m *Model) openWheelMaxStepModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Wheel: max rows per accepted tick (continuous)",
		fmt.Sprintf("rows · blank or 0 = default (%d) · continuous mode only", settings.DefaultWheelMaxStep),
		"wheel max step saved",
		m.settings.WheelMaxStep(),
		m.settings.SetWheelMaxStep,
	)
}

// openRecentMaxEntriesModal — local visit log size.
func (m *Model) openRecentMaxEntriesModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Recent: max entries",
		fmt.Sprintf("rows · blank or 0 = default (%d)", settings.DefaultRecentMaxEntries),
		"recent max entries saved",
		m.settings.RecentMaxEntries(),
		m.settings.SetRecentMaxEntries,
	)
}

// openExportHistoryMaxModal — kept-history cap for the export tracker.
func (m *Model) openExportHistoryMaxModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Exports: history cap",
		fmt.Sprintf("entries · blank or 0 = default (%d)", settings.DefaultExportHistoryMax),
		"export history cap saved",
		m.settings.ExportHistoryMax(),
		m.settings.SetExportHistoryMax,
	)
}

// openSearchProjectBoostModal — global-search rank bump for items in
// the loaded project.
func (m *Model) openSearchProjectBoostModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.floatEditModal(
		"Search: project-membership boost",
		fmt.Sprintf("score points added · blank or 0 = default (%.2f)", settings.DefaultLoadedProjectBoost),
		"project boost saved",
		m.settings.LoadedProjectBoost(),
		m.settings.SetLoadedProjectBoost,
	)
}

// openSearchRecentDecayModal — half-life for the recent-visit boost.
func (m *Model) openSearchRecentDecayModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Search: recent-visit decay window",
		fmt.Sprintf("hours · blank or 0 = default (%d)", settings.DefaultRecentBoostDecayHours),
		"recent decay window saved",
		m.settings.RecentBoostDecayHours(),
		m.settings.SetRecentBoostDecayHours,
	)
}

// openHomeBannerIntervalModal — animation tick speed.
func (m *Model) openHomeBannerIntervalModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Home: banner animation interval",
		fmt.Sprintf("ms · blank or 0 = default (%d) · floors at 50ms", settings.DefaultHomeBannerIntervalMs),
		"banner interval saved",
		m.settings.HomeBannerIntervalMs(),
		m.settings.SetHomeBannerIntervalMs,
	)
}

// openHomeBannerDisableModal — flip the disable flag. Implemented as
// a yes/no choice modal rather than an edit field since it's binary.
func (m *Model) openHomeBannerDisableModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	current := m.settings.DisableHomeBanner()
	state := choiceModalState{
		Title: "Home: disable banner animation",
		Hint:  fmt.Sprintf("currently %v · pick to set", current),
		Options: []choiceOption{
			{Label: "Disabled", Hint: "static banner — no rotation", Value: "true"},
			{Label: "Enabled", Hint: "animated rotation (default)", Value: "false"},
		},
		Cursor: 0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			m.settings.SetDisableHomeBanner(pick == "true")
			_ = m.settings.Save()
			m.flash("banner setting saved")
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// openDebugModal lists the developer/testing toggles.
func (m *Model) openDebugModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	forceHint := fmt.Sprintf("currently %v · show the first-launch welcome modal every launch",
		m.settings.DebugForceWelcome())
	return m.settingsSubmenu("Debug", "debug", []choiceOption{
		{Label: "Force welcome modal", Hint: forceHint, Value: "force_welcome"},
	})
}

// openDebugForceWelcomeModal flips the force-welcome toggle. When on, the
// welcome modal appears on every launch (welcome_seen is ignored) so it
// can be tested repeatedly.
func (m *Model) openDebugForceWelcomeModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	current := m.settings.DebugForceWelcome()
	state := choiceModalState{
		Title: "Debug: force welcome modal",
		Hint:  fmt.Sprintf("currently %v · pick to set", current),
		Options: []choiceOption{
			{Label: "On", Hint: "show the welcome modal on every launch (testing)", Value: "true"},
			{Label: "Off", Hint: "normal — show once, then never again (default)", Value: "false"},
		},
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			m.settings.SetDebugForceWelcome(pick == "true")
			_ = m.settings.Save()
			if pick == "true" {
				m.flash("Debug: welcome modal will show on every launch.")
			} else {
				m.flash("Debug: force-welcome off.")
			}
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// openHomeBannerHideModal — flip the hide-entirely flag. Distinct from
// disable (which only freezes the animation): hide removes the banner
// block from /home altogether.
func (m *Model) openHomeBannerHideModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	current := m.settings.HideHomeBanner()
	state := choiceModalState{
		Title: "Home: hide banner entirely",
		Hint:  fmt.Sprintf("currently %v · pick to set", current),
		Options: []choiceOption{
			{Label: "Hidden", Hint: "no banner — ORG card starts at the details", Value: "true"},
			{Label: "Shown", Hint: "banner visible (default)", Value: "false"},
		},
		Cursor: 0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			m.settings.SetHideHomeBanner(pick == "true")
			_ = m.settings.Save()
			m.flash("banner setting saved")
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// openListViewPreviewLimitModal — row cap for SF list-view chips on
// /records.
func (m *Model) openListViewPreviewLimitModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(
		"Chips: SF list-view preview limit",
		fmt.Sprintf("rows · blank or 0 = default (%d)", settings.DefaultListViewPreviewLimit),
		"listview preview limit saved",
		m.settings.ListViewPreviewLimit(),
		m.settings.SetListViewPreviewLimit,
	)
}

// ============================================================================
// Behaviour-group submenus + their leaf modals.
//
// The top-level settings menu drills into one of these per group; each
// lists the individual knobs as rows that re-emit openSettingsSubmenuMsg
// with a "<group>.<id>" pick. Leaf modals reuse numericEditModal /
// editModalState / the boolSettingModal + enumSettingModal helpers below.
// ============================================================================

// boolSettingModal renders a two-option on/off picker. trueFirst puts
// the "on" option at the top (cursor default). save persists; the
// caller's accessor + Set* twin handle the tri-state bookkeeping.
func (m *Model) boolSettingModal(title, onLabel, onHint, offLabel, offHint string, current bool, save func(bool)) tea.Cmd {
	cursor := 1
	if current {
		cursor = 0
	}
	state := choiceModalState{
		Title:  title,
		Hint:   fmt.Sprintf("currently %v · pick to set", current),
		Cursor: cursor,
		Options: []choiceOption{
			{Label: onLabel, Hint: onHint, Value: "true"},
			{Label: offLabel, Hint: offHint, Value: "false"},
		},
		OnSuccessTyped: func(val any) tea.Cmd {
			if m.settings == nil {
				return nil
			}
			pick, _ := val.(string)
			save(pick == "true")
			_ = m.settings.Save()
			m.flash("setting saved")
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// enumSettingModal renders a picker over a fixed set of string values.
// opts pairs the stored value with its display label/hint; current is
// the value highlighted on open.
func (m *Model) enumSettingModal(title string, opts []choiceOption, current string, save func(string)) tea.Cmd {
	cursor := 0
	for i, o := range opts {
		if v, _ := o.Value.(string); v == current {
			cursor = i
			break
		}
	}
	state := choiceModalState{
		Title:   title,
		Hint:    "Enter to apply  ·  Esc to cancel",
		Options: opts,
		Cursor:  cursor,
		OnSuccessTyped: func(val any) tea.Cmd {
			if m.settings == nil {
				return nil
			}
			v, _ := val.(string)
			save(v)
			_ = m.settings.Save()
			m.flash("setting saved")
			return nil
		},
	}
	return m.openChoiceModal(state)
}

// --- Appearance ---------------------------------------------------------

func (m *Model) openAppearanceModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	bannerHint := fmt.Sprintf("current: %d ms", m.settings.HomeBannerIntervalMs())
	if m.settings.DisableHomeBanner() {
		bannerHint = "current: disabled"
	}
	opts := []choiceOption{
		{Label: "Theme", Hint: "current: " + m.settings.Theme(), Value: "theme"},
		{Label: "Home: banner animation interval", Hint: bannerHint, Value: "home_banner_interval"},
		{Label: "Home: disable banner animation", Hint: fmt.Sprintf("current: %v · banner stays, just static", m.settings.DisableHomeBanner()), Value: "home_banner_disable"},
		{Label: "Home: hide banner entirely", Hint: fmt.Sprintf("current: %v · removes the banner, not just its motion", m.settings.HideHomeBanner()), Value: "home_banner_hide"},
	}
	return m.settingsSubmenu("Appearance", "appearance", opts)
}

// --- Startup & defaults -------------------------------------------------

func (m *Model) openStartupModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	s := m.settings
	opts := []choiceOption{
		{Label: "Sidebar open on launch", Hint: "current: " + onOff(s.StartupSidebarOpen(true)), Value: "sidebar_open"},
		{Label: "Sidebar position", Hint: "current: " + sidebarPositionLabel(s.SidebarPosition()) + " · right, bottom, or auto", Value: "sidebar_position"},
		{Label: "SOQL query line visible", Hint: "current: " + onOff(s.StartupQueryLineVisible(false)), Value: "query_line_visible"},
		{Label: "Left org rail pinned open", Hint: "current: " + onOff(s.StartupLeftRailOpen(false)), Value: "left_rail_open"},
		{Label: "Start tab", Hint: "current: " + s.StartupStartTab(), Value: "start_tab"},
		{Label: "Tab bar slots (1-8)", Hint: "choose which tabs the 1-8 number keys jump to", Value: "tab_bar"},
		{Label: "Default sort direction", Hint: "current: " + startupSortLabel(s.StartupDefaultSortDesc()), Value: "default_sort"},
		{Label: "Sort by Last Modified (q-s) direction", Hint: "current: " + startupSortLabel(s.ChordSortModifiedDesc()), Value: "chord_sort_modified"},
		{Label: "Global search default mode", Hint: "current: " + startupGSLabel(s.StartupGlobalSearchRecordsMode()), Value: "global_search_mode"},
		{Label: "SOQL editor seed query", Hint: "current: " + ansiTrunc(s.StartupSOQLSeed(), 40), Value: "soql_seed"},
	}
	return m.settingsSubmenu("Startup & defaults", "startup", opts)
}

func startupSortLabel(desc bool) string {
	if desc {
		return "descending"
	}
	return "ascending"
}

// sidebarPositionLabel names the sidebar-position value for hints.
func sidebarPositionLabel(pos string) string {
	switch pos {
	case settings.SidebarPositionBottom:
		return "bottom (stacked below main)"
	case settings.SidebarPositionAuto:
		return "auto (coming soon)"
	default:
		return "right (beside main)"
	}
}

// openSidebarPositionModal is the RHS / Bottom / Auto picker. Applies
// the choice live (the sidebar moves immediately) via a msg so the
// mutation lands on the live Model, then persists.
func (m *Model) openSidebarPositionModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	opts := []choiceOption{
		{Label: "Right (beside main)", Hint: "sidebar sits to the right of the main pane (default)", Value: settings.SidebarPositionRHS},
		{Label: "Bottom (stacked below main)", Hint: "sidebar sits below main (2/3 + 1/3) — more column width", Value: settings.SidebarPositionBottom},
		{Label: "Auto (coming soon)", Hint: "reactive placement by terminal width — not yet active; leaves the sidebar where it is", Value: settings.SidebarPositionAuto},
	}
	cursor := 0
	for i, o := range opts {
		if v, _ := o.Value.(string); v == m.settings.SidebarPosition() {
			cursor = i
			break
		}
	}
	state := choiceModalState{
		Title:   "Sidebar position",
		Hint:    "Enter to apply  ·  Esc to cancel",
		Options: opts,
		Cursor:  cursor,
		OnSuccessTyped: func(val any) tea.Cmd {
			pos, _ := val.(string)
			return func() tea.Msg { return sidebarPositionChangedMsg{pos: pos} }
		},
		OnCancel: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "startup"} }
		},
	}
	return m.openChoiceModal(state)
}

// sidebarPositionChangedMsg carries the picked sidebar position so
// Update can persist it AND apply it to the live Model (moving the
// sidebar immediately), sidestepping the captured-pointer staleness of
// the modal's OnSuccess closure.
type sidebarPositionChangedMsg struct{ pos string }

func startupGSLabel(records bool) string {
	if records {
		return "records (SOSL)"
	}
	return "metadata (local index)"
}

// ansiTrunc is a tiny helper for hint text — trims long values for the
// one-line hint without pulling in the ansi pkg here.
func ansiTrunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}

// --- Lists & limits -----------------------------------------------------

func (m *Model) openLimitsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	excluded := s.RecentExcludedKinds()
	excludedHint := "(none)"
	if len(excluded) > 0 {
		excludedHint = strings.Join(excluded, ", ")
	}
	opts := []choiceOption{
		{Label: "Recent records fetch", Hint: fmt.Sprintf("current: %d rows", s.LimitRecentRecords()), Value: "recent_records"},
		{Label: "Notifications", Hint: fmt.Sprintf("current: %d rows", s.LimitNotifications()), Value: "notifications"},
		{Label: "Deploy history", Hint: fmt.Sprintf("current: %d rows", s.LimitDeployHistory()), Value: "deploy_history"},
		{Label: "Home activity widgets (jobs)", Hint: fmt.Sprintf("current: %d rows", s.LimitAsyncJobHistory()), Value: "async_job_history"},
		{Label: "Recent logins (/users)", Hint: fmt.Sprintf("current: %d rows", s.LimitRecentLogins()), Value: "recent_logins"},
		{Label: "Reference-field picker results", Hint: fmt.Sprintf("current: %d rows", s.LimitReferencePicker()), Value: "reference_picker"},
		{Label: "Global search (SOSL) results", Hint: fmt.Sprintf("current: %d rows · SF max 50", s.LimitGlobalSearch()), Value: "global_search"},
		{Label: "── chips & recent ──", Hint: "", Value: "_sep", Heading: true},
		{Label: "Chips: default row cap", Hint: fmt.Sprintf("current: %d rows · per-chip Limit overrides", s.DefaultChipLimit()), Value: "chip_default_limit"},
		{Label: "Chips: SF list-view preview limit", Hint: fmt.Sprintf("current: %d rows", s.ListViewPreviewLimit()), Value: "listview_preview_limit"},
		{Label: "Recent: display cap", Hint: fmt.Sprintf("current: %d rows", s.RecentLimit()), Value: "recent_limit"},
		{Label: "Recent: max entries (local log)", Hint: fmt.Sprintf("current: %d", s.RecentMaxEntries()), Value: "recent_max_entries"},
		{Label: "Recent: excluded kinds", Hint: "current: " + excludedHint, Value: "recent_excluded_kinds"},
		{Label: "Recent: excluded SF types", Hint: "current: " + recentSFTypesHint(s.RecentExcludedSFTypes()), Value: "recent_excluded_sf_types"},
	}
	return m.settingsSubmenu("Lists & limits", "limits", opts)
}

// recentSFTypesHint summarises the SF-type exclusion list for the menu.
func recentSFTypesHint(types []string) string {
	if len(types) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%d types · %s", len(types), ansiTrunc(strings.Join(types, ", "), 44))
}

// --- Search -------------------------------------------------------------

func (m *Model) openSearchSettingsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	opts := []choiceOption{
		{Label: "Project-membership boost", Hint: fmt.Sprintf("current: %.2f · added to score for active-project items", s.LoadedProjectBoost()), Value: "search_project_boost"},
		{Label: "Recent-visit decay window", Hint: fmt.Sprintf("current: %d hours", s.RecentBoostDecayHours()), Value: "search_recent_decay"},
	}
	return m.settingsSubmenu("Search", "search", opts)
}

// --- Layout & sizing ----------------------------------------------------

func (m *Model) openLayoutModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	opts := []choiceOption{
		{Label: "Object drill: pinned subtabs", Hint: fmt.Sprintf("current: %d · rest go to More…", s.LayoutObjectPinnedSubtabs()), Value: "object_pinned_subtabs"},
		{Label: "SOQL autocomplete popup rows", Hint: fmt.Sprintf("current: %d rows", s.LayoutAutocompleteRows()), Value: "autocomplete_rows"},
		{Label: "Column resize step ([ / ])", Hint: fmt.Sprintf("current: %d cells", s.LayoutColumnResizeStep()), Value: "column_resize_step"},
		{Label: "Downloads modal visible rows", Hint: fmt.Sprintf("current: %d rows", s.LayoutDownloadsModalRows()), Value: "downloads_modal_rows"},
		{Label: "Command palette visible rows", Hint: fmt.Sprintf("current: %d rows", s.LayoutCommandPaletteRows()), Value: "command_palette_rows"},
		{Label: "Global search result rows", Hint: fmt.Sprintf("current: %d rows", s.LayoutGlobalSearchRows()), Value: "global_search_rows"},
	}
	return m.settingsSubmenu("Layout & sizing", "layout", opts)
}

// --- API & network ------------------------------------------------------

func (m *Model) openAPISettingsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	apiVer := s.APIVersionOverride()
	if apiVer == "" {
		apiVer = "(org default)"
	}
	opts := []choiceOption{
		{Label: "HTTP request timeout", Hint: fmt.Sprintf("current: %d s", s.APIHTTPTimeoutSec()), Value: "http_timeout"},
		{Label: "CLI (sf) timeout", Hint: fmt.Sprintf("current: %d s", s.APICLITimeoutSec()), Value: "cli_timeout"},
		{Label: "Retrieve/deploy shell-out timeout", Hint: fmt.Sprintf("current: %d s", s.APIRetrieveTimeoutSec()), Value: "retrieve_timeout"},
		{Label: "Deploy poll deadline", Hint: fmt.Sprintf("current: %d s", s.APIDeployTimeoutSec()), Value: "deploy_timeout"},
		{Label: "Deploy poll interval", Hint: fmt.Sprintf("current: %d ms", s.APIDeployPollMs()), Value: "deploy_poll"},
		{Label: "Deploys watch interval", Hint: fmt.Sprintf("current: %d s · /deploys live refresh while a deploy runs", s.APIDeployWatchSec()), Value: "deploy_watch"},
		{Label: "Bulk job poll interval", Hint: fmt.Sprintf("current: %d ms · ramps ½×→2× around this", s.APIBulkPollMs()), Value: "bulk_poll"},
		{Label: "Forced API version", Hint: "current: " + apiVer, Value: "api_version"},
	}
	return m.settingsSubmenu("API & network", "api", opts)
}

// --- Export -------------------------------------------------------------

func (m *Model) openExportSettingsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	exportDir := "(default ~)"
	if s.UI.ReportExportDir != "" {
		exportDir = s.UI.ReportExportDir
	}
	opts := []choiceOption{
		{Label: "Report export defaults", Hint: "save dir: " + exportDir + " · filename · post-processors", Value: "report_export"},
		{Label: "Export history cap", Hint: fmt.Sprintf("current: %d jobs", s.ExportHistoryMax()), Value: "export_history_max"},
	}
	return m.settingsSubmenu("Export", "export", opts)
}

// --- Integrations -------------------------------------------------------

func (m *Model) openIntegrationsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	s := m.settings
	inspector := "(unset)"
	if s.InspectorURL() != "" {
		inspector = s.InspectorURL()
	}
	browser := "(default)"
	if s.Browser() != "" {
		browser = s.Browser()
	}
	opts := []choiceOption{
		{Label: "Salesforce Inspector URL", Hint: "current: " + inspector, Value: "inspector"},
		{Label: "Browser (for extension URLs)", Hint: "current: " + browser, Value: "browser"},
		{Label: "Open auth (o key)", Hint: "current: " + s.OpenAuth() + " · frontdoor = auto-login via sfdx token, direct = reuse browser session", Value: "open_auth"},
		{Label: "Flow open version (o key)", Hint: "current: " + s.FlowOpenVersion() + " · latest = most recent version regardless of status, active = running version", Value: "flow_open_version"},
	}
	return m.settingsSubmenu("Integrations", "integrations", opts)
}

// settingsSubmenu is the shared builder for a group submenu: a choice
// modal whose picks re-emit openSettingsSubmenuMsg as "<group>.<id>".
func (m *Model) settingsSubmenu(title, group string, opts []choiceOption) tea.Cmd {
	state := choiceModalState{
		Title:   title,
		Hint:    "Enter to drill in  ·  Esc to go back",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			id, _ := val.(string)
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: group + "." + id} }
		},
		// Esc from a submenu pops back to the top-level settings menu.
		OnCancel: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "__root__"} }
		},
	}
	return m.openChoiceModal(state)
}

// --- Startup leaf modals ------------------------------------------------

func (m *Model) openStartupStartTabModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	opts := []choiceOption{
		{Label: "Home", Value: "home"},
		{Label: "SOQL", Value: "soql"},
		{Label: "Objects", Value: "objects"},
		{Label: "Flows", Value: "flows"},
		{Label: "Apex", Value: "apex"},
		{Label: "Users", Value: "users"},
		{Label: "Perms", Value: "perms"},
	}
	return m.enumSettingModal("Start tab", opts, m.settings.StartupStartTab(), m.settings.SetStartupStartTab)
}

func (m *Model) openStartupDefaultSortModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	cur := "asc"
	if m.settings.StartupDefaultSortDesc() {
		cur = "desc"
	}
	opts := []choiceOption{
		{Label: "Ascending", Hint: "first sort press goes ↑", Value: "asc"},
		{Label: "Descending", Hint: "first sort press goes ↓", Value: "desc"},
	}
	return m.enumSettingModal("Default sort direction", opts, cur, m.settings.SetStartupDefaultSort)
}

func (m *Model) openSortPerViewModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	state := choiceModalState{
		Title: "Sort scope",
		Hint:  fmt.Sprintf("currently %s · pick to set", sortScopeLabel(m.settings.SortPerView())),
		Options: []choiceOption{
			{Label: "Shared across views", Hint: "one sort per surface — follows you as you flip views (default)", Value: "shared"},
			{Label: "Per view", Hint: "each view remembers its own sort", Value: "view"},
		},
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			m.settings.SetSortPerView(pick == "view")
			_ = m.settings.Save()
			m.flash("sort scope: " + sortScopeLabel(pick == "view"))
			return nil
		},
	}
	return m.openChoiceModal(state)
}

func (m *Model) openChordSortModifiedModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	cur := "asc"
	if m.settings.ChordSortModifiedDesc() {
		cur = "desc"
	}
	opts := []choiceOption{
		{Label: "Newest first", Hint: "q-s starts descending ↓ (default)", Value: "desc"},
		{Label: "Oldest first", Hint: "q-s starts ascending ↑", Value: "asc"},
	}
	return m.enumSettingModal("Sort by Last Modified (q-s) direction", opts, cur, m.settings.SetChordSortModified)
}

func (m *Model) openStartupGlobalSearchModeModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	cur := "metadata"
	if m.settings.StartupGlobalSearchRecordsMode() {
		cur = "records"
	}
	opts := []choiceOption{
		{Label: "Metadata", Hint: "local index — sObjects, fields, flows…", Value: "metadata"},
		{Label: "Records", Hint: "SOSL across the org", Value: "records"},
	}
	return m.enumSettingModal("Global search default mode", opts, cur, m.settings.SetStartupGlobalSearchMode)
}

func (m *Model) openStartupSOQLSeedModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	state := editModalState{
		Title:       "SOQL editor seed query",
		Hint:        "query pre-loaded into a fresh SOQL editor · blank to reset to default",
		InitialBody: m.settings.StartupSOQLSeed(),
		Multiline:   false,
		SuccessMsg:  "seed query saved",
		Save: func(val string, _ any) error {
			m.settings.SetStartupSOQLSeed(val)
			return m.settings.Save()
		},
	}
	return m.openEditModal(state)
}

// dispatchSettingsPick routes a settings menu pick ("<group>" or
// "<group>.<id>") to the matching submenu / leaf modal. Keeps the big
// switch out of update.go.
func (m *Model) dispatchSettingsPick(pick string) tea.Cmd {
	cmd := m.dispatchSettingsPickInner(pick)
	// Leaf picks are "group.leaf" (e.g. "startup.auto_layout"). Wire
	// esc on the just-opened leaf modal to pop back to its parent
	// submenu instead of closing the whole settings stack. Top-level
	// group picks ("startup") and the root sentinel have no dot and
	// keep their own OnCancel (submenu → root, set by settingsSubmenu).
	if group, _, ok := strings.Cut(pick, "."); ok && group != "" {
		back := func() tea.Cmd {
			g := group
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: g} }
		}
		if m.choiceModal != nil {
			m.choiceModal.OnCancel = back
		} else if m.editModal != nil {
			m.editModal.OnCancel = back
		}
	}
	return cmd
}

func (m *Model) dispatchSettingsPickInner(pick string) tea.Cmd {
	// Per-slot tab-bar picks are "startup.tab_bar.slot.<N>" — a variable
	// tail the flat switch can't enumerate, so handle the prefix first.
	if rest, ok := strings.CutPrefix(pick, "startup.tab_bar.slot."); ok {
		if n, err := strconv.Atoi(rest); err == nil {
			return m.openTabBarSlotPicker(n)
		}
	}
	switch pick {
	// Back-navigation sentinel: esc from a submenu reopens the
	// top-level settings menu (see settingsSubmenu's OnCancel).
	case "__root__":
		return m.openSettingsModal()

	// Top-level groups.
	case "appearance":
		return m.openAppearanceModal()
	case "startup":
		return m.openStartupModal()
	case "input":
		return m.openInputModal()
	case "limits":
		return m.openLimitsModal()
	case "search":
		return m.openSearchSettingsModal()
	case "layout":
		return m.openLayoutModal()
	case "api":
		return m.openAPISettingsModal()
	case "cache":
		return m.openCacheSettingsModal()
	case "export":
		return m.openExportSettingsModal()
	case "integrations":
		return m.openIntegrationsModal()
	case "keybindings":
		return m.openKeybindingsModal()
	case "debug":
		return m.openDebugModal()

	// Debug leaves.
	case "debug.force_welcome":
		return m.openDebugForceWelcomeModal()

	// Appearance leaves.
	case "appearance.theme":
		return m.openThemePicker()
	case "appearance.home_banner_interval":
		return m.openHomeBannerIntervalModal()
	case "appearance.home_banner_disable":
		return m.openHomeBannerDisableModal()
	case "appearance.home_banner_hide":
		return m.openHomeBannerHideModal()

	// Startup leaves.
	case "startup.sidebar_open":
		return m.boolSettingModal("Sidebar open on launch",
			"Open", "right sidebar visible on launch",
			"Closed", "start with the sidebar hidden",
			m.settings.StartupSidebarOpen(true), m.settings.SetStartupSidebarOpen)
	case "startup.sidebar_position":
		return m.openSidebarPositionModal()
	case "startup.query_line_visible":
		return m.boolSettingModal("SOQL query line visible",
			"Visible", "show the SOQL line under the records chip strip",
			"Hidden", "hide it (default — saves a row)",
			m.settings.StartupQueryLineVisible(false), m.settings.SetStartupQueryLineVisible)
	case "startup.left_rail_open":
		return m.boolSettingModal("Left org rail pinned open",
			"Pinned open", "org rail starts expanded",
			"Collapsed", "org rail starts as a pill (default)",
			m.settings.StartupLeftRailOpen(false), m.settings.SetStartupLeftRailOpen)
	case "startup.start_tab":
		return m.openStartupStartTabModal()
	case "startup.tab_bar":
		return m.openTabBarModal()
	case "startup.tab_bar.reset":
		m.applyTabBarReset()
		return m.openTabBarModal()
	case "startup.default_sort":
		return m.openStartupDefaultSortModal()
	case "startup.chord_sort_modified":
		return m.openChordSortModifiedModal()
	case "startup.global_search_mode":
		return m.openStartupGlobalSearchModeModal()
	case "startup.soql_seed":
		return m.openStartupSOQLSeedModal()

	// Input leaves.
	case "input.jump_rows":
		return m.openJumpRowsModal()
	case "input.wheel_quiet_gap":
		return m.openWheelQuietGapModal()
	case "input.wheel_min_interval":
		return m.openWheelMinIntervalModal()
	case "input.wheel_max_step":
		return m.openWheelMaxStepModal()
	case "input.sort_per_view":
		return m.openSortPerViewModal()
	case "input.flow_version_enter":
		return m.boolSettingModal("Flow version: Enter behaviour",
			"Open Flow Builder", "Enter opens the version in Flow Builder (browser) — same as o",
			"View definition", "Enter drills into the in-terminal definition viewer (JSON)",
			m.settings.FlowVersionEnterOpens(), m.settings.SetFlowVersionEnterOpens)

	// Limits leaves.
	case "limits.recent_records":
		return m.limitEditModal("Recent records fetch", m.settings.LimitRecentRecords(), m.settings.SetLimitRecentRecords)
	case "limits.notifications":
		return m.limitEditModal("Notifications", m.settings.LimitNotifications(), m.settings.SetLimitNotifications)
	case "limits.deploy_history":
		return m.limitEditModal("Deploy history", m.settings.LimitDeployHistory(), m.settings.SetLimitDeployHistory)
	case "limits.async_job_history":
		return m.limitEditModal("Home activity widgets", m.settings.LimitAsyncJobHistory(), m.settings.SetLimitAsyncJobHistory)
	case "limits.recent_logins":
		return m.limitEditModal("Recent logins (/users)", m.settings.LimitRecentLogins(), m.settings.SetLimitRecentLogins)
	case "limits.reference_picker":
		return m.limitEditModal("Reference-field picker results", m.settings.LimitReferencePicker(), m.settings.SetLimitReferencePicker)
	case "limits.global_search":
		return m.limitEditModal("Global search (SOSL) results · SF max 50", m.settings.LimitGlobalSearch(), m.settings.SetLimitGlobalSearch)
	case "limits.chip_default_limit":
		return m.openChipDefaultLimitModal()
	case "limits.listview_preview_limit":
		return m.openListViewPreviewLimitModal()
	case "limits.recent_limit":
		return m.openRecentLimitModal()
	case "limits.recent_max_entries":
		return m.openRecentMaxEntriesModal()
	case "limits.recent_excluded_kinds":
		return m.openRecentExcludedKindsModal()
	case "limits.recent_excluded_sf_types":
		return m.openRecentExcludedSFTypesModal()

	// Search leaves.
	case "search.search_project_boost":
		return m.openSearchProjectBoostModal()
	case "search.search_recent_decay":
		return m.openSearchRecentDecayModal()

	// Layout leaves.
	case "layout.object_pinned_subtabs":
		return m.limitEditModal("Object drill: pinned subtabs", m.settings.LayoutObjectPinnedSubtabs(), m.settings.SetLayoutObjectPinnedSubtabs)
	case "layout.autocomplete_rows":
		return m.limitEditModal("SOQL autocomplete popup rows", m.settings.LayoutAutocompleteRows(), m.settings.SetLayoutAutocompleteRows)
	case "layout.column_resize_step":
		return m.limitEditModal("Column resize step", m.settings.LayoutColumnResizeStep(), m.settings.SetLayoutColumnResizeStep)
	case "layout.downloads_modal_rows":
		return m.limitEditModal("Downloads modal visible rows", m.settings.LayoutDownloadsModalRows(), m.settings.SetLayoutDownloadsModalRows)
	case "layout.command_palette_rows":
		return m.limitEditModal("Command palette visible rows", m.settings.LayoutCommandPaletteRows(), m.settings.SetLayoutCommandPaletteRows)
	case "layout.global_search_rows":
		return m.limitEditModal("Global search result rows", m.settings.LayoutGlobalSearchRows(), m.settings.SetLayoutGlobalSearchRows)

	// API leaves. Saving any of these re-applies the live sf config.
	case "api.http_timeout":
		return m.apiEditModal("HTTP request timeout (seconds)", m.settings.APIHTTPTimeoutSec(), m.settings.SetAPIHTTPTimeoutSec)
	case "api.cli_timeout":
		return m.apiEditModal("CLI (sf) timeout (seconds)", m.settings.APICLITimeoutSec(), m.settings.SetAPICLITimeoutSec)
	case "api.retrieve_timeout":
		return m.apiEditModal("Retrieve/deploy shell-out timeout (seconds)", m.settings.APIRetrieveTimeoutSec(), m.settings.SetAPIRetrieveTimeoutSec)
	case "api.deploy_timeout":
		return m.apiEditModal("Deploy poll deadline (seconds)", m.settings.APIDeployTimeoutSec(), m.settings.SetAPIDeployTimeoutSec)
	case "api.deploy_poll":
		return m.apiEditModal("Deploy poll interval (ms)", m.settings.APIDeployPollMs(), m.settings.SetAPIDeployPollMs)
	case "api.deploy_watch":
		// Read live at each tick arm — no sf-config re-apply needed, but
		// apiEditModal's harmless re-apply keeps the leaves uniform.
		return m.apiEditModal("Deploys watch interval (seconds)", m.settings.APIDeployWatchSec(), m.settings.SetAPIDeployWatchSec)
	case "api.bulk_poll":
		return m.apiEditModal("Bulk job poll interval (ms)", m.settings.APIBulkPollMs(), m.settings.SetAPIBulkPollMs)
	case "api.api_version":
		return m.openAPIVersionModal()

	// Export leaves.
	case "export.report_export":
		return m.openReportExportSettingsModal()
	case "export.export_history_max":
		return m.openExportHistoryMaxModal()

	// Integrations leaves.
	case "integrations.inspector":
		return m.openInspectorURLModal()
	case "integrations.open_auth":
		cur := m.settings.OpenAuth()
		return m.openChoiceModal(choiceModalState{
			Title: "Open auth (o key)",
			Hint:  "How `o` authenticates the browser",
			Options: []choiceOption{
				{Label: "frontdoor", Hint: "one-time login URL from the sfdx token — works with no browser session (current: " + cur + ")", Value: "frontdoor"},
				{Label: "direct", Hint: "plain URL — reuses the existing browser session; login page if none", Value: "direct"},
				{Label: "Cancel", Cancel: true},
			},
			OnSuccessTyped: func(val any) tea.Cmd {
				mode, _ := val.(string)
				if mode == "" {
					return nil
				}
				m.settings.SetOpenAuth(mode)
				m.saveSettings("open auth → " + mode)
				return nil
			},
		})
	case "integrations.flow_open_version":
		cur := m.settings.FlowOpenVersion()
		return m.openChoiceModal(choiceModalState{
			Title: "Flow open version (o key)",
			Hint:  "Which flow version `o` opens from the flows list",
			Options: []choiceOption{
				{Label: "latest", Hint: "most recent version regardless of status — the draft when one is newer than active (current: " + cur + ")", Value: "latest"},
				{Label: "active", Hint: "the running version; a newer draft stays in the " + firstPretty(Keys.OpenMenu) + " menu", Value: "active"},
				{Label: "Cancel", Cancel: true},
			},
			OnSuccessTyped: func(val any) tea.Cmd {
				mode, _ := val.(string)
				if mode == "" {
					return nil
				}
				m.settings.SetFlowOpenVersion(mode)
				m.saveSettings("flow open version → " + mode)
				// Flow.Targets() reads the sf package config, so push the
				// change down immediately — no restart needed.
				applySFConfig(m.settings)
				return nil
			},
		})
	case "integrations.browser":
		return m.openBrowserModal()
	}
	return nil
}

// limitEditModal is numericEditModal with the standard "blank or 0 =
// default" hint, for the row-count / sizing knobs.
func (m *Model) limitEditModal(title string, current int, save func(int)) tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(title, "blank or 0 = built-in default", "setting saved", current, save)
}

// apiEditModal is limitEditModal that ALSO re-applies the live sf
// config after saving, so API timeout / poll changes take effect
// immediately without a restart.
func (m *Model) apiEditModal(title string, current int, save func(int)) tea.Cmd {
	if m.settings == nil {
		return nil
	}
	state := editModalState{
		Title:       title,
		Hint:        "blank or 0 = built-in default · applies to new + existing connections",
		InitialBody: strconv.Itoa(current),
		Multiline:   false,
		SuccessMsg:  "setting saved · applied",
		Save: func(val string, _ any) error {
			n, _ := strconv.Atoi(val)
			save(n)
			if err := m.settings.Save(); err != nil {
				return err // leave runtime untouched on a failed save
			}
			applyAPIConfigLive(m.settings)
			return nil
		},
	}
	return m.openEditModal(state)
}

// openAPIVersionModal edits the forced API version. Blank = use the
// org-reported version. Re-applies the live sf config on save.
func (m *Model) openAPIVersionModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	state := editModalState{
		Title:       "Forced API version",
		Hint:        "e.g. 65.0 · blank = use the org-reported version · re-connects open orgs",
		InitialBody: m.settings.APIVersionOverride(),
		Multiline:   false,
		SuccessMsg:  "API version saved · applied",
		Save: func(val string, _ any) error {
			m.settings.SetAPIVersionOverride(val)
			if err := m.settings.Save(); err != nil {
				return err
			}
			applyAPIConfigLive(m.settings)
			return nil
		},
	}
	return m.openEditModal(state)
}

// applyAPIConfigLive pushes the saved [ui.api] settings into the sf
// package AND drops every cached REST client. Existing clients bake
// the HTTP timeout + apiVersion into themselves at bootstrap, so
// without the invalidation a saved change would only affect orgs
// connected for the first time after the change — open orgs would keep
// the old values, contradicting the "applied" flash. Next API call per
// org re-bootstraps with the new config (one extra `sf org display`).
func applyAPIConfigLive(st *settings.Settings) {
	applySFConfig(st)
	sf.InvalidateRESTClients()
}
