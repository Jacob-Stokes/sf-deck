package ui

// Demo-org injection. The welcome modal's "import demo org" action pours
// the existing demo fixture world (the same data `--demo` seeds) into the
// REAL cache under the demo usernames, registers those usernames as demo
// targets in the sf layer (so any live-call attempt short-circuits to
// seeded cache), and merges the demo orgs into the live org list. Unlike
// `--demo` this coexists with real orgs in one session: real orgs fetch
// live, the demo org serves seed.
//
// Persistence: a settings flag (ui.demo_org_imported) records the import
// so the demo org is re-seeded + re-registered on the next boot. Removal
// clears the flag, unregisters the targets, and purges the demo cache
// namespaces.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// demoOrgUsernames returns the canonical usernames of the injected demo
// orgs — the cache namespaces their data lives under and the targets the
// sf layer treats as demo.
func demoOrgUsernames() []string {
	us := make([]string, 0, len(demoOrgs()))
	for _, o := range demoOrgs() {
		us = append(us, o.Username)
	}
	return us
}

// demoTargetKeys returns every alias AND username for the demo orgs, so
// registration covers whichever identifier a call site passes to
// RESTClient.
func demoTargetKeys() []string {
	keys := make([]string, 0, len(demoOrgs())*2)
	for _, o := range demoOrgs() {
		if o.Alias != "" {
			keys = append(keys, o.Alias)
		}
		if o.Username != "" {
			keys = append(keys, o.Username)
		}
	}
	return keys
}

// registerDemoTargets marks the demo orgs as demo in the sf layer. Called
// on import and on boot when the demo org persists.
func registerDemoTargets() {
	sf.RegisterDemoTargets(demoTargetKeys()...)
}

// importDemoOrg seeds the demo world into the real cache, registers the
// demo targets, flips the persistent flag, and merges the demo orgs into
// the live list. Idempotent: re-importing just re-seeds. Returns a flash
// describing the outcome.
func (m *Model) importDemoOrg() {
	if m.cache != nil {
		if err := seedDemoOrgData(m.cache, demoOrgs()); err != nil {
			m.flash("demo import failed: " + err.Error())
			return
		}
	}
	// Note: fixture DevProjects/tags are deliberately NOT seeded into the
	// user's real devprojects.db — that store is shared across all orgs
	// and seeding it would pollute the user's own projects. The demo org
	// shows its read surfaces (records/flows/apex/…) from cache; project
	// collection against it works like any other org.
	registerDemoTargets()
	m.settings.SetDemoOrgImported(true)
	_ = m.settings.Save()
	m.orgs = mergeDemoOrgs(m.orgs, m.settings.DemoOrgImported())
	m.flash("Demo org imported — find the 'northwind' orgs in your org panel.")
}

// mergeDemoOrgs returns orgs with the demo orgs appended when imported is
// true (de-duplicated by username), or orgs unchanged when false. Applied
// at every point m.orgs is refreshed from orgsRes so a live org reload
// doesn't drop the injected demo orgs.
func mergeDemoOrgs(orgs []sf.Org, imported bool) []sf.Org {
	if !imported {
		return orgs
	}
	have := make(map[string]bool, len(orgs))
	for _, o := range orgs {
		have[o.Username] = true
	}
	out := orgs
	for _, d := range demoOrgs() {
		if !have[d.Username] {
			out = append(out, d)
		}
	}
	return out
}

// removeDemoOrg tears down the injected demo org: unregisters the demo
// targets, clears the persistent flag, drops the demo orgs from the live
// list, and purges their cache namespaces. Their cache rows are cleanly
// keyed by the demo usernames, so the purge touches nothing real.
func (m *Model) removeDemoOrg() {
	sf.UnregisterDemoTargets(demoTargetKeys()...)
	m.settings.SetDemoOrgImported(false)
	_ = m.settings.Save()
	if m.cache != nil {
		for _, u := range demoOrgUsernames() {
			_, _ = m.cache.DeleteScope(u)
		}
	}
	demoSet := map[string]bool{}
	for _, u := range demoOrgUsernames() {
		demoSet[u] = true
		// Drop the in-memory orgData too — the disk cache purge above
		// doesn't touch m.data, which previously kept the demo's stale
		// UI state (cursors, lists) alive after removal.
		delete(m.data, u)
	}
	kept := m.orgs[:0:0]
	for _, o := range m.orgs {
		if !demoSet[o.Username] {
			kept = append(kept, o)
		}
	}
	m.orgs = kept
	// Re-anchor the selection properly. A bare `m.selected = 0` left
	// selectedUsername pointing at the removed demo org — a dangling
	// anchor the next orgs refetch had to correct by accident. Also
	// re-anchor when the SELECTED username was a demo org even if the
	// index still happens to be in range.
	if m.selected >= len(m.orgs) || demoSet[m.selectedUsername] {
		if len(m.orgs) > 0 {
			m.setSelectedOrg(0)
		} else {
			m.selected = 0
			m.selectedUsername = ""
		}
	}
	m.flash("Demo org removed.")
}

// restoreDemoOrgOnBoot re-registers the demo targets when the demo org
// persisted from a previous session, so its surfaces serve seed rather
// than attempting live calls on this launch. The org list merge happens
// wherever m.orgs is set (see mergeDemoOrgs). No cmd needed — the seed
// data is already in the cache from the original import.
func restoreDemoOrgOnBoot(imported bool) {
	if imported {
		registerDemoTargets()
	}
}
