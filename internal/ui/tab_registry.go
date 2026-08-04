package ui

// Tab registry — declarative metadata for each top-level Tab so the
// common dispatchers don't have to grow a switch arm per tab.
//
// The main per-tab surface areas are covered by TabSpec hooks:
//
//   1. EnsureData    — fires on tab entry; primes Resources.
//   2. RefreshData   — fires on `r`.
//   3. SearchPtr     — returns the active list's *searchState.
//   4. MoveCursor    — applies a delta to the active list.
//   5. ResetCursor   — used by `home`, search activation, etc.
//   6. Activate      — what Enter does.
//   7. EscBack       — what tab to pop to on Esc; 0 means default.
//
// Uniform chip/open/list behavior lives in chipSurface, openSurface,
// and listSurface vars referenced by the spec. Bespoke surfaces use
// closures on TabSpec/SubtabSpec as escape hatches.
//
// Adding a new tab now means:
//   1. Declare a TabSpec with whichever hooks make sense.
//   2. Register it in tabRegistry().
//   3. Add renderer/data/surface hooks here before reaching for a
//      dispatcher switch.
//
// Per-hook fallback: every dispatcher consults the registry first,
// then falls through to its in-place switch for tabs that haven't
// been migrated yet. This lets us migrate one tab at a time without
// rewiring every dispatcher in lockstep.

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// TabSpec captures the data-lifecycle + cursor-management +
// surface-behavior of a Tab. Hooks are called from the central
// dispatchers with the live Model; TabSpec instances are pure
// values so a registry can safely share them across Models.
//
// The ENTIRE set of per-tab behavior lives here — uniform surfaces
// register chip/open/list pointers; bespoke surfaces register
// hand-rolled closures in the same fields. Either way the
// dispatcher walks one registry. Nothing per-tab should live in a
// `case Tab...` switch outside this file once migration is complete.
//
// Adding a new tab — contributor checklist
// ----------------------------------------
// Walk the fields below in order and decide which apply. Most tabs
// only need a handful; nil fields are fine and mean "this gesture
// doesn't apply here."
//
//	Identity       — does the user need to tag / collect / yank the
//	                 cursored item? Wire one closure here and the
//	                 tag picker, openable lookup, and (future) collect
//	                 target all work for free.
//
//	Sidebar        — do you have a right-pane context panel? If no,
//	                 the sidebar is empty.
//
//	Renderer       — required. Main-pane content.
//
//	Chips/Open/List — only if the tab has a uniform list-shape
//	                 surface that fits the registry pattern. Bespoke
//	                 surfaces leave these nil and use the per-hook
//	                 closures (SearchPtr / MoveCursor / Activate / …).
//
//	Breadcrumb     — does the header path need to show context (parent
//	                 object, subtab, cursored row)? Optional.
//
//	BusyLabel      — does the tab have a network resource whose load
//	                 state should appear in the header's activity zone?
//	                 Optional but recommended for any tab that can take
//	                 >100ms to populate.
//
//	ErrorLabel     — paired with BusyLabel; surfaces a fetch error.
//
//	ListTable      — does the tab have a sortable / column-mode-aware
//	                 list-table view? Only set when the column spec is
//	                 dynamic (not registered via the listSurface).
//
//	EnsureData /
//	RefreshData    — wire the tab's primary resource lifecycle. Drives
//	                 auto-fetch on tab entry + manual r refresh.
//
//	Subtabs        — only if the tab branches by subtab. Each SubtabSpec
//	                 can override every behavioral hook above.
type TabSpec struct {
	Tab Tab
	// Stem is the top-level Tab this one belongs to for number-key
	// nav; drill tabs return their parent. For non-drill tabs this
	// is the tab itself. Tab.stem() reads this field.
	Stem Tab
	// OverflowHint is the one-line description shown beside the tab
	// in the More… overflow modal. Optional; blank falls back to
	// the slug.
	OverflowHint string
	// TransientDrill marks per-row/per-event drill tabs that must NOT
	// be remembered by LastTabInStem — recording them makes a later
	// number-key hop teleport into a stale row. Per-entity drills
	// (object, flow, apex class) stay false: users expect to return
	// to the entity they were inspecting. isTransientDrill reads this.
	TransientDrill bool

	// OrgIndependent marks tabs whose content doesn't come from the
	// active org (local SQLite stores, static link lists, multi-org
	// surfaces). Everything else is gated by the disconnected-org
	// guard in renderMain: cached data is HIDDEN (never deleted)
	// while the org can't authenticate, and reappears + refreshes on
	// re-auth. Without this central gate only ~half the tab
	// renderers checked canUseOrg themselves (field report
	// 2026-06-14) — per-tab checks are the seam class.
	OrgIndependent bool

	// ---- Data lifecycle ----
	EnsureData  func(m *Model, d *orgData, o sf.Org) tea.Cmd
	RefreshData func(m Model, d *orgData) tea.Cmd

	// ---- Top-level surface bindings ----
	// These apply when no subtab is active (or as the default for
	// every subtab unless overridden). Subtabs[i].XYZ shadows the
	// tab-level XYZ when set.
	Renderer func(m Model, w, innerH int) string // top-level render path; ignored when Subtabs non-empty unless subtab has nil Renderer

	// Surface registries — pointers so nil = "no chip strip / no
	// list / no o-target on this surface".
	Chips *chipSurface
	Open  *openSurface
	List  *listSurface

	// ---- Bespoke escape hatches ----
	// SearchPtr / MoveCursor / ResetCursor / Activate are the
	// hand-rolled closures used by surfaces whose behavior doesn't
	// fit the surface registries. The dispatcher calls these AFTER
	// the registry pointers fail to resolve, so a TabSpec entry can
	// say "uniform list surface for chip+open+list" or "everything
	// uniform but Activate is bespoke" or "all four bespoke".
	SearchPtr   func(m Model) *searchState
	MoveCursor  func(m *Model, delta int)
	ResetCursor func(m *Model)
	Activate    func(m *Model) tea.Cmd
	// CycleChip is the bespoke ← / → handler for tabs whose chip
	// strip lives outside the chipSurface registry — multi-axis
	// chip cursors (TabObjectDetail's per-subtab chip strips,
	// TabPermParentDetail's per-sobject FLS drill, TabRecords'
	// synth-views in drill mode).
	CycleChip func(m *Model, delta int) tea.Cmd

	// Identity returns the stable selected item under the cursor on
	// this tab. Strict scope: it answers ONE question — "what
	// concrete thing is highlighted right now?" — and returns the
	// canonical (kind, ref, label, openable) tuple.
	//
	// Identity is NOT for arbitrary tab actions, side-effects, or
	// contextual data. Surfaces that need bespoke gesture behavior
	// (multi-target opens, tab-specific commit flows, …) keep their
	// own dispatchers; this hook only feeds the things that key off
	// "the row I'm currently on": tag picker, openable lookup,
	// future collect/yank/where-used routes.
	//
	// Returns ok=false when there's nothing actionable selected
	// (empty list, fetch in flight, no org).
	Identity func(m Model) (ItemIdentity, bool)

	// NoCollectReason documents why a List+Open tab deliberately has no
	// Identity resolver — see SubtabSpec.NoCollectReason. Set it to opt
	// a tab out of TestListOpenSurfacesHaveIdentity.
	NoCollectReason string

	// Sidebar renders the right-pane context panel for this tab.
	// Same shape as Renderer but gets `inner` instead of (w,
	// innerH) — the sidebar manages its own outer chrome
	// wrapping. Returns the rendered string. Nil = fall through
	// to the legacy renderSidebar dispatch (no sidebar registered
	// yet for this tab).
	Sidebar func(m Model, inner int) string

	// ListTable returns the per-surface list-table state +
	// columns for the c (column-mode) / s (sort) / [ ] (resize)
	// / , . (scroll) gestures. Surfaces with dynamic columns
	// (SOQL, ReportDetail's run, Records list-view results)
	// declare their own resolver here; uniform list surfaces
	// reuse the listSurface registry via List + a thin shim.
	// Nil = fall through to listSurface-driven default.
	ListTable func(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn)

	// MeasureCell, when non-nil, returns the rendered width of
	// the widest cell in column `col` across the data currently
	// behind ListTable. Drives snap-to-content (}). Only
	// meaningful when ListTable is set; uniform list surfaces
	// derive this automatically from listSurface.BuildRenderModel.
	MeasureCell func(m *Model, col int) int

	// Breadcrumb returns the path segments shown after the tab
	// name in the header — typically (sObject, subtab, cursored
	// item) for drill tabs. Segments render with " › " between
	// them; nil / empty slice = no breadcrumb beyond the tab name.
	// Per-subtab breadcrumbs declared on SubtabSpec.Breadcrumb
	// take precedence.
	Breadcrumb func(m Model) []string

	// BusyLabel returns the activity-zone label rendered with a
	// spinner glyph when this tab's primary resource is loading
	// ("syncing flows…", "describing Account…"). Empty string =
	// no activity to surface. The activity zone fades when both
	// busy and error are empty so narrow terminals collapse
	// gracefully.
	BusyLabel func(m Model, d *orgData) string

	// ErrorLabel returns the activity-zone error text when this
	// tab's primary resource has an Err. Empty string = no error.
	// Rendered red with a "!" prefix.
	ErrorLabel func(m Model, d *orgData) string

	// PrimaryFetchedAt returns the FetchedAt() time of the resource
	// that represents the "main data on screen" for this tab. Drives
	// the header's "X seconds ago" age stamp. Return zero time when
	// the view doesn't map to a single resource or nothing's loaded.
	// Per-subtab variants live on SubtabSpec.
	PrimaryFetchedAt func(m Model, d *orgData) time.Time

	// Help returns the per-tab `?` help-modal state. Empty
	// infoModalState (Title == "") means "no tab-specific help —
	// fall back to the generic placeholder." Per-subtab variants
	// live on SubtabSpec.Help.
	Help func(m Model) infoModalState

	// RecordRecentVisit, if non-nil, fires on tab entry to register
	// a recent-visit entry in the per-org log. Drill tabs implement
	// this; list/dashboard tabs leave it nil so the recent log only
	// captures real "I drilled into X" events.
	RecordRecentVisit func(m *Model, d *orgData, orgUser string)

	// EscBack is the tab to pop to on Esc. Zero (== TabHome) is
	// treated as "no override — let the global handler decide."
	// Set explicitly for drill tabs that pop to a parent.
	EscBack Tab

	// SidebarFocusable, when true, opts the tab into Tab/Shift+Tab
	// swapping focus between the main pane and the right sidebar
	// (m.bodyFocus). Detail surfaces that pair an interactive
	// sidebar action menu with the body set this true so j/k/Enter
	// route to whichever pane is currently focused. Default false:
	// Tab cycles subtabs as on every list-shaped tab.
	SidebarFocusable bool

	// ---- Subtabs ----
	// Subtabs is the per-subtab spec list. When non-empty the
	// tab's behavior is per-subtab — the dispatcher resolves to
	// the active SubtabSpec before walking the spec hooks above.
	Subtabs []SubtabSpec

	// GetSubtabIdx / SetSubtabIdx / SubtabReloadOnSwitch are the
	// subtab cursor accessors. Required when Subtabs is non-empty.
	GetSubtabIdx         func(m Model) int
	SetSubtabIdx         func(m *Model, i int)
	SubtabReloadOnSwitch func(m Model, idx int) bool

	// SubtabsResolver, when non-nil, returns the *dynamic* subtab
	// list for this tab. Beats the static Subtabs slice when set —
	// the resolver runs per-render so subtabs whose shape depends on
	// model state (LWC bundle's per-file subtabs, perm-parent's
	// per-kind subtabs) get the right list at the right time.
	// Static-subtab tabs leave this nil and use Subtabs directly.
	SubtabsResolver func(m Model) []subtabInfo

	// SubtabPinned, when > 0, caps the strip-rendered subtab count.
	// Subtabs beyond this index become reachable via the More…
	// overflow modal instead of the strip. Default 0 = no cap, every
	// subtab pinned. TabObjectDetail uses this to fit its 9 subtabs
	// onto a 6-slot strip without truncation.
	SubtabPinned int

	// ---- Widget ----
	// Widget is an interactive component pinned at the top of the
	// tab body — Home's ORG card, SOQL's editor, future deploy /
	// import widgets. Nil = no widget. See Widget docs for focus
	// + key-routing semantics.
	Widget *Widget
}

