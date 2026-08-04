package app

import (
	"os"
	"strings"
	"testing"
)

// TestNoCacheUsesTempAndCleansUp verifies --no-cache opens the cache
// in a throwaway temp dir (not ~/.sf-deck) and removes it on Close.
func TestNoCacheUsesTempAndCleansUp(t *testing.T) {
	a, err := Open(OpenOptions{NoCache: true, SkipApplog: true, SkipOrgs: true})
	if err != nil {
		t.Fatalf("Open(no-cache): %v", err)
	}
	dir := a.cacheDir
	if dir == "" {
		t.Fatal("no-cache did not create a cacheDir")
	}
	if !strings.Contains(dir, "sf-deck-nocache-") {
		t.Errorf("cacheDir %q is not the expected temp dir", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("cacheDir should exist while open: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cacheDir survived Close: %v", err)
	}
}

// TestNoTagsUsesTempAndCleansUp verifies --no-tags opens the
// dev-project store in a throwaway temp dir and removes it on Close.
// (No --no-cache here, so it must create its own tagsDir.)
func TestNoTagsUsesTempAndCleansUp(t *testing.T) {
	a, err := Open(OpenOptions{NoTags: true, SkipApplog: true, SkipOrgs: true})
	if err != nil {
		t.Fatalf("Open(no-tags): %v", err)
	}
	dir := a.tagsDir
	if dir == "" {
		t.Fatal("no-tags did not create a tagsDir")
	}
	if !strings.Contains(dir, "sf-deck-notags-") {
		t.Errorf("tagsDir %q is not the expected temp dir", dir)
	}
	if a.Projects == nil {
		t.Fatal("no-tags should still open a (temp) dev-project store")
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("tagsDir survived Close: %v", err)
	}
}

// TestNoCacheAndNoTagsShareTempDir verifies that when BOTH are set,
// the tag store reuses the no-cache temp dir rather than making a
// second one (so there's exactly one temp dir to clean up).
func TestNoCacheAndNoTagsShareTempDir(t *testing.T) {
	a, err := Open(OpenOptions{NoCache: true, NoTags: true, SkipApplog: true, SkipOrgs: true})
	if err != nil {
		t.Fatalf("Open(no-cache+no-tags): %v", err)
	}
	if a.cacheDir == "" {
		t.Fatal("expected a cacheDir")
	}
	if a.tagsDir != "" {
		t.Errorf("tagsDir should be empty (reusing cacheDir), got %q", a.tagsDir)
	}
	cacheDir := a.cacheDir
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("shared temp dir survived Close: %v", err)
	}
}
