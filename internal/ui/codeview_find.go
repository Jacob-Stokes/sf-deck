package ui

// codeview_find.go — in-code find + horizontal scroll for the shared
// code viewer (Apex class / trigger bodies, LWC & Aura file sources,
// flow-version JSON, exec debug logs).
//
// Find-NEXT over the visible body, not a filter:
//   /            open the find bar (typing live-jumps to the first
//                match at/after the cursor)
//   ↵ / shift+↵  cycle forward / back through matches while typing
//   esc          close the bar, keep the query + highlights (n / N
//                cycle, C clears — mirrors the record-detail find)
//   ctrl+u       clear the query while typing
//
// The bar shows "x of y" so the user knows where they are. All matched
// substrings highlight in the body; the current match gets a distinct
// colour. Matched lines re-render from the RAW source (chroma's ANSI
// output can't be safely re-backgrounded mid-span), so syntax colour
// yields to match colour on exactly those lines.
//
// ←/→ scroll the body horizontally (gutter stays put) for lines that
// run off the right edge — the "…" prefix marks a scrolled view.
//
// Key routing: while the bar has focus, printables are consumed in
// handleInputModeKey — BEFORE the q-chord leader — so searching for
// "query" doesn't open chord mode. Idle-state keys (/ n N C ← →) ride
// handlePreGlobalTabKey, gated on "the last paint drew a code body on
// this exact tab+subtab".

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// codeViewLastPaint identifies the code body the most recent frame
// rendered. Body is a string reference (no copy).
type codeViewLastPaint struct {
	Tab    Tab
	Sub    Subtab
	BodyID string
	Body   string
}

// codeMatch is one query occurrence: 0-based line + byte offsets into
// the raw line.
type codeMatch struct {
	Line       int
	Start, End int
}

// codeFindState is the per-body find state. The memo fields cache the
// match scan for the current (query, body) so the per-frame render is
// a map lookup, not an O(body) rescan.
type codeFindState struct {
	Buffer string
	Active bool // bar has input focus
	Idx    int  // current match index into memoMatches

	memoQuery   string
	memoBody    string
	memoMatches []codeMatch
	memoByLine  map[int][]int // line -> indices into memoMatches
	memoLines   []string      // raw body lines (for match re-render)
}

// codeFindMaxMatches bounds the scan so a 1-char query on a huge body
// can't allocate unbounded match slices. Past the cap the counter
// shows "cap+" semantics implicitly (cycling still works over the
// scanned prefix).
const codeFindMaxMatches = 2000

// codeFindHScrollStep is how many display cells one ←/→ press shifts.
const codeFindHScrollStep = 8

// ----------------------------------------------------------------------
// State access
// ----------------------------------------------------------------------

// codeFindStateFor returns the find state for bodyID, creating it when
// create is set.
func codeFindStateFor(d *orgData, bodyID string, create bool) *codeFindState {
	if d == nil || bodyID == "" {
		return nil
	}
	st := d.CodeFind[bodyID]
	if st == nil && create {
		if d.CodeFind == nil {
			d.CodeFind = map[string]*codeFindState{}
		}
		st = &codeFindState{}
		d.CodeFind[bodyID] = st
	}
	return st
}

// codeFindMatchesFor returns the match list for the state's current
// buffer against body, recomputing only when the query or body
// changed. Case-insensitive substring. Also clamps st.Idx into range.
func codeFindMatchesFor(st *codeFindState, body string) []codeMatch {
	if st == nil || st.Buffer == "" {
		return nil
	}
	if st.memoQuery == st.Buffer && st.memoBody == body && st.memoLines != nil {
		return st.memoMatches
	}
	q := strings.ToLower(st.Buffer)
	lines := strings.Split(body, "\n")
	var matches []codeMatch
	byLine := map[int][]int{}
scan:
	for i, line := range lines {
		low := strings.ToLower(line)
		from := 0
		for {
			rel := strings.Index(low[from:], q)
			if rel < 0 {
				break
			}
			start := from + rel
			matches = append(matches, codeMatch{Line: i, Start: start, End: start + len(q)})
			byLine[i] = append(byLine[i], len(matches)-1)
			from = start + len(q)
			if len(matches) >= codeFindMaxMatches {
				break scan
			}
		}
	}
	st.memoQuery = st.Buffer
	st.memoBody = body
	st.memoMatches = matches
	st.memoByLine = byLine
	st.memoLines = lines
	if st.Idx >= len(matches) {
		st.Idx = 0
	}
	if st.Idx < 0 {
		st.Idx = 0
	}
	return matches
}

