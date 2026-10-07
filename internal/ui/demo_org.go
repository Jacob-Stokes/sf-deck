package ui

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func demoOrgUsernames() []string {
	us := make([]string, 0, len(demoOrgs()))
	for _, o := range demoOrgs() {
		us = append(us, o.Username)
	}
	return us
}

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
		delete(m.data, u)
	}
	kept := m.orgs[:0:0]
	for _, o := range m.orgs {
		if !demoSet[o.Username] {
			kept = append(kept, o)
		}
	}
	m.orgs = kept
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

func restoreDemoOrgOnBoot(imported bool) {
	if imported {
		registerDemoTargets()
	}
}
