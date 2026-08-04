package ui

// Org-lifecycle message dispatch.
//
// Pairs with update_export_dispatch.go + update_modal_dispatch.go +
// update_perms_dispatch.go. Handles the callback messages that land
// when the authed-org set changes (login/logout/alias/default), when
// persisted group state mutates via a modal, or when a generic org-
// lifecycle action reports back.
//
// Note this is distinct from the User detail callbacks (which live in
// update_perms_dispatch.go) — "orgs" here means the rail's list of
// connected orgs, not Salesforce User records.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// dispatchOrgsMsg routes org-lifecycle callbacks. Returns (Model,
// tea.Cmd, bool); concrete Model is fine for every case here.
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
