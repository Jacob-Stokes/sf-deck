package settings

import "testing"

// The flows-list `o` default is "latest": open the most recent version
// regardless of status, matching Setup's own flow list — when a draft
// is newer than the active version, opening the flow means editing that
// draft. A flip to "active" here would silently send users back to
// editing stale versions whenever a newer draft exists.
func TestFlowOpenVersionDefaultsToLatest(t *testing.T) {
	if got := (&Settings{}).FlowOpenVersion(); got != "latest" {
		t.Fatalf("FlowOpenVersion() default = %q, want \"latest\"", got)
	}
	// nil receiver takes the same default (used on the nil-settings path).
	var s *Settings
	if got := s.FlowOpenVersion(); got != "latest" {
		t.Fatalf("nil FlowOpenVersion() = %q, want \"latest\"", got)
	}
	// Unknown values fall back to the default rather than leaking a
	// nonsense mode into Flow.Targets().
	bad := &Settings{}
	bad.SetFlowOpenVersion("draft")
	if got := bad.FlowOpenVersion(); got != "latest" {
		t.Fatalf("FlowOpenVersion() with unknown value = %q, want \"latest\"", got)
	}
}

// An explicit choice still wins over the default.
func TestFlowOpenVersionRespectsExplicit(t *testing.T) {
	s := &Settings{}
	s.SetFlowOpenVersion("active")
	if got := s.FlowOpenVersion(); got != "active" {
		t.Fatalf("FlowOpenVersion() after SetFlowOpenVersion(active) = %q, want \"active\"", got)
	}
}
