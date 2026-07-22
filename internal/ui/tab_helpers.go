package ui

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// currentOrg returns the selected org, if any.
func (m Model) currentOrg() (sf.Org, bool) {
	if len(m.orgs) == 0 {
		return sf.Org{}, false
	}
	return m.orgs[m.selected], true
}

// safetyFor returns the effective SafetyLevel for the given org,
// resolving through the loaded settings file + kind-based defaults.
// Passes both username and alias so users editing settings.toml by
// hand can use whichever key is more natural.
func (m Model) safetyFor(o sf.Org) settings.SafetyLevel {
	return m.settings.Resolve(o.Username, settings.OrgKind(o.Kind()), o.Alias)
}

// orgForTarget resolves an sf target string (alias or username) to an
// authed org row so safety checks can use the target that will actually
// receive a write, not necessarily the currently highlighted org.
func (m Model) orgForTarget(target string) (sf.Org, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return m.currentOrg()
	}
	for _, o := range m.orgs {
		if target == o.Username || target == o.Alias || target == targetArg(o) {
			return o, true
		}
	}
	return sf.Org{}, false
}

func (m Model) canWriteOrg(o sf.Org, k settings.WriteKind) (bool, string) {
	lvl := m.safetyFor(o)
	if lvl.Allows(k) {
		return true, ""
	}
	return false, o.Display() + " is " + lvl.String() + " — change in ~/.sf-deck/settings.toml"
}

// canWriteCurrent reports whether the currently-selected org allows
// the given WriteKind under the active safety policy.
func (m Model) canWriteCurrent(k settings.WriteKind) (bool, string) {
	o, ok := m.currentOrg()
	if !ok {
		return false, "no org selected"
	}
	return m.canWriteOrg(o, k)
}

// ensureOrgDataRef returns the orgData for key, allocating if missing.
// Kept as a read-side helper for renderers that accept Model by value —
// Update paths go through ensureOrgData on the pointer receiver first.
func (m Model) ensureOrgDataRef(key string) *orgData {
	target := m.targetForUsername(key)
	d, ok := m.data[key]
	if !ok || d.target != target {
		// Look up the alias for Fetch wiring. Callers that render should
		// almost never hit this (Update paths allocate first), but for
		// safety we create a usable orgData either way.
		d = newOrgData(key, target, m.cache, m.settings)
		m.data[key] = d
	}
	return d
}
