package ui

// User + permission write-result message dispatch.
//
// Pairs with update_export_dispatch.go + update_modal_dispatch.go.
// Handles the callback messages that land after user-related fetches
// + perm writes: userFetched (User detail panel hydration),
// userActionDone (flash + refresh after a User mutation), and the
// three perm-write done callbacks (FLS, object perms, system perms).
//
// Returns (tea.Model, tea.Cmd, bool) — same shape as the modal
// dispatcher because some apply* methods return tea.Model.

import (
	tea "charm.land/bubbletea/v2"
)

// dispatchPermsMsg routes user-fetch and permission-write callbacks.
func (m Model) dispatchPermsMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	// --- User detail panel + actions ------------------------------------
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
