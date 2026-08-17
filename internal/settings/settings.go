// Package settings reads + writes ~/.sf-deck/settings.toml.
package settings

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

// Settings is the full on-disk config. All maps/keys default to zero
// which means "no override" — callers fall back to Defaults.
type Settings struct {
	Defaults Defaults             `toml:"defaults"`
	Orgs     map[string]OrgConfig `toml:"orgs"`
	UI       UIConfig             `toml:"ui"`

	mu           sync.RWMutex
	loadedDigest string
}

// UIConfig contains persisted interface preferences.
type UIConfig struct {
	Theme           string           `toml:"theme"` // e.g. "tokyo-night", "catppuccin"
	ThemeFavourites []string         `toml:"theme_favourites,omitempty"`
	Cache           CacheConfig      `toml:"cache"`
	Extensions      ExtensionsConfig `toml:"extensions"`
	Debug           DebugConfig      `toml:"debug"`
	Updates         UpdatesConfig    `toml:"updates,omitempty"`

	SortPerView string `toml:"sort_per_view,omitempty"`

	SidebarPosition string `toml:"sidebar_position,omitempty"`

	// WelcomeSeen is set true once the first-launch welcome modal has
	// been shown and dismissed. Absent/false means "never launched" —
	// the trigger for the welcome overlay. Persisted so it fires exactly
	// once, even for users who connect an org via the sf CLI before ever
	// opening the TUI.
	WelcomeSeen bool `toml:"welcome_seen,omitempty"`

	// LegalAcceptedVersion records the privacy/user-terms revision accepted
	// before sf-deck first contacts a real Salesforce org. The timestamp is
	// informational; the version is the actual gate. Demo mode never needs
	// acceptance because it cannot access Salesforce.
	LegalAcceptedVersion string `toml:"legal_accepted_version,omitempty"`
	LegalAcceptedAt      string `toml:"legal_accepted_at,omitempty"`

	DemoOrgImported bool `toml:"demo_org_imported,omitempty"`

	// Chips is the unified user-defined chip catalogue. Each entry
	// is a query.QueryYAML plus identity/origin metadata. Domain
	// (records / objects / flows / …) is encoded in Domain so a
	// single list per user covers every surface.
	//
	// Replaces the legacy Lenses + ObjectFilters + FlowFilters trio
	// (kept here for one release so existing settings files still
	// load — the UI layer migrates entries into Chips on first run
	// and the next Save() drops the old sections).
	Chips []ChipConfig `toml:"chips,omitempty"`

	ChipBuiltinFavOverrides map[string]bool `toml:"chip_builtin_fav_overrides,omitempty"`

	// Legacy fields — kept for one release for backwards compat. The
	// UI layer reads them at startup, converts to ChipConfig, then
	// calls ClearLegacyChips so Save writes only the new format.
	Lenses        []LensConfig   `toml:"lenses,omitempty"`
	ObjectFilters []FilterConfig `toml:"object_filters,omitempty"`
	FlowFilters   []FilterConfig `toml:"flow_filters,omitempty"`

	// TagColumnHidden is the legacy on/off flag retained for one
	// release of TOML compat. New writes use TagColumnMode below;
	// reads fall back to this when TagColumnMode is empty (zero
	// value of the new enum). When both are absent, the gutter
	// renders in compact (dot) mode.
	TagColumnHidden bool `toml:"tag_column_hidden,omitempty"`

	TagColumnMode string `toml:"tag_column_mode,omitempty"`

	// ProjectColumnHidden hides the synthetic project-membership
	// gutter (sister to the tag gutter). Same inverted shape as
	// TagColumnHidden — zero-value = column visible. Toggled via
	// Ctrl+P; persists across sessions.
	ProjectColumnHidden bool `toml:"project_column_hidden,omitempty"`

	FlagColumnMode string `toml:"flag_column_mode,omitempty"`

	ReportExportPostProcessors  map[string][]string `toml:"report_export_postprocessors,omitempty"`
	ReportExportDefault         []string            `toml:"report_export_default,omitempty"`
	ReportExportDir             string              `toml:"report_export_dir,omitempty"`
	ReportExportFilenamePattern string              `toml:"report_export_filename_pattern,omitempty"`

	RecentByOrg map[string][]RecentConfig `toml:"recent_by_org,omitempty"`

	TreeChipByOrg map[string]map[string]TreeChipConfig `toml:"treechip_by_org,omitempty"`

	// LoadedDevProjectByOrg maps org-user → loaded dev-project id.
	// Persists across sessions so users don't have to re-load their
	// working project every restart. Stale ids (project deleted while
	// sf-deck wasn't running) are tolerated — the Scope hydrator
	// returns an empty Scope and the surface code clears the entry
	// the next time the user explicitly re-loads.
	//
	// Per-org rather than global because different orgs can plausibly
	// have different active projects at the same time. Backward-compat:
	// the older `loaded_org_project_by_org` toml key is read once at
	// load time and copied into this map, then dropped on next save.
	LoadedDevProjectByOrg map[string]string `toml:"loaded_dev_project_by_org,omitempty"`
	// Legacy field — pre-flatten layout. Read once for migration; not
	// written back. Cleared after the first successful save under the
	// new key.
	LoadedOrgProjectByOrgLegacy map[string]string `toml:"loaded_org_project_by_org,omitempty"`

	Input InputConfig `toml:"input,omitempty"`

	Recent RecentConfigSection `toml:"recent,omitempty"`

	Exports ExportsConfigSection `toml:"exports,omitempty"`

	Search SearchConfigSection `toml:"search,omitempty"`

	Home HomeConfigSection `toml:"home,omitempty"`

	ChipDefaults ChipsConfigSection `toml:"chip_defaults,omitempty"`

	TabBar TabBarConfig `toml:"tab_bar,omitempty"`

	// OrgGroups is the user's tree-shaped grouping of authenticated
	// orgs in the left rail. Each authed org belongs to exactly one
	// group (or to none — those render under a synthetic "Ungrouped"
	// section at the bottom). The list of authed orgs itself is
	// owned by `sf` (~/.sfdx/); we just decorate it. Stale members
	// (orgs that have since been logged out) are pruned on every
	// refresh.
	OrgGroups OrgGroupsConfig `toml:"org_groups,omitempty"`

	Compare CompareConfig `toml:"compare,omitempty"`

	Startup StartupConfigSection `toml:"startup,omitempty"`

	Limits LimitsConfigSection `toml:"limits,omitempty"`

	Layout LayoutConfigSection `toml:"layout,omitempty"`

	// API tunes the Salesforce API client behaviour — HTTP/CLI
	// timeouts, deploy + bulk poll cadence, default API version. The
	// sf package can't import settings (it's a lower layer), so the UI
	// pushes these into sf via sf.ApplyConfig at startup.
	API APIConfigSection `toml:"api,omitempty"`
}

// InputConfig tunes navigation behaviour: jump-step size, wheel
// scroll feel.
type InputConfig struct {
	// JumpRows is the row count for ctrl+arrow / J / K nav. 0 falls
	// back to the package default (5). Negatives clamp to 1.
	JumpRows int `toml:"jump_rows,omitempty"`

	// WheelQuietGapMs is the idle time (in ms) after which the wheel
	// throttle resets — the next tick is treated as the start of a
	// fresh gesture and accepted immediately. Lower = faster
	// re-engagement (good for high-DPI mice); higher = more grouping
	// (good for inertial trackpads that emit a long tail). 0 falls
	// back to the package default (80ms). Negatives clamp to 1.
	WheelQuietGapMs int `toml:"wheel_quiet_gap_ms,omitempty"`

	// WheelMinIntervalMs is the minimum gap (in ms) between accepted
	// wheel ticks within a single gesture. Lower = smoother but more
	// CPU; higher = chunkier but less work. 0 falls back to the
	// package default (24ms). Negatives clamp to 1.
	WheelMinIntervalMs int `toml:"wheel_min_interval_ms,omitempty"`

	// WheelMaxStep caps the cursor delta per accepted wheel tick
	// in continuous-scroll mode. The wheel runtime accumulates raw
	// events (one per inertial scroll tick) into a pending delta
	// and drains it on each accepted render — so a fast trackpad
	// flick that produces 200 events naturally wants to advance
	// the cursor 200 rows per frame, which feels like teleportation.
	// Capping smooths the motion: a 200-event flick still scrolls
	// 200 rows but spread across more frames. 0 falls back to
	// DefaultWheelMaxStep (20). Negatives clamp to 1.
	WheelMaxStep int `toml:"wheel_max_step,omitempty"`

	FlowVersionEnterOpens    bool `toml:"flow_version_enter_opens,omitempty"`
	FlowVersionEnterOpensSet bool `toml:"flow_version_enter_opens_set,omitempty"`
}

// TabBarConfig customises slots 1-8 on the top tab bar. Slot 9 is
// always the "More…" overflow modal sentinel — every tab not in
// Pinned is reachable from there.
type TabBarConfig struct {
	Pinned []string `toml:"pinned,omitempty"`

	// UserSetPinned mirrors the recent-excluded-kinds pattern —
	// distinguishes "user explicitly cleared" (empty list, all
	// tabs in overflow) from "user never touched it" (use defaults).
	UserSetPinned bool `toml:"user_set_pinned,omitempty"`
}

// PinnedTabs returns the user's configured slot-1-through-8 list,
// or nil when unset (caller falls back to package defaults).
func (s *Settings) PinnedTabs() []string {
	if s == nil || !s.UI.TabBar.UserSetPinned {
		return nil
	}
	out := make([]string, len(s.UI.TabBar.Pinned))
	copy(out, s.UI.TabBar.Pinned)
	return out
}

// SetPinnedTabs replaces the slot list. Marks the setting as
// user-touched so subsequent reads don't fall back to defaults.
// Pass nil to opt OUT of all defaults (every tab in overflow).
func (s *Settings) SetPinnedTabs(ids []string) {
	if s == nil {
		return
	}
	s.UI.TabBar.UserSetPinned = true
	if ids == nil {
		s.UI.TabBar.Pinned = []string{}
		return
	}
	out := make([]string, len(ids))
	copy(out, ids)
	s.UI.TabBar.Pinned = out
}

