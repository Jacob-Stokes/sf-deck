package ui

import "testing"

func TestCompareSetupRowsAlwaysPlainFive(t *testing.T) {
	for _, d := range []*orgData{
		func() *orgData { o := &orgData{}; o.Run = &compareRun{Phase: comparePhaseSetup}; return o }(),
		func() *orgData {
			o := &orgData{}
			o.Run = &compareRun{Phase: comparePhaseSetup, OriginSavedID: "cmp_1", OriginSavedName: "X"}
			return o
		}(),
	} {
		rows := compareSetupRowsFor(d)
		if len(rows) != 5 {
			t.Fatalf("setup rows = %d, want 5 (plain form)", len(rows))
		}
		if rows[0] != setupRowSource || rows[len(rows)-1] != setupRowCompare {
			t.Errorf("row order wrong: %v", rows)
		}
	}
}

func TestCompareEditModalToggleAndSeed(t *testing.T) {
	st := &compareEditModalState{OriginID: "cmp_1", OriginName: "X"}
	if st.SaveAsNew {
		t.Error("default edit modal should overwrite, not save-as-new")
	}
	st.SaveAsNew = !st.SaveAsNew // simulate Enter on the Save (mode) row
	if !st.SaveAsNew {
		t.Error("toggle should flip to save-as-new")
	}
	if len(compareEditRows()) != 6 {
		t.Fatalf("edit-modal rows = %d, want 6", len(compareEditRows()))
	}
}
