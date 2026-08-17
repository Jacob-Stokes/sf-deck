package ui

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
)

type wheelRuntime struct {
	lastSeen     time.Time
	lastAccepted time.Time
	lastButton   tea.MouseButton

	// pending is the accumulated cursor delta from wheel events that
	// arrived but were throttle-deferred. Continuous mode drains
	// this on each accepted tick (capped) so a fast flick still
	// advances the cursor by every event the user produced.
	// Paginated mode does NOT use this — see wheelStepSimple.
	//
	// Sign: positive = down, negative = up. Reset when direction
	// flips (a deliberate reverse cancels the queued momentum) or
	// after the burst goes quiet.
	pending int

	streamKey string
	switchAt  time.Time
	// gapHist is a ring of the most recent inter-event gaps,
	// maintained on EVERY wheel event (accepted, deferred, or
	// swallowed). Trackpad inertia decays monotonically — gaps only
	// widen as the coast dies — so a sudden gap SHRINK against the
	// rolling median is new finger energy, i.e. deliberate input.
	// Measured on a real trace (2026-06-12): coasts start at 1-5ms
	// gaps and decay to 20-70ms over 0.9-2.4s; a fresh flick lands
	// at 1-5ms. The median discriminates with a wide margin.
	gapHist  [6]time.Duration
	gapCount int
	gapIdx   int
}

const (
	// wheelGateHardCap releases the gate unconditionally — longer
	// than any observed coast (max 2.4s) so it only catches
	// pathologies, never real input.
	wheelGateHardCap      = 2500 * time.Millisecond
	wheelReaccelFraction  = 0.4
	wheelReaccelMinMedian = 12 * time.Millisecond
	// wheelGateAdoptQuiet is the silence required before a FOREIGN
	// stream's next event counts as fresh deliberate input. Coast
	// tails emit final stragglers spaced 100-200ms apart — wider
	// than the ordinary 80ms quietGap — and each one masqueraded as
	// a deliberate tick, landing the "single row jumps after a
	// while" phantom (field report 2026-06-12). 300ms outlasts the
	// stragglers; a deliberate scroll inside that window loses at
	// most its first tick before released_reaccel catches the rest.
	wheelGateAdoptQuiet = 300 * time.Millisecond
)

func (w *wheelRuntime) recordGap(g time.Duration) {
	if g <= 0 {
		return
	}
	w.gapHist[w.gapIdx] = g
	w.gapIdx = (w.gapIdx + 1) % len(w.gapHist)
	if w.gapCount < len(w.gapHist) {
		w.gapCount++
	}
}

func (w *wheelRuntime) gapMedian() time.Duration {
	if w.gapCount < 3 {
		return 0
	}
	tmp := make([]time.Duration, w.gapCount)
	copy(tmp, w.gapHist[:w.gapCount])
	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })
	return tmp[len(tmp)/2]
}

func (m Model) wheelTimings() (quietGap, minInterval time.Duration) {
	if m.settings == nil {
		return 80 * time.Millisecond, 12 * time.Millisecond
	}
	return time.Duration(m.settings.WheelQuietGapMs()) * time.Millisecond,
		time.Duration(m.settings.WheelMinIntervalMs()) * time.Millisecond
}

func (m Model) wheelSurfaceKey() string {
	key := m.tab().String() + "|" + string(m.currentSubtab())
	if len(m.orgs) > 0 && m.selected < len(m.orgs) {
		key += "|" + m.orgs[m.selected].Username
	}
	if m.anyModalActive() {
		key += "|modal"
	}
	if m.activeAutocompleteSession() != nil {
		key += "|ac"
	}
	return key
}

