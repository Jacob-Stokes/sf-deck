package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

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
	// Supports generic per-tab search-state lookups:
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

	sidebarInnerH   int
	queryLineHidden bool // ctrl+- toggle — hides the SOQL query line under the chip strip on records surfaces; defaults to true (hidden)

	// sidebarTitleW, when >0, is the FULL sidebar inner width during a
	// stacked-mode NOTE-box split. The note box narrows the content
	// column, but the panel's title row (with its right-aligned project
	// pills) still spans the whole panel — the box starts one row
	// below. Set by renderSidebar on its local copy before resolving;
	// zero everywhere else, meaning "title width == content width".
	sidebarTitleW int

	autocompletePending []tea.Cmd

	zenMode bool

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

	exportTickRunning bool

	deployWatchRunning  bool
	exportActivityFrame int

	orgQuickJumpActive bool

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
