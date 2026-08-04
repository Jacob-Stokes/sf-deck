package ui

import "testing"

// TestFlowVersionsDrillRefreshLatch pins the drill-refresh behaviour:
// drilling into a new flow refreshes once; returning to the SAME flow
// (esc-back from the version viewer) doesn't re-fetch; resetting the
// latch (back on the flows list) makes the next drill refresh again —
// even for a flow just visited (picks up Salesforce-side changes).
func TestFlowVersionsDrillRefreshLatch(t *testing.T) {
	d := &orgData{}

	// Drill into flow A → refresh.
	d.FlowCur = "A"
	if !d.takeFlowVersionsDrillRefresh() {
		t.Fatal("first drill into A should refresh")
	}
	// Return to A (same flow) → no re-fetch.
	if d.takeFlowVersionsDrillRefresh() {
		t.Error("returning to the same flow should not refresh")
	}
	// Drill into B → refresh.
	d.FlowCur = "B"
	if !d.takeFlowVersionsDrillRefresh() {
		t.Error("drilling into a different flow should refresh")
	}
	// Back on the flows list clears the latch → re-drill A refreshes.
	d.flowVersionsLoadedFor = "" // what ensureFlowsListData does
	d.FlowCur = "A"
	if !d.takeFlowVersionsDrillRefresh() {
		t.Error("after visiting the list, re-drilling a flow should refresh (SF may have changed)")
	}
}