// RecentConfigSection tunes the /recent surface.
type RecentConfigSection struct {
	// Limit caps the rendered /recent list AFTER merge + chip
	// filter + MRU sort. 0 falls back to the package default (50).
	// Negatives clamp to 1.
	Limit int `toml:"limit,omitempty"`

	// MaxEntries caps the per-org local visit log (the bag we
	// remember; display cap above narrows what's shown). Older
	// entries fall off the tail of the FIFO. 0 falls back to the
	// package default (50). Negatives clamp to 1.
	MaxEntries int `toml:"max_entries,omitempty"`

	ExcludedKinds []string `toml:"excluded_kinds,omitempty"`

	// excludedKindsSet is set non-nil by SetRecentExcludedKinds to
	// distinguish "user explicitly cleared" from "user never
	// touched it". RecentExcludedKinds() returns defaults only when
	// this flag is unset.
	UserSetExcludedKinds bool `toml:"excluded_kinds_set,omitempty"`

	ExcludedSFTypes        []string `toml:"excluded_sf_types,omitempty"`
	UserSetExcludedSFTypes bool     `toml:"excluded_sf_types_set,omitempty"`
}

// ExportsConfigSection tunes the in-memory + on-disk export tracker.
type ExportsConfigSection struct {
	// HistoryMax caps the export tracker's history list (completed
	// + failed jobs). New entries push old ones off the tail once
	// the cap is hit. 0 falls back to the package default (200).
	// Negatives clamp to 1.
	HistoryMax int `toml:"history_max,omitempty"`
}

// SearchConfigSection tunes search behaviour: the global modal's
// ranking knobs AND the adaptive-debounce filter for list-table
// search inputs.
type SearchConfigSection struct {
	LoadedProjectBoost float64 `toml:"loaded_project_boost,omitempty"`

	// RecentBoostDecayHours controls how long a recent visit keeps
	// boosting search rank. Visits within this window get a decayed
	// boost (max → 0 linearly); older ones contribute nothing. 0
	// falls back to the package default (24). Negatives clamp to 1.
	RecentBoostDecayHours int `toml:"recent_boost_decay_hours,omitempty"`

	DebounceMs int `toml:"debounce_ms,omitempty"`

	FastFilterThresholdMs int `toml:"fast_filter_threshold_ms,omitempty"`
}

// HomeConfigSection tunes the /home landing surface.
type HomeConfigSection struct {
	// BannerIntervalMs controls the cloud-banner animation tick. 0
	// falls back to the package default (400ms). Negative or
	// extremely small values clamp to 50ms (anything faster wastes
	// CPU). Set the explicit DisableBanner field below to turn the
	// animation off entirely.
	BannerIntervalMs int `toml:"banner_interval_ms,omitempty"`

	DisableBanner bool `toml:"disable_banner,omitempty"`

	HideBanner bool `toml:"hide_banner,omitempty"`
}

// UpdatesConfig controls release discovery. The paired Set field preserves
// the built-in default (enabled) while still allowing a user to explicitly
// choose false.
type UpdatesConfig struct {
	Automatic    bool `toml:"automatic,omitempty"`
	AutomaticSet bool `toml:"automatic_set,omitempty"`
}

// ChipsConfigSection tunes shared chip-driven fetch behaviour. Used
// by both /records (server-side SOQL chips) and /users · All users
// (the same shape, different sObject).
type ChipsConfigSection struct {
	// DefaultLimit is the per-chip row cap inherited by built-in
	// chips that don't pin their own Limit. 0 falls back to the
	// package default (DefaultChipLimitFallback). Negatives clamp
	// to 1. The cap stops cursor-following once we've pulled this
	// many rows, but Salesforce's first-page totalSize still
	// reports the true unbounded match count so the UI can show a
	// "showing X of Y · capped" hint.
	DefaultLimit int `toml:"default_limit,omitempty"`

	// ListViewPreviewLimit caps the row count when /records is
	// driven by a Salesforce List View chip (vs a SOQL-defined
	// chip). The List View describe endpoint returns matched IDs;
	// we fetch a preview window of N. 0 falls back to the package
	// default (50). Negatives clamp to 1.
	ListViewPreviewLimit int `toml:"listview_preview_limit,omitempty"`
}

// StartupConfigSection captures the initial-Model defaults model.go
// bakes in. Every bool is a *tri-state* via a paired "...Set" flag so
// "user wants false" is distinguishable from "user never said" — the
// zero value of a plain bool is false, which would otherwise be
// indistinguishable from "leave the built-in default." Accessors take
// the built-in default as an argument and return it untouched unless
// the user set an override.
type StartupConfigSection struct {
	SidebarOpen    bool `toml:"sidebar_open,omitempty"`
	SidebarOpenSet bool `toml:"sidebar_open_set,omitempty"`

	SidebarStacked    bool `toml:"sidebar_stacked,omitempty"`
	SidebarStackedSet bool `toml:"sidebar_stacked_set,omitempty"`

	QueryLineVisible    bool `toml:"query_line_visible,omitempty"`
	QueryLineVisibleSet bool `toml:"query_line_visible_set,omitempty"`

	LeftRailOpen    bool `toml:"left_rail_open,omitempty"`
	LeftRailOpenSet bool `toml:"left_rail_open_set,omitempty"`

	StartTab string `toml:"start_tab,omitempty"`

	DefaultSort string `toml:"default_sort,omitempty"`

	// ChordSortModifiedDesc is the first-press direction for the q-s
	// chord (sort by Last Modified). "" / "desc" = newest-first (the
	// default, since "sort by Last Modified" almost always means "what
	// changed recently"); "asc" = oldest-first. Separate from DefaultSort
	// because the intuitive default differs: ascending for a generic
	// column, descending for a recency sort.
	ChordSortModifiedDesc string `toml:"chord_sort_modified_desc,omitempty"`

	GlobalSearchMode string `toml:"global_search_mode,omitempty"`

	SOQLSeed string `toml:"soql_seed,omitempty"`

	AutoLayout    bool `toml:"auto_layout,omitempty"`
	AutoLayoutSet bool `toml:"auto_layout_set,omitempty"`

	AutoLayoutMinWidth int `toml:"auto_layout_min_width,omitempty"`
}

// Built-in startup defaults (mirrors model.go newModel).
const (
	StartupStartTabFallback         = "home"
	StartupDefaultSortFallback      = "asc"
	StartupGlobalSearchModeFallback = "metadata"
	StartupSOQLSeedFallback         = "SELECT Id, Name FROM Account LIMIT 20"

	// StartupAutoLayoutMinWidthFallback is the terminal width below
	// which auto-layout stacks the sidebar at startup. 175 cols keeps
	// the sidebar on the right only when there's genuinely room for a
	// wide multi-column list AND the sidebar side-by-side — below that
	// the columns feel cramped, so the sidebar drops below main to give
	// the list the full width. (Deliberately aggressive: a right
	// sidebar is only worth it on a genuinely wide terminal.)
	StartupAutoLayoutMinWidthFallback = 175
)

func boolOr(val, set, def bool) bool {
	if set {
		return val
	}
	return def
}

// UpdateChecksDisabledByEnv reports the process-level escape hatch. It only
// disables automatic checks; an explicit Settings → Updates → Check now or
// `sf-deck update check` remains an intentional network action.
func UpdateChecksDisabledByEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SF_DECK_NO_UPDATE_CHECK"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// AutomaticUpdateChecks reports whether the TUI should perform its cached
// daily stable-release lookup. Default true.
func (s *Settings) AutomaticUpdateChecks() bool {
	if UpdateChecksDisabledByEnv() {
		return false
	}
	if s == nil {
		return true
	}
	return boolOr(s.UI.Updates.Automatic, s.UI.Updates.AutomaticSet, true)
}

// SetAutomaticUpdateChecks persists the user's preference. Caller Saves.
func (s *Settings) SetAutomaticUpdateChecks(enabled bool) {
	if s == nil {
		return
	}
	s.UI.Updates.Automatic = enabled
	s.UI.Updates.AutomaticSet = true
}

// LimitsConfigSection holds the default row counts for server fetches.
// Each 0 falls back to the matching package default; negatives clamp
// to 1.
type LimitsConfigSection struct {
	RecentRecords   int `toml:"recent_records,omitempty"`    // /records "recent" tab fetch
	Notifications   int `toml:"notifications,omitempty"`     // bell stream
	DeployHistory   int `toml:"deploy_history,omitempty"`    // recent deploys list
	AsyncJobHistory int `toml:"async_job_history,omitempty"` // home activity widgets (jobs)
	RecentLogins    int `toml:"recent_logins,omitempty"`     // /users · Recent logins list
	ReferencePicker int `toml:"reference_picker,omitempty"`  // record-edit reference-field SOSL picker
	GlobalSearch    int `toml:"global_search,omitempty"`     // SOSL result cap (SF max 50)
}

// Built-in row-limit defaults (mirror the sf package call sites).
const (
	LimitRecentRecordsFallback   = 50
	LimitNotificationsFallback   = 50
	LimitDeployHistoryFallback   = 50
	LimitAsyncJobHistoryFallback = 10
	LimitRecentLoginsFallback    = 50
	LimitReferencePickerFallback = 20
	LimitGlobalSearchFallback    = 50
)

// clampLimit applies the zero-fallback / negative-clamp rule.
func clampLimit(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	if v < 0 {
		return 1
	}
	return v
}

// LayoutConfigSection holds pane + modal sizing knobs. 0 falls back to
// the package default; negatives clamp to the documented minimum.
type LayoutConfigSection struct {
	ObjectPinnedSubtabs int `toml:"object_pinned_subtabs,omitempty"` // object-drill pinned subtab count
	AutocompleteRows    int `toml:"autocomplete_rows,omitempty"`     // SOQL autocomplete popup height
	ColumnResizeStep    int `toml:"column_resize_step,omitempty"`    // [ / ] column resize increment
	DownloadsModalRows  int `toml:"downloads_modal_rows,omitempty"`  // downloads overlay visible rows
	CommandPaletteRows  int `toml:"command_palette_rows,omitempty"`  // command-palette visible rows
	GlobalSearchRows    int `toml:"global_search_rows,omitempty"`    // global-search result rows
}

// Built-in layout defaults (mirror the UI const sites).
const (
	LayoutObjectPinnedSubtabsFallback = 6
	LayoutAutocompleteRowsFallback    = 8
	LayoutColumnResizeStepFallback    = 4
	LayoutDownloadsModalRowsFallback  = 16
	LayoutCommandPaletteRowsFallback  = 18
	LayoutGlobalSearchRowsFallback    = 40
)

