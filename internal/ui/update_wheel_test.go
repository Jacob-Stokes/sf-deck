package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// wheelStep replaced shouldDropWheel: instead of dropping bursty events,
// the runtime accumulates them into `pending` and drains the queue on
// the next accepted tick. Tests assert that semantic — every event the
// user produced eventually translates into one cursor step, even if
// several arrived faster than the throttle's min interval.

func TestWheelStepAccumulatesAcrossDeferredEvents(t *testing.T) {
	m := Model{modelRuntime: modelRuntime{wheel: &wheelRuntime{}}}
	down := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})

	// First event after quiet: accepted, drains the +1 it just added.
	if got := m.wheelStep(down); got != 1 {
		t.Fatalf("first wheel event after quiet should drain +1, got %d", got)
	}
	// Two more arrive too fast → deferred (return 0) but pending grows.
	if got := m.wheelStep(down); got != 0 {
		t.Fatalf("dense same-direction event should defer (got %d)", got)
	}
	if got := m.wheelStep(down); got != 0 {
		t.Fatalf("second dense event should defer (got %d)", got)
	}
	// Advance time past min interval; next event drains pending+self.
	m.wheel.lastAccepted = time.Now().Add(-24 * time.Millisecond)
	if got := m.wheelStep(down); got != 3 {
		t.Fatalf("post-interval drain should deliver +3 (2 deferred + 1 fresh), got %d", got)
	}
}

func TestWheelStepDirectionChangeCancelsPending(t *testing.T) {
	m := Model{modelRuntime: modelRuntime{wheel: &wheelRuntime{}}}
	down := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	up := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp})

	_ = m.wheelStep(down) // +1, pending drained
	_ = m.wheelStep(down) // deferred, pending=1
	if got := m.wheelStep(up); got != -1 {
		// Direction flip cancels queued downward momentum and
		// drains just the new -1.
		t.Fatalf("direction change should drain -1 (no carry-over), got %d", got)
	}
}

func TestWheelStepQuietGapResetsPending(t *testing.T) {
	m := Model{modelRuntime: modelRuntime{wheel: &wheelRuntime{}}}
	down := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})

	_ = m.wheelStep(down)
	_ = m.wheelStep(down) // deferred, pending=1

	// Long pause — fresh gesture, prior accumulator stale.
	m.wheel.lastSeen = time.Now().Add(-200 * time.Millisecond)
	if got := m.wheelStep(down); got != 1 {
		t.Fatalf("first event after quiet gap should drain just +1, got %d", got)
	}
}
