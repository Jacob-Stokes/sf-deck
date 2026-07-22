package settings

import "testing"

// The o-key open default is "direct": reuse the existing browser session
// rather than minting a fresh one via frontdoor. frontdoor re-triggers
// identity verification (passkeys) on strict orgs for sensitive entities
// (User, payment objects), so direct is the friendlier default. A flip
// back to "frontdoor" here would silently re-introduce that prompt.
func TestOpenAuthDefaultsToDirect(t *testing.T) {
	if got := (&Settings{}).OpenAuth(); got != "direct" {
		t.Fatalf("OpenAuth() default = %q, want \"direct\"", got)
	}
	// nil receiver takes the same default (used on the nil-settings path).
	var s *Settings
	if got := s.OpenAuth(); got != "direct" {
		t.Fatalf("nil OpenAuth() = %q, want \"direct\"", got)
	}
}

// An explicit choice still wins over the default.
func TestOpenAuthRespectsExplicit(t *testing.T) {
	s := &Settings{}
	s.SetOpenAuth("frontdoor")
	if got := s.OpenAuth(); got != "frontdoor" {
		t.Fatalf("OpenAuth() after SetOpenAuth(frontdoor) = %q, want \"frontdoor\"", got)
	}
}
