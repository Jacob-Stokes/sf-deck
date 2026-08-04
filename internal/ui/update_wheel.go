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

	// --- cross-surface momentum isolation (see wheelStreamGate) ---

	// streamKey fingerprints the surface that OWNS the current wheel
	// stream (tab + subtab + org + modal/popup overlay).
	streamKey string
	// switchAt is when a mid-stream surface change was first seen;
	// zero when the stream owner matches the active surface.
	switchAt time.Time
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

// Tunables for the cross-surface gate. Derived from the 2026-06-12
// trace session, not theory — see the wheel saga memory for the two
// failed designs that preceded this one.
const (
	// wheelGateHardCap releases the gate unconditionally — longer
	// than any observed coast (max 2.4s) so it only catches
	// pathologies, never real input.
	wheelGateHardCap = 2500 * time.Millisecond
	// wheelReaccelFraction: a gap under this fraction of the rolling
	// median (coast tail: 20-70ms) means re-acceleration.
	wheelReaccelFraction = 0.4
	// wheelReaccelMinMedian disables re-accel detection while the
	// coast is still violent (median gaps under ~12ms) — user events
	// are statistically invisible inside that firehose anyway.
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

// gapMedian returns the median of the recorded gaps (0 when fewer
// than 3 samples — not enough signal to judge).
func (w *wheelRuntime) gapMedian() time.Duration {
	if w.gapCount < 3 {
		return 0
	}
	tmp := make([]time.Duration, w.gapCount)
	copy(tmp, w.gapHist[:w.gapCount])
	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })
	return tmp[len(tmp)/2]
}

// wheelTimings reads the per-call throttle values from settings, falling
// back to the package defaults (80ms quiet gap, 12ms min interval).
// Reading per-call is fine — this runs once per accepted wheel tick at
// most, dwarfed by the actual scroll work.
func (m Model) wheelTimings() (quietGap, minInterval time.Duration) {
	if m.settings == nil {
		return 80 * time.Millisecond, 12 * time.Millisecond
	}
	return time.Duration(m.settings.WheelQuietGapMs()) * time.Millisecond,
		time.Duration(m.settings.WheelMinIntervalMs()) * time.Millisecond
}

// wheelSurfaceKey fingerprints the surface a wheel event would
// scroll: active tab + subtab + org + whether a modal / the SOQL
// autocomplete popup currently owns the wheel.
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

// wheelStep handles one incoming wheel event and returns the cursor
// delta that should be applied on this tick (0 means "skip render,
// no cursor move"). Replaces the older boolean-drop API: instead of
// dropping excess events, every event contributes +1 (or -1 for up)
// to a pending accumulator, and an accepted tick drains it.
//
// The result: a fast trackpad flick that produces 100 events in
// 200ms still results in 100 cursor steps total — but distributed
// across maybe 8 actual renders (one per minInterval). Each render
// moves the cursor by `pending`, not by 1, so visible scroll speed
// matches finger speed.
//
// Sign convention: positive = down, negative = up.
func (m Model) wheelStep(msg tea.MouseWheelMsg) int {
	_, minInterval := m.wheelTimings()
	return m.wheelStepWithCap(msg, m.wheelMaxStep(), int(minInterval/time.Millisecond))
}

// wheelMaxStep reads the per-tick cursor-delta cap for continuous
// mode from settings. Reading per-call is fine — only fires once
// per accepted wheel.
func (m Model) wheelMaxStep() int {
	if m.settings == nil {
		return 20
	}
	return m.settings.WheelMaxStep()
}

// wheelStepWithCap is wheelStep with parameterised cap and
// per-mode min-interval override. Continuous mode passes the
// continuous cap (~20) and the standard min-interval (24ms);
// paginated mode passes the paged cap (~2) and a min-interval of 0
// (no throttle, every event accepted) — pagination's row cache
// makes per-frame cost tiny enough that display-rate updates feel
// smooth instead of choppy.
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
	// minInterval == 0 → no throttle gate. Every event accepted;
	// the renderer's own per-vsync coalescing rate-limits us
	// naturally. This is the paginated-mode default, justified
	// by the row cache making per-frame cost ~0.13ms.
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

// handleWheelContinuous is the default-mode wheel handler. Each
// accepted tick drains the accumulator (capped by WheelMaxStep) so
// a fast flick scrolls many rows. The org quick-jump overlay
// dismisses on first wheel; focus snaps to body.
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

// wheelStepSimple is the paginated-mode state machine. Plain
// throttle: one accepted event per minInterval, returns +1/-1/0.
// No accumulator (each event is one row, period — bulk traversal
// is keyboard's job). No gesture budget. Inertial events get
// throttle-dropped naturally.
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
