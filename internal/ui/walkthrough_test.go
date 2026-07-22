package ui

import (
	"testing"
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

// stepSatisfied drives the ✓ indicator: true only when the current
// step's predicate holds; never true for a predicate-less info step.
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

func TestTourStepsWellFormed(t *testing.T) {
	for i, s := range tourSteps() {
		if s.Title == "" || s.Instruction == "" {
			t.Errorf("step %d missing title/instruction", i)
		}
	}
}

// stepByTitle finds a tour step by title for predicate testing.
func stepByTitle(t *testing.T, title string) tourStep {
	t.Helper()
	for _, s := range tourSteps() {
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
