package ui

// Guided walkthrough. A task-driven tour: sf-deck shows the user a small
// non-blocking corner panel ("do X") that lets them navigate the UI
// while the instruction stays visible. The tour NEVER auto-advances —
// the user does the action (the panel shows a ✓ once the step's
// predicate is met, so they know they did it right) and presses w to
// move on when THEY choose. This lets them linger, try variations, or
// just read an info-only step. ctrl+w exits; esc is left to the app.
//
// Each step's Done predicate reads EXISTING model state (active tab,
// selected org, zen, sort/search/column state) rather than new
// instrumentation, and is used only to render the ✓ — not to advance.
// Info-only steps (safety, open/yank, view management, refresh, projects
// and tags) carry no predicate and simply teach; the user presses w to
// continue.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// tourStep is one instruction in the walkthrough. Done reports whether
// the user has completed the step given the current Model. Advance is
// prev-state-aware where needed: a step captures a baseline when it
// becomes active (see walkthroughState.baseline) so "the org changed"
// or "a filter was typed" can be detected as a delta.
type tourStep struct {
	Title       string
	Instruction string
	// Done reports whether the user has performed the step's action —
	// used ONLY to show a ✓ in the panel so they know they did it right.
	// The tour never auto-advances; the user presses w to move on. Nil
	// for pure-info steps (no action to confirm, no ✓).
	Done func(m Model, prev walkthroughBaseline) bool
	// Keys is a short list of contextual shortcut reminders shown in the
	// panel for this step — the basics a new user may not have
	// internalised yet (esc to go back, C to clear a search, etc.).
	// Each entry is {key, what}. Kept brief (2–4 per step).
	Keys []tourKey
}

// tourKey is one contextual shortcut reminder rendered in a step's panel.
type tourKey struct {
	Key  string
	What string
}

// walkthroughBaseline snapshots the model state a step needs to detect a
// delta. Captured when a step becomes active.
type walkthroughBaseline struct {
	tab                     Tab
	selectedOrg             int
	zen                     bool
	overflowSet             bool
	subtabIdx               int
	chipIdx                 int
	rightSidebarOpen        bool
	sorted                  bool
	colSig                  string // combined tag/project/flag column visibility
	homeRecentSalesforce    bool
	recordsSourceSalesforce bool
	soqlRunGen              uint64
	usersSubtab             int
	permsSubtab             int
	systemSubtab            int
}

// sortActive reports whether the active list surface has a sort applied.
func (m Model) sortActive() bool {
	st := (&m).activeListTableState()
	return st != nil && st.SortColumn != ""
}

// listSearchActive reports whether the active surface's inline search is
// engaged (the user pressed / and is typing, or has a filter applied).
func (m Model) listSearchActive() bool {
	s := (&m).currentSearch()
	return s != nil && (s.Active || s.Buffer() != "")
}

// recordsSourceIsSalesforce reports whether the active records surface is
// showing the org's Salesforce List Views (vs sf-deck's own views) — the
// completion signal for the "switch view source (L)" step.
func (m Model) recordsSourceIsSalesforce() bool {
	d, sobj := m.activeRecordsSObject()
	// The source-toggle step follows record detail. Preserve the parent
	// object's mode while the user is drilled into that record so merely
	// pressing Esc cannot look like a source change.
	if d == nil && m.tab() == TabRecordDetail && m.recordDetailReturnTab == TabObjectDetail {
		d = m.activeOrgData()
		if d != nil {
			sobj = d.DescribeCur
		}
	}
	return d != nil && sobj != "" && currentChipMode(d, sobj) == ChipModeSalesforce
}

// homeRecentSourceIsSalesforce reports whether Home → Recently Viewed is
// sourced from Salesforce's RecentlyViewed (vs sf-deck's local visit
// log) — the completion signal for the home source-switch step.
func (m Model) homeRecentSourceIsSalesforce() bool {
	d := m.activeOrgData()
	return d != nil && d.HomeRecentMode == ChipModeSalesforce
}

