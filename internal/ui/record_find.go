package ui

// record_find.go — find-next semantics for TabRecordDetail.
//
// Find-NEXT, not filter. The full field list stays visible; / opens
// a small input pill, each keystroke live-jumps the cursor to the
// next matching field, Enter commits, n / N cycle matches, esc
// dismisses (cursor stays where it landed).
//
// Matches against three slots per field, case-insensitive substring:
//   1. API name (`Account__c`)
//   2. Human label from describe (`Account`)
//   3. Stringified value (`Acme Holdings Ltd`)
//
// Use cases:
//   - "get me to OwnerId"        — match api name
//   - "where's the Status field" — match label
//   - "which field has Acme?"    — match value

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// recordFindBuffer reads the current find buffer for the active
// record. Empty when find isn't active.
func (m Model) recordFindBuffer() string {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" || d.RecordFindBuffer == nil {
		return ""
	}
	return d.RecordFindBuffer[d.RecordDetailCur]
}

// recordFindActive reports whether the find pill is in editing-
// focus state for the active record.
func (m Model) recordFindActive() bool {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" || d.RecordFindActive == nil {
		return false
	}
	return d.RecordFindActive[d.RecordDetailCur]
}

// openRecordFind transitions the active record into find-input
// mode. Resets the buffer if find wasn't already active so each
// "press /" starts fresh; otherwise re-focuses the existing buffer
// so the user can refine.
func (m *Model) openRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		d.RecordFindBuffer = map[string]string{}
	}
	if d.RecordFindActive == nil {
		d.RecordFindActive = map[string]bool{}
	}
	if !d.RecordFindActive[d.RecordDetailCur] {
		// Fresh open — clear any stale buffer.
		d.RecordFindBuffer[d.RecordDetailCur] = ""
	}
	d.RecordFindActive[d.RecordDetailCur] = true
}

// closeRecordFind exits find-input mode but PRESERVES the buffer
// so n / N cycling keeps working. Esc semantics: "I'm done typing,
// not done finding." Use clearRecordFind (bound to C) to actually
// wipe the buffer.
func (m *Model) closeRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
}

// clearRecordFind wipes the find buffer + exits input mode. Bound
// to capital C from idle state; from input mode, also reachable via
// the C key (won't be swallowed as a literal — the input handler
// only accepts printable lowercase + symbols for typing).
func (m *Model) clearRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
	if d.RecordFindBuffer != nil {
		d.RecordFindBuffer[d.RecordDetailCur] = ""
	}
}

// recordFindHint returns a single-line pill describing the
// current find state, or "" when find is inert. The pill shows
// "find: <query>_" when typing (with a cursor caret), and
// "find: <query> · n/N cycle · esc clear" after Enter commits.
func (m Model) recordFindHint() string {
	buf := m.recordFindBuffer()
	if buf == "" && !m.recordFindActive() {
		return ""
	}
	if m.recordFindActive() {
		// Editing mode: show buffer + caret. Pill in yellow so it
		// reads "active input mode" against the surrounding muted
		// text.
		body := "  find: " + buf + "_"
		return lipgloss.NewStyle().Foreground(theme.Yellow).Render(body) +
			lipgloss.NewStyle().Foreground(theme.FgDim).Render("   ↵ commit · esc exit · C clear")
	}
	// Committed: cycle hint.
	body := "  find: " + buf
	return lipgloss.NewStyle().Foreground(theme.Yellow).Render(body) +
		lipgloss.NewStyle().Foreground(theme.FgDim).Render("   n / N cycle · C clear")
}

