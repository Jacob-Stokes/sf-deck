package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAtTightensUsageDatabasePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tracker, err := openAt(path)
	if err != nil {
		t.Fatalf("openAt: %v", err)
	}
	t.Cleanup(func() { _ = tracker.Close() })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("usage.db mode = %o, want 600", got)
	}
}