func layoutVal(v, fallback, min int) int {
	if v == 0 {
		return fallback
	}
	if v < min {
		return min
	}
	return v
}

// APIConfigSection tunes the Salesforce API client. Durations are in
// the unit named by each field; 0 falls back to the package default.
// The UI pushes these into the sf package via sf.ApplyConfig at startup
// (sf can't import settings).
type APIConfigSection struct {
	HTTPTimeoutSec     int    `toml:"http_timeout_sec,omitempty"`     // REST client timeout
	CLITimeoutSec      int    `toml:"cli_timeout_sec,omitempty"`      // `sf` CLI invocation timeout
	RetrieveTimeoutSec int    `toml:"retrieve_timeout_sec,omitempty"` // deploy/retrieve shell-out cap
	DeployTimeoutSec   int    `toml:"deploy_timeout_sec,omitempty"`   // deploy poll deadline
	DeployPollMs       int    `toml:"deploy_poll_ms,omitempty"`       // deploy poll interval
	DeployWatchSec     int    `toml:"deploy_watch_sec,omitempty"`     // /deploys live-watch refresh interval
	BulkPollMs         int    `toml:"bulk_poll_ms,omitempty"`         // bulk-job poll interval (steady state)
	APIVersion         string `toml:"api_version,omitempty"`          // forced API version (e.g. "65.0"); empty = org default
}

// Built-in API defaults (mirror the sf package).
const (
	APIHTTPTimeoutSecFallback     = 30
	APICLITimeoutSecFallback      = 30
	APIRetrieveTimeoutSecFallback = 1200 // 20 min — covers most validates with Apex test runs
	APIDeployTimeoutSecFallback   = 60
	// Deploy status polls every 5s once past the 500ms fast-start —
	// quick deploys still land on the fast-start polls; anything
	// slower is minutes-scale on Salesforce's side, where the old 1s
	// steady poll bought nothing but API calls.
	APIDeployPollMsFallback   = 5000
	APIDeployWatchSecFallback = 5 // /deploys live watch — one tooling SOQL per tick
	APIBulkPollMsFallback     = 5000
)

func (s *Settings) APIHTTPTimeoutSec() int {
	if s == nil {
		return APIHTTPTimeoutSecFallback
	}
	return layoutVal(s.UI.API.HTTPTimeoutSec, APIHTTPTimeoutSecFallback, 1)
}
func (s *Settings) APICLITimeoutSec() int {
	if s == nil {
		return APICLITimeoutSecFallback
	}
	return layoutVal(s.UI.API.CLITimeoutSec, APICLITimeoutSecFallback, 1)
}
func (s *Settings) APIRetrieveTimeoutSec() int {
	if s == nil {
		return APIRetrieveTimeoutSecFallback
	}
	return layoutVal(s.UI.API.RetrieveTimeoutSec, APIRetrieveTimeoutSecFallback, 1)
}
func (s *Settings) APIDeployTimeoutSec() int {
	if s == nil {
		return APIDeployTimeoutSecFallback
	}
	return layoutVal(s.UI.API.DeployTimeoutSec, APIDeployTimeoutSecFallback, 1)
}
func (s *Settings) APIDeployPollMs() int {
	if s == nil {
		return APIDeployPollMsFallback
	}
	return layoutVal(s.UI.API.DeployPollMs, APIDeployPollMsFallback, 50)
}

// APIDeployWatchSec is the /deploys live-watch refresh interval — how
// often the deploys list re-fetches (one tooling SOQL per tick) while
// any deploy in the cached window is still in flight. Floor of 2s so a
// typo can't turn the watch into a query hammer.
func (s *Settings) APIDeployWatchSec() int {
	if s == nil {
		return APIDeployWatchSecFallback
	}
	return layoutVal(s.UI.API.DeployWatchSec, APIDeployWatchSecFallback, 2)
}

func (s *Settings) APIBulkPollMs() int {
	if s == nil {
		return APIBulkPollMsFallback
	}
	return layoutVal(s.UI.API.BulkPollMs, APIBulkPollMsFallback, 250)
}

// APIVersionOverride returns the user-forced API version, or "" when
// unset (caller uses the org-reported version).
func (s *Settings) APIVersionOverride() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.UI.API.APIVersion)
}

func (s *Settings) SetAPIHTTPTimeoutSec(n int) {
	if s != nil {
		s.UI.API.HTTPTimeoutSec = n
	}
}
func (s *Settings) SetAPICLITimeoutSec(n int) {
	if s != nil {
		s.UI.API.CLITimeoutSec = n
	}
}
func (s *Settings) SetAPIRetrieveTimeoutSec(n int) {
	if s != nil {
		s.UI.API.RetrieveTimeoutSec = n
	}
}
func (s *Settings) SetAPIDeployTimeoutSec(n int) {
	if s != nil {
		s.UI.API.DeployTimeoutSec = n
	}
}
func (s *Settings) SetAPIDeployPollMs(n int) {
	if s != nil {
		s.UI.API.DeployPollMs = n
	}
}
func (s *Settings) SetAPIDeployWatchSec(n int) {
	if s != nil {
		s.UI.API.DeployWatchSec = n
	}
}
func (s *Settings) SetAPIBulkPollMs(n int) {
	if s != nil {
		s.UI.API.BulkPollMs = n
	}
}
func (s *Settings) SetAPIVersionOverride(v string) {
	if s != nil {
		s.UI.API.APIVersion = strings.TrimSpace(v)
	}
}

// SearchDebounceMsFallback is the debounce window applied when the
// last filter exceeded the fast-filter threshold. 100ms is short
// enough that human typists hit the gap on every char (each char
// fires immediately), but long enough that fast burst typing
// (programmer one-finger drumming) collapses into one filter.
const SearchDebounceMsFallback = 100

// SearchFastFilterThresholdMsFallback is the wall-time under which
// a filter is "instant" — used to decide whether to debounce the
// next update. 50ms is roughly the perceptual threshold for
// "responsive vs sluggish" in terminal UIs.
const SearchFastFilterThresholdMsFallback = 50

// SearchDebounceMs resolves the effective debounce window.
func (s *Settings) SearchDebounceMs() int {
	if s == nil || s.UI.Search.DebounceMs <= 0 {
		return SearchDebounceMsFallback
	}
	return s.UI.Search.DebounceMs
}

// SearchFastFilterThresholdMs resolves the effective fast-filter
// cutoff.
func (s *Settings) SearchFastFilterThresholdMs() int {
	if s == nil || s.UI.Search.FastFilterThresholdMs <= 0 {
		return SearchFastFilterThresholdMsFallback
	}
	return s.UI.Search.FastFilterThresholdMs
}

// SetSearchDebounceMs writes the debounce override. Pass 0 to
// clear (back to fallback).
func (s *Settings) SetSearchDebounceMs(n int) {
	if s == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	s.UI.Search.DebounceMs = n
}

// SetSearchFastFilterThresholdMs writes the fast-filter cutoff
// override. Pass 0 to clear.
func (s *Settings) SetSearchFastFilterThresholdMs(n int) {
	if s == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	s.UI.Search.FastFilterThresholdMs = n
}

// DefaultChipLimitFallback is the row cap used when neither the chip
// nor the settings file specifies one. Set to 2000 because that's
// what one Salesforce REST page returns — so the default fetch
// completes in a single API call. Users who want more raise the
// override via settings (ctrl+,) or per-chip via the chip wizard's
// $limit field; full-dataset export goes through ctrl+x (Bulk API).
const DefaultChipLimitFallback = 2000

// TreeChipConfig persists treechip state for one (org, domain) pair.
// Pins is an unordered list of node IDs the user has pinned to their
// strip; LastPath is the breadcrumb path they were on when they last
// left the tab, used to restore position on re-entry.
type TreeChipConfig struct {
	Pins     []string `toml:"pins,omitempty"`
	LastPath []string `toml:"last_path,omitempty"`
}

// RecentConfig is the persisted form of one recent-visit entry.
// Mirrors ui.RecentEntry but lives here to keep settings dependency-
// free (the ui package imports settings, not the other way round).
type RecentConfig struct {
	Kind      string    `toml:"kind"`
	ID        string    `toml:"id"`
	Name      string    `toml:"name,omitempty"`
	Type      string    `toml:"type,omitempty"`
	VisitedAt time.Time `toml:"visited_at"`
}

// ChipConfig is the persistence form of a unified chip. The Where /
// OrderBy / Limit / Columns mirror query.QueryYAML — kept as `any`
// here because internal/settings has no dependency on internal/query
// (avoids a circular import; the UI layer does the conversion).
type ChipConfig struct {
	ID     string `toml:"id"`
	Label  string `toml:"label"`
	Scope  string `toml:"scope,omitempty"`  // "*" / sObject API name
	Domain string `toml:"domain,omitempty"` // "records" | "objects" | "flows"
	Origin string `toml:"origin,omitempty"` // "user" (default) | "imported"

	// OrgUser is the LEGACY single-org binding (the user's canonical
	// username, NOT alias). Superseded by Share, which expresses richer
	// scoping (single org / explicit list / org-group / global). For
	// back-compat the loader normalises any leftover OrgUser into a
	// `Share{Kind: ChipShareOrg, Orgs: [OrgUser]}` on read and never
	// writes this field again. New code reads Share via the
	// EffectiveShare() helper which handles both shapes.
	// SUNSET: once released settings have all round-tripped through a
	// version that writes Share (any post-2026-06 build), this field
	// and the fallback in EffectiveShare can be deleted.
	OrgUser string `toml:"org_user,omitempty"`

	// Share controls which orgs this chip appears for. Replaces OrgUser.
	// See ChipShare for the four kinds (single org / list / group / global).
	// Empty (zero value) means "fall back to OrgUser" — i.e. a chip
	// written before this field existed. New writes always populate Share.
	Share ChipShare `toml:"share,omitempty"`

	Query ChipQueryYAML `toml:"query"`

	SourceID   string `toml:"source_id,omitempty"`
	SourceName string `toml:"source_name,omitempty"`
	ImportedAt string `toml:"imported_at,omitempty"`

	// Favourite controls whether the chip appears on the strip
	// directly (true) or only in the "+ N more" overflow modal
	// (false). Default-false on disk; the runtime adds it to chips
	// the user marks via the chip manager. Built-in defaults are
	// in code, not TOML.
	Favourite bool `toml:"favourite,omitempty"`
}