// ----------------------------------------------------------------------
// Key handling
// ----------------------------------------------------------------------

// codeFindTarget resolves the on-screen code body IF the active
// tab+subtab is the one the last paint drew. This is the gate that
// keeps code-view keys from firing on unrelated surfaces after the
// user navigates away.
func (m Model) codeFindTarget() (*orgData, codeViewLastPaint, bool) {
	d := m.activeOrgData()
	if d == nil || d.CodeViewLast.BodyID == "" {
		return nil, codeViewLastPaint{}, false
	}
	last := d.CodeViewLast
	if last.Tab != m.tab() || last.Sub != m.currentSubtab() {
		return nil, codeViewLastPaint{}, false
	}
	return d, last, true
}

// codeFindInputActive reports whether the find bar owns typed keys.
func (m Model) codeFindInputActive() bool {
	d, last, ok := m.codeFindTarget()
	if !ok {
		return false
	}
	st := codeFindStateFor(d, last.BodyID, false)
	return st != nil && st.Active
}

// handleCodeFindInput consumes keys while the find bar has focus.
// Returns handled=false for keys the bar doesn't own (arrows, ctrl
// combos) so global dispatch — including ←/→ horizontal scroll —
// still works mid-find.
func (m Model) handleCodeFindInput(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	d, last, ok := m.codeFindTarget()
	if !ok {
		return m, nil, false
	}
	st := codeFindStateFor(d, last.BodyID, false)
	if st == nil || !st.Active {
		return m, nil, false
	}
	key := msg.String()
	switch key {
	case "esc":
		// Keep the query + highlights; n / N keep cycling. C clears.
		st.Active = false
		return m, nil, true
	case "enter":
		m.codeFindCycle(d, last, st, +1)
		return m, nil, true
	case "shift+enter":
		m.codeFindCycle(d, last, st, -1)
		return m, nil, true
	case "backspace":
		if st.Buffer != "" {
			_, size := lastRune(st.Buffer)
			st.Buffer = st.Buffer[:len(st.Buffer)-size]
			m.codeFindJumpFromCursor(d, last, st)
		}
		return m, nil, true
	case "ctrl+u":
		st.Buffer = ""
		return m, nil, true
	case "space":
		// Space arrives as the named key "space", not a printable —
		// without this, queries like "new Set" can't be typed.
		st.Buffer += " "
		m.codeFindJumpFromCursor(d, last, st)
		return m, nil, true
	}
	// Printable ASCII appends to the query. (Capital letters included —
	// class names are searched more often than reverse-cycle, which
	// lives on shift+enter / N-after-esc.)
	if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
		st.Buffer += key
		m.codeFindJumpFromCursor(d, last, st)
		return m, nil, true
	}
	return m, nil, false
}

// onCodeViewKey is the idle-state (bar unfocused) key handler, riding
// handlePreGlobalTabKey. Also owns horizontal scroll in BOTH states
// (arrows fall through the input handler above).
func (m Model) onCodeViewKey(key string) (Model, tea.Cmd, bool) {
	d, last, ok := m.codeFindTarget()
	if !ok {
		return m, nil, false
	}
	// Horizontal scroll works whether or not the bar has focus.
	switch key {
	case "left":
		m.codeViewHScroll(d, last, -codeFindHScrollStep)
		return m, nil, true
	case "right":
		m.codeViewHScroll(d, last, +codeFindHScrollStep)
		return m, nil, true
	}
	st := codeFindStateFor(d, last.BodyID, false)
	if st != nil && st.Active {
		// Typing mode already consumed everything it owns.
		return m, nil, false
	}
	switch key {
	case "/":
		st = codeFindStateFor(d, last.BodyID, true)
		st.Active = true
		return m, nil, true
	case "n":
		if st == nil || st.Buffer == "" {
			return m, nil, false
		}
		m.codeFindCycle(d, last, st, +1)
		return m, nil, true
	case "N":
		if st == nil || st.Buffer == "" {
			return m, nil, false
		}
		m.codeFindCycle(d, last, st, -1)
		return m, nil, true
	case "C":
		if st == nil || st.Buffer == "" {
			return m, nil, false
		}
		st.Buffer = ""
		st.Active = false
		return m, nil, true
	}
	return m, nil, false
}

// codeFindCycle advances the current match by delta (wrapping) and
// moves the body cursor to its line — the render's cursor-follow
// scroll brings it into view.
func (m Model) codeFindCycle(d *orgData, last codeViewLastPaint, st *codeFindState, delta int) {
	matches := codeFindMatchesFor(st, last.Body)
	n := len(matches)
	if n == 0 {
		return
	}
	st.Idx = ((st.Idx+delta)%n + n) % n
	setBodyCursor(d, last.BodyID, matches[st.Idx].Line)
}