// SubtabSpec is the per-subtab spec — same shape as TabSpec for the
// behavioral fields, so each subtab can override what its parent
// tab declared. Anything left nil falls through to the parent tab's
// declaration.
type SubtabSpec struct {
	ID    Subtab
	Label string

	Renderer func(m Model, w, innerH int) string
	Chips    *chipSurface
	Open     *openSurface
	List     *listSurface

	// Per-subtab hand-rolled hooks. When TabObjectDetail's Schema
	// subtab needs a different cursor model from its Records subtab,
	// each subtab's MoveCursor closure encapsulates its own logic
	// — the dispatcher just walks subtab.MoveCursor without caring
	// what's inside. Nil = inherit from parent TabSpec.
	SearchPtr   func(m Model) *searchState
	MoveCursor  func(m *Model, delta int)
	ResetCursor func(m *Model)
	Activate    func(m *Model) tea.Cmd

	// Identity is the per-subtab cursored-item resolver. See
	// TabSpec.Identity. Subtabs typically need their own resolver
	// since each subtab has a different "what's under the cursor"
	// answer (Object Detail's Schema vs Validation vs Records).
	Identity func(m Model) (ItemIdentity, bool)

	// NoCollectReason documents why a List+Open surface deliberately
	// has NO Identity resolver — i.e. its rows are intentionally not
	// taggable / collectable / movable / yankable. Set this (with a
	// real reason) to opt a surface out of the identity-coverage drift
	// test (TestListOpenSurfacesHaveIdentity). Leaving BOTH Identity
	// and NoCollectReason empty on a List+Open surface fails that test
	// — the point being that "this surface's rows can't be worked with"
	// must be a conscious decision, not an oversight.
	NoCollectReason string

	// Sidebar is the per-subtab right-pane renderer. See
	// TabSpec.Sidebar. Object Detail's subtabs each have a
	// different sidebar (object actions / per-field detail / per-
	// rule detail / record KV dump) — declaring per-subtab keeps
	// the dispatch in one place.
	Sidebar func(m Model, inner int) string

	// ListTable is the per-subtab list-table resolver. See
	// TabSpec.ListTable. Object Detail's Records subtab has a
	// dynamic column spec (visible record-list cols) so it
	// declares its own resolver here.
	ListTable func(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn)

	// MeasureCell mirrors TabSpec.MeasureCell at the subtab
	// scope. Set when ListTable is set per-subtab and the
	// surface wants snap-to-content (}) without falling
	// back to header-width.
	MeasureCell func(m *Model, col int) int

	// Breadcrumb is the per-subtab breadcrumb-segment resolver.
	// See TabSpec.Breadcrumb.
	Breadcrumb func(m Model) []string

	// BusyLabel is the per-subtab activity-zone resolver. See
	// TabSpec.BusyLabel.
	BusyLabel func(m Model, d *orgData) string

	// ErrorLabel is the per-subtab error-zone resolver. See
	// TabSpec.ErrorLabel.
	ErrorLabel func(m Model, d *orgData) string

	// PrimaryFetchedAt is the per-subtab age-stamp resolver. See
	// TabSpec.PrimaryFetchedAt.
	PrimaryFetchedAt func(m Model, d *orgData) time.Time

	// Help is the per-subtab help-modal resolver. See TabSpec.Help.
	Help func(m Model) infoModalState

	// Widget is a per-subtab pinned widget — e.g. an "add user"
	// search-box on /perms permset Members. Nil = no widget for
	// this subtab.
	Widget *Widget

	// EnsureData fires on subtab entry when SubtabReloadOnSwitch
	// returns true. Lets a subtab pull data the parent tab didn't
	// load up front.
	EnsureData func(m *Model, d *orgData, o sf.Org) tea.Cmd

	// OnEnter fires synchronously when the user navigates into
	// this subtab. Use it for synchronous, non-network setup —
	// lazy-loading a cached snapshot from the local store, resetting
	// a per-subtab cursor, etc. Network fetches belong in EnsureData
	// so they go through the standard tea.Cmd path.
	//
	// Called by the subtab dispatcher (see TabSOQL's SetSubtabIdx)
	// after the cursor index moves but before the next render.
	// Idempotent: gets called every time the user enters the
	// subtab, so the closure should guard against repeated work
	// (e.g. check a Loaded flag before reloading).
	OnEnter func(m *Model)
}

