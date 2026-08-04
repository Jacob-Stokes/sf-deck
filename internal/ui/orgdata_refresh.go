package ui

// Global refresh — re-fetch every ALREADY-LOADED resource for the
// active org (bound to ctrl+r). The middle ground between `r` (refresh
// just the current view) and the cache modal's `C` (wipe everything +
// reload only the current view): "everything I've looked at in this org
// is stale — go get the current server version", without eagerly
// pulling things I've never opened or touching other orgs.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
)

// loadedResources returns every Resource on this org that has been
// loaded (cache or fetch), as the type-erased Refreshable view.
//
// Maintenance: the guard test TestLoadedResourcesCoversTopLevel asserts
// every top-level `Resource[` field declared in orgdata_resources.go is
// listed here, so a newly-added resource can't silently miss global
// refresh. Keyed per-(sobject/id) collections are ranged generically.
func (d *orgData) loadedResources() []resource.Refreshable {
	if d == nil {
		return nil
	}
	var out []resource.Refreshable
	add := func(rs ...resource.Refreshable) {
		for _, r := range rs {
			if r != nil && r.Loaded() {
				out = append(out, r)
			}
		}
	}

	// Registered list resources (see list_resource_registrations.go) are
	// enumerated from the registry — no manual line per resource.
	for _, key := range listResourceOrder {
		add(listResourceHandlers[key].loadedResource(d))
	}
	// Resources NOT on the list-resource registry (bespoke apply, non-[]T
	// shape, or not a list surface) stay explicit here.
	add(
		&d.SObjects, &d.PermSets, &d.PSGs, &d.Profiles,
		&d.PermissionSets, &d.Deploys, &d.Notifications,
		&d.RecentlyViewed, &d.Home, &d.OrgInfo,
	)

	// Keyed per-(sobject / id) collections — refresh every loaded entry.
	for _, r := range d.Describes {
		add(r)
	}
	for _, r := range d.CustomObjectBaselines {
		add(r)
	}
	for _, r := range d.FLS {
		add(r)
	}
	for _, r := range d.ObjectPerms {
		add(r)
	}
	for _, r := range d.SystemPerms {
		add(r)
	}
	for _, r := range d.AssignedUsers {
		add(r)
	}
	for _, r := range d.GroupMembers {
		add(r)
	}
	for _, r := range d.UserSessions {
		add(r)
	}
	for _, r := range d.CommunityPages {
		add(r)
	}
	for _, r := range d.ApexClassDetail {
		add(r)
	}
	for _, r := range d.LWCDetail {
		add(r)
	}
	for _, r := range d.AuraDetail {
		add(r)
	}
	for _, r := range d.FlowVersions {
		add(r)
	}
	for _, r := range d.Records {
		add(r)
	}
	for _, r := range d.ChipRecords {
		add(r)
	}
	for _, r := range d.RecordDetails {
		add(r)
	}
	for _, r := range d.RecordReferenceNames {
		add(r)
	}
	for _, r := range d.RecordChildCounts {
		add(r)
	}
	for _, r := range d.ListViewsPerSObject {
		add(r)
	}
	for _, r := range d.ListViewResults {
		add(r)
	}
	for _, r := range d.RecentlyViewedPerSObject {
		add(r)
	}
	for _, r := range d.ReportRuns {
		add(r)
	}
	for _, r := range d.ChipUsers {
		add(r)
	}
	return out
}

// refreshAllLoaded re-fetches every loaded resource for the active org
// (bypassing TTL), drops the per-frame projection/gutter caches so the
// display rebuilds cleanly, and flashes the count. Cold resources and
// other orgs are left untouched.
func (m *Model) refreshAllLoaded() (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return *m, m.orgsRes.Refresh(m.cache)
	}
	o := m.orgs[m.selected]
	if !canUseOrg(o) {
		return *m, nil
	}
	d := m.ensureOrgData(o.Username)
	res := d.loadedResources()
	if len(res) == 0 {
		m.flash("nothing loaded yet to refresh")
		return *m, nil
	}
	cmds := make([]tea.Cmd, 0, len(res))
	for _, r := range res {
		if c := r.Refresh(m.cache); c != nil {
			cmds = append(cmds, c)
		}
	}
	// Clean repaint: drop the per-render gutter/projection cache so the
	// refreshed data isn't masked by a stale render-side cache. It
	// re-allocates lazily on the next render (ensureGutterCache).
	d.gutterCache = nil
	m.flash(refreshCountMsg(len(cmds)))
	return *m, tea.Batch(cmds...)
}

func refreshCountMsg(n int) string {
	if n == 1 {
		return "refreshing 1 resource…"
	}
	return "refreshing " + itoa(n) + " resources…"
}
