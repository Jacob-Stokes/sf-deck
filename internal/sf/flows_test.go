package sf

import "testing"

// TestAdoptNewerModified covers the reconciliation that fixed flows
// looking stale when a draft (or activation change) landed after the
// active version. Mirrors the edge case: active v3 @ 07-03, draft
// v4 @ 07-07 — the flow's modified date must become the v4 draft's.
func TestAdoptNewerModified(t *testing.T) {
	const (
		defDate    = "2026-07-07T11:36:06.000+0000" // FlowDefinition (activation-ish)
		activeDate = "2026-07-03T16:04:08.000+0000" // active v3
		latestDate = "2026-07-07T11:36:08.000+0000" // draft v4 (newest)
	)

	// Seed from the definition (as listFlowDefinitions now does).
	f := &Flow{LastModifiedDate: defDate, LastModifiedBy: "Gar Mun Hui"}

	active := FlowVersion{ID: "301A", LastModifiedDate: activeDate, LastModifiedBy: "Someone Else"}
	latest := FlowVersion{ID: "301B", LastModifiedDate: latestDate, LastModifiedBy: "Gar Mun Hui"}

	// Same order ListFlows applies them: active then latest.
	adoptNewerModified(f, active) // older than seed → no change
	if f.LastModifiedDate != defDate {
		t.Fatalf("active (older) overwrote seed: got %q", f.LastModifiedDate)
	}
	adoptNewerModified(f, latest) // newest → wins
	if f.LastModifiedDate != latestDate {
		t.Errorf("latest draft should win: got %q want %q", f.LastModifiedDate, latestDate)
	}
	if f.LastModifiedBy != "Gar Mun Hui" {
		t.Errorf("modified-by should follow the winning date: got %q", f.LastModifiedBy)
	}
}

func TestAdoptNewerModified_IgnoresEmpty(t *testing.T) {
	f := &Flow{LastModifiedDate: "2026-07-07T11:36:06.000+0000", LastModifiedBy: "A"}
	adoptNewerModified(f, FlowVersion{ID: "", LastModifiedDate: "2027-01-01T00:00:00.000+0000"}) // no ID
	adoptNewerModified(f, FlowVersion{ID: "301X", LastModifiedDate: ""})                         // no date
	if f.LastModifiedDate != "2026-07-07T11:36:06.000+0000" || f.LastModifiedBy != "A" {
		t.Errorf("empty candidates must not change f: got %q / %q", f.LastModifiedDate, f.LastModifiedBy)
	}
}