// lazyLoadOnEnter builds an OnEnter handler for the common lazy-load
// idiom: on first entry to a subtab, if the active org's data exists and
// hasn't loaded this slice yet, reload it. notLoaded reports whether the
// reload is still needed (e.g. !d.SOQLSavedLoaded); reload does the work.
// Idempotent — reload is skipped once notLoaded returns false.
func lazyLoadOnEnter(notLoaded func(d *orgData) bool, reload func(m *Model, d *orgData)) func(m *Model) {
	return func(m *Model) {
		if d, ok := m.activeOrgState(); ok && notLoaded(d) {
			reload(m, d)
		}
	}
}

// Widget is an interactive component pinned at the top of a tab or
// subtab body. Concretely: Home's ORG card (read-only), SOQL's
// editor (focus-grabbing), future deploy/import wizards.
//
// Render is the always-called paint function; Focusable + HandlesKey
// participate in the key-routing dance — when the widget is the
// "active focus target", the global handler defers to HandlesKey
// before its own dispatch.
type Widget struct {
	// Render returns the widget block. innerH is its budget — the
	// widget decides how tall to be. Empty string = invisible
	// (e.g. SOQL editor collapsed).
	Render func(m Model, w, innerH int) string

	// Focusable reports whether the widget participates in the
	// tab's focus order. Read-only widgets (Home's ORG card) are
	// non-focusable; editors and forms are focusable.
	Focusable bool

	// HandlesKey is consulted when the widget owns focus. Returns
	// (cmd, true) to consume the key; (_, false) to pass through
	// to the global dispatcher. Nil = pass everything through (a
	// non-interactive focusable widget — uncommon).
	HandlesKey func(m *Model, msg tea.KeyMsg) (tea.Cmd, bool)
}

