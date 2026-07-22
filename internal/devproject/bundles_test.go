package devproject

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Bundles attach to DevProjects 1:N. All operations are SQLite
// CRUD against the in-temp-dir devprojects.db; no FS work beyond
// the on-disk directory presence-check in Stale().

func newBundleStore(t *testing.T) (*Store, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Every test needs at least one DevProject to link bundles to.
	dp := DevProject{ID: "test-project", Name: "Test", CreatedAt: time.Now(), TouchedAt: time.Now()}
	if err := s.CreateDevProject(dp); err != nil {
		t.Fatal(err)
	}
	return s, dp.ID
}

// ----- CreateBundle ----------------------------------------------

func TestCreateBundle_HappyPath(t *testing.T) {
	s, projectID := newBundleStore(t)
	dir := t.TempDir()
	b, err := s.CreateBundle(projectID, dir, "dev")
	if err != nil {
		t.Fatalf("CreateBundle: %v", err)
	}
	if b.ID == "" {
		t.Error("ID empty")
	}
	if b.DevProjectID != projectID {
		t.Errorf("DevProjectID = %q, want %q", b.DevProjectID, projectID)
	}
	if !filepath.IsAbs(b.Path) {
		t.Errorf("Path should be absolute, got %q", b.Path)
	}
	if b.DefaultOrgAlias != "dev" {
		t.Errorf("Alias = %q", b.DefaultOrgAlias)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt unset")
	}
}

func TestCreateBundle_RejectsEmptyArgs(t *testing.T) {
	s, projectID := newBundleStore(t)
	if _, err := s.CreateBundle("", "/tmp/x", ""); err == nil {
		t.Error("expected error for empty project id")
	}
	if _, err := s.CreateBundle(projectID, "", ""); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestCreateBundle_AbsolutisesPath(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, err := s.CreateBundle(projectID, "./relative/path", "")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(b.Path) {
		t.Errorf("Path not absolute: %q", b.Path)
	}
}

// ----- Get / List ------------------------------------------------

func TestGetBundle_ReturnsCreated(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "")
	got, err := s.GetBundle(b.ID)
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if got.ID != b.ID || got.Path != b.Path {
		t.Errorf("mismatch: got %+v want %+v", got, b)
	}
}

func TestGetBundle_NotFound(t *testing.T) {
	s, _ := newBundleStore(t)
	_, err := s.GetBundle("nonexistent")
	if err == nil {
		t.Error("expected error for missing id")
	}
}

func TestListBundlesFor_Project(t *testing.T) {
	s, projectID := newBundleStore(t)
	_, _ = s.CreateBundle(projectID, t.TempDir(), "")
	_, _ = s.CreateBundle(projectID, t.TempDir(), "")

	// Second project should not see the first's bundles.
	other := DevProject{ID: "other", Name: "Other", CreatedAt: time.Now(), TouchedAt: time.Now()}
	_ = s.CreateDevProject(other)
	_, _ = s.CreateBundle(other.ID, t.TempDir(), "")

	bundles, err := s.ListBundlesFor(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 {
		t.Errorf("len = %d, want 2", len(bundles))
	}

	all, err := s.ListAllBundles()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("len = %d, want 3", len(all))
	}
}

// ----- Stale -----------------------------------------------------

func TestStale_EmptyPath(t *testing.T) {
	b := Bundle{}
	if !b.Stale() {
		t.Error("empty path should be stale")
	}
}

func TestStale_NonexistentDir(t *testing.T) {
	b := Bundle{Path: "/nonexistent/path/at/all"}
	if !b.Stale() {
		t.Error("nonexistent path should be stale")
	}
}

func TestStale_DirectoryWithoutSfdxJson(t *testing.T) {
	dir := t.TempDir()
	b := Bundle{Path: dir}
	if !b.Stale() {
		t.Error("dir without sfdx-project.json should be stale")
	}
}

func TestStale_FullSfdxProjectNotStale(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sfdx-project.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	b := Bundle{Path: dir}
	if b.Stale() {
		t.Error("dir with sfdx-project.json should not be stale")
	}
}

// ----- MarkRetrieved / MarkDeployed ------------------------------

func TestMarkRetrieved_SetsTimestamp(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "")
	if !b.LastRetrievedAt.IsZero() {
		t.Errorf("LastRetrievedAt should be zero on create")
	}
	if err := s.MarkRetrieved(b.ID); err != nil {
		t.Fatalf("MarkRetrieved: %v", err)
	}
	got, _ := s.GetBundle(b.ID)
	if got.LastRetrievedAt.IsZero() {
		t.Error("LastRetrievedAt still zero after MarkRetrieved")
	}
}

func TestMarkDeployed_SetsTimestamp(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "")
	if err := s.MarkDeployed(b.ID); err != nil {
		t.Fatalf("MarkDeployed: %v", err)
	}
	got, _ := s.GetBundle(b.ID)
	if got.LastDeployedAt.IsZero() {
		t.Error("LastDeployedAt still zero")
	}
}

// ----- SetDefaultOrgAlias / SetBundlePath -----------------------

func TestSetDefaultOrgAlias_Updates(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "dev")
	if err := s.SetDefaultOrgAlias(b.ID, "uat"); err != nil {
		t.Fatalf("SetDefaultOrgAlias: %v", err)
	}
	got, _ := s.GetBundle(b.ID)
	if got.DefaultOrgAlias != "uat" {
		t.Errorf("Alias = %q, want uat", got.DefaultOrgAlias)
	}
}

func TestSetBundlePath_Updates(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "")
	newDir := t.TempDir()
	if err := s.SetBundlePath(b.ID, newDir); err != nil {
		t.Fatalf("SetBundlePath: %v", err)
	}
	got, _ := s.GetBundle(b.ID)
	if got.Path != newDir {
		t.Errorf("Path = %q, want %q", got.Path, newDir)
	}
}

// ----- DeleteBundle ---------------------------------------------

func TestDeleteBundle_RemovesRow(t *testing.T) {
	s, projectID := newBundleStore(t)
	b, _ := s.CreateBundle(projectID, t.TempDir(), "")
	if err := s.DeleteBundle(b.ID); err != nil {
		t.Fatalf("DeleteBundle: %v", err)
	}
	if _, err := s.GetBundle(b.ID); err == nil {
		t.Error("bundle still exists after delete")
	}
}

// ----- newBundleID ----------------------------------------------

func TestNewBundleID_IsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id := newBundleID()
		if id == "" {
			t.Fatal("empty id")
		}
		if seen[id] {
			t.Errorf("duplicate id %q at iteration %d", id, i)
		}
		seen[id] = true
	}
}
