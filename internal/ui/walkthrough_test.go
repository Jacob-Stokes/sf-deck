package ui

import (
	"errors"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// tourModel returns a Model with the walkthrough active on a given step list.
func tourModel(steps []tourStep) Model {
	m := Model{}
	m.walkthrough = walkthroughState{active: true, steps: steps, cursor: 0}
	m.walkthrough.baseline = captureBaseline(m)
	return m
}

// The tour is manual-advance: w moves to the next step regardless of
// whether the predicate is satisfied. advanceWalkthrough is the engine
// behind that.
func TestWalkthroughAdvanceIsManual(t *testing.T) {
	steps := []tourStep{
		{Title: "one", Done: func(m Model, _ walkthroughBaseline) bool { return false }},
		{Title: "two", Done: func(m Model, _ walkthroughBaseline) bool { return false }},
	}
	m := tourModel(steps)
	if m.walkthrough.cursor != 0 {
		t.Fatalf("start cursor = %d", m.walkthrough.cursor)
	}
	// Advancing works even though step one's predicate is false — the user
	// is in control, not the predicate.
	m.advanceWalkthrough()
	if m.walkthrough.cursor != 1 {
		t.Fatalf("after advance cursor = %d, want 1", m.walkthrough.cursor)
	}
}

func TestWalkthroughEndsAfterLastStep(t *testing.T) {
	m := tourModel([]tourStep{{Title: "only"}})
	m.advanceWalkthrough() // past the single step -> tour ends
	if m.walkthrough.active {
		t.Fatal("walkthrough should be inactive after the last step")
	}
}

func TestWalkthroughExit(t *testing.T) {
	m := tourModel([]tourStep{{Title: "a"}, {Title: "b"}})
	m.exitWalkthrough()
	if m.walkthrough.active {
		t.Fatal("exit should deactivate the tour")
	}
}

// stepSatisfied drives the ✓ indicator: true when the current step's
// predicate holds (or has been latched); never true for a predicate-less
// info step.
func TestStepSatisfied(t *testing.T) {
	done := false
	m := tourModel([]tourStep{
		{Title: "action", Done: func(mm Model, _ walkthroughBaseline) bool { return done }},
		{Title: "info"}, // no predicate
	})
	if m.stepSatisfied() {
		t.Error("action step should not be satisfied yet")
	}
	done = true
	if !m.stepSatisfied() {
		t.Error("action step should be satisfied once predicate holds")
	}
	m.advanceWalkthrough() // move to the info step
	if m.stepSatisfied() {
		t.Error("info step (no predicate) is never satisfied / shows no ✓")
	}
}

func TestWalkthroughCompletionLatchesUntilAdvance(t *testing.T) {
	done := false
	m := tourModel([]tourStep{
		{Title: "action", Done: func(Model, walkthroughBaseline) bool { return done }},
		{Title: "next", Done: func(Model, walkthroughBaseline) bool { return false }},
	})

	done = true
	m.observeWalkthrough()
	done = false
	if !m.stepSatisfied() {
		t.Fatal("completion should remain latched after the predicate becomes false")
	}

	m.advanceWalkthrough()
	if m.walkthrough.satisfied {
		t.Fatal("advancing should clear the previous step's completion latch")
	}
	if m.stepSatisfied() {
		t.Fatal("the next unsatisfied step should not inherit the previous ✓")
	}
}

func TestGlobalSearchCompletionSurvivesModalClose(t *testing.T) {
	step := stepByTitle(t, "Global search")
	m := tourModel([]tourStep{step})
	m.globalSearch = &globalSearchState{}
	m.observeWalkthrough()
	m.globalSearch = nil

	if !m.stepSatisfied() {
		t.Fatal("global-search completion should remain after its modal closes")
	}
}

func TestTourStepsWellFormed(t *testing.T) {
	steps := tourSteps()
	if got, want := len(steps), 29; got != want {
		t.Fatalf("tour has %d steps, want %d", got, want)
	}

	wantTitles := []string{
		"Move between orgs",
		"Know your safety level",
		"Switch tabs — and reach the rest",
		"Your org at a glance (home)",
		"Two sources for 'Recently Viewed'",
		"Run a read-only SOQL query",
		"Explore your flows",
		"Open & yank",
		"Back up a level",
		"Filter with views",
		"Manage and pin views",
		"Sort a list",
		"Search and clear a list filter",
		"Tags, projects & flags columns",
		"Objects have subtabs",
		"Views inside a subtab",
		"Open a record's detail",
		"Switch view source (L)",
		"Read Apex & component code",
		"Reports",
		"Browse users and active sessions",
		"Browse permissions",
		"See what happened in System",
		"Show, hide & move the sidebar",
		"Global search",
		"Refreshing data",
		"Organise work with projects and tags",
		"Zen mode",
		"You're all set",
	}
	seen := make(map[string]bool, len(steps))
	for i, s := range steps {
		if s.Title == "" || s.Instruction == "" {
			t.Errorf("step %d missing title/instruction", i)
		}
		if s.Title != wantTitles[i] {
			t.Errorf("step %d title = %q, want %q", i, s.Title, wantTitles[i])
		}
		if seen[s.Title] {
			t.Errorf("duplicate step title %q", s.Title)
		}
		seen[s.Title] = true
	}
}

// stepByTitle finds a tour step by title for predicate testing.
func stepByTitle(t *testing.T, title string) tourStep {
	t.Helper()
	return stepByTitleIn(t, tourSteps(), title)
}

func stepByTitleIn(t *testing.T, steps []tourStep, title string) tourStep {
	t.Helper()
	for _, s := range steps {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("tour step %q not found", title)
	return tourStep{}
}

func TestSwitchTabsStep(t *testing.T) {
	step := stepByTitle(t, "Switch tabs — and reach the rest")
	var m Model // tab == TabHome, overflowSet false
	if step.Done(m, walkthroughBaseline{tab: TabHome, overflowSet: false}) {
		t.Error("should not be satisfied with no change")
	}
	if !step.Done(m, walkthroughBaseline{tab: TabFlows}) {
		t.Error("should be satisfied on a tab change")
	}
	m.overflowSet = true
	if !step.Done(m, walkthroughBaseline{tab: TabHome, overflowSet: false}) {
		t.Error("should be satisfied on overflow activation")
	}
}

func TestBackUpStep(t *testing.T) {
	step := stepByTitle(t, "Back up a level")
	var m Model // TabHome is a stem -> canEscBack false
	if !step.Done(m, walkthroughBaseline{}) {
		t.Error("back-up satisfied once on a stem (cannot esc back)")
	}
}

func walkthroughOrgModel() (Model, *orgData) {
	const username = "tour@example.com"
	d := &orgData{}
	var m Model
	m.orgs = []sf.Org{{Username: username}}
	m.selected = 0
	m.data = map[string]*orgData{username: d}
	return m, d
}

func TestSourceToggleStepsAcceptEitherDirection(t *testing.T) {
	t.Run("home recent", func(t *testing.T) {
		step := stepByTitle(t, "Two sources for 'Recently Viewed'")
		m, d := walkthroughOrgModel()
		d.Tab = TabHome
		d.HomeRecentMode = ChipModeSalesforce
		if !step.Done(m, walkthroughBaseline{homeRecentSalesforce: false}) {
			t.Fatal("local → Salesforce should satisfy the step")
		}
		d.HomeRecentMode = ChipModeLocal
		if !step.Done(m, walkthroughBaseline{homeRecentSalesforce: true}) {
			t.Fatal("Salesforce → local should satisfy the step")
		}
	})

	t.Run("records", func(t *testing.T) {
		step := stepByTitle(t, "Switch view source (L)")
		m, d := walkthroughOrgModel()
		d.Tab = TabRecords
		d.RecordsSObjectCur = "Account"
		d.ChipMode = map[string]ChipMode{"Account": ChipModeSalesforce}
		if !step.Done(m, walkthroughBaseline{recordsSourceSalesforce: false}) {
			t.Fatal("local → Salesforce should satisfy the step")
		}
		d.ChipMode["Account"] = ChipModeLocal
		if !step.Done(m, walkthroughBaseline{recordsSourceSalesforce: true}) {
			t.Fatal("Salesforce → local should satisfy the step")
		}

		// This step follows a record drill. Capturing its baseline while
		// still in record detail must preserve the parent list's source;
		// otherwise pressing Esc alone would produce a false ✓.
		d.Tab = TabRecordDetail
		d.DescribeCur = "Account"
		d.ChipMode["Account"] = ChipModeSalesforce
		m.recordDetailReturnTab = TabObjectDetail
		if !m.recordsSourceIsSalesforce() {
			t.Fatal("record detail should retain its parent object list's source signal")
		}
	})
}

func TestSOQLStepRequiresNewSuccessfulRun(t *testing.T) {
	step := stepByTitle(t, "Run a read-only SOQL query")
	var m Model
	m.noOrgTab = TabSOQL
	prev := walkthroughBaseline{soqlRunGen: 4}

	m.soqlRunGen = 4
	if step.Done(m, prev) {
		t.Fatal("an old result should not satisfy the SOQL step")
	}
	m.soqlRunGen = 5
	m.soqlRunning = true
	if step.Done(m, prev) {
		t.Fatal("an in-flight query should not satisfy the SOQL step")
	}
	m.soqlRunning = false
	m.soqlErr = errors.New("query failed")
	if step.Done(m, prev) {
		t.Fatal("a failed query should not satisfy the SOQL step")
	}
	m.soqlErr = nil
	if !step.Done(m, prev) {
		t.Fatal("a newly completed successful query should satisfy the SOQL step")
	}
}

func TestSOQLStepInDemoCompletesOnWorkspaceOpen(t *testing.T) {
	step := stepByTitleIn(t, tourStepsForDemo(true), "Run a read-only SOQL query")
	var m Model
	m.noOrgTab = TabHome
	if step.Done(m, walkthroughBaseline{}) {
		t.Fatal("another workspace should not satisfy the demo SOQL step")
	}
	m.noOrgTab = TabSOQL
	if !step.Done(m, walkthroughBaseline{}) {
		t.Fatal("opening SOQL should satisfy the step when demo blocks live queries")
	}
}

func TestCoreWorkspaceSteps(t *testing.T) {
	tests := []struct {
		title string
		tab   Tab
		set   func(*orgData)
		prev  walkthroughBaseline
	}{
		{
			title: "Browse users and active sessions",
			tab:   TabUsers,
			set:   func(d *orgData) { d.UsersSubtab = 1 },
			prev:  walkthroughBaseline{usersSubtab: 0},
		},
		{
			title: "Browse permissions",
			tab:   TabPerms,
			set:   func(d *orgData) { d.PermsDashboardSubtab = 1 },
			prev:  walkthroughBaseline{permsSubtab: 0},
		},
		{
			title: "See what happened in System",
			tab:   TabSystem,
			set:   func(d *orgData) { d.SystemSubtab = 1 },
			prev:  walkthroughBaseline{systemSubtab: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			step := stepByTitle(t, tt.title)
			m, d := walkthroughOrgModel()
			d.Tab = tt.tab
			if step.Done(m, tt.prev) {
				t.Fatal("opening the workspace without browsing should not satisfy the step")
			}
			tt.set(d)
			if !step.Done(m, tt.prev) {
				t.Fatal("changing a subtab in the requested workspace should satisfy the step")
			}
		})
	}
}

func TestRecordDetailStep(t *testing.T) {
	step := stepByTitle(t, "Open a record's detail")
	var m Model
	m.noOrgTab = TabObjectDetail
	if step.Done(m, walkthroughBaseline{}) {
		t.Fatal("object detail should not satisfy the record-detail step")
	}
	m.noOrgTab = TabRecordDetail
	if !step.Done(m, walkthroughBaseline{}) {
		t.Fatal("record detail should satisfy the step")
	}
}

func TestSidebarStepWatchesRightSidebar(t *testing.T) {
	step := stepByTitle(t, "Show, hide & move the sidebar")
	var m Model
	prev := walkthroughBaseline{rightSidebarOpen: false}

	m.leftOpen = true
	if step.Done(m, prev) {
		t.Fatal("changing the left rail must not satisfy the right-sidebar step")
	}
	m.sidebarOpen = true
	if !step.Done(m, prev) {
		t.Fatal("changing the right sidebar should satisfy the step")
	}
}

func TestWalkthroughRendersOnNarrowTerminal(t *testing.T) {
	m := tourModel([]tourStep{{
		Title:       "Narrow",
		Instruction: "A walkthrough instruction that needs to wrap over several lines.",
		Keys:        []tourKey{{Key: "w", What: "next"}},
	}})
	m.width = 40
	m.height = 20

	rendered := m.renderWalkthrough()
	if rendered == "" {
		t.Fatal("active walkthrough should render")
	}
	if got := lipgloss.Width(rendered); got > m.width {
		t.Fatalf("walkthrough width = %d, terminal width = %d", got, m.width)
	}
}

func TestZenStep(t *testing.T) {
	step := stepByTitle(t, "Zen mode")
	var m Model
	m.zenMode = true
	if !step.Done(m, walkthroughBaseline{zen: false}) {
		t.Error("zen step satisfied on false->true")
	}
	if step.Done(m, walkthroughBaseline{zen: true}) {
		t.Error("zen step not satisfied when unchanged")
	}
}