// onRecordDetailFindKey is the pre-global key handler for find on
// TabRecordDetail. Two modes:
//
//   - When find is ACTIVE (input has focus) — keystrokes go into
//     the buffer + re-jump the cursor live.
//   - When find is INACTIVE — / opens find, n cycles to the next
//     match against the existing buffer (if any), N to previous.
//
// Returns (m, cmd, true) when the key was consumed; (m, nil, false)
// when the caller should fall through to global dispatch.
func (m Model) onRecordDetailFindKey(key string) (Model, tea.Cmd, bool) {
	if m.tab() != TabRecordDetail {
		return m, nil, false
	}
	if m.recordFindActive() {
		switch key {
		case "esc":
			// Esc preserves the buffer — exit input mode but keep
			// the cycle hint live so n / N still work.
			mm := m
			(&mm).closeRecordFind()
			return mm, nil, true
		case "enter":
			mm := m
			(&mm).commitRecordFind()
			return mm, nil, true
		case "backspace":
			mm := m
			(&mm).backspaceRecordFind()
			return mm, nil, true
		case "C":
			// Capital C clears the buffer + exits input mode. Wins
			// over the printable-append fallback below so it never
			// gets appended as a literal "C".
			mm := m
			(&mm).clearRecordFind()
			return mm, nil, true
		}
		// Single printable character — append to buffer.
		if len(key) == 1 && key[0] >= 0x20 && key[0] < 0x7f {
			mm := m
			(&mm).appendRecordFindRune(rune(key[0]))
			return mm, nil, true
		}
		// Anything else (arrow keys, ctrl combos) — let the global
		// handler process it. The find pill stays open with its
		// current buffer.
		return m, nil, false
	}
	// Inactive: / opens; n / N cycle matches against existing buffer;
	// C clears any committed buffer.
	switch key {
	case "/":
		mm := m
		(&mm).openRecordFind()
		return mm, nil, true
	case "n":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).jumpToRecordFindMatch(false)
		return mm, nil, true
	case "N":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).jumpToRecordFindPrev()
		return mm, nil, true
	case "C":
		if m.recordFindBuffer() == "" {
			return m, nil, false
		}
		mm := m
		(&mm).clearRecordFind()
		return mm, nil, true
	}
	return m, nil, false
}

// commitRecordFind exits editing mode but keeps the buffer so n /
// N can cycle matches. Fires on Enter inside the find pill.
func (m *Model) commitRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindActive != nil {
		d.RecordFindActive[d.RecordDetailCur] = false
	}
}

// appendRecordFindRune adds one character to the buffer + jumps
// the cursor to the first match. Called from the find-input key
// handler on every typed key.
func (m *Model) appendRecordFindRune(r rune) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		d.RecordFindBuffer = map[string]string{}
	}
	d.RecordFindBuffer[d.RecordDetailCur] += string(r)
	m.jumpToRecordFindMatch(true)
}

// backspaceRecordFind removes one character from the buffer +
// re-runs the search. Called on backspace inside the find pill.
func (m *Model) backspaceRecordFind() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	if d.RecordFindBuffer == nil {
		return
	}
	buf := d.RecordFindBuffer[d.RecordDetailCur]
	if buf == "" {
		return
	}
	d.RecordFindBuffer[d.RecordDetailCur] = buf[:len(buf)-1]
	m.jumpToRecordFindMatch(true)
}

// jumpToRecordFindMatch moves the cursor to the next field
// matching the current buffer. fromCurrent: when true, search
// starts at the current cursor position (find-next semantics);
// when false, starts after the current cursor (n / N cycle).
//
// No-op when the buffer is empty or no fields match.
func (m *Model) jumpToRecordFindMatch(fromCurrent bool) {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	query := ""
	if d.RecordFindBuffer != nil {
		query = d.RecordFindBuffer[d.RecordDetailCur]
	}
	if strings.TrimSpace(query) == "" {
		return
	}
	r := d.RecordDetails[d.RecordDetailCur]
	if r == nil || r.FetchedAt().IsZero() {
		return
	}
	sobj, _ := splitRecordKey(d.RecordDetailCur)
	sfDescribe, labels := recordFindDescribe(d, sobj)
	fields := orderedRecordFieldsWithDescribe(r.Value(), sfDescribe)
	if len(fields) == 0 {
		return
	}
	cur := ""
	if d.RecordFieldCursor != nil {
		cur = d.RecordFieldCursor[d.RecordDetailCur]
	}
	curIdx := indexOfString(fields, cur)
	startIdx := 0
	if curIdx >= 0 {
		startIdx = curIdx
		if !fromCurrent {
			startIdx = curIdx + 1
		}
	}
	rec := r.Value()
	q := strings.ToLower(query)
	for offset := 0; offset < len(fields); offset++ {
		i := (startIdx + offset) % len(fields)
		if recordFieldMatchesQuery(fields[i], rec, labels, q) {
			if d.RecordFieldCursor == nil {
				d.RecordFieldCursor = map[string]string{}
			}
			d.RecordFieldCursor[d.RecordDetailCur] = fields[i]
			return
		}
	}
}

