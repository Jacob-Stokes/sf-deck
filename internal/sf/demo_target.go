package sf

// Per-org demo support. Distinct from the global DemoMode flag (which
// freezes the WHOLE data layer for `sf-deck --demo`): here a specific
// set of org targets is marked demo so they coexist with real,
// live-fetching orgs in the same session. An injected demo org reads
// entirely from its pre-seeded cache; any live-call attempt against it
// short-circuits at RESTClient with ErrDemoTarget instead of shelling
// out to `sf` (there's no auth behind a demo org, so the call could only
// fail with a confusing token error).

import (
	"errors"
	"sync"
)

// ErrDemoTarget is returned by RESTClient for a registered demo target.
// Callers (and the resource refresh path) treat it as "serve the seeded
// cache; this org has no live backend" rather than a real failure.
var ErrDemoTarget = errors.New("demo org: live Salesforce calls are disabled")

// IsDemoTargetErr reports whether err is (or wraps) ErrDemoTarget, so the
// UI can distinguish a demo no-op from a genuine fetch failure and avoid
// flashing an error banner over good seeded data.
func IsDemoTargetErr(err error) bool {
	return errors.Is(err, ErrDemoTarget)
}

var (
	demoTargetsMu sync.RWMutex
	// demoTargets holds every alias AND username that identifies a demo
	// org. Both are registered because different call sites pass either
	// (targetArg prefers alias; some paths pass the canonical username).
	demoTargets = map[string]bool{}
)

// RegisterDemoTargets marks the given aliases/usernames as demo targets.
// Idempotent. Called when the demo org is imported (and re-registered on
// boot when the demo org persists across restarts).
func RegisterDemoTargets(targets ...string) {
	demoTargetsMu.Lock()
	defer demoTargetsMu.Unlock()
	for _, t := range targets {
		if t != "" {
			demoTargets[t] = true
		}
	}
}

// UnregisterDemoTargets removes demo markers (demo-org removal path).
func UnregisterDemoTargets(targets ...string) {
	demoTargetsMu.Lock()
	defer demoTargetsMu.Unlock()
	for _, t := range targets {
		delete(demoTargets, t)
	}
}

// isDemoTarget reports whether the alias/username identifies a demo org.
func isDemoTarget(target string) bool {
	demoTargetsMu.RLock()
	defer demoTargetsMu.RUnlock()
	return demoTargets[target]
}

// IsDemoOrgTarget is the exported form used by the UI to gate sfdx-only
// actions (org-open, login-as, deploy) against an injected demo org,
// which has no live backend.
func IsDemoOrgTarget(target string) bool {
	return isDemoTarget(target)
}
