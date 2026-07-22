package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// searchDebounceTickMsg fires after the configured debounce window
// has elapsed since the last search-buffer mutation. The Update
// handler sweeps every per-org per-surface search state and
// promotes Buffer→Effective for any state still flagged pending,
// which busts the projection caches and triggers one filter pass.
type searchDebounceTickMsg struct{}

// clearCommittedSearch resets the current view's search state when a
// committed (or active-with-text) filter is narrowing the list. Used
// by both `C` (Keys.SearchClear) and the top-level Esc fallback.
// Returns true when something was actually cleared so callers can
// short-circuit further key dispatch.
//
// For records views, also restores the pre-search cursor position
// from the anchor captured at SearchStart — so clearing returns the
// user to where they were, not wherever the filtered view's cursor
// happened to land.
func (m *Model) clearCommittedSearch() bool {
	if m.focus != focusMain {
		return false
	}
	s := m.currentSearch()
	if s == nil || !s.Applied() {
		return false
	}
	s.SetBuffer("")
	s.Committed = false
	s.Active = false
	m.resetCursorForCurrentView()
	m.restoreRecordsCursorAnchor()
	return true
}

// captureRecordsCursorAnchor stashes the current records cursor (in
// unfiltered coords) before search becomes active, so a later clear
// can restore it. No-op for non-records surfaces. Always overwrites
// — each `/` keypress re-anchors at the current visible position.
func (m *Model) captureRecordsCursorAnchor() {
	d, sobject := m.currentRecordsContext()
	if d == nil || sobject == "" {
		return
	}
	cur := d.Cursors.Peek(cursorKindRecordsRow, sobject)
	d.Cursors.Set(cursorKindRecordsAnchor, cur, 0, sobject)
}

// restoreRecordsCursorAnchor moves the records cursor back to the
// position captured at SearchStart, then drops the anchor. No-op for
// non-records surfaces or when no anchor was captured.
func (m *Model) restoreRecordsCursorAnchor() {
	d, sobject := m.currentRecordsContext()
	if d == nil || sobject == "" {
		return
	}
	anchor := d.Cursors.Peek(cursorKindRecordsAnchor, sobject)
	d.Cursors.Set(cursorKindRecordsRow, anchor, 0, sobject)
	d.Cursors.Reset(cursorKindRecordsAnchor, sobject)
}

// currentRecordsContext returns the (orgData, sobject) for whichever
// records surface the user is on right now — TabRecords' record-list
// mode or TabObjectDetail's records subtab. Returns (nil, "") for
// any other surface.
func (m *Model) currentRecordsContext() (*orgData, string) {
	if len(m.orgs) == 0 {
		return nil, ""
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	switch m.tab() {
	case TabRecords:
		if d.RecordsSObjectCur != "" {
			return d, d.RecordsSObjectCur
		}
	case TabObjectDetail:
		if d.DescribeCur != "" && m.currentSubtab() == SubtabRecords {
			return d, d.DescribeCur
		}
	}
	return nil, ""
}

// handleSearchInput captures keystrokes while a view's search buffer
// is open (`/`). Our state machine keys (esc commit-and-exit, enter
// commit, tab hand-off) stay hardcoded; typing + editing forward to
// the bubbles/textinput widget so the user gets cursor nav + word
// jumps.
//
// Esc here behaves the same as Enter: exits input mode but keeps the
// buffer as committed. The "clear the filter" gesture lives entirely
// on `C` (any level) and Esc-when-nothing-else-to-do (top level).
// Conflating "exit typing" with "throw away filter" surprised users
// who pressed Esc to stop typing without losing what they'd built up.
//
// After any text mutation, the view's cursor resets to the top of
// the filtered list so search-as-you-type works the way you'd expect.
func (m Model) handleSearchInput(msg tea.KeyMsg, s *searchState) (tea.Model, tea.Cmd) {
	s.EnsureInit()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "enter", "tab":
		s.Active = false
		s.Committed = s.Buffer() != ""
		return m, nil
	case "ctrl+u":
		s.Active = false
		s.Committed = false
		s.SetBuffer("")
		m.resetCursorForCurrentView()
		return m, nil
	}
	before := s.Buffer()
	newInput, cmd := s.Input.Update(msg)
	s.Input = newInput
	if s.Buffer() != before {
		m.resetCursorForCurrentView()
		// Tell the search state the buffer changed. Adaptive: if
		// the last filter was fast, NoteBufferChanged syncs Effective
		// immediately (no debounce, feels instant). If slow, marks
		// the state pending and we schedule a tick to promote it.
		threshold := m.settings.SearchFastFilterThresholdMs()
		s.NoteBufferChanged(threshold)
		if s.DebouncePending() {
			window := time.Duration(m.settings.SearchDebounceMs()) * time.Millisecond
			cmd = tea.Batch(cmd, tea.Tick(window, func(time.Time) tea.Msg {
				return searchDebounceTickMsg{}
			}))
		}
	}
	return m, cmd
}
