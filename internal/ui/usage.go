package ui

import "time"

// UsageTracker is the minimal surface the UI needs from the usage
// package. We keep it as an interface so the ui/ package doesn't have
// to import internal/usage directly — main wires the concrete tracker.
type UsageTracker interface {
	Today() int
	TodayForOrg(alias string) int
	// TodayForOrgKeys sums an org's calls across its several keys (short
	// alias AND username) so the count reconciles regardless of which the
	// recording code path used.
	TodayForOrgKeys(aliases ...string) int
	Recent() []UsageCall
}

// UsageCall mirrors usage.Call so the ui/ package doesn't need to
// import usage just for the type. main wires an adapter from
// usage.Call -> ui.UsageCall.
type UsageCall struct {
	At      time.Time
	Alias   string
	Command string
	Args    []string
	OK      bool
	Err     string
	Caller  string
}

// Usage is the active tracker the header reads from. nil = no display.
// Set by main if tracker opening succeeded.
var Usage UsageTracker
