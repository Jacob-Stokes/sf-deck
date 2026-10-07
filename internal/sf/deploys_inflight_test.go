package sf

import "testing"

// InFlight gates which deploy rows the list re-polls: delta refresh
// skips existing rows by CreatedDate, so any non-terminal status
// missing from this set gets CACHED FOREVER at that status — across
// refresh and restart. It happened twice: InProgress (2026-06-12) and
// Finalizing (2026-07-18). This test pins the full non-terminal set so
// the next Salesforce status addition fails loudly here instead.
func TestDeployInFlightCoversNonTerminalStatuses(t *testing.T) {
	inFlight := []string{"Pending", "InProgress", "Canceling", "Finalizing"}
	for _, s := range inFlight {
		if !(DeployRow{Status: s}).InFlight() {
			t.Errorf("status %q must be in-flight — a cached row at this status would never be re-polled and would stick forever", s)
		}
	}
	terminal := []string{"Succeeded", "SucceededPartial", "Failed", "Canceled"}
	for _, s := range terminal {
		if (DeployRow{Status: s}).InFlight() {
			t.Errorf("terminal status %q must NOT be in-flight — the watcher would poll it forever", s)
		}
	}
}
