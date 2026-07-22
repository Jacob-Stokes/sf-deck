package settings

import "testing"

// These cover the new [ui.startup] / [ui.limits] / [ui.layout] / [ui.api]
// accessors: nil-safety, the zero-value fallback rule, overrides,
// negative clamping, the tri-state startup booleans, and the
// global-search SF-50 hard clamp.

func TestStartupBools_TriState(t *testing.T) {
	// nil settings → built-in default passed through unchanged.
	var nilS *Settings
	if !nilS.StartupSidebarOpen(true) {
		t.Errorf("nil StartupSidebarOpen(true) = false, want true")
	}
	if nilS.StartupSidebarOpen(false) {
		t.Errorf("nil StartupSidebarOpen(false) = true, want false")
	}

	s := &Settings{}
	// Unset (Set flag false) → built-in default wins regardless of the
	// stored bool's zero value.
	if !s.StartupSidebarOpen(true) {
		t.Errorf("unset StartupSidebarOpen(true) = false, want true (default)")
	}
	// User explicitly set false → must override a true default.
	s.UI.Startup.SidebarOpen = false
	s.UI.Startup.SidebarOpenSet = true
	if s.StartupSidebarOpen(true) {
		t.Errorf("explicit false StartupSidebarOpen(true) = true, want false")
	}
	// User explicitly set true → must override a false default.
	s.UI.Startup.SidebarOpen = true
	if !s.StartupSidebarOpen(false) {
		t.Errorf("explicit true StartupSidebarOpen(false) = false, want true")
	}
}

func TestStartupStringDefaults(t *testing.T) {
	s := &Settings{}
	if got := s.StartupStartTab(); got != StartupStartTabFallback {
		t.Errorf("StartupStartTab() = %q, want %q", got, StartupStartTabFallback)
	}
	if got := s.StartupSOQLSeed(); got != StartupSOQLSeedFallback {
		t.Errorf("StartupSOQLSeed() = %q, want fallback", got)
	}
	if s.StartupDefaultSortDesc() {
		t.Errorf("default StartupDefaultSortDesc() = true, want false")
	}
	if s.StartupGlobalSearchRecordsMode() {
		t.Errorf("default StartupGlobalSearchRecordsMode() = true, want false")
	}

	s.UI.Startup.StartTab = "soql"
	s.UI.Startup.SOQLSeed = "SELECT Id FROM Contact"
	s.UI.Startup.DefaultSort = "DESC" // case-insensitive
	s.UI.Startup.GlobalSearchMode = "records"
	if got := s.StartupStartTab(); got != "soql" {
		t.Errorf("StartupStartTab() = %q, want soql", got)
	}
	if got := s.StartupSOQLSeed(); got != "SELECT Id FROM Contact" {
		t.Errorf("StartupSOQLSeed() = %q, want override", got)
	}
	if !s.StartupDefaultSortDesc() {
		t.Errorf("StartupDefaultSortDesc() = false, want true for 'DESC'")
	}
	if !s.StartupGlobalSearchRecordsMode() {
		t.Errorf("StartupGlobalSearchRecordsMode() = false, want true")
	}
}

func TestLimitAccessors_ZeroFallbackAndClamp(t *testing.T) {
	var nilS *Settings
	if got := nilS.LimitRecentRecords(); got != LimitRecentRecordsFallback {
		t.Errorf("nil LimitRecentRecords() = %d, want %d", got, LimitRecentRecordsFallback)
	}

	s := &Settings{}
	// Zero → fallback.
	if got := s.LimitNotifications(); got != LimitNotificationsFallback {
		t.Errorf("zero LimitNotifications() = %d, want %d", got, LimitNotificationsFallback)
	}
	// Override honoured.
	s.UI.Limits.Notifications = 7
	if got := s.LimitNotifications(); got != 7 {
		t.Errorf("LimitNotifications() = %d, want 7", got)
	}
	// Negative clamps to 1.
	s.UI.Limits.DeployHistory = -5
	if got := s.LimitDeployHistory(); got != 1 {
		t.Errorf("negative LimitDeployHistory() = %d, want 1", got)
	}
}

func TestLimitGlobalSearch_HardClampTo50(t *testing.T) {
	s := &Settings{}
	s.UI.Limits.GlobalSearch = 500 // user asks for more than SF allows
	if got := s.LimitGlobalSearch(); got != 50 {
		t.Errorf("LimitGlobalSearch() = %d, want 50 (SF hard max)", got)
	}
	s.UI.Limits.GlobalSearch = 0 // fallback also respects the cap
	if got := s.LimitGlobalSearch(); got != LimitGlobalSearchFallback {
		t.Errorf("zero LimitGlobalSearch() = %d, want %d", got, LimitGlobalSearchFallback)
	}
}

func TestLayoutAccessors_ZeroFallbackAndFloor(t *testing.T) {
	s := &Settings{}
	if got := s.LayoutObjectPinnedSubtabs(); got != LayoutObjectPinnedSubtabsFallback {
		t.Errorf("zero LayoutObjectPinnedSubtabs() = %d, want %d", got, LayoutObjectPinnedSubtabsFallback)
	}
	// Override.
	s.UI.Layout.AutocompleteRows = 12
	if got := s.LayoutAutocompleteRows(); got != 12 {
		t.Errorf("LayoutAutocompleteRows() = %d, want 12", got)
	}
	// Below the per-knob floor clamps up (autocomplete floor is 3).
	s.UI.Layout.AutocompleteRows = 1
	if got := s.LayoutAutocompleteRows(); got != 3 {
		t.Errorf("LayoutAutocompleteRows() = %d, want floor 3", got)
	}
}

func TestAPIAccessors_ZeroFallback(t *testing.T) {
	var nilS *Settings
	if got := nilS.APIHTTPTimeoutSec(); got != APIHTTPTimeoutSecFallback {
		t.Errorf("nil APIHTTPTimeoutSec() = %d, want %d", got, APIHTTPTimeoutSecFallback)
	}
	s := &Settings{}
	if got := s.APIDeployPollMs(); got != APIDeployPollMsFallback {
		t.Errorf("zero APIDeployPollMs() = %d, want %d", got, APIDeployPollMsFallback)
	}
	s.UI.API.HTTPTimeoutSec = 120
	if got := s.APIHTTPTimeoutSec(); got != 120 {
		t.Errorf("APIHTTPTimeoutSec() = %d, want 120", got)
	}
	// API version: empty unless explicitly set; trimmed.
	if got := s.APIVersionOverride(); got != "" {
		t.Errorf("default APIVersionOverride() = %q, want empty", got)
	}
	s.UI.API.APIVersion = "  65.0  "
	if got := s.APIVersionOverride(); got != "65.0" {
		t.Errorf("APIVersionOverride() = %q, want trimmed 65.0", got)
	}
}
