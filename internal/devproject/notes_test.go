package devproject

import "testing"

func newNoteStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNotes_RoundtripUpsertDelete(t *testing.T) {
	s := newNoteStore(t)
	const org = "user@example.com"

	// Missing note is "", not an error.
	if body, err := s.NoteFor(KindFlow, "300x", org); err != nil || body != "" {
		t.Fatalf("NoteFor(missing) = %q, %v; want \"\", nil", body, err)
	}

	// Set + read back.
	gen := s.Generation()
	if err := s.SetNote(KindFlow, "300x", org, "check batch size\nbefore go-live"); err != nil {
		t.Fatal(err)
	}
	if s.Generation() == gen {
		t.Error("SetNote must bump the store generation (memo invalidation)")
	}
	body, err := s.NoteFor(KindFlow, "300x", org)
	if err != nil || body != "check batch size\nbefore go-live" {
		t.Fatalf("NoteFor = %q, %v", body, err)
	}

	// One note per item: a second Set replaces, never appends.
	if err := s.SetNote(KindFlow, "300x", org, "updated"); err != nil {
		t.Fatal(err)
	}
	if body, _ := s.NoteFor(KindFlow, "300x", org); body != "updated" {
		t.Fatalf("after upsert NoteFor = %q, want \"updated\"", body)
	}

	// Org-scoped: same (kind, ref) in another org is a different note.
	if body, _ := s.NoteFor(KindFlow, "300x", "other@example.com"); body != "" {
		t.Fatalf("note leaked across orgs: %q", body)
	}

	// Blank body removes the note.
	if err := s.SetNote(KindFlow, "300x", org, "  \n\t "); err != nil {
		t.Fatal(err)
	}
	if body, _ := s.NoteFor(KindFlow, "300x", org); body != "" {
		t.Fatalf("blank SetNote should delete; NoteFor = %q", body)
	}
}

// Closed/nil store fails soft — the UI treats an error as "no note".
func TestNotes_ClosedStore(t *testing.T) {
	var s *Store
	if _, err := s.NoteFor(KindFlow, "x", ""); err == nil {
		t.Error("nil store NoteFor should error")
	}
	if err := s.SetNote(KindFlow, "x", "", "body"); err == nil {
		t.Error("nil store SetNote should error")
	}
}
