package ui

// Org list + per-org state map.
//
// Extracted from model.go. modelOrgs is embedded into Model so existing
// field access (m.orgs, m.selected, m.data, …) keeps working unchanged.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// modelOrgs owns the org list and the per-org state map.
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

	// pinnedDefaultRestored guards the "honour DefaultOrgUsername on
	// startup" branch in the orgs Apply handler. The previous heuristic
	// ("if m.selected == 0") fired again on later refetches when the
	// pinned org happened to land at index 0 — feeling like the
	// selection jumped. This flag flips true on the FIRST orgs load
	// and stays true for the rest of the session.
	pinnedDefaultRestored bool

	// noOrgTab is the active tab when no org is selected yet (startup or
	// empty org list). Once orgs exist, the active tab is per-org (see
	// orgData.Tab) so switching orgs preserves each org's context.
	noOrgTab Tab

	data map[string]*orgData // per-org state keyed by username
}

// setSelectedOrg sets the active-org index AND the username anchor in
// one call. Use this everywhere instead of `m.selected = i` so a future
// orgs refetch can re-locate the same org by username.
func (m *Model) setSelectedOrg(i int) {
	if i < 0 || i >= len(m.orgs) {
		return
	}
	m.selected = i
	m.selectedUsername = m.orgs[i].Username
}