// ChipShareKind is the discriminator for ChipShare.
//
// Stored as a string on disk so adding new kinds later (e.g. an
// "everywhere except prod" inverse list) doesn't break old files —
// unknown kinds load as zero and fall through to the OrgUser back-compat
// path, which is the safe degraded behaviour.
type ChipShareKind string

const (
	// ChipShareOrg: the chip belongs to exactly one org. Orgs holds a
	// single canonical username. This is what every new user chip starts
	// as and what legacy OrgUser-only chips become on first read.
	ChipShareOrg ChipShareKind = "org"

	// ChipShareOrgs: the chip is shared with an explicit set of orgs.
	// Orgs holds N canonical usernames. The user picked them in the
	// scope-chooser UI.
	ChipShareOrgs ChipShareKind = "orgs"

	// ChipShareGroup: the chip is shared with every org in a named
	// OrgGroup. Group holds the group id; membership is resolved at
	// read time so renaming/adding members propagates without rewriting
	// chips. If the group is deleted the chip falls through to "no
	// orgs match" — safer than silently going global.
	ChipShareGroup ChipShareKind = "group"

	// ChipShareGlobal: the chip appears for every org. Reserved for
	// power-user opt-in — the per-org default exists for a reason
	// (sObject API names collide across orgs but the data behind them
	// differs).
	ChipShareGlobal ChipShareKind = "global"
)

// ChipShare describes which orgs a chip is visible in. Zero value is the
// "legacy" sentinel meaning "fall back to ChipConfig.OrgUser." A populated
// Share takes precedence over OrgUser.
type ChipShare struct {
	Kind  ChipShareKind `toml:"kind,omitempty"`
	Orgs  []string      `toml:"orgs,omitempty"`  // canonical usernames, used by org / orgs
	Group string        `toml:"group,omitempty"` // OrgGroup id, used by group
}

// IsZero reports whether the share is the unset sentinel (callers should
// fall back to OrgUser back-compat in that case).
func (s ChipShare) IsZero() bool {
	return s.Kind == "" && len(s.Orgs) == 0 && s.Group == ""
}

// IsShared reports whether this chip is visible to more than just a
// single org — i.e. it carries cross-org visibility the user should be
// aware of when editing or activating it. Single-org and zero (legacy
// OrgUser-only) chips return false because they're functionally
// equivalent to "private to this org." Drives the cross-org marking on
// chip-strip and chip-manager rows.
func (s ChipShare) IsShared() bool {
	switch s.Kind {
	case ChipShareGlobal, ChipShareGroup:
		return true
	case ChipShareOrgs:
		return len(s.Orgs) > 1
	}
	return false
}

// Allows reports whether a chip with this share should be visible for the
// given org. groupMembers tells the share how to resolve "in the group" —
// it returns true when the username is a member of the named group. (We
// inject the lookup rather than reach back into settings to keep ChipShare
// pure and easy to test.)
func (s ChipShare) Allows(orgUser string, groupMembers func(groupID, username string) bool) bool {
	switch s.Kind {
	case ChipShareGlobal:
		return true
	case ChipShareOrg, ChipShareOrgs:
		if orgUser == "" {
			return false
		}
		for _, u := range s.Orgs {
			if u == orgUser {
				return true
			}
		}
		return false
	case ChipShareGroup:
		if s.Group == "" || orgUser == "" || groupMembers == nil {
			return false
		}
		return groupMembers(s.Group, orgUser)
	default:
		return false
	}
}

// EffectiveShare returns the chip's share, normalising the legacy
// OrgUser-only shape to ChipShareOrg so callers never need to branch.
// An empty OrgUser AND empty Share yields a global share (built-in /
// pre-OrgUser legacy chips).
func (c ChipConfig) EffectiveShare() ChipShare {
	if !c.Share.IsZero() {
		return c.Share
	}
	if c.OrgUser != "" {
		return ChipShare{Kind: ChipShareOrg, Orgs: []string{c.OrgUser}}
	}
	return ChipShare{Kind: ChipShareGlobal}
}

// NormaliseShare migrates a legacy OrgUser stamp into the new Share field
// in place. Used by callers that intend to write — keeps disk shape
// uniform so newly-saved chips never carry both fields.
func (c *ChipConfig) NormaliseShare() {
	if c.Share.IsZero() && c.OrgUser != "" {
		c.Share = ChipShare{Kind: ChipShareOrg, Orgs: []string{c.OrgUser}}
	}
	if !c.Share.IsZero() {
		c.OrgUser = "" // single source of truth going forward
	}
}

// ChipQueryYAML mirrors query.QueryYAML on the persistence side. We
// keep a copy here rather than importing the query package because
// settings sits underneath every other layer and circular imports are
// a pain to unwind later.
type ChipQueryYAML struct {
	Where   *ChipNodeYAML     `toml:"where,omitempty"`
	OrderBy []ChipOrderByYAML `toml:"order_by,omitempty"`
	Limit   int               `toml:"limit,omitempty"`
	Columns []string          `toml:"columns,omitempty"`
}

// ChipNodeYAML mirrors query.NodeYAML.
type ChipNodeYAML struct {
	Kind     string         `toml:"kind"`
	Field    string         `toml:"field,omitempty"`
	Op       string         `toml:"op,omitempty"`
	Value    any            `toml:"value,omitempty"`
	Children []ChipNodeYAML `toml:"children,omitempty"`
	Child    *ChipNodeYAML  `toml:"child,omitempty"`
}

// ChipOrderByYAML mirrors query.OrderByYAML.
type ChipOrderByYAML struct {
	Field     string `toml:"field"`
	Direction string `toml:"direction,omitempty"`
	NullsLast bool   `toml:"nulls_last,omitempty"`
}

// FilterConfig is one user-defined client-side filter (for /objects,
// /flows, etc.). Persistable form of internal/ui/filter.Filter; the
// Spec section maps to the same-named struct on the runtime side.
type FilterConfig struct {
	ID     string         `toml:"id"`
	Label  string         `toml:"label"`
	Scope  string         `toml:"scope,omitempty"`
	Origin string         `toml:"origin,omitempty"` // "user" (default) | "imported"
	Spec   FilterSpecYAML `toml:"spec"`
}

// FilterSpecYAML mirrors filter.Spec on the persistence side. Same
// fields, same TOML keys; we keep it here rather than importing the
// runtime package so settings has no UI dependency.
type FilterSpecYAML struct {
	NameContains           string `toml:"name_contains,omitempty"`
	LabelContains          string `toml:"label_contains,omitempty"`
	DescriptionContains    string `toml:"description_contains,omitempty"`
	Suffix                 string `toml:"suffix,omitempty"`
	Prefix                 string `toml:"prefix,omitempty"`
	NamespaceEquals        string `toml:"namespace_equals,omitempty"`
	StatusEquals           string `toml:"status_equals,omitempty"`
	CategoryEquals         string `toml:"category_equals,omitempty"`
	DeploymentStatusEquals string `toml:"deployment_status_equals,omitempty"`
	KeyPrefixEquals        string `toml:"key_prefix_equals,omitempty"`
	APIVersionGTE          int    `toml:"api_version_gte,omitempty"`
	APIVersionLTE          int    `toml:"api_version_lte,omitempty"`
	ModifiedAfter          string `toml:"modified_after,omitempty"`
	ModifiedBefore         string `toml:"modified_before,omitempty"`
	ModifiedBy             string `toml:"modified_by,omitempty"`
	IsCustom               *bool  `toml:"is_custom,omitempty"`
	IsApexTriggerable      *bool  `toml:"is_apex_triggerable,omitempty"`
	IsWorkflowEnabled      *bool  `toml:"is_workflow_enabled,omitempty"`
	HasActiveVersion       *bool  `toml:"has_active_version,omitempty"`
}

// LensConfig is one user-defined record-list lens. The runtime
// rehydrates these into an in-memory lens.Lens after Load(). Built-in
// lenses ship in code, not here.
//
// Example:
//
//	[[ui.lenses]]
//	id        = "open-cases"
//	label     = "Open cases"
//	scope     = "Case"
//	soql_where = "IsClosed = false"
//	order_by  = "LastModifiedDate DESC"
//	limit     = 100
//	columns   = ["Id", "CaseNumber", "Subject", "Priority", "Status"]
type LensConfig struct {
	ID        string   `toml:"id"`
	Label     string   `toml:"label"`
	Scope     string   `toml:"scope"` // sObject API name, or "*" for universal
	SOQLWhere string   `toml:"soql_where"`
	OrderBy   string   `toml:"order_by"`
	Limit     int      `toml:"limit"`
	Columns   []string `toml:"columns"`

	// Origin is "user" or "imported". Built-ins ship in code and
	// never round-trip through TOML. Empty / unknown values default
	// to "user" on load — settings files predating this field stay
	// valid.
	Origin string `toml:"origin,omitempty"`

	SourceID   string `toml:"source_id,omitempty"`
	SourceName string `toml:"source_name,omitempty"`
	ImportedAt string `toml:"imported_at,omitempty"`
}

// ExtensionsConfig captures user-specific browser-extension URLs that
// sf-deck knows how to link into. These are local to the user's
// machine (extension GUIDs differ per browser + per install) so they
// only make sense as user config, never as shared defaults.
//
// Example TOML:
//
//	[ui.extensions]
//	# Salesforce Inspector Reloaded — grab from the extension's
//	# options page or any page served by the extension.
//	inspector = "moz-extension://31ebc04c-15c6-4497-aa47-e6e7261eaeca/inspect.html"
type ExtensionsConfig struct {
	Inspector string `toml:"inspector"` // base URL of Inspector's inspect.html
	// Browser is the macOS application name used to launch extension
	// URLs. Must be whichever browser hosts the extension ("Firefox",
	// "Google Chrome", "Arc", etc.) since moz-extension:// and
	// chrome-extension:// aren't global URL schemes — macOS can't route
	// them without an -a hint. Empty => fall back to bare `open <url>`,
	// which works for https:// links but fails for extension URLs.
	Browser string `toml:"browser"`
	// OpenAuth controls how `o` opens Salesforce URLs:
	//   "direct" (default) — navigate straight to the URL, reusing
	//     whatever browser session exists. Avoids per-open session
	//     churn / identity-verification prompts (e.g. passkeys) on
	//     strictly-configured orgs. If no live session exists the open
	//     lands on the login page.
	//   "frontdoor" — exchange the sfdx token for a one-time login URL
	//     (singleaccess), so the browser lands authenticated even with
	//     no existing session. What modern `sf org open` does; the cost
	//     is a fresh session each open, which strict orgs may re-verify.
	OpenAuth string `toml:"open_auth,omitempty"`
	// FlowOpenVersion controls which flow version `o` opens from the
	// flows list:
	//   "latest" (default) — the most recent version regardless of
	//     status, matching Setup's own flow list: when a draft is newer
	//     than the active version, opening the flow means editing that
	//     draft.
	//   "active" — the currently active version; the newer draft stays
	//     available as a secondary target in the ctrl+o menu. Falls back
	//     to the latest version for flows that were never activated.
	FlowOpenVersion string `toml:"flow_open_version,omitempty"`
}

