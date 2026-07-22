package diff

import "testing"

func TestLinesIdentical(t *testing.T) {
	r := Lines([]string{"a", "b", "c"}, []string{"a", "b", "c"})
	if r.Changed() {
		t.Fatalf("identical inputs reported changed: %+v", r)
	}
	if len(r.Lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(r.Lines))
	}
	for _, l := range r.Lines {
		if l.Op != OpEqual {
			t.Errorf("line %q op = %v, want equal", l.Text, l.Op)
		}
	}
}

func TestLinesPureInsert(t *testing.T) {
	r := Lines([]string{"a", "c"}, []string{"a", "b", "c"})
	if r.Added != 1 || r.Removed != 0 {
		t.Fatalf("added=%d removed=%d, want 1/0", r.Added, r.Removed)
	}
	// Expect: =a, +b, =c
	want := []Op{OpEqual, OpInsert, OpEqual}
	if len(r.Lines) != 3 {
		t.Fatalf("got %d lines, want 3: %+v", len(r.Lines), r.Lines)
	}
	for i, w := range want {
		if r.Lines[i].Op != w {
			t.Errorf("line %d op = %v, want %v", i, r.Lines[i].Op, w)
		}
	}
	if r.Lines[1].Text != "b" || r.Lines[1].BLine != 2 || r.Lines[1].ALine != 0 {
		t.Errorf("insert line wrong: %+v", r.Lines[1])
	}
}

func TestLinesPureDelete(t *testing.T) {
	r := Lines([]string{"a", "b", "c"}, []string{"a", "c"})
	if r.Added != 0 || r.Removed != 1 {
		t.Fatalf("added=%d removed=%d, want 0/1", r.Added, r.Removed)
	}
	if r.Lines[1].Op != OpDelete || r.Lines[1].Text != "b" || r.Lines[1].ALine != 2 {
		t.Errorf("delete line wrong: %+v", r.Lines[1])
	}
}

func TestLinesReplace(t *testing.T) {
	r := Lines([]string{"a", "x", "c"}, []string{"a", "y", "c"})
	if r.Added != 1 || r.Removed != 1 {
		t.Fatalf("added=%d removed=%d, want 1/1", r.Added, r.Removed)
	}
	// A replace shows as delete(x) then insert(y), bracketed by equals.
	if !r.Changed() {
		t.Fatal("replace not reported changed")
	}
}

func TestTextSplitsAndTrimsCRLF(t *testing.T) {
	// CRLF on one side, LF on the other — must compare equal.
	r := Text("line1\r\nline2\r\n", "line1\nline2\n")
	if r.Changed() {
		t.Fatalf("CRLF vs LF reported changed: %+v", r.Lines)
	}
}

func TestTextEmpty(t *testing.T) {
	if Text("", "").Changed() {
		t.Error("empty vs empty changed")
	}
	r := Text("", "new")
	if r.Added != 1 || r.Removed != 0 {
		t.Errorf("empty vs one-line: added=%d removed=%d", r.Added, r.Removed)
	}
}