// codeFindJumpFromCursor re-runs the search after a buffer edit and
// lands on the first match at/after the current cursor line (wrapping
// to the top) — find-next-from-here semantics, so typing refines
// without yanking the user back to line 1.
func (m Model) codeFindJumpFromCursor(d *orgData, last codeViewLastPaint, st *codeFindState) {
	matches := codeFindMatchesFor(st, last.Body)
	if len(matches) == 0 {
		return
	}
	cursor := 0
	if d.BodyCursor != nil {
		cursor = d.BodyCursor[last.BodyID]
	}
	st.Idx = 0
	for i, mt := range matches {
		if mt.Line >= cursor {
			st.Idx = i
			break
		}
	}
	setBodyCursor(d, last.BodyID, matches[st.Idx].Line)
}

// codeViewHScroll shifts the horizontal offset, clamped to [0, longest
// raw line]. The upper clamp is loose (byte length over-estimates
// display width) — scrolling a few cells past the end is harmless.
func (m Model) codeViewHScroll(d *orgData, last codeViewLastPaint, delta int) {
	if d.BodyHScroll == nil {
		d.BodyHScroll = map[string]int{}
	}
	hs := d.BodyHScroll[last.BodyID] + delta
	if hs < 0 {
		hs = 0
	}
	if max := longestLineLen(last.Body); hs > max {
		hs = max
	}
	d.BodyHScroll[last.BodyID] = hs
}

func setBodyCursor(d *orgData, bodyID string, line int) {
	if d.BodyCursor == nil {
		d.BodyCursor = map[string]int{}
	}
	d.BodyCursor[bodyID] = line
}

func longestLineLen(body string) int {
	max, cur := 0, 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			if cur > max {
				max = cur
			}
			cur = 0
			continue
		}
		cur++
	}
	if cur > max {
		max = cur
	}
	return max
}

func lastRune(s string) (rune, int) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i]&0xc0 != 0x80 { // not a UTF-8 continuation byte
			return rune(s[i]), len(s) - i
		}
	}
	return 0, len(s)
}

// ----------------------------------------------------------------------
// Rendering
// ----------------------------------------------------------------------

// renderCodeFindBar draws the one-line find bar: query (with caret
// while focused), the "x of y" counter, and mode-appropriate hints.
func renderCodeFindBar(st *codeFindState, total, inner int) string {
	counter := ""
	switch {
	case total > 0:
		counter = fmt.Sprintf("%d of %d", st.Idx+1, total)
		if total >= codeFindMaxMatches {
			counter = fmt.Sprintf("%d of %d+", st.Idx+1, total)
		}
	case st.Buffer != "":
		counter = "no matches"
	}
	queryStyle := lipgloss.NewStyle().Foreground(theme.Yellow)
	dim := lipgloss.NewStyle().Foreground(theme.FgDim)
	var line string
	if st.Active {
		line = queryStyle.Render("  /"+st.Buffer+"▏") +
			dim.Render("  "+counter+"   ↵ next · shift+↵ prev · esc done · ctrl+u clear")
	} else {
		line = queryStyle.Render("  /"+st.Buffer) +
			dim.Render("  "+counter+"   n/N cycle · / edit · C clear")
	}
	return ansi.Truncate(line, inner, "…")
}

// renderCodeFindLine rebuilds one matched line from its RAW source
// with match spans styled — every match in the line highlights, the
// current one distinctly. hs (horizontal scroll) is applied by the
// caller after composition.
func renderCodeFindLine(raw string, matches []codeMatch, idxs []int, currentIdx int) string {
	var b strings.Builder
	prev := 0
	for _, mi := range idxs {
		mt := matches[mi]
		if mt.Start < prev || mt.End > len(raw) {
			continue // stale memo bounds — render plain rather than panic
		}
		b.WriteString(raw[prev:mt.Start])
		style := codeFindMatchStyle()
		if mi == currentIdx {
			style = codeFindCurrentStyle()
		}
		b.WriteString(style.Render(raw[mt.Start:mt.End]))
		prev = mt.End
	}
	b.WriteString(raw[prev:])
	return b.String()
}

// Match styles resolve at call time (theme can change at runtime).
func codeFindMatchStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(theme.Yellow).Foreground(theme.Bg)
}

func codeFindCurrentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(theme.Orange).Foreground(theme.Bg).Bold(true)
}
