package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type modelOrgs struct {
	orgsRes  Resource[[]sf.Org]
	orgs     []sf.Org // mirror of orgsRes.Value() for convenience
	selected int

	// selectedUsername is the source of truth for "which org is active"
	// across orgs-list refetches. m.selected is an int index INTO m.orgs,
	// which is fragile because the underlying slice can reorder (manual
	// rename / new group / future sort change) and the index would then
	// silently point at a different org. After every orgs Apply we
	// re-anchor m.selected to the row whose Username matches this field,
	// preserving the user's active context across the refresh.
	//
	// Set whenever m.selected is set (via setSelectedOrg). Empty until
	// the first successful orgs load.
	selectedUsername string

	pinnedDefaultRestored bool

	noOrgTab Tab

	data map[string]*orgData // per-org state keyed by username
}

func (m *Model) setSelectedOrg(i int) {
	if i < 0 || i >= len(m.orgs) {
		return
	}
	m.selected = i
	m.selectedUsername = m.orgs[i].Username
}
