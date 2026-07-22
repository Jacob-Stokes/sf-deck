package app

import (
	"os"
	"testing"
)

// TestOpenDemoIsolatesState covers the OpenOptions.Demo branch: demo
// mode must boot an isolated cache + devproject store in a throwaway
// temp dir, skip org enumeration and usage tracking, and never touch
// the user's real on-disk stores. The seeded fixtures and demo flags
// are the caller's job; Open only provides the isolated stores.
func TestOpenDemoIsolatesState(t *testing.T) {
	a, err := Open(OpenOptions{Demo: true, SkipApplog: true})
	if err != nil {
		t.Fatalf("Open(demo): %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	if a.Settings == nil {
		t.Error("demo App.Settings is nil")
	}
	if a.Cache == nil {
		t.Fatal("demo App.Cache is nil — the seeded cache IS the org list")
	}
	if a.Projects == nil {
		t.Error("demo App.Projects is nil — saved queries/projects won't work")
	}
	// Demo never enumerates real orgs or opens the usage tracker.
	if len(a.Orgs) != 0 {
		t.Errorf("demo enumerated %d orgs; want 0", len(a.Orgs))
	}
	if a.Usage != nil {
		t.Error("demo opened a usage tracker; want nil")
	}
	if a.demoDir == "" {
		t.Fatal("demo App.demoDir is empty; isolated stores not wired")
	}
	if _, err := os.Stat(a.demoDir); err != nil {
		t.Errorf("demo dir not present while open: %v", err)
	}
}

// TestCloseRemovesDemoDir verifies Close tears down the throwaway demo
// dir, and that a second Close is a safe no-op (callers defer it).
func TestCloseRemovesDemoDir(t *testing.T) {
	a, err := Open(OpenOptions{Demo: true, SkipApplog: true})
	if err != nil {
		t.Fatalf("Open(demo): %v", err)
	}
	dir := a.demoDir

	if err := a.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("demo dir survived Close: stat err = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close should be a no-op, got: %v", err)
	}
}
