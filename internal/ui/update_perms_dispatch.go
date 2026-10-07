package ui

// User + permission write-result message dispatch.

import (
	tea "charm.land/bubbletea/v2"
)

// dispatchPermsMsg routes user-fetch and permission-write callbacks.
func (m Model) dispatchPermsMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case userFetchedMsg:
		return m, m.applyUserFetched(msg), true
	case userActionDoneMsg:
		return m, m.applyUserActionDone(msg), true

	// --- Permission writes ----------------------------------------------
	case flsWriteDoneMsg:
		mm, cmd := m.applyFLSWriteDone(msg)
		return mm, cmd, true
	case objPermWriteDoneMsg:
		mm, cmd := m.applyObjPermWriteDone(msg)
		return mm, cmd, true
	case sysPermWriteDoneMsg:
		mm, cmd := m.applySysPermWriteDone(msg)
		return mm, cmd, true
	}
	return m, nil, false
}