// DebugConfig holds developer/testing toggles that aren't meant for
// everyday use. Kept in its own [ui.debug] section so it's obvious these
// are debug knobs, not features.
type DebugConfig struct {
	ForceWelcome bool `toml:"force_welcome,omitempty"`
}

// CacheConfig controls Resource[T] freshness across the UI. TTLs are
// parsed with time.ParseDuration ("15m", "1h", "30s"). Empty values
// fall through to hardcoded sensible defaults in CacheTTL below.
//
// Keys map to the Key prefix Resource instances use so new resources
// slot in without code changes.
type CacheConfig struct {
	DefaultTTL string            `toml:"default_ttl"`
	TTL        map[string]string `toml:"ttl"` // per-resource overrides, e.g. "describes" = "4h"
}

// Defaults holds the per-org-kind baseline applied when an org has no
// explicit override. Field tags control TOML shape. Zero-value
// interpretation: an empty string means "fall back to the hardcoded
// safe default" (see resolveDefault below).
type Defaults struct {
	Production string `toml:"production"`
	Sandbox    string `toml:"sandbox"`
	Scratch    string `toml:"scratch"`
	DevHub     string `toml:"devhub"`
}

// OrgConfig is one [orgs."<username-or-alias>"] block. We key on
// username when we have one (stable); alias is a convenience for users
// editing the file by hand and is resolved by the caller.
//
// Default is the sf-deck-level "open this org first at startup" pin.
// Independent of sf CLI's default (which reflects whichever org was
// most-recently shelled to). Only one entry should have Default=true;
// PinDefault below enforces this on writes.
type OrgConfig struct {
	Safety  string `toml:"safety,omitempty"`
	Default bool   `toml:"default,omitempty"`
}

// Path returns the absolute path to settings.toml in the user's
// sf-deck dir (~/.sf-deck/settings.toml by default).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sf-deck", "settings.toml"), nil
}

// Ephemeral makes the whole package in-memory only: Load returns
// fresh defaults without reading disk and Save succeeds without
// writing. Set by `sf-deck --demo` before anything loads — a demo
// session must neither see the user's real settings.toml nor ever
// write to it, and every Load/Save call site stays unchanged.
var Ephemeral bool

// Load reads settings.toml, returning an empty Settings (with safe
// hardcoded defaults) if the file doesn't exist. Unparseable files
// are treated as empty — we never prevent the app from booting over a
// settings problem.
func Load() (*Settings, error) {
	s := &Settings{Orgs: map[string]OrgConfig{}}
	if Ephemeral {
		return s, nil
	}
	p, err := Path()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		s.loadedDigest = ""
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := toml.Unmarshal(b, s); err != nil {
		return s, err
	}
	s.loadedDigest = digestBytes(b)
	if s.Orgs == nil {
		s.Orgs = map[string]OrgConfig{}
	}
	return s, nil
}

// ErrConcurrentModification means settings.toml changed on disk since
// this Settings value was loaded or last successfully saved. Returning
// this is preferable to resurrecting stale in-memory state from another
// sf-deck process.
type ErrConcurrentModification struct {
	Path string
}

func (e ErrConcurrentModification) Error() string {
	return fmt.Sprintf("%s changed on disk; reload settings before saving", e.Path)
}

