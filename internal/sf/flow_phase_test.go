package sf

import (
	"sort"
	"testing"
)

// TestFlowPhaseRank pins the Flow-Trigger-Explorer phase ordering that
// ListObjectFlows sorts by: before-save → after-save (sync) → after-save
// (async) → before-delete → scheduled → platform-event → other. The
// async split is the key one — it comes from HasAsyncPath, so the rank
// takes the whole row.
func TestFlowPhaseRank(t *testing.T) {
	ranked := []ObjectFlowRow{
		{TriggerType: "RecordBeforeSave"},
		{TriggerType: "RecordAfterSave"},
		{TriggerType: "RecordAfterSave", HasAsyncPath: true},
		{TriggerType: "RecordBeforeDelete"},
		{TriggerType: "Scheduled"},
		{TriggerType: "PlatformEvent"},
		{TriggerType: "Whatever"},
	}
	for i := 1; i < len(ranked); i++ {
		if FlowPhaseRank(ranked[i-1]) >= FlowPhaseRank(ranked[i]) {
			t.Errorf("row %d (%s async=%v) should rank before row %d (%s async=%v)",
				i-1, ranked[i-1].TriggerType, ranked[i-1].HasAsyncPath,
				i, ranked[i].TriggerType, ranked[i].HasAsyncPath)
		}
	}
}

// TestFlowPhaseAsyncSplit: two after-save flows differing only by
// HasAsyncPath land in different phase groups (Actions vs Run Async),
// with async ordered after sync.
func TestFlowPhaseAsyncSplit(t *testing.T) {
	sync := ObjectFlowRow{TriggerType: "RecordAfterSave", HasAsyncPath: false}
	async := ObjectFlowRow{TriggerType: "RecordAfterSave", HasAsyncPath: true}
	if FlowPhaseRank(sync) == FlowPhaseRank(async) {
		t.Error("async after-save flows must group separately from sync ones")
	}
	if FlowPhaseRank(async) <= FlowPhaseRank(sync) {
		t.Error("Run Asynchronously should come after Actions and Related Records")
	}
}

// TestFlowPhaseSortByTriggerOrder: within a phase, rows sort by the real
// TriggerOrder (execution sequence), stable on ties.
func TestFlowPhaseSortByTriggerOrder(t *testing.T) {
	rows := []ObjectFlowRow{
		{Label: "third", TriggerType: "RecordAfterSave", TriggerOrder: 50, HasTriggerOrder: true},
		{Label: "first", TriggerType: "RecordAfterSave", TriggerOrder: 10, HasTriggerOrder: true},
		{Label: "second-a", TriggerType: "RecordAfterSave", TriggerOrder: 20, HasTriggerOrder: true},
		{Label: "second-b", TriggerType: "RecordAfterSave", TriggerOrder: 20, HasTriggerOrder: true},
		{Label: "before", TriggerType: "RecordBeforeSave"},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := FlowPhaseRank(rows[i]), FlowPhaseRank(rows[j])
		if pi != pj {
			return pi < pj
		}
		return rows[i].TriggerOrder < rows[j].TriggerOrder
	})
	want := []string{"before", "first", "second-a", "second-b", "third"}
	for i, w := range want {
		if rows[i].Label != w {
			t.Errorf("row %d = %q, want %q (order: %v)", i, rows[i].Label, w, flowLabels(rows))
		}
	}
}

func flowLabels(rows []ObjectFlowRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Label
	}
	return out
}
