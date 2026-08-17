package ui

// Org-lifecycle message dispatch.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m Model) dispatchOrgsMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case orgGroupsChangedMsg:
		// Persisted group state changed via a modal. Re-clamp the rail
		// cursor so it stays on a valid row, and let the next render
		// pick up the new shape (renderOrgsWidget reads settings live).
		_ = msg
		m.clampOrgRailCursor()
		return m, nil, true

	case orgsChangedMsg:
		// Authed-org list may have changed (login / logout / alias /
		// default). Refetch the resource that backs m.orgs; the
		// resource update msg flows through resource.UpdatedMsg
		// elsewhere and m.orgs is rebuilt there.
		_ = msg
		sf.InvalidateRESTClients()
		return m, m.orgsRes.Refresh(m.cache), true

	case orgLifecycleResultMsg:
		if msg.Message != "" {
			m.flash(msg.Message)
		}
		if msg.Refetch {
			sf.InvalidateRESTClients()
			return m, m.orgsRes.Refresh(m.cache), true
		}
		return m, nil, true
	}
	return m, nil, false
}
