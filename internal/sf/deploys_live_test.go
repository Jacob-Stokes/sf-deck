package sf

import (
	"os"
	"testing"
)

// TestLiveRefreshDeploys is a manual diagnostic — only runs when
// SF_DECK_LIVE_ORG is set. Not part of the suite.
func TestLiveRefreshDeploys(t *testing.T) {
	org := os.Getenv("SF_DECK_LIVE_ORG")
	if org == "" {
		t.Skip("set SF_DECK_LIVE_ORG to run")
	}
	rows, err := RecentDeploys(org, 5)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	t.Logf("recent: %d rows", len(rows))
	if len(rows) == 0 {
		return
	}
	t.Logf("first: %s %s inflight=%v", rows[0].ID, rows[0].Status, rows[0].InFlight())
	fresh, err := RefreshDeploys(org, []string{rows[0].ID})
	t.Logf("refresh err: %v", err)
	for id, r := range fresh {
		t.Logf("fresh: %s %s", id, r.Status)
	}
}
