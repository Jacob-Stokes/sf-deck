package sf

import "time"

// OnCallFunc is invoked once per completed API call (success OR
// failure — we count attempts, since every attempt is an API call
// as far as Salesforce is concerned). `alias` is the org the call
// went to; empty string for org-agnostic calls (e.g. `sf org list`).
// `dur` is wall-clock time from request start to fireOnCall; zero
// if the call site didn't measure (legacy sites are being migrated).
type OnCallFunc func(alias string, args []string, err error, dur time.Duration)

// OnCall is the registered hook. Nil by default = no tracking.
var OnCall OnCallFunc

func fireOnCall(alias string, args []string, err error, dur time.Duration) {
	if OnCall != nil {
		OnCall(alias, args, err, dur)
	}
}

// aliasFromArgs extracts the target-org alias from an sf CLI argv
// slice. Looks for -o <x> / --target-org <x>. Returns "" when not
// present (org-list, auth-list, etc).
func aliasFromArgs(args []string) string {
	for i, a := range args {
		if (a == "-o" || a == "--target-org") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