// Save writes settings.toml atomically (tmp → rename). Creates the
// parent directory if needed. It also protects against stale
// multi-process overwrites: a process may only save if the on-disk
// file still matches the version it loaded, or the version it wrote on
// its previous successful Save.
func (s *Settings) Save() error {
	if Ephemeral {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	unlock, err := lockSettingsFile(p)
	if err != nil {
		return err
	}
	defer unlock()

	currentDigest, err := digestFile(p)
	if err != nil {
		return err
	}
	if currentDigest != s.loadedDigest {
		return ErrConcurrentModification{Path: p}
	}

	var buf bytes.Buffer
	// Encode a snapshot with cloned maps, NOT the live struct: toml
	// encoding iterates s.Orgs / s.UI.* maps, and a mutator writing one
	// of those maps concurrently would trigger Go's fatal "concurrent map
	// iteration and map write" (unrecoverable). The snapshot is taken
	// under s.mu (held for the whole Save), and the map-mutators take s.mu
	// too, so the clone can't race a write.
	if err := toml.NewEncoder(&buf).Encode(s.snapshotLocked()); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	s.loadedDigest = digestBytes(buf.Bytes())
	return nil
}

// snapshotLocked returns a copy of s safe to hand to the TOML encoder
// without touching any of s's live maps OR slice backing arrays. Caller
// must hold s.mu. Cloning is required because the encoder iterates maps
// and reads slice lengths/elements while the mutators (which also hold
// s.mu) write them — an unguarded encode of the live struct hit Go's
// fatal "concurrent map iteration and map write" and racy slice reads.
func (s *Settings) snapshotLocked() *Settings {
	cp := &Settings{
		Defaults: s.Defaults,
		Orgs:     maps.Clone(s.Orgs),
		UI:       s.UI, // value copy; clone its maps + slices below
	}
	cp.UI.ChipBuiltinFavOverrides = maps.Clone(s.UI.ChipBuiltinFavOverrides)
	cp.UI.ReportExportPostProcessors = maps.Clone(s.UI.ReportExportPostProcessors)
	cp.UI.RecentByOrg = maps.Clone(s.UI.RecentByOrg)
	cp.UI.LoadedDevProjectByOrg = maps.Clone(s.UI.LoadedDevProjectByOrg)
	cp.UI.LoadedOrgProjectByOrgLegacy = maps.Clone(s.UI.LoadedOrgProjectByOrgLegacy)
	cp.UI.Cache.TTL = maps.Clone(s.UI.Cache.TTL)
	if s.UI.TreeChipByOrg != nil { // map of maps — clone outer + each inner
		outer := make(map[string]map[string]TreeChipConfig, len(s.UI.TreeChipByOrg))
		for k, inner := range s.UI.TreeChipByOrg {
			outer[k] = maps.Clone(inner)
		}
		cp.UI.TreeChipByOrg = outer
	}
	cp.UI.ThemeFavourites = slices.Clone(s.UI.ThemeFavourites)
	cp.UI.Chips = slices.Clone(s.UI.Chips)
	cp.UI.Lenses = slices.Clone(s.UI.Lenses)
	cp.UI.ObjectFilters = slices.Clone(s.UI.ObjectFilters)
	cp.UI.FlowFilters = slices.Clone(s.UI.FlowFilters)
	cp.UI.ReportExportDefault = slices.Clone(s.UI.ReportExportDefault)
	cp.UI.OrgGroups.Groups = slices.Clone(s.UI.OrgGroups.Groups)
	return cp
}

func digestFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return digestBytes(b), nil
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func lockSettingsFile(settingsPath string) (func(), error) {
	lockPath := settingsPath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// InspectorURL returns the user's configured Salesforce Inspector
// base URL, or "" if unset. Callers pass "" through as "don't surface
// the Inspector target" rather than 404ing into a generic link.
func (s *Settings) InspectorURL() string {
	if s == nil {
		return ""
	}
	return s.UI.Extensions.Inspector
}

// SetInspectorURL persists an Inspector base URL. Caller owns Save().
func (s *Settings) SetInspectorURL(u string) {
	s.UI.Extensions.Inspector = u
}

// Browser returns the macOS application name used to open extension
// URLs (Firefox, Google Chrome, Arc, …). Empty when unset.
func (s *Settings) Browser() string {
	if s == nil {
		return ""
	}
	return s.UI.Extensions.Browser
}

// OpenAuth returns the o-key auth mode: "frontdoor" or "direct".
//
// Default is "direct": navigate straight to instanceURL+path and reuse
// the browser's existing session cookie. frontdoor (oauth2/singleaccess)
// mints a FRESH session on every open, which strictly-configured orgs
// answer with an identity-verification prompt (e.g. a passkey) for
// sensitive entities like User or payment objects — so a plain "open
// this record" re-verified identity every time. direct avoids that by
// reusing the session you already have; its only downside is that if no
// live browser session exists for the org, the open lands on the login
// page instead of auto-authenticating. Users who routinely open cold
// orgs can switch back to frontdoor in settings.
func (s *Settings) OpenAuth() string {
	if s == nil || s.UI.Extensions.OpenAuth == "" {
		return "direct"
	}
	return s.UI.Extensions.OpenAuth
}

// SetOpenAuth persists the o-key auth mode. Caller owns Save().
func (s *Settings) SetOpenAuth(mode string) {
	if s == nil {
		return
	}
	s.UI.Extensions.OpenAuth = mode
}

// FlowOpenVersion returns which flow version the flows-list `o` opens:
// "latest" (default — the most recent version regardless of status) or
// "active" (the running version, with the newer draft as a secondary
// target). Unset / unknown values fall back to "latest".
func (s *Settings) FlowOpenVersion() string {
	if s == nil || s.UI.Extensions.FlowOpenVersion != "active" {
		return "latest"
	}
	return "active"
}

// SetFlowOpenVersion persists the flow-open preference. Caller owns
// Save().
func (s *Settings) SetFlowOpenVersion(mode string) {
	if s == nil {
		return
	}
	s.UI.Extensions.FlowOpenVersion = mode
}

// SetBrowser persists the browser name. Caller owns Save().
func (s *Settings) SetBrowser(name string) {
	s.UI.Extensions.Browser = name
}

func (s *Settings) FlowFilters() []FilterConfig {
	if s == nil {
		return nil
	}
	return s.UI.FlowFilters
}
func (s *Settings) SetFlowFilters(fs []FilterConfig) { s.UI.FlowFilters = fs }
func (s *Settings) UpsertFlowFilter(f FilterConfig) {
	for i, x := range s.UI.FlowFilters {
		if x.ID == f.ID {
			s.UI.FlowFilters[i] = f
			return
		}
	}
	s.UI.FlowFilters = append(s.UI.FlowFilters, f)
}
func (s *Settings) DeleteFlowFilter(id string) {
	out := s.UI.FlowFilters[:0]
	for _, x := range s.UI.FlowFilters {
		if x.ID != id {
			out = append(out, x)
		}
	}
	s.UI.FlowFilters = out
}

// ReportExportTransforms returns the post-processor list for the given
// report id, falling back to the user's default. Empty slice means
// "vanilla xlsx, no post-processing".
func (s *Settings) ReportExportTransforms(reportID string) []string {
	if s == nil {
		return nil
	}
	if ts, ok := s.UI.ReportExportPostProcessors[reportID]; ok {
		out := make([]string, len(ts))
		copy(out, ts)
		return out
	}
	out := make([]string, len(s.UI.ReportExportDefault))
	copy(out, s.UI.ReportExportDefault)
	return out
}

// SetReportExportTransforms persists the post-processor list for one
// report. nil clears the override (the default list applies again).
func (s *Settings) SetReportExportTransforms(reportID string, transforms []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UI.ReportExportPostProcessors == nil {
		s.UI.ReportExportPostProcessors = map[string][]string{}
	}
	if transforms == nil {
		delete(s.UI.ReportExportPostProcessors, reportID)
		return
	}
	cp := make([]string, len(transforms))
	copy(cp, transforms)
	s.UI.ReportExportPostProcessors[reportID] = cp
}

// RecentForOrg returns the persisted recent-visit list for an org.
// Empty slice when nothing's been recorded yet. Returns a fresh copy
// so callers can mutate freely.
func (s *Settings) RecentForOrg(orgUser string) []RecentConfig {
	if s == nil || len(s.UI.RecentByOrg) == 0 {
		return nil
	}
	src, ok := s.UI.RecentByOrg[orgUser]
	if !ok {
		return nil
	}
	out := make([]RecentConfig, len(src))
	copy(out, src)
	return out
}

// SetRecentForOrg replaces the recent-visit list for one org. Empty
// slice clears the entry so settings.toml stays tidy when a user
// resets their history.
func (s *Settings) SetRecentForOrg(orgUser string, list []RecentConfig) {
	if s == nil || orgUser == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UI.RecentByOrg == nil {
		s.UI.RecentByOrg = map[string][]RecentConfig{}
	}
	if len(list) == 0 {
		delete(s.UI.RecentByOrg, orgUser)
		return
	}
	cp := make([]RecentConfig, len(list))
	copy(cp, list)
	s.UI.RecentByOrg[orgUser] = cp
}

// LoadedDevProjectForOrg returns the persisted loaded-project id for
// the given org. Empty string when nothing's loaded.
//
// Reads the new map first; falls back to the legacy
// LoadedOrgProjectByOrgLegacy map for users upgrading from the
// pre-flatten schema. Note: legacy values were OrgProject ids, not
// DevProject ids — the migration in devproject.Open transforms the
// store contents but the settings ids still point at OrgProject rows
// that no longer exist. Resolving to "no project loaded" on the
// first launch is acceptable; the user re-loads explicitly.
func (s *Settings) LoadedDevProjectForOrg(orgUser string) string {
	if s == nil || orgUser == "" {
		return ""
	}
	if id, ok := s.UI.LoadedDevProjectByOrg[orgUser]; ok && id != "" {
		return id
	}
	// Don't fall through to the legacy map: those ids point at the
	// old OrgProject rows which no longer have a stable mapping to
	// any DevProject after migration. Returning "" is the honest
	// answer.
	return ""
}

// SetLoadedDevProjectForOrg records the loaded-project id for one
// org. Empty id clears the entry — same shape as the other per-org
// setters so the toml stays tidy when nothing is loaded.
func (s *Settings) SetLoadedDevProjectForOrg(orgUser, projectID string) {
	if s == nil || orgUser == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.UI.LoadedDevProjectByOrg == nil {
		s.UI.LoadedDevProjectByOrg = map[string]string{}
	}
	if projectID == "" {
		delete(s.UI.LoadedDevProjectByOrg, orgUser)
	} else {
		s.UI.LoadedDevProjectByOrg[orgUser] = projectID
	}
	// Also clear the legacy map on any save — the new key is
	// authoritative going forward.
	s.UI.LoadedOrgProjectByOrgLegacy = nil
}

// ReportExportDir returns the configured export directory. Default
// is the user's home directory (~) — chosen because Downloads on
// macOS triggers TCC permission prompts that break sfdx tools running
// from inside the export bundle. Home is universally readable by the
// user and works the same across macOS / Linux / Windows.
//
// Tilde expansion is the caller's responsibility (the UI layer
// has expandTilde).
func (s *Settings) ReportExportDir() string {
	if s != nil && s.UI.ReportExportDir != "" {
		return s.UI.ReportExportDir
	}
	return defaultExportDir()
}

// SetReportExportDir persists the export directory (no validation —
// tilde-expansion + creation happen at write time). Empty string
// clears the override.
func (s *Settings) SetReportExportDir(dir string) {
	if s == nil {
		return
	}
	s.UI.ReportExportDir = strings.TrimSpace(dir)
}

// TagColumnModeCompact / Expanded / Hidden are the canonical
// values for Settings.UI.TagColumnMode. Centralised here so the UI
// layer doesn't sprinkle string literals around.
const (
	TagColumnModeCompact  = "compact"
	TagColumnModeExpanded = "expanded"
	TagColumnModeHidden   = "hidden"
)

// TagColumnDisplayMode resolves the effective 3-state mode for the
// tag gutter, honouring the legacy TagColumnHidden bool when the new
// TagColumnMode hasn't been set. Always returns one of the canonical
// constants above.
func (s *Settings) TagColumnDisplayMode() string {
	if s == nil {
		return TagColumnModeCompact
	}
	switch s.UI.TagColumnMode {
	case TagColumnModeExpanded:
		return TagColumnModeExpanded
	case TagColumnModeHidden:
		return TagColumnModeHidden
	case TagColumnModeCompact:
		return TagColumnModeCompact
	}
	// Unset → consult the legacy bool. Pre-Ctrl+T-cycle users may
	// have toggled the column off via the old code path; honour it.
	if s.UI.TagColumnHidden {
		return TagColumnModeHidden
	}
	return TagColumnModeCompact
}

// SetTagColumnMode persists the 3-state mode. mode must be one of
// the TagColumnMode* constants; anything else is normalised to
// compact. Also clears the legacy TagColumnHidden bool so the new
// mode is the single source of truth going forward.
func (s *Settings) SetTagColumnMode(mode string) {
	if s == nil {
		return
	}
	switch mode {
	case TagColumnModeExpanded, TagColumnModeHidden:
		s.UI.TagColumnMode = mode
	default:
		s.UI.TagColumnMode = TagColumnModeCompact
	}
	s.UI.TagColumnHidden = false
}

// TagColumnVisible is a convenience for callers that only care
// "should I render anything at all?" Returns false in hidden mode,
// true in compact + expanded.
func (s *Settings) TagColumnVisible() bool {
	return s.TagColumnDisplayMode() != TagColumnModeHidden
}

// FlagColumnModeFull / Letter / Hidden are the canonical values
// for Settings.UI.FlagColumnMode. Mirrors TagColumn cycling:
// full = labels, letter = first-letter glyphs, hidden = column off.
const (
	FlagColumnModeFull   = "full"
	FlagColumnModeLetter = "letter"
	FlagColumnModeHidden = "hidden"
)

// FlagColumnDisplayMode resolves the effective 3-state mode for the
// FLAGS column. Empty falls back to "letter" so a fresh user sees
// the compact glyph form (full labels are an opt-in via Ctrl+G).
func (s *Settings) FlagColumnDisplayMode() string {
	if s == nil {
		return FlagColumnModeLetter
	}
	switch s.UI.FlagColumnMode {
	case FlagColumnModeFull:
		return FlagColumnModeFull
	case FlagColumnModeHidden:
		return FlagColumnModeHidden
	}
	return FlagColumnModeLetter
}

// SetFlagColumnMode persists the 3-state mode. Anything not in the
// canonical set is normalised to letter (the default).
func (s *Settings) SetFlagColumnMode(mode string) {
	if s == nil {
		return
	}
	switch mode {
	case FlagColumnModeFull, FlagColumnModeHidden:
		s.UI.FlagColumnMode = mode
	default:
		s.UI.FlagColumnMode = FlagColumnModeLetter
	}
}

// FlagColumnVisible reports whether anything should be rendered.
func (s *Settings) FlagColumnVisible() bool {
	return s.FlagColumnDisplayMode() != FlagColumnModeHidden
}

// ProjectColumnVisible reports whether list-table renderers should
// show the synthetic project-membership gutter. Inverted from the
// on-disk ProjectColumnHidden so callers think in terms of "show
// pills?" rather than "is the column suppressed?"
func (s *Settings) ProjectColumnVisible() bool {
	if s == nil {
		return true
	}
	return !s.UI.ProjectColumnHidden
}

// SetProjectColumnHidden persists the project-gutter toggle. true =
// hide across every list, false = show. Wired to Ctrl+P at runtime.
func (s *Settings) SetProjectColumnHidden(hidden bool) {
	if s == nil {
		return
	}
	s.UI.ProjectColumnHidden = hidden
}

// OrgGroupsConfig is the tree-shaped grouping of authed orgs in the
// left rail. Order is the render order of the groups; Groups are
// keyed by stable id (slug derived from name, plus a numeric suffix
// when the user reuses a name).
type OrgGroupsConfig struct {
	Order  []string         `toml:"order,omitempty"`
	Groups []OrgGroupConfig `toml:"groups,omitempty"`
}

// OrgGroupConfig is one user-defined group of orgs. Members holds
// the username (stable sf identifier) of every org in this group;
// users-with-no-group orgs render under a synthetic "Ungrouped"
// section at the bottom of the rail.
//
// Empty Members is fine — empty groups still render so the user
// can move orgs in via keyboard.
type OrgGroupConfig struct {
	ID        string   `toml:"id"`
	Name      string   `toml:"name"`
	Collapsed bool     `toml:"collapsed,omitempty"`
	Color     string   `toml:"color,omitempty"`
	Members   []string `toml:"members,omitempty"`
}

func cloneOrgGroup(g OrgGroupConfig) OrgGroupConfig {
	out := OrgGroupConfig{
		ID:        g.ID,
		Name:      g.Name,
		Collapsed: g.Collapsed,
		Color:     g.Color,
	}
	if len(g.Members) > 0 {
		out.Members = make([]string, len(g.Members))
		copy(out.Members, g.Members)
	}
	return out
}

// CompareConfig holds the user's saved org-to-org comparison templates
// plus retrieval tuning.
type CompareConfig struct {
	Defs []CompareDef `toml:"defs,omitempty"`

	// Concurrency caps how many Salesforce API calls run at once during a
	// comparison. The retrieve fan-out is otherwise large enough to swamp
	// the machine and trip the org's API concurrency limit. 0 / unset →
	// defaultCompareConcurrency.
	Concurrency int `toml:"concurrency,omitempty"`

	BodyCapKB int `toml:"body_cap_kb,omitempty"`

	// RetainCeilingMB is the safety net: once retained bodies total this
	// many MB across the run, stop retaining further bodies (hash-only
	// from there). Catches the pathological "many medium bodies" shape the
	// per-body cap alone wouldn't bound. 0 / unset → defaultCompareRetainCeilingMB.
	RetainCeilingMB int `toml:"retain_ceiling_mb,omitempty"`
}

// defaultCompareConcurrency is the fallback parallel-retrieve cap. Chosen
// conservative: enough to keep the pipe full without melting the box or
// tripping Salesforce's per-org concurrent-request limit.
const defaultCompareConcurrency = 6

const defaultCompareBodyCapKB = 256

const defaultCompareRetainCeilingMB = 150

// CompareDef is one reusable comparison: a name, source/target org
// usernames, and the metadata-type scope (provider TypeLabels like
// "ApexClass"). Mirrors OrgGroupConfig's persistence shape.
type CompareDef struct {
	Name   string   `toml:"name"`
	Source string   `toml:"source"`
	Target string   `toml:"target"`
	Scope  []string `toml:"scope,omitempty"`
	Method string   `toml:"method,omitempty"` // "Auto" / "Tooling" / "Metadata API"
}

func cloneCompareDef(d CompareDef) CompareDef {
	out := CompareDef{Name: d.Name, Source: d.Source, Target: d.Target, Method: d.Method}
	if len(d.Scope) > 0 {
		out.Scope = make([]string, len(d.Scope))
		copy(out.Scope, d.Scope)
	}
	return out
}

// RecentLimit returns the user's configured /recent display cap.
// Defaults to 50 when unset; clamps to >= 1.
func (s *Settings) RecentLimit() int {
	const def = 50
	if s == nil || s.UI.Recent.Limit == 0 {
		return def
	}
	if s.UI.Recent.Limit < 1 {
		return 1
	}
	return s.UI.Recent.Limit
}

// SetRecentLimit persists the cap. n <= 0 resets to default.
func (s *Settings) SetRecentLimit(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Recent.Limit = 0
		return
	}
	s.UI.Recent.Limit = n
}

// defaultRecentExcludedKinds returns the built-in noise list. Kinds
// here are dropped from the merged /recent stream by default; users
// override via SetRecentExcludedKinds.
//
// Rationale (one entry per noisy kind):
//
//	listview      — saved filter configs; clicking through Lightning
//	                logs one per LV pick. Most aren't intentional
//	                "I want to come back here" actions.
//	public_group  — Salesforce's Group sObject. Admin-only, almost
//	                never useful in a "what was I doing?" log.
//	package       — managed package landing pages. One-time visits.
//	user          — User detail pages. Useful for some workflows but
//	                noisy by default; users can opt back in.
func defaultRecentExcludedKinds() []string {
	return []string{"listview", "public_group", "package", "user"}
}

// RecentExcludedKinds returns the effective exclude list. Returns
// the built-in defaults when the user hasn't touched the setting;
// returns the user's slice (possibly empty) when they have.
func (s *Settings) RecentExcludedKinds() []string {
	if s == nil {
		return defaultRecentExcludedKinds()
	}
	if !s.UI.Recent.UserSetExcludedKinds {
		return defaultRecentExcludedKinds()
	}
	if s.UI.Recent.ExcludedKinds == nil {
		return nil
	}
	out := make([]string, len(s.UI.Recent.ExcludedKinds))
	copy(out, s.UI.Recent.ExcludedKinds)
	return out
}

// SetRecentExcludedKinds persists the user's exclude list. Passing
// nil opts the user out of all default exclusions (every kind shows).
// Passing a non-nil slice (even empty) marks the setting as
// "user-touched" so RecentExcludedKinds doesn't fall back to defaults.
func (s *Settings) SetRecentExcludedKinds(kinds []string) {
	if s == nil {
		return
	}
	s.UI.Recent.UserSetExcludedKinds = true
	if len(kinds) == 0 {
		s.UI.Recent.ExcludedKinds = []string{}
		return
	}
	out := make([]string, len(kinds))
	copy(out, kinds)
	s.UI.Recent.ExcludedKinds = out
}

func defaultRecentExcludedSFTypes() []string {
	return []string{
		"FlowRecordElement", "FlowRecordVersion", "FlowRecord",
		"OmniProcessElement", "OmniDataTransformItem",
		"BatchJob", "CalculationMatrix", "ActionPlanTemplateVersion",
		"DataLakeObjectInstance", "DataSpace",
	}
}

// RecentExcludedSFTypes returns the effective sObject-type exclusion
// list for the RecentlyViewed query: the built-in defaults until the
// user overrides, then their slice (possibly empty = filter nothing).
func (s *Settings) RecentExcludedSFTypes() []string {
	if s == nil || !s.UI.Recent.UserSetExcludedSFTypes {
		return defaultRecentExcludedSFTypes()
	}
	if s.UI.Recent.ExcludedSFTypes == nil {
		return nil
	}
	out := make([]string, len(s.UI.Recent.ExcludedSFTypes))
	copy(out, s.UI.Recent.ExcludedSFTypes)
	return out
}

// SetRecentExcludedSFTypes persists the sObject-type exclusion list.
// Caller Saves. Empty slice = filter nothing (still marks user-set).
func (s *Settings) SetRecentExcludedSFTypes(types []string) {
	if s == nil {
		return
	}
	s.UI.Recent.UserSetExcludedSFTypes = true
	if len(types) == 0 {
		s.UI.Recent.ExcludedSFTypes = []string{}
		return
	}
	out := make([]string, len(types))
	copy(out, types)
	s.UI.Recent.ExcludedSFTypes = out
}

// JumpRows returns the configured jump-step size for shift+arrow /
// J / K navigation. Defaults to 5 when unset; clamps to >= 1.
func (s *Settings) JumpRows() int {
	const def = 5
	if s == nil || s.UI.Input.JumpRows == 0 {
		return def
	}
	if s.UI.Input.JumpRows < 1 {
		return 1
	}
	return s.UI.Input.JumpRows
}

// SetJumpRows persists the jump-step size. n <= 0 resets to default.
func (s *Settings) SetJumpRows(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Input.JumpRows = 0
		return
	}
	s.UI.Input.JumpRows = n
}