// columnSignal encodes the tag/project/flag column visibility so a step
// can detect the user toggling any of them. Settings-backed, so it's a
// stable read.
func (m Model) columnSignal() string {
	if m.settings == nil {
		return ""
	}
	tag, proj := "0", "0"
	if m.settings.TagColumnVisible() {
		tag = "1"
	}
	if m.settings.ProjectColumnVisible() {
		proj = "1"
	}
	return tag + proj + m.settings.FlagColumnDisplayMode()
}

// currentSubtabIdx returns the active tab's subtab index (0 when the tab
// has no subtabs or no resolver). Used by the subtab-navigation step to
// detect the user moving between subtabs.
func (m Model) currentSubtabIdx() int {
	spec, _ := m.activeSpec()
	if spec == nil || spec.GetSubtabIdx == nil {
		return 0
	}
	return spec.GetSubtabIdx(m)
}

// chipIdxSignal returns a combined signal that changes whenever the user
// moves ANY surface's chip selection for the active org. Summing the
// per-surface chip indices avoids a per-tab switch (per-tab behaviour
// belongs on TabSpec, not a switch here) while still detecting "the user
// applied a different chip" regardless of which list they're on — which
// is all the chip-navigation tour step needs.
func (m Model) chipIdxSignal() int {
	return m.objectsChipIdx() +
		m.recordsChipIdx() +
		m.flowsChipIdx() +
		m.apexChipIdx() +
		m.apexTriggersChipIdx() +
		m.lwcChipIdx() +
		m.auraChipIdx() +
		m.permsetsChipIdx() +
		m.psgsChipIdx() +
		m.profilesChipIdx()
}

// walkthroughState holds the live tour. Zero value = inactive.
type walkthroughState struct {
	active    bool
	steps     []tourStep
	cursor    int
	baseline  walkthroughBaseline
	satisfied bool
}

// captureBaseline snapshots the current model for the active step's
// delta detection.
func captureBaseline(m Model) walkthroughBaseline {
	sel := 0
	if m.selected >= 0 {
		sel = m.selected
	}
	return walkthroughBaseline{
		tab:                     m.tab(),
		selectedOrg:             sel,
		zen:                     m.zenMode,
		overflowSet:             m.overflowSet,
		subtabIdx:               m.currentSubtabIdx(),
		chipIdx:                 m.chipIdxSignal(),
		rightSidebarOpen:        m.sidebarOpen,
		sorted:                  m.sortActive(),
		colSig:                  m.columnSignal(),
		homeRecentSalesforce:    m.homeRecentSourceIsSalesforce(),
		recordsSourceSalesforce: m.recordsSourceIsSalesforce(),
		soqlRunGen:              m.soqlRunGen,
		usersSubtab:             m.usersSubtab(),
		permsSubtab:             m.permsDashboardSubtab(),
		systemSubtab:            m.systemSubtab(),
	}
}

// tourSteps is the mapped-out tour (see the design doc). Runs best
// against the demo org, where every step has data. Ordered
// simplest-first so momentum builds.
func tourSteps() []tourStep {
	return tourStepsForDemo(Demo)
}

