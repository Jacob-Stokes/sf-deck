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
		{Label: "Updates", Hint: "stable releases · " + m.updateStatusLabel(), Value: "updates"},
		{Label: "Privacy & local data", Hint: "what stays in memory, what is stored, and how to erase it", Value: "privacy"},
		{Label: "Keybindings", Hint: "edit and save key bindings", Value: "keybindings"},
		{Label: "Debug", Hint: "developer/testing toggles (force welcome modal)", Value: "debug"},
		{Label: "About sf-deck", Hint: aboutSettingsHint(), Value: "about"},
	}
	state := choiceModalState{
		Title:   "Settings",
		Hint:    "Enter to drill in  ·  Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
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

func sortScopeLabel(perView bool) string {
	if perView {
		return "per view (each view keeps its own sort)"
	}
	return "shared across views"
}

func flowVersionEnterLabel(opens bool) string {
	if opens {
		return "open Flow Builder"
	}
	return "view definition (in-terminal)"
}

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
				m.settings.SetRecentExcludedSFTypes(nil)
				m.settings.UI.Recent.UserSetExcludedSFTypes = false
			} else {
				m.settings.SetRecentExcludedSFTypes(types)
			}
			return m.settings.Save()
		},
		OnSuccess: func() tea.Cmd {
			if d := m.activeOrgData(); d != nil {
				return d.RecentlyViewed.Refresh(m.cache)
			}
			return nil
		},
	}
	return m.openEditModal(state)
}

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

func (m *Model) openRecentExcludedKindsModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
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
		{Label: "Sidebar position", Hint: "current: " + sidebarPositionLabel(s.SidebarPosition()) + " · right or bottom", Value: "sidebar_position"},
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

func sidebarPositionLabel(pos string) string {
	switch pos {
	case settings.SidebarPositionBottom:
		return "bottom (stacked below main)"
	case settings.SidebarPositionAuto:
		return "right (beside main)"
	default:
		return "right (beside main)"
	}
}

func (m *Model) openSidebarPositionModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	opts := []choiceOption{
		{Label: "Right (beside main)", Hint: "sidebar sits to the right of the main pane (default)", Value: settings.SidebarPositionRHS},
		{Label: "Bottom (stacked below main)", Hint: "sidebar sits below main (2/3 + 1/3) — more column width", Value: settings.SidebarPositionBottom},
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

type sidebarPositionChangedMsg struct{ pos string }

func startupGSLabel(records bool) string {
	if records {
		return "records (SOSL)"
	}
	return "metadata (local index)"
}

func ansiTrunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}

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

func recentSFTypesHint(types []string) string {
	if len(types) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%d types · %s", len(types), ansiTrunc(strings.Join(types, ", "), 44))
}

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
		OnCancel: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "__root__"} }
		},
	}
	return m.openChoiceModal(state)
}

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
	case "updates":
		return m.openUpdatesModal()
	case "privacy":
		return m.openPrivacyModal()
	case "keybindings":
		return m.openKeybindingsModal()
	case "debug":
		return m.openDebugModal()
	case "about":
		return m.openAboutModal()

	case "updates.automatic":
		return m.boolSettingModal("Automatic update checks",
			"On", "check GitHub Releases at most once every 24 hours",
			"Off", "only check when you explicitly ask",
			m.settings.AutomaticUpdateChecks(), m.settings.SetAutomaticUpdateChecks)
	case "updates.check_now":
		return m.updateCheckCmd(true)

	case "debug.force_welcome":
		return m.openDebugForceWelcomeModal()

	case "appearance.theme":
		return m.openThemePicker()
	case "appearance.home_banner_interval":
		return m.openHomeBannerIntervalModal()
	case "appearance.home_banner_disable":
		return m.openHomeBannerDisableModal()
	case "appearance.home_banner_hide":
		return m.openHomeBannerHideModal()

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

	case "search.search_project_boost":
		return m.openSearchProjectBoostModal()
	case "search.search_recent_decay":
		return m.openSearchRecentDecayModal()

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
		return m.apiEditModal("Deploys watch interval (seconds)", m.settings.APIDeployWatchSec(), m.settings.SetAPIDeployWatchSec)
	case "api.bulk_poll":
		return m.apiEditModal("Bulk job poll interval (ms)", m.settings.APIBulkPollMs(), m.settings.SetAPIBulkPollMs)
	case "api.api_version":
		return m.openAPIVersionModal()

	case "export.report_export":
		return m.openReportExportSettingsModal()
	case "export.export_history_max":
		return m.openExportHistoryMaxModal()

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
				applySFConfig(m.settings)
				return nil
			},
		})
	case "integrations.browser":
		return m.openBrowserModal()
	}
	return nil
}

func (m *Model) limitEditModal(title string, current int, save func(int)) tea.Cmd {
	if m.settings == nil {
		return nil
	}
	return m.numericEditModal(title, "blank or 0 = built-in default", "setting saved", current, save)
}

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

func applyAPIConfigLive(st *settings.Settings) {
	applySFConfig(st)
	sf.InvalidateRESTClients()
}