// FlowVersionEnterOpens reports whether Enter on a flow version row
// opens Flow Builder (true, the default) rather than drilling into the
// in-terminal definition viewer (false).
func (s *Settings) FlowVersionEnterOpens() bool {
	if s == nil {
		return true
	}
	return boolOr(s.UI.Input.FlowVersionEnterOpens, s.UI.Input.FlowVersionEnterOpensSet, true)
}

// SetFlowVersionEnterOpens persists the flow-version Enter behaviour.
// Caller Saves.
func (s *Settings) SetFlowVersionEnterOpens(v bool) {
	if s == nil {
		return
	}
	s.UI.Input.FlowVersionEnterOpens, s.UI.Input.FlowVersionEnterOpensSet = v, true
}

// DefaultWheelQuietGapMs is the wheel-throttle idle reset window
// when the user hasn't overridden it. Used by the wheel runtime so
// the trackpad-inertial-tail vs mouse-tick distinction stays
// reasonable out of the box.
const DefaultWheelQuietGapMs = 80

// DefaultWheelMinIntervalMs is the minimum gap between accepted
// wheel ticks within a single gesture. 12ms ≈ 80 accepted ticks
// per second, which combined with one-row-per-tick (paginated) or
// up-to-cap-per-tick (continuous) feels smooth without burning
// CPU. Older value of 24ms was set when render cost a lot more
// per frame; with pagination + the row cache it's overkill.
const DefaultWheelMinIntervalMs = 12

// DefaultWheelMaxStep caps the per-accepted-tick cursor delta. 20
// rows ≈ one screenful in a typical 60-row terminal — so even a
// flick producing 200 events advances ~10 frames, which reads as
// "scrolled fast" rather than "teleported." Lower = chunkier but
// more readable; higher = matches finger speed more aggressively.
const DefaultWheelMaxStep = 20

// DefaultRecentMaxEntries is the per-org local visit log cap when
// the user hasn't configured one.
const DefaultRecentMaxEntries = 50

// RecentMaxEntries returns the cap on the per-org local visit log.
func (s *Settings) RecentMaxEntries() int {
	if s == nil || s.UI.Recent.MaxEntries == 0 {
		return DefaultRecentMaxEntries
	}
	if s.UI.Recent.MaxEntries < 1 {
		return 1
	}
	return s.UI.Recent.MaxEntries
}

// SetRecentMaxEntries persists the local-log cap. n <= 0 resets.
func (s *Settings) SetRecentMaxEntries(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Recent.MaxEntries = 0
		return
	}
	s.UI.Recent.MaxEntries = n
}

// DefaultExportHistoryMax is the export tracker's history cap when
// the user hasn't configured one.
const DefaultExportHistoryMax = 200

