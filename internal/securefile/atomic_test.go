package securefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileIsPrivateAndRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "export.csv")
	if err := WriteFile(path, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	if err := WriteFile(path, []byte("second"), false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrite err = %v, want os.ErrExist", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "first" {
		t.Fatalf("existing content changed: %q", body)
	}
	if err := WriteFile(path, []byte("second"), true); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if string(body) != "second" {
		t.Fatalf("overwrite content = %q", body)
	}
}

func TestFailedWriteLeavesNoDestinationOrTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.csv")
	want := errors.New("write failed")
	err := Write(path, false, func(w io.Writer) error {
		_, _ = w.Write([]byte("partial"))
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("left files behind: %v", entries)
	}
}
