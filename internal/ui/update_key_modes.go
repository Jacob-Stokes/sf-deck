package ui

import tea "charm.land/bubbletea/v2"

func (m Model) handleInputModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.soqlEditing {
		next, cmd := m.handleSOQLKey(msg)
		return next, cmd, true
	}
	if m.execEditing {
		next, cmd := m.handleExecKey(msg)
		return next, cmd, true
	}
	if _, session := m.currentEditSession(); session != nil && session.Editing != nil {
		next, cmd := m.handleRecordEditKey(msg, session)
		return next, cmd, true
	}
	if m.focus == focusMain {
		if s := m.currentSearch(); s != nil && s.Active {
			next, cmd := m.handleSearchInput(msg, s)
			return next, cmd, true
		}
		if m.codeFindInputActive() {
			if next, cmd, ok := m.handleCodeFindInput(msg); ok {
				return next, cmd, true
			}
		}
	}
	return m, nil, false
}

func (m Model) handlePreGlobalTabKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.focus != focusMain {
		return m, nil, false
	}
	if mm, cmd, ok := m.onHomeDestinationsKey(key); ok {
		return mm, cmd, true
	}
	if mm, cmd, ok := m.onRecordDetailFindKey(key); ok {
		return mm, cmd, true
	}
	if mm, cmd, ok := m.onCodeViewKey(key); ok {
		return mm, cmd, true
	}
	if m.onHomeDownloadsKey(key) {
		return m, nil, true
	}
	if consumed, cmd := m.onBundlesKey(key); consumed {
		return m, cmd, true
	}
	if consumed, cmd := m.onBundleDetailKey(key); consumed {
		return m, cmd, true
	}
	return m, nil, false
}
