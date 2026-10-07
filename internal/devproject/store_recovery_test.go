package devproject

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenAtRejectsCorruptDB pins the quick_check probe: a file of
// garbage bytes must fail openAt rather than limping into queries.
func TestOpenAtRejectsCorruptDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devprojects.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database, sorry"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openAt(path); err == nil {
		t.Fatal("openAt accepted a corrupt file")
	}
}

// TestCopyFileRoundTrip covers the backup primitive (tmp+rename).
func TestCopyFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a")
	dst := filepath.Join(dir, "b")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "payload" {
		t.Fatalf("dst = %q, err %v", got, err)
	}
}