// wheelStreamGate is the cross-surface momentum guard shared by both
// wheel state machines. Returns (swallow, reason):
//
//	swallow=true  — the event is inertial spill-over from a stream
//	                that started on a DIFFERENT surface; drop it.
//	swallow=false — process normally; reason is non-empty when the
//	                gate just RELEASED (handed the stream to the new
//	                surface) and names which rule fired.
//
// Release rules — three of the four fire while the user is still
// scrolling, which is the specific failure of the first design
// (swallow-until-quiet froze the new surface for as long as the user
// kept scrolling, because their own events kept the stream alive):
//
//	released_quiet    — a >quietGap pause ended the coast
//	released_reversal — wheel direction flipped (coasts never reverse)
//	released_reaccel  — gaps suddenly tightened against the rolling
//	                    median (coasts only decay; new energy = user)
//	released_cap      — hard 2.5s ceiling (longer than any real coast)
func (m *Model) wheelStreamGate(now time.Time, quietGap time.Duration, button tea.MouseButton) (bool, string) {
	gap := time.Duration(0)
	if !m.wheel.lastSeen.IsZero() {
		gap = now.Sub(m.wheel.lastSeen)
	}
	key := m.wheelSurfaceKey()
	if m.wheel.streamKey == key {
		m.wheel.switchAt = time.Time{}
		m.wheel.recordGap(gap)
		return false, ""
	}
	adopt := func() {
		m.wheel.streamKey = key
		m.wheel.switchAt = time.Time{}
		m.wheel.pending = 0
	}
	// NOTE: deliberately wheelGateAdoptQuiet, not the ordinary
	// quietGap — see the const doc. quietGap stays the threshold for
	// everything else (accumulator reset, accept cadence).
	live := !m.wheel.lastSeen.IsZero() && gap < wheelGateAdoptQuiet
	if !live {
		adopt()
		m.wheel.recordGap(gap)
		return false, "released_quiet"
	}
	if m.wheel.switchAt.IsZero() {
		m.wheel.switchAt = now
	}
	if button != m.wheel.lastButton {
		adopt()
		m.wheel.recordGap(gap)
		return false, "released_reversal"
	}
	if med := m.wheel.gapMedian(); med >= wheelReaccelMinMedian &&
		gap > 0 && float64(gap) < float64(med)*wheelReaccelFraction {
		adopt()
		m.wheel.recordGap(gap)
		return false, "released_reaccel"
	}
	if now.Sub(m.wheel.switchAt) >= wheelGateHardCap {
		adopt()
		m.wheel.recordGap(gap)
		return false, "released_cap"
	}
	// Swallow: keep the stream tracked (lastSeen) and its decay
	// profile current (gapHist); drop any queued delta so it can't
	// drain into the new surface.
	m.wheel.recordGap(gap)
	m.wheel.lastSeen = now
	m.wheel.pending = 0
	return true, ""
}

func (m Model) wheelStep(msg tea.MouseWheelMsg) int {
	_, minInterval := m.wheelTimings()
	return m.wheelStepWithCap(msg, m.wheelMaxStep(), int(minInterval/time.Millisecond))
}

func (m Model) wheelMaxStep() int {
	if m.settings == nil {
		return 20
	}
	return m.settings.WheelMaxStep()
}

func (m Model) wheelStepWithCap(msg tea.MouseWheelMsg, cap, minIntervalMs int) int {
	if m.wheel == nil {
		return 0
	}
	now := time.Now()
	button := tea.Mouse(msg).Button
	step := 0
	switch button {
	case tea.MouseWheelDown:
		step = 1
	case tea.MouseWheelUp:
		step = -1
	default:
		return 0
	}
	quietGap, _ := m.wheelTimings()
	minInterval := time.Duration(minIntervalMs) * time.Millisecond
	if swallow, release := (&m).wheelStreamGate(now, quietGap, button); swallow {
		m.traceWheel(button, true, "dropped_surface_change", -1, -1, quietGap, minInterval)
		return 0
	} else if release != "" {
		m.traceWheel(button, false, release, -1, -1, quietGap, minInterval)
	}

	sinceSeen := time.Duration(-1)
	if !m.wheel.lastSeen.IsZero() {
		sinceSeen = now.Sub(m.wheel.lastSeen)
	}
	sinceAccepted := time.Duration(-1)
	if !m.wheel.lastAccepted.IsZero() {
		sinceAccepted = now.Sub(m.wheel.lastAccepted)
	}
	quiet := m.wheel.lastSeen.IsZero() || now.Sub(m.wheel.lastSeen) >= quietGap
	directionChanged := button != m.wheel.lastButton

	if directionChanged {
		m.wheel.pending = 0
	}
	m.wheel.lastSeen = now
	if quiet {
		m.wheel.pending = 0
	}
	m.wheel.pending += step

	if quiet || directionChanged {
		reason := "accepted_quiet"
		if directionChanged && !quiet {
			reason = "accepted_direction_change"
		}
		delta := drainPending(m.wheel, cap)
		m.traceWheel(button, false, reason, sinceSeen, sinceAccepted, quietGap, minInterval)
		m.wheel.lastAccepted = now
		m.wheel.lastButton = button
		return delta
	}
	if minInterval > 0 && now.Sub(m.wheel.lastAccepted) < minInterval {
		m.traceWheel(button, true, "deferred_min_interval", sinceSeen, sinceAccepted, quietGap, minInterval)
		return 0
	}
	delta := drainPending(m.wheel, cap)
	m.traceWheel(button, false, "accepted_interval", sinceSeen, sinceAccepted, quietGap, minInterval)
	m.wheel.lastAccepted = now
	m.wheel.lastButton = button
	return delta
}

