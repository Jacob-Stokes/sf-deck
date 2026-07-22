package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogAndDumpFilesArePrivate(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Cleanup(func() {
		Close()
		_ = os.Setenv("HOME", oldHome)
	})

	path := Init()
	if path == "" {
		t.Fatal("Init returned empty path")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log mode = %o, want 600", got)
	}

	dump := Dump([]string{"frontdoor", "test"}, "html", []byte("<html>secret</html>"))
	if dump == "" {
		t.Fatal("Dump returned empty path")
	}
	if filepath.Dir(dump) != filepath.Join(tmp, ".sf-deck", "log", "dumps") {
		t.Fatalf("dump path = %q, not under temp dump dir", dump)
	}
	info, err = os.Stat(dump)
	if err != nil {
		t.Fatalf("stat dump: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("dump mode = %o, want 600", got)
	}
}

// pruneOldFiles is best-effort and silently no-ops on errors, so the
// test asserts its behaviour against a directory it controls
// directly rather than going through Init.
func TestPruneOldFiles_KeepsMostRecent(t *testing.T) {
	dir := t.TempDir()

	// Create 12 files with mtimes spaced 1 second apart so sorting is
	// deterministic. Older files get earlier indices.
	now := time.Now()
	for i := 0; i < 12; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%02d.log", i))
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		when := now.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}

	pruneOldFiles(dir, "*.log", 5)

	// Five most-recent files (08..11) should survive; the rest are gone.
	for i := 0; i < 12; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%02d.log", i))
		_, err := os.Stat(path)
		exists := !os.IsNotExist(err)
		shouldExist := i >= 7 // i.e. 07..11 — the five most-recent
		if exists && !shouldExist {
			t.Errorf("file %s should have been pruned", path)
		}
		if !exists && shouldExist {
			t.Errorf("file %s should have been kept", path)
		}
	}
}

func TestPruneOldFiles_KeepUnderThreshold(t *testing.T) {
	dir := t.TempDir()
	// Only 3 files, keep 100 — should be a no-op.
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%02d.log", i))
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneOldFiles(dir, "*.log", 100)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(matches) != 3 {
		t.Errorf("expected 3 files to survive (keep < count); got %d", len(matches))
	}
}

func TestPruneOldFiles_HandlesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	pruneOldFiles(dir, "*.log", 5) // no panic, no error
}

func TestPruneOldFiles_HandlesMissingDir(t *testing.T) {
	pruneOldFiles("/nonexistent/path", "*.log", 5) // best-effort, no panic
}

func TestPruneOldFiles_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	// Two regular files + one nested directory matching the glob.
	if err := os.MkdirAll(filepath.Join(dir, "subdir.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d.log", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneOldFiles(dir, "*.log", 1)
	if _, err := os.Stat(filepath.Join(dir, "subdir.log")); os.IsNotExist(err) {
		t.Error("directory matching glob should not be pruned")
	}
}
