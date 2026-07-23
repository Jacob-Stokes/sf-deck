package ui

// Terminal/render/runtime state that is not tied to a specific org or
// surface.
//
// Extracted from model.go. modelRuntime is embedded into Model so
// existing field access (m.width, m.focus, m.renderCache, …) keeps
// working unchanged.

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

// modelRuntime groups terminal/render/runtime state that is not tied to a
// specific org or surface.
type modelRuntime struct {
	width, height  int
	lastCompositor *lipgloss.Compositor
	renderCache    *renderCache
	renderTrace    *renderTracer
	wheel          *wheelRuntime
	focus          focus

	// tabOverride lets a read-only render helper (specifically
	// searchStateForTab) ask "what would m.tab() return if the user
	// were on tab X?" without mutating orgData.Tab.  When
	// tabOverrideSet is true, Model.tab() returns tabOverride
	// instead of the active org's tab.
	//
	// Used to make per-tab search-state lookups work generically:
	// SearchPtr closures that internally consult m.currentSubtab()
	// (objectDetailSearchPtr, reportsSearchPtr, etc.) need to see
	// the right tab's subtab, not the user's currently-active one.
	// Set via the local helper; always reverted before the model is
	// returned to bubbletea.
	//
	// Separate bool because TabHome == iota 0 is a valid override
	// target, so a "zero means unset" check would silently mask it.
	tabOverride    Tab
	tabOverrideSet bool

	sidebarOpen        bool
	sidebarStacked     bool // | toggle — sidebar sits BELOW main pane (2/3 main, 1/3 sidebar) instead of beside it; useful on narrow terminals
	sidebarForModal    bool // set on the throwaway Model clone fullSidebarContent renders for the inspect (i) modal — suppresses stacked-mode compaction so the modal always shows the full, roomy layout
	dashboardCollapsed bool // ctrl+= toggle — hides chip strip

	// startupLayoutDone latches after the first WindowSizeMsg applies
	// the auto-layout decision (see applyStartupAutoLayout). One-shot
	// by design: auto-layout picks sidebar placement from the initial
	// terminal width and never re-runs on later resizes, so the user's
	// subsequent manual ctrl+\ toggles stick.
	startupLayoutDone bool

	// sidebarInnerH is the sidebar's available content height (rows
	// inside the border), stashed by viewImpl before the body is
	// composed so sidebar renderers that reflow into columns (the
	// detail context panels) know how tall they can be. Width still
	// arrives as the `inner` arg; height was previously known only to
	// renderSidebar, which clipped after the fact.
	sidebarInnerH   int
	queryLineHidden bool // ctrl+- toggle — hides the SOQL query line under the chip strip on records surfaces; defaults to true (hidden)

	// sidebarTitleW, when >0, is the FULL sidebar inner width during a
	// stacked-mode NOTE-box split. The note box narrows the content
	// column, but the panel's title row (with its right-aligned project
	// pills) still spans the whole panel — the box starts one row
	// below. Set by renderSidebar on its local copy before resolving;
	// zero everywhere else, meaning "title width == content width".
	sidebarTitleW int

	// autocompletePending buffers tea.Cmds the SOQL autocomplete
	// engine wants to fire (typically describe-ensure cmds when a
	// relationship hop touches an uncached sObject). The edit-key
	// handler drains this buffer once per tick so the cmds land on
	// the next bubbletea round.
	autocompletePending []tea.Cmd

	// zenMode is the universal fallback zen toggle for tabs that
	// don't have a list-table state of their own (detail tabs, code
	// bodies, dashboards). When true, render hides every chrome
	// element — left rail, sidebar, tab bars, status bar — and the
	// active main pane takes the whole terminal. Independent of the
	// per-list-table `Zen` flag so each list keeps remembering its
	// own zen state across tab switches.
	zenMode bool

	// Left rail state. The narrow icon strip is always visible; the
	// wider widget pane is toggled by `ctrl+\`. leftUtilityIdx
	// selects which utility (Orgs, future: Bookmarks, History, ...)
	// the widget pane is showing — see leftrail.go.
	leftOpen bool
	// leftPinned distinguishes "user pinned it open with `ctrl+\`" from
	// "rail opened transiently because user hit ' / clicked the
	// Orgs pill / used a quick-jump." When the rail is unpinned and
	// the user picks an org or navigates to another tab, we auto-
	// collapse it. Pinned-open survives both — the rail stays.
	leftPinned     bool
	leftUtilityIdx int

	// overflowTab + overflowSet — slot 0 holds the most recently
	// activated tab whose stem isn't on the pinned bar. Use the
	// bool rather than a sentinel because TabHome is iota 0 and
	// would otherwise look identical to "unset."
	overflowTab Tab
	overflowSet bool

	banner      string
	bannerUntil time.Time

	// Update discovery is process-wide, read-only state. Automatic checks run
	// at most daily and never block first paint; these fields drive the Home
	// notice, header badge, Settings status, and About modal.
	updateResult   updatecheck.Result
	updateChecked  bool
	updateChecking bool
	updateErr      string

	// exportTickRunning guards a single-flight tea.Tick that drives
	// the activity ellipsis animation while exports are in flight.
	// Same shape as homeBadgeTickRunning — without the flag we'd
	// kick a fresh tick every Update pass and the ellipsis would
	// accelerate over time.
	exportTickRunning bool

	// deployWatchRunning guards the single-flight /deploys live-watch
	// tick — re-armed after every deploys_v2 apply while any row is
	// still Pending / InProgress; dies once everything is terminal.
	deployWatchRunning  bool
	exportActivityFrame int

	// orgQuickJumpActive is the "ultra-shortcut" overlay flag for the
	// Orgs left-rail panel. Set true when the user presses the
	// FocusOrgs key (default `0`); cleared by any nav action (j/k,
	// arrow keys, scroll, esc) or after a quick-jump letter fires.
	// While true, renderOrgsWidget shows a QWERTY letter to the LEFT
	// of each org's cursor indicator (q for index 0, w for 1, …); the
	// keymap dispatcher routes the matching letter through to "select
	// that org + return focus to main."
	orgQuickJumpActive bool

	// chordActive is the leader-key state: true after q is pressed in
	// normal-nav mode, until the next key fires (or cancels) a q-<letter>
	// chord. While true the status bar shows a CHORD alert. See chord.go.
	chordActive bool

	// orgRailCursor addresses the unified header+org row list that
	// renderOrgsWidget walks (see buildRailRows). Group headers
	// occupy cursor positions so the user can land on a header
	// with j/k and act on it (space to expand/collapse, R to
	// rename). When the cursor lands on an org row m.selected
	// mirrors the row's OrgIdx so every existing "current org"
	// consumer keeps working unchanged.
	//
	// 0 is a safe default — buildRailRows always emits at least
	// one row when len(m.orgs) > 0.
	orgRailCursor int
}
