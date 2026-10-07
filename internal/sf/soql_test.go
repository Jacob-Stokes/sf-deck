package sf

import "testing"

func TestCapQueryResult_TruncatesAndMarksNotDone(t *testing.T) {
	in := QueryResult{
		Records: []map[string]any{
			{"Id": "001A"},
			{"Id": "001B"},
			{"Id": "001C"},
		},
		TotalSize: 3,
		Done:      true,
	}

	got := capQueryResult(in, 2)
	if len(got.Records) != 2 {
		t.Fatalf("len(Records) = %d, want 2", len(got.Records))
	}
	if got.Done {
		t.Error("Done = true, want false after cap truncation")
	}
	if got.TotalSize != 3 {
		t.Errorf("TotalSize = %d, want 3", got.TotalSize)
	}
}

func TestCapQueryResult_NoopsWhenCapDisabledOrLarger(t *testing.T) {
	in := QueryResult{
		Records:   []map[string]any{{"Id": "001A"}},
		TotalSize: 1,
		Done:      true,
	}

	for _, cap := range []int{0, -1, 1, 2} {
		got := capQueryResult(in, cap)
		if len(got.Records) != 1 {
			t.Errorf("cap %d len(Records) = %d, want 1", cap, len(got.Records))
		}
		if !got.Done {
			t.Errorf("cap %d Done = false, want true", cap)
		}
	}
}
