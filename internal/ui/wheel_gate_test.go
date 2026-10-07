package ui

// wheelStreamGate behaviour tests. The gate's release rules were
// derived from a real trace (2026-06-12): trackpad coasts decay
// monotonically from ~1-5ms gaps to 20-70ms over 0.9-2.4s; fresh
// deliberate input re-tightens gaps or reverses direction. Two
// earlier designs failed in the field (swallow-until-quiet froze on
// the user's own input; a fixed window both froze and bled) — these
// tests pin the rules that specifically close those failure modes.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func gateModel() *Model {
	m := &Model{}
	m.wheel = &wheelRuntime{}
	m.renderCache = newRenderCache()
	return m
}

const gateQuiet = 80 * time.Millisecond

func feed(m *Model, at time.Time, button tea.MouseButton) (bool, string) {
	swallowed, release := m.wheelStreamGate(at, gateQuiet, button)
	if !swallowed {
		m.wheel.lastSeen = at
		m.wheel.lastButton = button
	}
	return swallowed, release
}

func TestWheelGate_CoastSwallowedAfterSurfaceChange(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	if m.wheel.streamKey == "" {
		t.Fatal("first event should adopt a stream key")
	}
	m.wheel.streamKey = "elsewhere"
	at := t0
	for i := 0; i < 20; i++ {
		at = at.Add(3 * time.Millisecond)
		swallowed, _ := feed(m, at, tea.MouseWheelDown)
		if !swallowed {
			t.Fatalf("violent coast event %d not swallowed", i)
		}
	}
	if m.wheel.pending != 0 {
		t.Fatalf("pending should be cleared while swallowing, got %d", m.wheel.pending)
	}
}

func TestWheelGate_ReversalReleasesImmediately(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	m.wheel.streamKey = "elsewhere"
	at := t0.Add(3 * time.Millisecond)
	if swallowed, _ := feed(m, at, tea.MouseWheelDown); !swallowed {
		t.Fatal("coast event should be swallowed")
	}
	// Opposite direction = deliberate, even mid-coast.
	at = at.Add(3 * time.Millisecond)
	swallowed, release := feed(m, at, tea.MouseWheelUp)
	if swallowed || release != "released_reversal" {
		t.Fatalf("reversal: swallowed=%v release=%q", swallowed, release)
	}
}

func TestWheelGate_ReaccelReleasesDuringCoastTail(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	m.wheel.streamKey = "elsewhere"
	at := t0
	for _, gapMS := range []int{25, 30, 35, 40, 45, 50} {
		at = at.Add(time.Duration(gapMS) * time.Millisecond)
		if swallowed, _ := feed(m, at, tea.MouseWheelDown); !swallowed {
			t.Fatalf("tail event (gap %dms) not swallowed", gapMS)
		}
	}
	at = at.Add(3 * time.Millisecond)
	swallowed, release := feed(m, at, tea.MouseWheelDown)
	if swallowed || release != "released_reaccel" {
		t.Fatalf("re-accel: swallowed=%v release=%q", swallowed, release)
	}
}

func TestWheelGate_QuietGapReleases(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	m.wheel.streamKey = "elsewhere"
	at := t0.Add(3 * time.Millisecond)
	feed(m, at, tea.MouseWheelDown) // swallowed
	// 400ms pause — coast properly dead; next scroll is deliberate.
	at = at.Add(400 * time.Millisecond)
	swallowed, release := feed(m, at, tea.MouseWheelDown)
	if swallowed || release != "released_quiet" {
		t.Fatalf("quiet release: swallowed=%v release=%q", swallowed, release)
	}
}

func TestWheelGate_TailStragglersStaySwallowed(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	m.wheel.streamKey = "elsewhere"
	// Coast tail stragglers: final inertia events spaced 100-200ms —
	// wider than the ordinary 80ms quiet gap but not deliberate.
	// These caused the "single row jumps after a while" phantom.
	at := t0
	for _, gapMS := range []int{3, 40, 90, 120, 150, 200, 250} {
		at = at.Add(time.Duration(gapMS) * time.Millisecond)
		if swallowed, release := feed(m, at, tea.MouseWheelDown); !swallowed {
			t.Fatalf("straggler (gap %dms) released as %q", gapMS, release)
		}
	}
}

func TestWheelGate_HardCapReleases(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	feed(m, t0, tea.MouseWheelDown)
	m.wheel.streamKey = "elsewhere"
	// Pathological coast: steady 30ms gaps forever (steady, so no
	// re-accel; median ~30ms but gaps never tighten). Cap releases
	// at 2.5s.
	at := t0
	released := ""
	for i := 0; i < 120; i++ {
		at = at.Add(30 * time.Millisecond)
		swallowed, release := feed(m, at, tea.MouseWheelDown)
		if !swallowed {
			released = release
			break
		}
	}
	if released != "released_cap" {
		t.Fatalf("expected released_cap, got %q", released)
	}
}

func TestWheelGate_SameSurfaceNeverSwallows(t *testing.T) {
	m := gateModel()
	t0 := time.Unix(1000, 0)
	at := t0
	for i := 0; i < 50; i++ {
		at = at.Add(2 * time.Millisecond)
		if swallowed, _ := feed(m, at, tea.MouseWheelDown); swallowed {
			t.Fatalf("event %d swallowed on its own surface", i)
		}
	}
}