// tabRegistry returns the full set of registered TabSpecs keyed by
// Tab. It's a function (not a var) so it's free of init-order
// surprises — every field that a spec closes over is already set
// on first call. Callers should use lookupTabSpec for lookup.
func tabRegistry() map[Tab]TabSpec {
	return map[Tab]TabSpec{
		TabObjects: {
			OverflowHint: "sObjects + records + fields",
			Tab:          TabObjects,
			Stem:         TabObjects,
			Renderer:     Model.renderObjects,
			Chips:        &objectsChipSurface,
			Open:         &objectsOpenSurface,
			List:         &objectsListSurface,
			Identity:     identityFromObjectsList,
			Sidebar:      Model.sidebarObjects,
			Breadcrumb:   breadcrumbFromObjectsList,
			BusyLabel:    busyObjects,
			ErrorLabel:   errObjects,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				return d.SObjects.FetchedAt()
			},
			EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
				return d.SObjects.Ensure(m.cache)
			},
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				return d.SObjects.Refresh(m.cache)
			},
		},
		TabHome: {
			OverflowHint: "landing / recent / notifications",
			Tab:          TabHome,
			Stem:         TabHome,
			Open:         &homeFallbackOpenSurface,
			Renderer:     Model.renderHome,
			Sidebar:      Model.sidebarHome,
			// Landing destinations move via the bespoke key handler too,
			// but wiring MoveCursor here lets the scroll wheel + arrow
			// keys drive the same cursor (the wheel handler calls
			// moveCursor → resolveMoveCursor). No-op off the Landing
			// subtab; the list-table subtabs (Limits/Licenses/etc.) own
			// their own cursors via their list surfaces.
			MoveCursor:           func(m *Model, delta int) { m.moveHomeDestCursor(delta) },
			BusyLabel:            busyHome,
			ErrorLabel:           errHome,
			PrimaryFetchedAt:     func(m Model, d *orgData) time.Time { return d.Home.FetchedAt() },
			Help:                 func(m Model) infoModalState { return helpSafety() },
			GetSubtabIdx:         func(m Model) int { return m.homeSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setHomeSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return false },
			EnsureData:           (*Model).ensureHomeData,
			RefreshData:          Model.refreshHomeData,
			Subtabs: []SubtabSpec{
				// Landing is the splash pane — figlet logo + tagline.
				{ID: SubtabHomeLanding, Label: "Landing"},
				{ID: SubtabHomeRecent, Label: "Recently Viewed", Open: &homeRecentOpenSurface, List: &homeRecentListSurface, Chips: &recentChipSurface,
					NoCollectReason: "recently-viewed history is a heterogeneous activity log, not a set of collectable resources; open (o) is the only meaningful gesture"},
				{ID: SubtabHomeNotifications, Label: "Notifications", Open: &homeNotificationsOpenSurface, List: &homeNotificationsListSurface,
					NoCollectReason: "notifications are ephemeral events, not deployable metadata"},
				{
					ID: SubtabHomeLimits, Label: "Limits",
					Open:            &homeLimitsOpenSurface,
					List:            &homeLimitsListSurface,
					NoCollectReason: "org limits are live metrics, not resources",
				},
				{
					ID: SubtabHomeLicenses, Label: "Licenses",
					Open:            &homeLicensesOpenSurface,
					List:            &homeLicensesListSurface,
					NoCollectReason: "license usage is a live metric, not a resource",
				},
				{
					ID: SubtabHomeDownloads, Label: "Downloads",
					MoveCursor:  homeDownloadsMoveCursor,
					ResetCursor: func(m *Model) { m.homeDownloadsCursor = 0 },
					Activate:    homeDownloadsActivate,
				},
			},
		},
		TabDeployDetail: {
			TransientDrill:   true,
			Tab:              TabDeployDetail,
			Stem:             TabSystem,
			Renderer:         Model.renderDeployDetail,
			Open:             &deployDetailOpenSurface,
			Sidebar:          Model.sidebarDeploy,
			EscBack:          TabSystem,
			MoveCursor:       (*Model).moveDeployDetailCursor,
			EnsureData:       (*Model).ensureDeployDetailData,
			RefreshData:      Model.refreshDeployDetailData,
			PrimaryFetchedAt: deployDetailFetchedAt,
		},
		TabApex: {
			OverflowHint:         "apex classes + triggers",
			Tab:                  TabApex,
			Stem:                 TabApex,
			Renderer:             Model.renderApex,
			EnsureData:           (*Model).ensureApexData,
			RefreshData:          Model.refreshApexData,
			Sidebar:              Model.sidebarApex,
			GetSubtabIdx:         func(m Model) int { return m.apexSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setApexSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, i int) bool { return true },
			// Subtab declarations carry the chip / open / list
			// surfaces directly. Anyone extending /apex starts here.
			Subtabs: []SubtabSpec{
				{ID: SubtabApexClasses, Label: "Classes", Chips: &apexClassesChipSurface, Open: &apexClassesOpenSurface, List: &apexClassesListSurface, Identity: identityFromApexClassesList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ApexClasses.FetchedAt() }},
				{ID: SubtabApexTriggers, Label: "Triggers", Chips: &apexTriggersChipSurface, Open: &apexTriggersOpenSurface, List: &apexTriggersListSurface, Identity: identityFromApexTriggersList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ApexTriggersFlat.FetchedAt() }},
			},
		},
		TabApexDetail: {
			RecordRecentVisit: recentVisitApexDetail,
			Tab:               TabApexDetail,
			Stem:              TabApex,
			Renderer:          Model.renderApexDetail,
			PrimaryFetchedAt:  apexDetailFetchedAt,
			Sidebar:           Model.sidebarApexDetail,
			MoveCursor:        (*Model).moveApexDetailCursor,
			RefreshData:       Model.refreshApexDetailData,
			EscBack:           TabApex,
		},
		TabLWC: {
			OverflowHint:         "LWC + Aura bundles",
			Tab:                  TabLWC,
			Stem:                 TabLWC,
			Renderer:             Model.renderComponents,
			Sidebar:              Model.sidebarComponents,
			EnsureData:           (*Model).ensureComponentsData,
			RefreshData:          Model.refreshComponentsData,
			GetSubtabIdx:         func(m Model) int { return m.componentsSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setComponentsSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, i int) bool { return true },
			Subtabs: []SubtabSpec{
				{ID: SubtabComponentsLWC, Label: "LWC", Chips: &lwcChipSurface, Open: &lwcOpenSurface, List: &lwcListSurface, Identity: identityFromLWCList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.LWCBundles.FetchedAt() }},
				{ID: SubtabComponentsAura, Label: "Aura", Chips: &auraChipSurface, Open: &auraOpenSurface, List: &auraListSurface, Identity: identityFromAuraList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.AuraBundles.FetchedAt() }},
			},
		},
		TabLWCDetail: {
			RefreshData:       Model.refreshLWCDetailData,
			RecordRecentVisit: recentVisitLWCDetail,
			Tab:               TabLWCDetail,
			Stem:              TabLWC,
			Renderer:          Model.renderComponentsDetail,
			PrimaryFetchedAt:  componentsDetailFetchedAt,
			Sidebar:           Model.sidebarComponentsDetail,
			// Dynamic per-bundle subtabs: one per resource file in the
			// drilled-in LWC / Aura bundle, in declared order. Each
			// becomes a real subtab so Tab / Shift+Tab / Shift+1..9 all
			// work the standard way for free.
			SubtabsResolver: func(m Model) []subtabInfo { return m.lwcDetailSubtabs() },
			MoveCursor:      (*Model).moveBundleDetailCursor,
			GetSubtabIdx:    (Model).bundleSubtabIdx,
			SetSubtabIdx:    (*Model).setBundleSubtabIdx,
			EscBack:         TabLWC,
		},
		TabMeta: {
			OverflowHint:         "metadata long-tail (browse all types, labels, …)",
			Tab:                  TabMeta,
			Stem:                 TabMeta,
			Renderer:             Model.renderMeta,
			EnsureData:           (*Model).ensureMetaData,
			RefreshData:          Model.refreshMetaData,
			GetSubtabIdx:         func(m Model) int { return m.metaSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setMetaSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, i int) bool { return true },
			Subtabs: []SubtabSpec{
				// The /meta subtabs below are genuine metadata that SHOULD
				// be collectable/taggable, but don't have Identity
				// resolvers yet — this is deferred work, not a design
				// choice. Tracked in docs/backlog.md ("Global Value Sets
				// as a trackable resource" covers the pattern). The
				// reason strings say "deferred" so they read as a TODO,
				// not an intentional opt-out.
				{ID: SubtabMetaBrowse, Label: "Browse", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.MetaTypes.FetchedAt() }, List: &metaTypesListSurface, Open: &metaBrowseOpenSurface, Sidebar: Model.sidebarMetaHub,
					NoCollectReason: "deferred: metadata-type rows drill into components rather than representing one collectable resource"},
				{ID: SubtabMetaCustomMetadata, Label: "Custom Metadata", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.CMTTypes.FetchedAt() }, List: &cmtListSurface, Open: &cmtOpenSurface,
					NoCollectReason: "deferred: add Identity to make CMT types collectable (see docs/backlog.md)"},
				{ID: SubtabMetaCustomLabels, Label: "Custom Labels", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.CustomLabels.FetchedAt() }, List: &customLabelsListSurface, Open: &customLabelsOpenSurface,
					NoCollectReason: "deferred: add Identity to make custom labels collectable (see docs/backlog.md)"},
				{ID: SubtabMetaCustomSettings, Label: "Custom Settings", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.CustomSettings.FetchedAt() }, List: &customSettingsListSurface, Open: &customSettingsOpenSurface,
					NoCollectReason: "deferred: add Identity to make custom settings collectable (see docs/backlog.md)"},
				{ID: SubtabMetaStaticResources, Label: "Static Resources", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.StaticResources.FetchedAt() }, List: &staticResourcesListSurface, Open: &staticResourcesOpenSurface,
					NoCollectReason: "deferred: add Identity to make static resources collectable (see docs/backlog.md)"},
				{ID: SubtabMetaNamedCredentials, Label: "Named Credentials", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.NamedCreds.FetchedAt() }, List: &namedCredsListSurface, Open: &namedCredsOpenSurface,
					NoCollectReason: "deferred: add Identity to make named credentials collectable (see docs/backlog.md)"},
				{ID: SubtabMetaRemoteSiteSettings, Label: "Remote Sites", PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.RemoteSites.FetchedAt() }, List: &remoteSitesListSurface, Open: &remoteSitesOpenSurface,
					NoCollectReason: "deferred: add Identity to make remote sites collectable (see docs/backlog.md)"},
			},
		},
		TabMetaTypeDetail: {
			TransientDrill:   true,
			Tab:              TabMetaTypeDetail,
			Stem:             TabMeta,
			Renderer:         Model.renderMetaTypeDetail,
			PrimaryFetchedAt: metaTypeDetailFetchedAt,
			Sidebar:          Model.sidebarMetaTypeDetail,
			List:             &metaTypeItemsListSurface,
			EscBack:          TabMeta,
			EnsureData:       (*Model).ensureMetaTypeDetailData,
			RefreshData:      Model.refreshMetaTypeDetailData,
		},
		TabPackages: {
			OverflowHint:    "installed packages",
			Tab:             TabPackages,
			Stem:            TabPackages,
			Open:            &packagesOpenSurface,
			Renderer:        Model.renderPackages,
			List:            &packagesListSurface,
			Sidebar:         Model.sidebarPackage,
			NoCollectReason: "an installed package is an install artifact, not a deployable component you'd collect into a project",
			BusyLabel:       busyPackages,
			ErrorLabel:      errPackages,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				return d.Packages.FetchedAt()
			},
			EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
				return d.Packages.Ensure(m.cache)
			},
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				return d.Packages.Refresh(m.cache)
			},
		},
		// /users — top-level User browser with two subtabs:
		//   Recent logins (HomeUserList, populated by d.Home.Ensure
		//     so /home and /users share the same recent-login slice)
		//   All users (AllUsers resource — broader pull with chip
		//     filters, capped at sf.AllUsersDefaultLimit)
		// Per-subtab List + Open declarations so cursor-aware actions
		// (open / yank / drill) read from whichever subtab is active.
		TabUsers: {
			OverflowHint:         "user list + recent logins",
			Tab:                  TabUsers,
			Stem:                 TabUsers,
			Renderer:             Model.renderUsers,
			Sidebar:              Model.sidebarUsers,
			GetSubtabIdx:         func(m Model) int { return m.usersSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setUsersSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, i int) bool { return true },
			Subtabs: []SubtabSpec{
				{
					ID:               SubtabUsersRecent,
					Label:            "Recent logins",
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.Home.FetchedAt() },
					List:             &recentUsersListSurface,
					Open:             &usersOpenSurface,
					EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
						return d.Home.Ensure(m.cache)
					},
					NoCollectReason: "users are org data, not deployable metadata; no KindUser collect kind (users do have their own YankTargets)",
				},
				{
					ID:               SubtabUsersAll,
					Label:            "All users",
					PrimaryFetchedAt: allUsersFetchedAt,
					Chips:            &usersChipSurface,
					List:             &allUsersListSurface,
					Open:             &allUsersOpenSurface,
					EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
						return ensureActiveUsersChip(m, d)
					},
					NoCollectReason: "users are org data, not deployable metadata; no KindUser collect kind (users do have their own YankTargets)",
				},
				{
					ID:               SubtabUsersActive,
					Label:            "Active",
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ActiveUsers.FetchedAt() },
					Chips:            &activeUsersChipSurface,
					List:             &activeUsersListSurface,
					Open:             &activeUsersOpenSurface,
					Sidebar:          Model.sidebarActiveUser,
					Activate:         (*Model).activateActiveUser,
					EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
						return d.ActiveUsers.Ensure(m.cache)
					},
					NoCollectReason: "an active session is live runtime state, not a deployable resource (rows open the user + yank IP)",
				},
			},
			EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
				return d.Home.Ensure(m.cache)
			},
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				cmds := []tea.Cmd{d.Home.Refresh(m.cache), refreshActiveUsersChip(m, d)}
				if m.currentSubtab() == SubtabUsersActive {
					cmds = append(cmds, d.ActiveUsers.Refresh(m.cache))
				}
				return tea.Batch(cmds...)
			},
		},
		TabUserDetail: {
			RecordRecentVisit: recentVisitUserDetail,
			Tab:               TabUserDetail,
			Stem:              TabUsers,
			Renderer:          Model.renderUserDetail,
			// The detail rows come from the ActiveUsers list fetch, so
			// its age IS the data's age.
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ActiveUsers.FetchedAt() },
			Sidebar:          Model.sidebarUserDetail,
			MoveCursor:       (*Model).moveUserDetailCursor,
			Activate:         (*Model).activateUserDetail,
			EscBack:          TabUsers,
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				if d == nil || d.UserCur == "" || len(m.orgs) == 0 {
					return nil
				}
				return userFetchCmd(targetArg(m.orgs[m.selected]), d.UserCur)
			},
		},
		TabUserSessions: {
			TransientDrill:   true,
			Tab:              TabUserSessions,
			Stem:             TabUsers,
			Renderer:         Model.renderUserSessions,
			PrimaryFetchedAt: userSessionsFetchedAt,
			Sidebar:          Model.sidebarUserSession,
			List:             &userSessionsListSurface,
			Open:             &userSessionsOpenSurface,
			EscBack:          TabUsers,
			EnsureData:       (*Model).ensureUserSessionsData,
			RefreshData:      Model.refreshUserSessionsData,
			NoCollectReason:  "a session is live runtime state, not a deployable resource (rows yank IP/location)",
		},
		TabCommunities: {
			OverflowHint:     "Experience sites (communities)",
			Tab:              TabCommunities,
			Stem:             TabCommunities,
			Renderer:         Model.renderCommunities,
			Sidebar:          Model.sidebarCommunity,
			List:             &communitiesListSurface,
			Open:             &communitiesOpenSurface,
			Activate:         (*Model).activateCommunities,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.Community.FetchedAt() },
			EnsureData: func(m *Model, d *orgData, o sf.Org) tea.Cmd {
				return d.Community.Ensure(m.cache)
			},
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				return d.Community.Refresh(m.cache)
			},
			NoCollectReason: "a community/Experience site isn't a bundleable metadata component here (it's an ExperienceBundle); rows open in Setup/Builder + yank name/url",
		},
		TabCommunityDetail: {
			TransientDrill:   true,
			Tab:              TabCommunityDetail,
			Stem:             TabCommunities,
			Renderer:         Model.renderCommunityDetail,
			PrimaryFetchedAt: communityDetailFetchedAt,
			List:             &communityPagesListSurface,
			Open:             &communityPagesOpenSurface,
			EscBack:          TabCommunities,
			EnsureData:       (*Model).ensureCommunityDetailData,
			RefreshData:      Model.refreshCommunityDetailData,
			NoCollectReason:  "community pages are best-effort FlexiPage rows, not individually bundleable here",
		},
		TabFlows: {
			OverflowHint: "flows + versions",
			Tab:          TabFlows,
			Stem:         TabFlows,
			Renderer:     Model.renderFlows,
			Chips:        &flowsChipSurface,
			Open:         &flowsOpenSurface,
			List:         &flowsListSurface,
			Identity:     identityFromFlowsList,
			Sidebar:      Model.sidebarFlow,
			Breadcrumb:   breadcrumbFromFlows,
			BusyLabel:    busyFlows,
			ErrorLabel:   errFlows,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				return d.Flows.FetchedAt()
			},
			EnsureData: (*Model).ensureFlowsListData,
			RefreshData: func(m Model, d *orgData) tea.Cmd {
				return d.Flows.Refresh(m.cache)
			},
		},
		TabFlowDetail: {
			Tab:               TabFlowDetail,
			Stem:              TabFlows,
			Open:              &flowDetailOpenSurface,
			Renderer:          Model.renderFlowDetail,
			EscBack:           TabFlows,
			MoveCursor:        (*Model).moveFlowDetailCursor,
			Activate:          (*Model).activateFlowVersionDetail,
			EnsureData:        (*Model).ensureFlowDetailData,
			RefreshData:       Model.refreshFlowDetailData,
			Sidebar:           Model.sidebarFlowVersion,
			Breadcrumb:        breadcrumbFromFlowDetail,
			BusyLabel:         busyFlowDetail,
			ErrorLabel:        errFlowDetail,
			RecordRecentVisit: recentVisitFlowDetail,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.FlowVersions[d.FlowCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabFlowVersionDetail: {
			Tab:            TabFlowVersionDetail,
			Stem:           TabFlows,
			Open:           &flowVersionDetailOpenSurface,
			Renderer:       Model.renderFlowVersionDetail,
			EscBack:        TabFlowDetail,
			TransientDrill: true, // per-version leaf; don't teleport the Flows number key back here
			MoveCursor:     (*Model).moveFlowVersionDetailCursor,
			EnsureData:     (*Model).ensureFlowVersionDetailData,
			RefreshData:    Model.refreshFlowVersionDetailData,
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.FlowVersionDetail[d.FlowVersionCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabReports: {
			OverflowHint:         "saved reports browser",
			Tab:                  TabReports,
			Stem:                 TabReports,
			Open:                 &reportsOpenSurface,
			Renderer:             Model.renderReports,
			Sidebar:              Model.sidebarReport,
			BusyLabel:            busyReports,
			ErrorLabel:           errReports,
			PrimaryFetchedAt:     func(m Model, d *orgData) time.Time { return d.Reports.FetchedAt() },
			GetSubtabIdx:         func(m Model) int { return m.reportsSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setReportsSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, i int) bool { return true },
			Subtabs: []SubtabSpec{
				{ID: SubtabReportsReports, Label: "Reports"},
				{ID: SubtabReportsDashboards, Label: "Dashboards", Chips: &dashboardsChipSurface, Open: &dashboardsOpenSurface, List: &dashboardsListSurface, Sidebar: Model.sidebarDashboard,
					NoCollectReason: "dashboard viewing/collect is not built yet (see Status & maturity); rows open in Lightning only"},
				{ID: SubtabReportsReportTypes, Label: "Report Types", Chips: &reportTypesChipSurface, Open: &reportTypesOpenSurface, List: &reportTypesListSurface, Sidebar: Model.sidebarReportType,
					NoCollectReason: "report types aren't a devproject collect kind; open-in-Setup only"},
			},
			Activate:    (*Model).activateReports,
			EnsureData:  (*Model).ensureReportsData,
			RefreshData: Model.refreshReportsData,
			SearchPtr:   Model.reportsSearchPtr,
			MoveCursor:  (*Model).moveReportsCursor,
			ResetCursor: (*Model).resetReportsCursor,
		},
		TabReportDetail: {
			TransientDrill:    true,
			Tab:               TabReportDetail,
			Stem:              TabReports,
			Open:              &reportDetailOpenSurface,
			Renderer:          Model.renderReportDetail,
			EscBack:           TabReports,
			MoveCursor:        (*Model).moveReportDetailCursor,
			ResetCursor:       (*Model).resetReportDetailCursor,
			Activate:          (*Model).activateReportDetail,
			EnsureData:        (*Model).ensureReportDetailData,
			RefreshData:       Model.refreshReportDetailData,
			Sidebar:           Model.sidebarReportRun,
			ListTable:         listTableReportDetail,
			MeasureCell:       measureCellReportDetail,
			Breadcrumb:        breadcrumbFromReportDetail,
			BusyLabel:         busyReportDetail,
			ErrorLabel:        errReportDetail,
			RecordRecentVisit: recentVisitReportDetail,
			PrimaryFetchedAt:  reportDetailFetchedAt,
		},
		// TabRecent registry entry intentionally removed — /recent
		// lives as /home → Recent now. The Tab constant remains so
		// drill-return logic that anchored on TabRecent compiles
		// (those callsites have been routed to TabHome). The home
		// subtab spec still references &recentListSurface +
		// &recentChipSurface so all the recent-stream logic stays
		// active; the only thing that disappeared is direct tab
		// addressability.
		TabDevProjects: {
			OrgIndependent:       true,
			OverflowHint:         "dev projects (your working sets)",
			Tab:                  TabDevProjects,
			Stem:                 TabDevProjects,
			Renderer:             Model.renderDevProjects,
			EnsureData:           (*Model).ensureDevProjectsData,
			Activate:             (*Model).activateDevProjects,
			SearchPtr:            Model.devProjectsSearchPtr,
			MoveCursor:           (*Model).moveDevProjectsCursor,
			ResetCursor:          (*Model).resetDevProjectsCursor,
			Sidebar:              Model.sidebarDevProject,
			GetSubtabIdx:         (Model).devProjectsSubtab,
			SetSubtabIdx:         (*Model).setDevProjectsSubtab,
			SubtabReloadOnSwitch: func(m Model, i int) bool { return false },
			Subtabs: []SubtabSpec{
				{ID: SubtabDevProjectsList, Label: "Projects"},
				{ID: SubtabDevProjectsBundles, Label: "Bundles",
					MoveCursor: moveAllBundlesCursor,
					Activate:   activateAllBundles},
			},
		},
		TabDevProjectDetail: {
			OrgIndependent:       true,
			Tab:                  TabDevProjectDetail,
			Stem:                 TabDevProjects,
			Renderer:             Model.renderDevProjectDetail,
			EscBack:              TabDevProjects,
			MoveCursor:           (*Model).moveDevProjectDetailCursor,
			EnsureData:           (*Model).ensureDevProjectDetailData,
			Activate:             (*Model).activateDevProjectDetail,
			Sidebar:              Model.sidebarDevProjectDetail,
			GetSubtabIdx:         (Model).devProjectDetailSubtab,
			SetSubtabIdx:         (*Model).setDevProjectDetailSubtab,
			SubtabReloadOnSwitch: func(m Model, i int) bool { return false },
			Subtabs: []SubtabSpec{
				{ID: SubtabDevProjectItems, Label: "Items",
					Identity: identityFromDevProjectItems},
				{ID: SubtabDevProjectBundles, Label: "Bundles",
					MoveCursor: moveBundlesCursor,
					Activate:   activateBundles},
			},
		},
		TabBundleDetail: {
			OrgIndependent:    true,
			Tab:               TabBundleDetail,
			Stem:              TabDevProjects,
			Renderer:          Model.renderProjectBundleDetail,
			EscBack:           TabDevProjectDetail,
			EnsureData:        ensureBundleDetailData,
			RecordRecentVisit: recentVisitBundleDetail,
			ListTable:         listTableBundleDetail,
			MoveCursor:        (*Model).moveBundleComponentCursor,
			ResetCursor:       (*Model).resetBundleComponentCursor,
			SearchPtr:         Model.bundleDetailSearchPtr,
			Sidebar:           Model.sidebarBundleDetail,
			Activate:          (*Model).activateBundleDetail,
		},
		TabTags: {
			OrgIndependent: true,
			OverflowHint:   "tag manager",
			Tab:            TabTags,
			Stem:           TabTags,
			Renderer:       Model.renderTags,
			MoveCursor:     (*Model).moveTagsCursor,
			Sidebar:        Model.sidebarTags,
			Activate:       (*Model).triggerTagDrill,
		},
		TabTagDetail: {
			OrgIndependent: true,
			Tab:            TabTagDetail,
			Stem:           TabTags,
			TransientDrill: true,
			Renderer:       Model.renderTagDetail,
			MoveCursor:     (*Model).moveTagDetailCursor,
			Sidebar:        Model.sidebarTagDetail,
			EscBack:        TabTags,
			Activate:       (*Model).activateTagDetailItem,
			Identity:       identityFromTagDetail,
		},
		TabProjects: {
			OrgIndependent: true,
			Tab:            TabProjects,
			Stem:           TabProjects,
			Renderer:       Model.renderProjects,
			EnsureData:     (*Model).ensureProjectsData,
			RefreshData:    Model.refreshProjectsData,
			SearchPtr:      Model.projectsSearchPtr,
			MoveCursor:     (*Model).moveProjectsCursor,
			ResetCursor:    (*Model).resetProjectsCursor,
			Sidebar:        Model.sidebarProject,
		},
		TabSetup: {
			OrgIndependent: true,
			OverflowHint:   "setup nav links",
			Tab:            TabSetup,
			Stem:           TabSetup,
			Open:           &setupOpenSurface,
			Renderer:       Model.renderSetup,
			SearchPtr:      Model.setupSearchPtr,
			MoveCursor:     (*Model).moveSetupCursor,
			ResetCursor:    (*Model).resetSetupCursor,
			Sidebar:        Model.sidebarSetup,
		},
		TabSystem: {
			OverflowHint: "logs / deploys / API usage",
			Tab:          TabSystem,
			Stem:         TabSystem,
			Renderer:     Model.renderSystem,
			EnsureData:   (*Model).ensureSystemData,
			RefreshData:  Model.refreshSystemData,
			GetSubtabIdx: func(m Model) int { return m.systemSubtab() },
			SetSubtabIdx: func(m *Model, i int) { m.setSystemSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, _ int) bool {
				// Each subtab pulls its own resource — switch must
				// kick the appropriate Ensure.
				return true
			},
			// Logs + Deploys carry the FULL former top-level
			// surfaces (2026-06-12 move): same list specs, chip
			// strip, Enter drill into the deploy detail, live
			// watch. /logs and /deploys as top-level tabs are gone.
			Subtabs: []SubtabSpec{
				{
					ID: SubtabSystemLogs, Label: "Logs",
					List: &apexLogsListSurface, Open: &apexLogsOpenSurface,
					Sidebar: Model.sidebarApexLog, BusyLabel: busySystemLogs, ErrorLabel: errSystemLogs,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ApexLogs.FetchedAt() },
					NoCollectReason:  "apex debug logs are transient observability records, not metadata (they have their own YankTargets)",
				},
				{
					ID: SubtabSystemDeploys, Label: "Deploys",
					List: &deploysListSurface, Chips: &deploysChipSurface, Open: &deploysOpenSurface,
					Sidebar: Model.sidebarDeploy, BusyLabel: busySystemDeploys, ErrorLabel: errSystemDeploys,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.Deploys.FetchedAt() },
					NoCollectReason:  "a deploy is a past event, not a resource; drill shows its components (they have their own YankTargets)",
				},
				{
					ID: SubtabSystemAudit, Label: "Audit Trail",
					List: &setupAuditListSurface, Open: &setupAuditOpenSurface,
					Sidebar:          Model.sidebarSetupAudit,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.SetupAudit.FetchedAt() },
					NoCollectReason:  "a Setup change is a past event, not a deployable resource (rows yank the change text + open the actor)",
				},
				{
					ID: SubtabSystemInterviews, Label: "Interviews",
					List: &flowInterviewsListSurface, Open: &flowInterviewsOpenSurface,
					Sidebar:          Model.sidebarFlowInterview,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.FlowInterviews.FetchedAt() },
					NoCollectReason:  "a flow interview is a runtime instance, not a deployable resource (rows yank the label/element + open the starter)",
				},
				{
					ID: SubtabSystemAsyncJobs, Label: "Async Jobs",
					List: &asyncJobsListSurface, Open: &asyncJobsOpenSurface,
					Activate:         (*Model).activateAsyncJob,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.AsyncJobs.FetchedAt() },
					NoCollectReason:  "an async apex job is a runtime execution, not a deployable resource (rows open the Apex Jobs Setup page)",
				},
				{
					ID: SubtabSystemScheduled, Label: "Scheduled Jobs",
					List: &scheduledJobsListSurface, Open: &scheduledJobsOpenSurface,
					Activate:         (*Model).activateScheduledJob,
					PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.ScheduledJobs.FetchedAt() },
					NoCollectReason:  "a scheduled job (CronTrigger) is a runtime schedule, not a deployable resource (rows open the Scheduled Jobs Setup page)",
				},
				{ID: SubtabSystemAPI, Label: "API", Sidebar: sidebarSystemAPI},
			},
		},
		TabPerms: {
			OverflowHint: "permsets / PSGs / profiles / queues / public groups",
			Tab:          TabPerms,
			Stem:         TabPerms,
			Renderer:     Model.renderPermsDashboard,
			EnsureData:   (*Model).ensurePermsDashboardData,
			RefreshData:  Model.refreshPermsDashboardData,
			GetSubtabIdx: func(m Model) int { return m.permsDashboardSubtab() },
			SetSubtabIdx: func(m *Model, i int) { m.setPermsDashboardSubtab(i) },
			Sidebar:      Model.sidebarPerms,
			// All five lists are pre-loaded by EnsureData above; no
			// need to re-fire onTabChanged on a switch.
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return false },
			Subtabs: []SubtabSpec{
				{ID: SubtabPermSets, Label: "Permission Sets", Chips: &permsetsChipSurface, Open: &permsetsOpenSurface, List: &permsetsListSurface, Identity: identityFromPermSetsList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.PermSets.FetchedAt() }},
				{ID: SubtabPSGs, Label: "Permission Set Groups", Chips: &psgsChipSurface, Open: &psgsOpenSurface, List: &psgsListSurface, Identity: identityFromPSGsList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.PSGs.FetchedAt() }},
				{ID: SubtabProfiles, Label: "Profiles", Chips: &profilesChipSurface, Open: &profilesOpenSurface, List: &profilesListSurface, Identity: identityFromProfilesList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.Profiles.FetchedAt() }},
				{ID: SubtabPermsQueues, Label: "Queues", Chips: &queuesChipSurface, Open: &queuesOpenSurface, List: &queuesListSurface, Identity: identityFromQueuesList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.Queues.FetchedAt() }},
				{ID: SubtabPermsPublicGroups, Label: "Public Groups", Chips: &publicGroupsChipSurface, Open: &publicGroupsOpenSurface, List: &publicGroupsListSurface, Identity: identityFromPublicGroupsList, PrimaryFetchedAt: func(m Model, d *orgData) time.Time { return d.PublicGroups.FetchedAt() }},
			},
		},
		// Renderer-only entries — these tabs share the registry's
		// renderer dispatch but their cursor/search/activate behavior
		// remains bespoke (multi-axis cursors, modal picker/list state,
		// per-record-detail drill stack). Wiring Renderer alone lets
		// renderMain collapse to a single resolveRenderer call without
		// forcing migration of the still-bespoke surfaces.
		TabSOQL: {
			OverflowHint:         "query editor + saved queries",
			Tab:                  TabSOQL,
			Stem:                 TabSOQL,
			Renderer:             Model.renderSOQL,
			MoveCursor:           (*Model).moveSOQLCursor,
			ResetCursor:          (*Model).resetSOQLCursor,
			Activate:             (*Model).activateSOQLResult,
			Sidebar:              Model.sidebarSOQL,
			ListTable:            listTableSOQL,
			MeasureCell:          measureCellSOQL,
			BusyLabel:            busySOQL,
			ErrorLabel:           errSOQL,
			SearchPtr:            Model.soqlSearchPtr,
			GetSubtabIdx:         func(m Model) int { return m.soqlSubtabIdx },
			SetSubtabIdx:         setSubtabWithOnEnter(TabSOQL, func(m *Model, i int) { m.soqlSubtabIdx = i }),
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return false },
			Subtabs: []SubtabSpec{
				// Open lives on the Editor subtab, not the tab: o/y
				// target the cursored RESULT row, which only exists
				// under the editor. On Saved/History a tab-level
				// surface would open a stale result record.
				{ID: SubtabSOQLEditor, Label: "Editor", Open: &soqlOpenSurface},
				{
					ID:       SubtabSOQLSaved,
					Label:    "Saved",
					Chips:    &savedQueriesChipSurface,
					List:     &soqlSavedListSurface,
					Identity: identityFromSOQLSaved,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.SOQLSavedLoaded },
						(*Model).reloadSOQLSaved),
				},
				{
					ID:    SubtabSOQLHistory,
					Label: "History",
					Chips: &soqlHistoryChipSurface,
					List:  &soqlHistoryListSurface,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.SOQLHistoryLoaded },
						(*Model).reloadSOQLHistory),
				},
			},
		},
		TabExec: {
			Tab:                  TabExec,
			Stem:                 TabExec,
			Renderer:             Model.renderExec,
			Activate:             (*Model).activateExecRun,
			GetSubtabIdx:         func(m Model) int { return m.execSubtabIdx },
			SetSubtabIdx:         setSubtabWithOnEnter(TabExec, func(m *Model, i int) { m.execSubtabIdx = i }),
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return false },
			Subtabs: []SubtabSpec{
				{ID: SubtabExecEditor, Label: "Editor"},
				{ID: SubtabExecOutput, Label: "Output"},
				{
					ID:    SubtabExecSaved,
					Label: "Saved",
					List:  &execSavedListSurface,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.ExecSavedLoaded },
						(*Model).reloadExecSaved),
				},
				{
					ID:    SubtabExecHistory,
					Label: "History",
					List:  &execHistoryListSurface,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.ExecHistoryLoaded },
						(*Model).reloadExecHistory),
				},
			},
		},
		TabCompare: {
			OrgIndependent:       true,
			Tab:                  TabCompare,
			Stem:                 TabCompare,
			Renderer:             Model.renderCompare,
			Activate:             (*Model).activateCompare,
			MoveCursor:           (*Model).moveCompareCursor,
			ResetCursor:          (*Model).resetCompareCursor,
			SearchPtr:            Model.compareSearchPtr,
			ListTable:            compareListTable,
			GetSubtabIdx:         func(m Model) int { return m.compareSubtabIdx },
			SetSubtabIdx:         setSubtabWithOnEnter(TabCompare, func(m *Model, i int) { m.compareSubtabIdx = i }),
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return false },
			Subtabs: []SubtabSpec{
				{
					ID:    SubtabCompareNew,
					Label: "New",
					// New is the setup form only. Running a comparison
					// auto-switches to Result (which owns the results views).
				},
				{
					ID:    SubtabCompareResult,
					Label: "Result",
					// Result has no static List: it shows the active run's
					// retrieving / inventory / drill-in diff. The inventory
					// list is routed via the tab-level MoveCursor / ListTable
					// hooks (gated to this subtab). Its Sidebar shows a live
					// diff preview of the selected inventory row.
					Sidebar: func(m Model, inner int) string {
						return m.renderComparePreviewSidebar(inner)
					},
				},
				{
					ID:    SubtabCompareSaved,
					Label: "Saved",
					List:  &compareSavedListSurface,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.SavedLoaded },
						(*Model).reloadCompareSaved),
				},
				{
					ID:    SubtabCompareHistory,
					Label: "History",
					List:  &compareHistoryListSurface,
					OnEnter: lazyLoadOnEnter(
						func(d *orgData) bool { return !d.HistoryLoaded },
						(*Model).reloadCompareHistory),
				},
			},
		},
		TabObjectDetail: {
			Tab:                  TabObjectDetail,
			Stem:                 TabObjects,
			Renderer:             Model.renderObjectDrill,
			EscBack:              TabObjects,
			GetSubtabIdx:         func(m Model) int { return m.objectSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setObjectSubtab(i) },
			Subtabs:              objectDrillSubtabSpecs(),
			SubtabPinned:         objectDrillPinnedCount,
			CycleChip:            (*Model).cycleObjectDetailChip,
			SubtabReloadOnSwitch: Model.objectDetailReloadOnSwitch,
			MoveCursor:           (*Model).moveObjectDetailCursor,
			SearchPtr:            Model.objectDetailSearchPtr,
			ResetCursor:          (*Model).resetObjectDetailCursor,
			EnsureData:           (*Model).ensureObjectDetailData,
			RefreshData:          Model.refreshObjectDetailData,
			Activate:             (*Model).activateObjectDetail,
			Identity:             identityFromObjectDetail,
			// Per-subtab sidebars still dispatch here while the renderer
			// and cursor hooks remain bespoke. Static subtabs above let
			// common navigation resolve through TabSpec.
			Sidebar:           sidebarObjectDetailDispatch,
			ListTable:         listTableObjectDetailDispatch,
			Breadcrumb:        breadcrumbFromObjectDetail,
			BusyLabel:         busyObjectDetail,
			ErrorLabel:        errObjectDetail,
			RecordRecentVisit: recentVisitObjectDetail,
			// Subtab variants override per-subtab below; this falls
			// through to "describe" for Details/Schema.
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.Describes[d.DescribeCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabFieldDetail: {
			TransientDrill:    true,
			Tab:               TabFieldDetail,
			Stem:              TabObjects,
			Renderer:          Model.renderFieldDetail,
			MoveCursor:        (*Model).moveFieldDetailCursor,
			Activate:          (*Model).activateFieldDetail,
			EnsureData:        (*Model).ensureFieldDetailData,
			Identity:          identityFromFieldDetail,
			Sidebar:           Model.sidebarFieldActions,
			Breadcrumb:        breadcrumbFromFieldDetail,
			RecordRecentVisit: recentVisitFieldDetail,
			Help:              func(m Model) infoModalState { return helpFieldDetail() },
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.Describes[d.DescribeCur]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabValidationDetail: {
			TransientDrill: true,
			Tab:            TabValidationDetail,
			Stem:           TabObjects,
			Renderer:       Model.renderValidationDetail,
			MoveCursor:     (*Model).moveValidationDetailCursor,
			Activate:       (*Model).activateValidationDetail,
			EnsureData:     (*Model).ensureValidationDetailData,
			RefreshData:    Model.refreshValidationDetailData,
			Sidebar:        Model.sidebarValidationActions,
			Breadcrumb:     breadcrumbFromValidationDetail,
			Help:           func(m Model) infoModalState { return helpValidationDetail() },
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.ValidationRules.Details[d.ValidationRules.DrillID]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabRecordTypeDetail: {
			TransientDrill: true,
			Tab:            TabRecordTypeDetail,
			Stem:           TabObjects,
			Renderer:       Model.renderRecordTypeDetail,
			MoveCursor:     (*Model).moveRecordTypeDetailCursor,
			Activate:       (*Model).activateRecordTypeDetail,
			EnsureData:     (*Model).ensureRecordTypeDetailData,
			RefreshData:    Model.refreshRecordTypeDetailData,
			Sidebar:        Model.sidebarRecordTypeActions,
			Breadcrumb:     breadcrumbFromRecordTypeDetail,
			Help:           func(m Model) infoModalState { return helpRecordTypeDetail() },
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.RecordTypes.Details[d.RecordTypes.DrillID]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabTriggerDetail: {
			TransientDrill:   true,
			Tab:              TabTriggerDetail,
			Stem:             TabObjects,
			Renderer:         Model.renderTriggerDetail,
			MoveCursor:       (*Model).moveTriggerDetailCursor,
			Activate:         (*Model).activateTriggerDetail,
			EnsureData:       (*Model).ensureTriggerDetailData,
			RefreshData:      Model.refreshTriggerDetailData,
			Sidebar:          Model.sidebarTriggerActions,
			Breadcrumb:       breadcrumbFromTriggerDetail,
			SidebarFocusable: true,
			Help:             func(m Model) infoModalState { return helpTriggerDetail() },
			PrimaryFetchedAt: func(m Model, d *orgData) time.Time {
				if r, ok := d.Triggers.Details[d.Triggers.DrillID]; ok {
					return r.FetchedAt()
				}
				return time.Time{}
			},
		},
		TabRecords: {
			Tab:              TabRecords,
			Stem:             TabRecords,
			Open:             &recordsTabOpenSurface,
			Renderer:         Model.renderRecords,
			CycleChip:        (*Model).cycleRecordsChip,
			MoveCursor:       (*Model).moveRecordsCursor,
			SearchPtr:        Model.recordsSearchPtr,
			ResetCursor:      (*Model).resetRecordsCursor,
			EnsureData:       (*Model).ensureRecordsData,
			RefreshData:      Model.refreshRecordsData,
			Activate:         (*Model).activateRecords,
			Sidebar:          Model.sidebarRecords,
			ListTable:        listTableRecords,
			Breadcrumb:       breadcrumbFromRecords,
			BusyLabel:        busyRecords,
			ErrorLabel:       errRecords,
			PrimaryFetchedAt: recordsFetchedAt,
		},
		TabRecordDetail: {
			TransientDrill:    true,
			Tab:               TabRecordDetail,
			Stem:              TabRecords,
			Renderer:          Model.renderRecordDetail,
			EnsureData:        (*Model).ensureRecordDetailData,
			RefreshData:       Model.refreshRecordDetailData,
			Identity:          identityFromRecordDetail,
			Sidebar:           Model.sidebarRecordDetail,
			Breadcrumb:        breadcrumbFromRecordDetail,
			BusyLabel:         busyRecordDetail,
			ErrorLabel:        errRecordDetail,
			RecordRecentVisit: recentVisitRecordDetail,
			MoveCursor:        (*Model).moveRecordFieldCursor,
			Activate:          (*Model).activateRecordDetail,
			PrimaryFetchedAt:  recordDetailFetchedAt,
		},
		TabPermParentDetail: {
			Tab:              TabPermParentDetail,
			Stem:             TabPerms,
			Open:             &permParentOpenSurface,
			Renderer:         Model.renderPermParentDetail,
			PrimaryFetchedAt: permParentFetchedAt,
			EscBack:          TabPerms,
			// Per-kind subtab list. permParentDetailSubtabs("permset")
			// vs ("psg") vs ("profile") return different shapes, so
			// the resolver reads the active kind off orgData each
			// render rather than freezing one list on the spec.
			SubtabsResolver:      permParentSubtabsResolver,
			GetSubtabIdx:         func(m Model) int { return m.permParentSubtab() },
			SetSubtabIdx:         func(m *Model, i int) { m.setPermParentSubtab(i) },
			SubtabReloadOnSwitch: func(m Model, _ int) bool { return true },
			CycleChip:            (*Model).cyclePermParentChip,
			MoveCursor:           (*Model).movePermParentCursor,
			SearchPtr:            Model.permParentSearchPtr,
			ResetCursor:          (*Model).resetPermParentCursor,
			EnsureData:           (*Model).ensurePermParentData,
			RefreshData:          Model.refreshPermParentData,
			Activate:             (*Model).activatePermParent,
			Sidebar:              Model.sidebarPermParent,
			RecordRecentVisit:    recentVisitPermParentDetail,
		},
		TabQueueDetail: {
			Tab:              TabQueueDetail,
			Stem:             TabPerms,
			Renderer:         Model.renderQueueDetail,
			PrimaryFetchedAt: groupMembersFetchedAt,
			Sidebar:          Model.sidebarQueueDetail,
			Open:             &queueDetailOpenSurface,
			EscBack:          TabPerms,
			EnsureData:       (*Model).ensureGroupMembersData,
			RefreshData:      Model.refreshGroupMembersData,
		},
		TabPublicGroupDetail: {
			Tab:              TabPublicGroupDetail,
			Stem:             TabPerms,
			Renderer:         Model.renderPublicGroupDetail,
			PrimaryFetchedAt: groupMembersFetchedAt,
			Sidebar:          Model.sidebarPublicGroupDetail,
			Open:             &queueDetailOpenSurface,
			EscBack:          TabPerms,
			EnsureData:       (*Model).ensureGroupMembersData,
			RefreshData:      Model.refreshGroupMembersData,
		},
	}
}

// activeSubtabSpec returns the SubtabSpec for the currently-selected
// subtab on the given TabSpec, or nil if the tab has no subtabs or
// the active index is out of range. Used by the unified dispatchers
// — they consult the subtab spec first, then fall back to the parent
// TabSpec's fields.
func (s *TabSpec) activeSubtabSpec(m Model) *SubtabSpec {
	if s == nil || len(s.Subtabs) == 0 {
		return nil
	}
	id := m.currentSubtab()
	for i := range s.Subtabs {
		if s.Subtabs[i].ID == id {
			return &s.Subtabs[i]
		}
	}
	// No match by ID — fall back to the GetSubtabIdx-resolved one.
	if s.GetSubtabIdx != nil {
		i := s.GetSubtabIdx(m)
		if i >= 0 && i < len(s.Subtabs) {
			return &s.Subtabs[i]
		}
	}
	return nil
}

var (
	tabRegistryOnce  sync.Once
	tabRegistryCache map[Tab]*TabSpec
)

func tabSpecs() map[Tab]*TabSpec {
	tabRegistryOnce.Do(func() {
		raw := tabRegistry()
		tabRegistryCache = make(map[Tab]*TabSpec, len(raw))
		for t, s := range raw {
			spec := s
			tabRegistryCache[t] = &spec
		}
	})
	return tabRegistryCache
}

// lookupTabSpec returns the registered spec for t, or nil if the tab
// is intentionally data-less/unknown. The registry is built once on
// first lookup so dispatch stays cheap in render and key hot paths.
func lookupTabSpec(t Tab) *TabSpec {
	return tabSpecs()[t]
}

// activeSpec returns the TabSpec + active SubtabSpec for the
// currently-rendered tab. Either may be nil — callers should
// nil-check.
//
// Walks the same subtab → tab chain every resolver wants ("does
// this subtab override the parent's hook?") so we only have to
// write that pattern once. Routes through the cached tabSpecs(),
// not the rebuilt tabRegistry(), so hot-path callers don't pay
// the map-rebuild cost on every keystroke.
func (m Model) activeSpec() (*TabSpec, *SubtabSpec) {
	spec := lookupTabSpec(m.tab())
	if spec == nil {
		return nil, nil
	}
	if len(spec.Subtabs) == 0 || spec.GetSubtabIdx == nil {
		return spec, nil
	}
	idx := spec.GetSubtabIdx(m)
	if idx < 0 || idx >= len(spec.Subtabs) {
		return spec, nil
	}
	return spec, &spec.Subtabs[idx]
}