func tourStepsForDemo(demo bool) []tourStep {
	soqlInstruction := "Open the 'soql' tab. Press " + firstPretty(Keys.SOQLEdit) + " to edit the seeded query, then Enter to run it. Results stay in the terminal; Enter on a result drills into that record. This tour only asks you to read — it never changes org data."
	soqlDone := func(m Model, prev walkthroughBaseline) bool {
		return m.tab() == TabSOQL &&
			m.soqlRunGen > prev.soqlRunGen &&
			!m.soqlRunning &&
			m.soqlErr == nil
	}
	if demo {
		// Demo mode deliberately blocks live sf CLI calls. Still introduce
		// the core workspace without asking for an action that cannot work.
		soqlInstruction = "Open the 'soql' tab and look around the Editor, Saved and History subtabs. Demo mode does not run live queries; with a connected org, press " + firstPretty(Keys.SOQLEdit) + " to edit the seeded read-only query and Enter to run it."
		soqlDone = func(m Model, _ walkthroughBaseline) bool {
			return m.tab() == TabSOQL
		}
	}

	return []tourStep{
		{
			// Teaches: org navigation + that MULTIPLE keys do the same
			// thing (j/k and arrows are interchangeable throughout).
			Title:       "Move between orgs",
			Instruction: "Press " + firstPretty(Keys.FocusOrgs) + " to focus the org panel, then move with " + firstPretty(Keys.MoveDown) + " / " + firstPretty(Keys.MoveUp) + " — or the ↑ / ↓ arrows. Most navigation in sf-deck accepts either.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.selected != prev.selectedOrg
			},
			Keys: []tourKey{
				{firstPretty(Keys.FocusOrgs), "focus the org panel"},
				{firstPretty(Keys.MoveDown) + " / " + firstPretty(Keys.MoveUp), "move down / up (or ↑ ↓)"},
				{firstPretty(Keys.Help), "full keymap, any time"},
			},
		},
		{
			// Safety is core context, but changing it is deliberately not
			// a tour task. The walkthrough must be harmless on a real org.
			Title:       "Know your safety level",
			Instruction: "The header shows the active org's safety level: READ blocks every write; REC allows record changes; META adds metadata changes and deploys; FULL adds anonymous Apex and destructive operations. Salesforce permissions still apply too. Leave the level as-is for this tour.",
			Keys: []tourKey{
				{firstPretty(Keys.Help), "open Help on home for the safety guide"},
				{firstPretty(Keys.CommandPalette), "search “Edit safety level”"},
			},
		},
		{
			// Teaches: the numbered tab bar + the 0/9 overflow mechanic,
			// plus the small set of controls that also accept the mouse.
			Title:       "Switch tabs — and reach the rest",
			Instruction: "Tabs across the top are numbered — press 1–8 to jump. Press 0 for the 'More…' picker; whatever you pick lands in slot 9, so 9 jumps back to it. You can also click tabs, subtabs, views and rail buttons; the wheel scrolls lists.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				// Either a plain tab change OR activating an overflow tab.
				return m.tab() != prev.tab || (m.overflowSet && !prev.overflowSet)
			},
			Keys: []tourKey{
				{"1–8", "jump to a tab"},
				{"0", "more… (overflow picker)"},
				{"9", "the tab you picked from overflow"},
				{"click", "tabs · subtabs · views · rail buttons"},
			},
		},
		// --- Home: the org-at-a-glance subtabs ---
		{
			// Teaches: the home subtabs are worth a browse. Info step —
			// they're all one subtab away and there's nothing to "do."
			Title:       "Your org at a glance (home)",
			Instruction: "Go to the 'home' tab. Its subtabs are a quick health panel: Recently Viewed, Notifications, Limits (governor/API usage), and Licenses. Step through them with tab / shift+tab and have a look.",
			Keys: []tourKey{
				{"tab / shift+tab", "step through home subtabs"},
				{"shift+1…9", "jump to a subtab"},
			},
		},
		{
			// Teaches: source switching again, this time on Home →
			// Recently Viewed (sf-deck visit log ↔ Salesforce
			// RecentlyViewed). Action step — the mode flip is detectable.
			Title:       "Two sources for 'Recently Viewed'",
			Instruction: "On home's 'Recently Viewed' subtab, press " + firstPretty(Keys.LensModeToggle) + " to switch source: sf-deck's own visit log ↔ Salesforce's RecentlyViewed. Same " + firstPretty(Keys.LensModeToggle) + " that switches source on records lists.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.homeRecentSourceIsSalesforce() != prev.homeRecentSalesforce
			},
			Keys: []tourKey{
				{firstPretty(Keys.LensModeToggle), "toggle visit-log ↔ Salesforce recent"},
			},
		},
		{
			// SOQL is a default workspace and one of sf-deck's core
			// workflows. Connected orgs run the seeded read-only query;
			// demo mode introduces the workspace without a live CLI call.
			Title:       "Run a read-only SOQL query",
			Instruction: soqlInstruction,
			Done:        soqlDone,
			Keys: []tourKey{
				{firstPretty(Keys.SOQLEdit), "edit the query"},
				{"enter", "run it / drill into a result"},
				{"ctrl+c", "cancel a running query"},
			},
		},
		// --- Flows: explore + views (everyone has flows) ---
		{
			// Teaches: drill DOWN on a surface every org has. Flows also
			// surfaces the version/created-by detail once drilled.
			Title:       "Explore your flows",
			Instruction: "Open the 'flows' tab, highlight a flow with " + firstPretty(Keys.MoveDown) + " / " + firstPretty(Keys.MoveUp) + " (or ↑ / ↓), and press Enter to drill into its detail.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabFlowDetail
			},
			Keys: []tourKey{
				{"enter", "drill into the highlighted row"},
				{"esc", "go back a level"},
				{firstPretty(Keys.GoTop) + " / " + firstPretty(Keys.GoBottom), "jump to top / bottom"},
			},
		},
		{
			// Teaches: open + yank — everyday actions, taught EARLY (right
			// after the first drill) since they're used constantly.
			Title:       "Open & yank",
			Instruction: "On any row: " + firstPretty(Keys.OpenDefault) + " opens it in Salesforce; " + firstPretty(Keys.YankDefault) + " yanks its URL to your clipboard. " + firstPretty(Keys.OpenMenu) + " and " + firstPretty(Keys.YankMenu) + " give menus with more targets (setup page, API, related links).",
			Keys: []tourKey{
				{firstPretty(Keys.OpenDefault), "open in Salesforce (Lightning)"},
				{firstPretty(Keys.YankDefault), "yank the URL"},
				{firstPretty(Keys.OpenMenu), "open menu (more targets)"},
				{firstPretty(Keys.YankMenu), "yank menu (more targets)"},
			},
		},
		{
			// Teaches: back UP a level — esc is the universal "go back."
			Title:       "Back up a level",
			Instruction: "Press Esc to step back up to the flows list. Esc always goes back one level.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return !m.canEscBack()
			},
		},
		{
			// Teaches: VIEWS (the UI calls the filter strip "views").
			// Done here on flows, where they already are.
			Title:       "Filter with views",
			Instruction: "Still on flows: the row under the tabs is 'views' — quick filters. Press " + firstPretty(Keys.NextView) + " / " + firstPretty(Keys.PrevView) + " (or shift+→ / shift+←) to switch view and re-filter the list live.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.chipIdxSignal() != prev.chipIdx
			},
			Keys: []tourKey{
				{firstPretty(Keys.NextView) + " / " + firstPretty(Keys.PrevView), "next / previous view"},
				{"shift+→ ←", "same as ] / ["},
				{firstPretty(Keys.OpenChipOverflow), "all views (overflow)"},
				{firstPretty(Keys.OpenLensManager), "view manager"},
			},
		},
		{
			// Teaches: the view overflow picker (M) and the view manager
			// (V), including where Salesforce list views can be imported.
			// Explain-only: the tour never asks the user to persist a view.
			Title:       "Manage and pin views",
			Instruction: "Only a few views fit under the tabs. Press " + firstPretty(Keys.OpenChipOverflow) + " for the overflow picker, or " + firstPretty(Keys.OpenLensManager) + " for the view manager. There you can add or edit your own views and import a Salesforce List View to pin its query as a reusable sf-deck view. You don't need to save anything now.",
			Keys: []tourKey{
				{firstPretty(Keys.OpenChipOverflow), "all views for this surface"},
				{firstPretty(Keys.OpenLensManager), "manage or import views"},
			},
		},
		// --- Working a list: sort, search, columns ---
		{
			// Teaches: sort by the cursored column.
			Title:       "Sort a list",
			Instruction: "Move the column cursor with " + firstPretty(Keys.ColScrollL) + " / " + firstPretty(Keys.ColScrollR) + ", then press " + firstPretty(Keys.ColSort) + " to sort by that column (" + firstPretty(Keys.ColSort) + " again reverses; " + firstPretty(Keys.ColSortClear) + " clears).",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.sortActive() && !prev.sorted
			},
			Keys: []tourKey{
				{firstPretty(Keys.ColScrollL) + " / " + firstPretty(Keys.ColScrollR), "move the column cursor"},
				{firstPretty(Keys.ColSort), "sort by it (press again to reverse)"},
				{firstPretty(Keys.ColSortClear), "clear the sort"},
			},
		},
		{
			// Teaches: inline search plus both clearing paths. Completion is
			// latched, so the ✓ remains after the user clears the filter.
			Title:       "Search and clear a list filter",
			Instruction: "Press " + firstPretty(Keys.SearchStart) + " and type to filter this list live. Enter keeps the filter while you navigate. Esc cancels while typing or clears an applied filter; " + firstPretty(Keys.SearchClear) + " also clears an applied filter from any depth.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				// Reaching the search input is the completion signal.
				return m.listSearchActive()
			},
			Keys: []tourKey{
				{firstPretty(Keys.SearchStart), "start an inline search"},
				{"enter", "keep the filter applied"},
				{"esc", "cancel while typing / clear when applied"},
				{firstPretty(Keys.SearchClear), "clear an applied filter, at any depth"},
			},
		},
		{
			// Teaches: the three metadata columns you can show/hide. All on
			// one step since they're the same idea.
			Title:       "Tags, projects & flags columns",
			Instruction: "Rows can show extra columns: " + firstPretty(Keys.TagColumn) + " toggles the Tags column, " + firstPretty(Keys.ProjectColumn) + " the Projects column, " + firstPretty(Keys.FlagColumn) + " cycles the Flags column (full / letter / hidden). Try one.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.columnSignal() != prev.colSig
			},
			Keys: []tourKey{
				{firstPretty(Keys.TagColumn), "toggle Tags column"},
				{firstPretty(Keys.ProjectColumn), "toggle Projects column"},
				{firstPretty(Keys.FlagColumn), "cycle Flags column"},
			},
		},
		// --- Objects: subtabs + views-in-a-subtab (the payoff) ---
		{
			// Teaches: subtabs — a distinct nav axis. Objects has no
			// subtabs at the LIST level; they appear once you drill into
			// an object (Details / Fields / Records / …). So: open
			// objects, drill an object, then move between its subtabs.
			Title:       "Objects have subtabs",
			Instruction: "Open the 'objects' tab and drill into an object with Enter. Now you'll see a second strip of subtabs — jump with shift+1 / shift+2 / …, or step through them with tab / shift+tab.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				// On an object drill AND moved off the first subtab.
				return m.tab() == TabObjectDetail && m.currentSubtabIdx() != prev.subtabIdx
			},
			Keys: []tourKey{
				{"enter", "drill into the object"},
				{"tab / shift+tab", "step through subtabs"},
				{"shift+1…9", "jump to a subtab"},
				{"esc", "back to the objects list"},
			},
		},
		{
			// Teaches: views appear INSIDE subtabs too. The Records subtab
			// of an object uses the records-domain views — reinforcing the
			// concept in a new context.
			Title:       "Views inside a subtab",
			Instruction: "Switch to the object's 'records' subtab. It has its own views (just like the flows list) — press " + firstPretty(Keys.NextView) + " / " + firstPretty(Keys.PrevView) + " (or shift+→ / shift+←) to filter the records.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabObjectDetail &&
					m.currentSubtab() == SubtabRecords &&
					m.chipIdxSignal() != prev.chipIdx
			},
			Keys: []tourKey{
				{firstPretty(Keys.NextView) + " / " + firstPretty(Keys.PrevView), "next / previous view"},
				{firstPretty(Keys.LensModeToggle), "switch view source (next step)"},
			},
		},
		{
			// Teaches: a record list is not a dead end. The detail view is
			// deliberately read-only for the tour even when the org's safety
			// level would permit edits.
			Title:       "Open a record's detail",
			Instruction: "With the object's 'records' subtab open, highlight a row and press Enter. The record detail shows every field and the contextual actions available for that record. Press Esc when you're ready to return to the list.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabRecordDetail
			},
			Keys: []tourKey{
				{"enter", "open the highlighted record"},
				{"esc", "return to the records list"},
			},
		},
		{
			// Teaches: L toggles the records view source between sf-deck's
			// own views and the org's real Salesforce List Views. Action
			// step — the mode flip is detectable.
			Title:       "Switch view source (L)",
			Instruction: "Press Esc to return to the records list if needed, then press " + firstPretty(Keys.LensModeToggle) + " to switch the view source: sf-deck's own views ↔ the org's actual Salesforce List Views. The view strip changes with the source.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.recordsSourceIsSalesforce() != prev.recordsSourceSalesforce
			},
			Keys: []tourKey{
				{firstPretty(Keys.LensModeToggle), "toggle sf-deck ↔ Salesforce list views"},
			},
		},
		// --- Reading code + reports ---
		{
			// Teaches: drilling into code surfaces shows the actual source.
			Title:       "Read Apex & component code",
			Instruction: "Open 'apex' and Enter into a class to read its source right here — no browser. 'components' does the same for LWC and Aura bundles (drill in to see each file). Syntax is highlighted; scroll with " + firstPretty(Keys.MoveDown) + " / " + firstPretty(Keys.MoveUp) + ".",
			Keys: []tourKey{
				{"enter", "open the class / bundle source"},
				{firstPretty(Keys.MoveDown) + " / " + firstPretty(Keys.MoveUp), "scroll the code"},
				{"esc", "back to the list"},
			},
		},
		{
			// Teaches: reports surface. Info step.
			Title:       "Reports",
			Instruction: "The 'reports' tab lists the org's reports; drill into one to preview a cached run without leaving the terminal.",
			Keys: []tourKey{
				{"enter", "preview the report"},
			},
		},
		{
			// Core admin discovery without requiring a User row or asking
			// the user to run any of the detail view's mutating actions.
			Title:       "Browse users and active sessions",
			Instruction: "Open the 'users' tab, then use tab / shift+tab to browse Recent logins, All users and Active sessions. Enter opens a user when rows are available; this tour does not ask you to run any user-management action.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabUsers && m.usersSubtab() != prev.usersSubtab
			},
			Keys: []tourKey{
				{"tab / shift+tab", "Recent · All · Active"},
				{"enter", "inspect a user or active session"},
			},
		},
		{
			// The permissions workspace is a default tab, but editing is a
			// higher-risk workflow and stays outside the core tour.
			Title:       "Browse permissions",
			Instruction: "Open the 'perms' tab and step through Permission Sets, Permission Set Groups, Profiles, Queues and Public Groups with tab / shift+tab. Enter drills into a row; permission editing is intentionally outside this tour.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabPerms && m.permsDashboardSubtab() != prev.permsSubtab
			},
			Keys: []tourKey{
				{"tab / shift+tab", "move between permission surfaces"},
				{"enter", "inspect the highlighted item"},
			},
		},
		{
			// System is the operational home for understanding what ran or
			// failed. Browsing subtabs is safe even when their lists are
			// empty or the org denies access to a resource.
			Title:       "See what happened in System",
			Instruction: "Open the 'system' tab and step through Logs, Deploys, Audit Trail, Flow Interviews, Async Jobs, Scheduled Jobs and API usage. These read-only surfaces are where you investigate activity and failures.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.tab() == TabSystem && m.systemSubtab() != prev.systemSubtab
			},
			Keys: []tourKey{
				{"tab / shift+tab", "move through system surfaces"},
				{"enter", "drill when a row has more detail"},
			},
		},
		// --- Workspace: sidebar, global search, refresh ---
		{
			// Teaches: the right sidebar — toggle + reposition.
			Title:       "Show, hide & move the sidebar",
			Instruction: firstPretty(Keys.ToggleSidebar) + " toggles the right sidebar (context for the selected row). " + firstPretty(Keys.ToggleSidebarStacked) + " moves it: beside the main pane vs stacked below. Try toggling it.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.sidebarOpen != prev.rightSidebarOpen
			},
		},
		{
			// Teaches: global search — the ctrl+f cross-org finder.
			Title:       "Global search",
			Instruction: "Press " + firstPretty(Keys.GlobalSearch) + " for global search — find any record or metadata across the org from one box. (" + firstPretty(Keys.SearchToggleMode) + " inside it toggles metadata vs records.)",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.globalSearch != nil
			},
		},
		{
			// Teaches: refresh semantics. Explain-only — pressing r would
			// just reload, nothing to verify.
			Title:       "Refreshing data",
			Instruction: "sf-deck caches org data so it's instant. Press " + firstPretty(Keys.Refresh) + " to refresh the current view, or " + firstPretty(Keys.GlobalRefresh) + " to refresh everything loaded for the active org — do this when you've changed something in Salesforce and want the latest.",
		},
		// --- Explain-only: dev projects + tags ---
		{
			// Both are organisational concepts. Creating projects, building
			// bundles and deploying remain power-user workflows.
			Title:       "Organise work with projects and tags",
			Instruction: "Tags group items your own way across orgs; the /tags tab manages them. Dev Projects are working sets: load one, then " + firstPretty(Keys.CollectItem) + " collects the cursored item (" + firstPretty(Keys.CollectItemPick) + " lets you pick a project). Projects can later become deployable sfdx bundles, but building and deploying is outside this core tour.",
			Keys: []tourKey{
				{firstPretty(Keys.Tag), "tag the highlighted item"},
				{firstPretty(Keys.CollectItem), "collect into the loaded project"},
				{firstPretty(Keys.CollectItemPick), "choose a project"},
			},
		},
		{
			// Teaches: zen — declutter. Also a "multiple ways" note (z or
			// esc restores).
			Title:       "Zen mode",
			Instruction: "Press " + firstPretty(Keys.ZenMode) + " to drop the chrome and focus on one pane. Press " + firstPretty(Keys.ZenMode) + " (or Esc) again to restore.",
			Done: func(m Model, prev walkthroughBaseline) bool {
				return m.zenMode != prev.zen
			},
		},
		{
			Title:       "You're all set",
			Instruction: "Press " + firstPretty(Keys.Help) + " on any screen for the full keymap, or " + firstPretty(Keys.CommandPalette) + " for the command palette to jump anywhere. That's the tour — explore on your own!",
			Done: func(m Model, _ walkthroughBaseline) bool {
				return m.keymapModalOpen()
			},
		},
	}
}