// drainPending pulls up to `cap` rows of cursor movement out of the
// accumulator and leaves the remainder for the next tick. Sign-
// preserving — negative pending (up-direction queue) drains as a
// negative delta of bounded magnitude.
//
// This is what makes a fast trackpad flick read as "scrolled
// quickly" rather than "teleported": a 200-event burst still moves
// the cursor 200 rows total, but spread across enough frames that
// the eye can follow.
func drainPending(w *wheelRuntime, cap int) int {
	if w == nil {
		return 0
	}
	if cap < 1 {
		cap = 1
	}
	p := w.pending
	if p == 0 {
		return 0
	}
	if p > cap {
		w.pending = p - cap
		return cap
	}
	if p < -cap {
		w.pending = p + cap
		return -cap
	}
	w.pending = 0
	return p
}

func (m Model) handleWheelContinuous(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	step := m.wheelStep(msg)
	if step == 0 {
		m.skipNextFrameRender()
		return m, nil
	}
	m.orgQuickJumpActive = false
	m.focus = focusMain
	return m.moveCursor(step)
}

// handleWheelPaginated is the pagination-mode wheel handler.
// Modest by design: 1 row per accepted event, throttled to ~40
// events/sec via the standard min-interval, no accumulator. The
// trackpad is for small adjustments; bulk traversal is via
// keyboard (Ctrl+D / Ctrl+U for half-page, Space / b for page,
// gg / G for top/bottom).
//
// We tried to make trackpad scroll feel native (accumulator,
// gesture budget, capped step, no throttle) — none of them work
// well because the terminal mouse protocol strips macOS's phase
// information (finger-down vs finger-up vs inertia), so every
// approach is a heuristic. Pagers like less and vim sidestep this
// by treating wheel as a secondary input. We do the same.
func (m Model) handleWheelPaginated(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	step := m.wheelStepSimple(msg)
	if step == 0 {
		m.skipNextFrameRender()
		return m, nil
	}
	m.orgQuickJumpActive = false
	m.focus = focusMain
	return m.moveCursor(step)
}

func (m Model) wheelStepSimple(msg tea.MouseWheelMsg) int {
	if m.wheel == nil {
		return 0
	}
	now := time.Now()
	button := tea.Mouse(msg).Button
	step := 0
	switch button {
	case tea.MouseWheelDown:
		step = 1
	case tea.MouseWheelUp:
		step = -1
	default:
		return 0
	}
	quietGap, minInterval := m.wheelTimings()
	if swallow, release := (&m).wheelStreamGate(now, quietGap, button); swallow {
		m.traceWheel(button, true, "dropped_surface_change", -1, -1, quietGap, minInterval)
		return 0
	} else if release != "" {
		m.traceWheel(button, false, release, -1, -1, quietGap, minInterval)
	}

	sinceSeen := time.Duration(-1)
	if !m.wheel.lastSeen.IsZero() {
		sinceSeen = now.Sub(m.wheel.lastSeen)
	}
	sinceAccepted := time.Duration(-1)
	if !m.wheel.lastAccepted.IsZero() {
		sinceAccepted = now.Sub(m.wheel.lastAccepted)
	}
	quiet := m.wheel.lastSeen.IsZero() || now.Sub(m.wheel.lastSeen) >= quietGap
	directionChanged := button != m.wheel.lastButton
	m.wheel.lastSeen = now

	if quiet || directionChanged {
		reason := "accepted_quiet"
		if directionChanged && !quiet {
			reason = "accepted_direction_change"
		}
		m.traceWheel(button, false, reason, sinceSeen, sinceAccepted, quietGap, minInterval)
		m.wheel.lastAccepted = now
		m.wheel.lastButton = button
		return step
	}
	if now.Sub(m.wheel.lastAccepted) < minInterval {
		m.traceWheel(button, true, "dropped_min_interval", sinceSeen, sinceAccepted, quietGap, minInterval)
		return 0
	}
	m.traceWheel(button, false, "accepted_interval", sinceSeen, sinceAccepted, quietGap, minInterval)
	m.wheel.lastAccepted = now
	m.wheel.lastButton = button
	return step
}
