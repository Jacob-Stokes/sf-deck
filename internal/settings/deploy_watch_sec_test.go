package settings

import "testing"

// The /deploys live watch defaults to a 5s refresh (one tooling SOQL
// per tick while a deploy is in flight), with a 2s floor so a typo in
// deploy_watch_sec can't turn the watch into a query hammer.
func TestDeployWatchSecDefaultAndFloor(t *testing.T) {
	if got := (&Settings{}).APIDeployWatchSec(); got != 5 {
		t.Fatalf("APIDeployWatchSec() default = %d, want 5", got)
	}
	var s *Settings
	if got := s.APIDeployWatchSec(); got != 5 {
		t.Fatalf("nil APIDeployWatchSec() = %d, want 5", got)
	}
	low := &Settings{}
	low.SetAPIDeployWatchSec(1)
	if got := low.APIDeployWatchSec(); got != 2 {
		t.Fatalf("APIDeployWatchSec() with sub-floor value = %d, want floor 2", got)
	}
	custom := &Settings{}
	custom.SetAPIDeployWatchSec(15)
	if got := custom.APIDeployWatchSec(); got != 15 {
		t.Fatalf("APIDeployWatchSec() explicit = %d, want 15", got)
	}
}