// startWalkthrough activates the tour from the first step and captures
// its baseline. Called from the welcome modal's walkthrough action and
// from the re-entry action.
func (m *Model) startWalkthrough() {
	m.walkthrough = walkthroughState{
		active: true,
		steps:  tourSteps(),
		cursor: 0,
	}
	m.walkthrough.baseline = captureBaseline(*m)
}

// advanceWalkthrough moves to the next step (or ends the tour after the
// last) and re-captures the baseline for the new step. Every advance is
// manual: w moves on whether or not the current predicate was satisfied.
func (m *Model) advanceWalkthrough() {
	if !m.walkthrough.active {
		return
	}
	m.walkthrough.cursor++
	if m.walkthrough.cursor >= len(m.walkthrough.steps) {
		m.walkthrough = walkthroughState{} // done
		m.flash("Walkthrough complete.")
		return
	}
	m.walkthrough.baseline = captureBaseline(*m)
	m.walkthrough.satisfied = false
}

// exitWalkthrough ends the tour immediately.
func (m *Model) exitWalkthrough() {
	if !m.walkthrough.active {
		return
	}
	m.walkthrough = walkthroughState{}
	m.flash("Walkthrough exited.")
}

// observeWalkthrough latches a completed action step. Predicates often
// describe transient state: global search and help are modals, search can
// be cleared, and zen can be toggled back off. Remembering completion means
// the ✓ remains visible after the user returns to the main surface.
func (m *Model) observeWalkthrough() {
	if !m.walkthrough.active || m.walkthrough.satisfied ||
		m.walkthrough.cursor >= len(m.walkthrough.steps) {
		return
	}
	step := m.walkthrough.steps[m.walkthrough.cursor]
	if step.Done != nil && step.Done(*m, m.walkthrough.baseline) {
		m.walkthrough.satisfied = true
	}
}

