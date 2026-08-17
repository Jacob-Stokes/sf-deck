package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type searchDebounceTickMsg struct{}

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

func (m *Model) captureRecordsCursorAnchor() {
	d, sobject := m.currentRecordsContext()
	if d == nil || sobject == "" {
		return
	}
	cur := d.Cursors.Peek(cursorKindRecordsRow, sobject)
	d.Cursors.Set(cursorKindRecordsAnchor, cur, 0, sobject)
}

func (m *Model) restoreRecordsCursorAnchor() {
	d, sobject := m.currentRecordsContext()
	if d == nil || sobject == "" {
		return
	}
	anchor := d.Cursors.Peek(cursorKindRecordsAnchor, sobject)
	d.Cursors.Set(cursorKindRecordsRow, anchor, 0, sobject)
	d.Cursors.Reset(cursorKindRecordsAnchor, sobject)
}

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