// ExportHistoryMax returns the export-history cap.
func (s *Settings) ExportHistoryMax() int {
	if s == nil || s.UI.Exports.HistoryMax == 0 {
		return DefaultExportHistoryMax
	}
	if s.UI.Exports.HistoryMax < 1 {
		return 1
	}
	return s.UI.Exports.HistoryMax
}

// SetExportHistoryMax persists the history cap. n <= 0 resets.
func (s *Settings) SetExportHistoryMax(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Exports.HistoryMax = 0
		return
	}
	s.UI.Exports.HistoryMax = n
}

// DefaultLoadedProjectBoost is the global-search ranking bump for
// items in the active dev project, when unset.
const DefaultLoadedProjectBoost = 3.5

// LoadedProjectBoost returns the configured project-membership rank
// boost. 0 = unset → default (3.5). User can set a negative value
// to push project items DOWN, but that's niche.
func (s *Settings) LoadedProjectBoost() float64 {
	if s == nil || s.UI.Search.LoadedProjectBoost == 0 {
		return DefaultLoadedProjectBoost
	}
	return s.UI.Search.LoadedProjectBoost
}

// SetLoadedProjectBoost persists the boost. 0 resets.
func (s *Settings) SetLoadedProjectBoost(v float64) {
	if s == nil {
		return
	}
	s.UI.Search.LoadedProjectBoost = v
}

// DefaultRecentBoostDecayHours is the half-life for fresh-visit
// search bumps when the user hasn't configured one.
const DefaultRecentBoostDecayHours = 24

// RecentBoostDecayHours returns the recency-decay window.
func (s *Settings) RecentBoostDecayHours() int {
	if s == nil || s.UI.Search.RecentBoostDecayHours == 0 {
		return DefaultRecentBoostDecayHours
	}
	if s.UI.Search.RecentBoostDecayHours < 1 {
		return 1
	}
	return s.UI.Search.RecentBoostDecayHours
}

// SetRecentBoostDecayHours persists the decay window. n <= 0 resets.
func (s *Settings) SetRecentBoostDecayHours(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.Search.RecentBoostDecayHours = 0
		return
	}
	s.UI.Search.RecentBoostDecayHours = n
}

// DefaultHomeBannerIntervalMs is the cloud-banner animation tick
// when the user hasn't configured one.
const DefaultHomeBannerIntervalMs = 400

// DefaultListViewPreviewLimit is the row count we fetch when
// /records is driven by a Salesforce List View chip and the user
// hasn't pinned a per-chip Limit. Same default as the legacy
// hardcoded constant.
const DefaultListViewPreviewLimit = 50

// ListViewPreviewLimit returns the row cap for /records list-view
// chips. 0 → default; negatives clamp to 1.
func (s *Settings) ListViewPreviewLimit() int {
	if s == nil || s.UI.ChipDefaults.ListViewPreviewLimit == 0 {
		return DefaultListViewPreviewLimit
	}
	if s.UI.ChipDefaults.ListViewPreviewLimit < 1 {
		return 1
	}
	return s.UI.ChipDefaults.ListViewPreviewLimit
}

// SetListViewPreviewLimit persists the cap. n <= 0 resets.
func (s *Settings) SetListViewPreviewLimit(n int) {
	if s == nil {
		return
	}
	if n <= 0 {
		s.UI.ChipDefaults.ListViewPreviewLimit = 0
		return
	}
	s.UI.ChipDefaults.ListViewPreviewLimit = n
}

// ReportExportFilenamePattern returns the user's configured filename
// pattern, or the default ("{name}-{timestamp}") when unset.
func (s *Settings) ReportExportFilenamePattern() string {
	if s == nil || strings.TrimSpace(s.UI.ReportExportFilenamePattern) == "" {
		return "{name}-{timestamp}"
	}
	return s.UI.ReportExportFilenamePattern
}

// SetReportExportFilenamePattern persists the pattern. Empty string
// clears the override.
func (s *Settings) SetReportExportFilenamePattern(pat string) {
	if s == nil {
		return
	}
	s.UI.ReportExportFilenamePattern = strings.TrimSpace(pat)
}

func defaultExportDir() string {
	// Home directory: the only path on every platform that the user
	// is guaranteed to own + read. macOS Downloads + Documents trigger
	// TCC prompts that block sfdx tools running inside an export
	// bundle (Node's getcwd() denies, sf project retrieve fails). ~
	// sidesteps that. Users who prefer Downloads can change it in
	// Settings → Report export defaults.
	return "~"
}

// ThemeFavourites returns the user's pinned theme ids. Empty when unset.
func (s *Settings) ThemeFavourites() []string {
	if s == nil {
		return nil
	}
	return s.UI.ThemeFavourites
}

// IsThemeFavourite reports whether id is in the favourites list.
func (s *Settings) IsThemeFavourite(id string) bool {
	if s == nil {
		return false
	}
	for _, f := range s.UI.ThemeFavourites {
		if f == id {
			return true
		}
	}
	return false
}

// ToggleThemeFavourite adds or removes id from the favourites list.
// Caller owns Save().
func (s *Settings) ToggleThemeFavourite(id string) {
	if s == nil || id == "" {
		return
	}
	for i, f := range s.UI.ThemeFavourites {
		if f == id {
			s.UI.ThemeFavourites = append(s.UI.ThemeFavourites[:i], s.UI.ThemeFavourites[i+1:]...)
			return
		}
	}
	s.UI.ThemeFavourites = append(s.UI.ThemeFavourites, id)
}

// Theme returns the active theme id, or "tokyo-night" if unset.
func (s *Settings) Theme() string {
	if s == nil || s.UI.Theme == "" {
		return "tokyo-night"
	}
	return s.UI.Theme
}

// SetTheme persists a theme id. Caller is responsible for Save().
func (s *Settings) SetTheme(id string) {
	s.UI.Theme = id
}

// SortPerView reports whether each view (chip) keeps its own sort.
// Default false (sort shared across views on a surface).
func (s *Settings) SortPerView() bool {
	if s == nil {
		return false
	}
	return strings.EqualFold(s.UI.SortPerView, "view")
}

// SetSortPerView persists the per-view-sort mode. Caller Saves.
func (s *Settings) SetSortPerView(perView bool) {
	if s == nil {
		return
	}
	if perView {
		s.UI.SortPerView = "view"
	} else {
		s.UI.SortPerView = ""
	}
}

// Sidebar-position values. "auto" is reserved for future reactive
// placement and currently behaves as a no-op ("coming soon").
const (
	SidebarPositionRHS    = "rhs"
	SidebarPositionBottom = "bottom"
	SidebarPositionAuto   = "auto"
)

// SidebarPosition returns the configured sidebar placement — one of
// SidebarPositionRHS (default), SidebarPositionBottom, or
// SidebarPositionAuto. Unknown/empty reads as RHS.
func (s *Settings) SidebarPosition() string {
	if s == nil {
		return SidebarPositionRHS
	}
	switch strings.ToLower(s.UI.SidebarPosition) {
	case SidebarPositionBottom:
		return SidebarPositionBottom
	case SidebarPositionAuto:
		return SidebarPositionAuto
	default:
		return SidebarPositionRHS
	}
}

// SetSidebarPosition persists the sidebar placement. Unknown values are
// coerced to RHS. Caller Saves.
func (s *Settings) SetSidebarPosition(pos string) {
	if s == nil {
		return
	}
	switch strings.ToLower(pos) {
	case SidebarPositionBottom:
		s.UI.SidebarPosition = SidebarPositionBottom
	case SidebarPositionAuto:
		s.UI.SidebarPosition = SidebarPositionAuto
	default:
		s.UI.SidebarPosition = SidebarPositionRHS
	}
}

// SidebarStartsStacked resolves the boot-time stacked flag from the
// position setting: only "bottom" stacks. "auto" is a no-op today, so
// it starts RHS (unstacked). Used in place of the old
// StartupSidebarStacked bool.
func (s *Settings) SidebarStartsStacked() bool {
	return s.SidebarPosition() == SidebarPositionBottom
}

// WelcomeSeen reports whether the first-launch welcome modal has already
// been shown. Nil-safe: a nil Settings reports "seen" so a degraded
// startup never pops the modal.
func (s *Settings) WelcomeSeen() bool {
	if s == nil {
		return true
	}
	return s.UI.WelcomeSeen
}

// SetWelcomeSeen records that the welcome modal has been shown. Caller
// is responsible for Save().
func (s *Settings) SetWelcomeSeen(v bool) {
	if s == nil {
		return
	}
	s.UI.WelcomeSeen = v
}

// LegalAccepted reports whether the current policy revision was accepted.
// Nil settings are not accepted: a degraded startup must not silently cross
// the acknowledgement gate.
func (s *Settings) LegalAccepted(version string) bool {
	if s == nil || strings.TrimSpace(version) == "" {
		return false
	}
	return s.UI.LegalAcceptedVersion == version
}

// AcceptLegal records acceptance of a specific policy revision and its UTC
// timestamp. Caller is responsible for Save().
func (s *Settings) AcceptLegal(version string, at time.Time) {
	if s == nil {
		return
	}
	s.UI.LegalAcceptedVersion = strings.TrimSpace(version)
	s.UI.LegalAcceptedAt = at.UTC().Format(time.RFC3339)
}

// LegalAcceptance returns the stored revision and timestamp for status UIs.
func (s *Settings) LegalAcceptance() (version, acceptedAt string) {
	if s == nil {
		return "", ""
	}
	return s.UI.LegalAcceptedVersion, s.UI.LegalAcceptedAt
}

// DebugForceWelcome reports whether the debug "always show welcome"
// toggle is set. Nil-safe.
func (s *Settings) DebugForceWelcome() bool {
	if s == nil {
		return false
	}
	return s.UI.Debug.ForceWelcome
}

// SetDebugForceWelcome flips the debug force-welcome toggle. Caller is
// responsible for Save().
func (s *Settings) SetDebugForceWelcome(v bool) {
	if s == nil {
		return
	}
	s.UI.Debug.ForceWelcome = v
}

// DemoOrgImported reports whether the demo org has been imported.
func (s *Settings) DemoOrgImported() bool {
	if s == nil {
		return false
	}
	return s.UI.DemoOrgImported
}

// SetDemoOrgImported records the demo-org import state. Caller is
// responsible for Save().
func (s *Settings) SetDemoOrgImported(v bool) {
	if s == nil {
		return
	}
	s.UI.DemoOrgImported = v
}