// stepSatisfied reports whether the active step's predicate has ever been
// met, or is currently met before the next Update has had a chance to latch
// it. This is used only to render a ✓; the user still presses w to advance.
// A step with no predicate (pure info) never shows a ✓.
func (m Model) stepSatisfied() bool {
	if !m.walkthrough.active || m.walkthrough.cursor >= len(m.walkthrough.steps) {
		return false
	}
	if m.walkthrough.satisfied {
		return true
	}
	step := m.walkthrough.steps[m.walkthrough.cursor]
	return step.Done != nil && step.Done(m, m.walkthrough.baseline)
}

// renderWalkthrough returns the corner-panel string for the active step,
// or "" when the tour is inactive. Composited as a non-dimming top-layer
// so the UI underneath stays interactive (see render.go).
func (m Model) renderWalkthrough() string {
	if !m.walkthrough.active || m.walkthrough.cursor >= len(m.walkthrough.steps) {
		return ""
	}
	step := m.walkthrough.steps[m.walkthrough.cursor]
	n := m.walkthrough.cursor + 1
	total := len(m.walkthrough.steps)

	// Wider panel so multi-sentence steps + the shortcut block wrap
	// cleanly instead of orphaning short words. Grows toward half the
	// screen on wide terminals; clamped so it always fits.
	width := 68
	if half := m.width / 2; width < half {
		width = half
	}
	if width > 88 {
		width = 88 // don't let it dominate on very wide terminals
	}
	if width > m.width-4 {
		width = m.width - 4
	}
	if width < 24 {
		width = 24
	}

	titleStyle := lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	dimStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	keyStyle := lipgloss.NewStyle().Foreground(theme.Cyan)
	okStyle := lipgloss.NewStyle().Foreground(theme.Green).Bold(true)

	// Header: title, step counter, and a ✓ once the step's action is
	// done (so the user knows they did it right — but they still press w
	// to move on; nothing auto-advances).
	header := titleStyle.Render(step.Title) + "  " +
		dimStyle.Render("("+itoa(n)+"/"+itoa(total)+")")
	if m.stepSatisfied() {
		header += "  " + okStyle.Render("✓ done")
	}
	instr := bodyStyle.Render(wrap(step.Instruction, width-4))

	inner := header + "\n\n" + instr

	// Contextual shortcuts: the basics a new user may not have
	// internalised yet, per step. Rendered as an aligned "key  what"
	// block under a faint divider.
	if len(step.Keys) > 0 {
		var b strings.Builder
		b.WriteString("\n" + dimStyle.Render(strings.Repeat("─", width-2)) + "\n")
		for _, k := range step.Keys {
			b.WriteString(keyStyle.Render(padTourKey(k.Key)) + " " +
				dimStyle.Render(k.What) + "\n")
		}
		inner += "\n" + strings.TrimRight(b.String(), "\n")
	}

	// Footer: w advances (always), ctrl+w exits. esc is intentionally not
	// listed — it belongs to the app (go back a level), and steps that
	// need it say so in their own text.
	footer := dimStyle.Render("w ") + bodyStyle.Render("next") +
		dimStyle.Render("  ·  ctrl+w exit tour")
	inner += "\n\n" + footer

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderHi).
		Background(theme.Panel).
		Padding(0, 1).
		Width(width).
		Render(inner)
	return box
}

// padTourKey right-pads a key label to a fixed column so the "what"
// text aligns down the shortcut block.
func padTourKey(k string) string {
	const col = 9
	if len(k) >= col {
		return k
	}
	return k + strings.Repeat(" ", col-len(k))
}

// keymapModalOpen reports whether the keymap (?) overlay is showing — the
// completion signal for the final tour step.
func (m Model) keymapModalOpen() bool {
	return m.keybindingsModal != nil
}

// walkthroughActive reports whether the tour is running (used by render
// + key handling).
func (m Model) walkthroughActive() bool { return m.walkthrough.active }