// jumpToRecordFindPrev is the n / N reverse direction. Same as
// jumpToRecordFindMatch but walks backwards.
func (m *Model) jumpToRecordFindPrev() {
	d := m.activeOrgData()
	if d == nil || d.RecordDetailCur == "" {
		return
	}
	query := ""
	if d.RecordFindBuffer != nil {
		query = d.RecordFindBuffer[d.RecordDetailCur]
	}
	if strings.TrimSpace(query) == "" {
		return
	}
	r := d.RecordDetails[d.RecordDetailCur]
	if r == nil || r.FetchedAt().IsZero() {
		return
	}
	sobj, _ := splitRecordKey(d.RecordDetailCur)
	sfDescribe, labels := recordFindDescribe(d, sobj)
	fields := orderedRecordFieldsWithDescribe(r.Value(), sfDescribe)
	if len(fields) == 0 {
		return
	}
	cur := ""
	if d.RecordFieldCursor != nil {
		cur = d.RecordFieldCursor[d.RecordDetailCur]
	}
	curIdx := indexOfString(fields, cur)
	startIdx := curIdx - 1
	if startIdx < 0 {
		startIdx = len(fields) - 1
	}
	rec := r.Value()
	q := strings.ToLower(query)
	for offset := 0; offset < len(fields); offset++ {
		i := (startIdx - offset + len(fields)) % len(fields)
		if recordFieldMatchesQuery(fields[i], rec, labels, q) {
			if d.RecordFieldCursor == nil {
				d.RecordFieldCursor = map[string]string{}
			}
			d.RecordFieldCursor[d.RecordDetailCur] = fields[i]
			return
		}
	}
}

// recordFieldMatchesQuery checks whether a field matches the
// lowercase find query. Hits any of API name, human label, or
// stringified value (case-insensitive substring).
func recordFieldMatchesQuery(fieldName string, rec map[string]any, labels map[string]string, q string) bool {
	if strings.Contains(strings.ToLower(fieldName), q) {
		return true
	}
	if label, ok := labels[fieldName]; ok && label != "" {
		if strings.Contains(strings.ToLower(label), q) {
			return true
		}
	}
	if v, ok := rec[fieldName]; ok && v != nil {
		val := formatCell(v)
		if strings.Contains(strings.ToLower(val), q) {
			return true
		}
	}
	return false
}

// recordFindDescribe returns (sObject describe ptr, label map)
// for the parent sObject. The describe pointer drives the
// shared field-ordering helper; the label map is the find
// matcher's per-field human-label slot. Both nil/empty when
// the describe isn't cached.
func recordFindDescribe(d *orgData, sobject string) (*sf.SObjectDescribe, map[string]string) {
	if d == nil || sobject == "" {
		return nil, nil
	}
	r, ok := d.Describes[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return nil, nil
	}
	desc := r.Value()
	labels := map[string]string{}
	for _, f := range desc.Fields {
		if f.Label != "" && f.Label != f.Name {
			labels[f.Name] = f.Label
		}
	}
	return &desc, labels
}
