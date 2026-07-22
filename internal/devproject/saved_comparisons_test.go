package devproject

import (
	"bytes"
	"testing"
)

func TestSavedComparisonRoundTrip(t *testing.T) {
	st := openTestStore(t)
	blob := []byte{0x1f, 0x8b, 0x08, 0x00, 0xde, 0xad, 0xbe, 0xef} // pretend gzip
	saved, err := st.SaveComparison(SavedComparison{
		Name:   "Test→Dev code",
		Source: "acme-test",
		Target: "acme-dev",
		Scope:  "ApexClass, Flow",
		Method: "Auto",
		Blob:   blob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.CreatedAt.IsZero() {
		t.Fatalf("id/timestamp not stamped: %+v", saved)
	}

	// List omits the blob.
	list, err := st.ListSavedComparisons()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Test→Dev code" || list[0].Source != "acme-test" {
		t.Fatalf("list wrong: %+v", list)
	}
	if list[0].Blob != nil {
		t.Error("ListSavedComparisons should not load the blob")
	}

	// Get includes the blob, byte-identical.
	got, err := st.GetSavedComparison(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Blob, blob) {
		t.Errorf("blob not preserved: got %v want %v", got.Blob, blob)
	}
	if got.Scope != "ApexClass, Flow" || got.Method != "Auto" {
		t.Errorf("scalar fields wrong: %+v", got)
	}

	// Rename.
	if err := st.RenameSavedComparison(saved.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.GetSavedComparison(saved.ID)
	if got2.Name != "Renamed" {
		t.Errorf("rename failed: %q", got2.Name)
	}

	// Delete.
	if err := st.DeleteSavedComparison(saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSavedComparison(saved.ID); err != ErrSavedComparisonNotFound {
		t.Errorf("expected NotFound after delete, got %v", err)
	}
}

func TestUpdateComparisonOverwrites(t *testing.T) {
	st := openTestStore(t)
	saved, err := st.SaveComparison(SavedComparison{
		Name: "X", Source: "a", Target: "b", Scope: "ApexClass", Method: "Auto",
		Blob: []byte("old"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateComparison(saved.ID, "a", "b", "ApexClass, Flow", "Tooling", []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSavedComparison(saved.ID)
	if string(got.Blob) != "new" || got.Scope != "ApexClass, Flow" || got.Method != "Tooling" {
		t.Errorf("overwrite didn't take: %+v", got)
	}
	if got.Name != "X" {
		t.Errorf("name should be unchanged by UpdateComparison: %q", got.Name)
	}
	// Unknown id → not found.
	if err := st.UpdateComparison("nope", "", "", "", "", nil); err != ErrSavedComparisonNotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestSavedComparisonEmptyNameRejected(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveComparison(SavedComparison{Name: "   "}); err != ErrSavedComparisonEmpty {
		t.Errorf("expected ErrSavedComparisonEmpty, got %v", err)
	}
}
