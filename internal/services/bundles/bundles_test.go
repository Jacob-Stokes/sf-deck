package bundles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// TestDirHasFiles guards the overwrite-protection check that keeps
// bundle create from truncating package.xml / sfdx-project.json /
// README.md inside a user's existing sfdx project (a mistyped --path).
// The three cases the guard must distinguish: missing dir (safe to
// create), empty dir (safe to write), non-empty dir (refuse without
// --force).
func TestDirHasFiles(t *testing.T) {
	base := t.TempDir()

	missing := filepath.Join(base, "does-not-exist")
	if got, err := dirHasFiles(missing); err != nil || got {
		t.Fatalf("missing dir: got (%v, %v), want (false, nil)", got, err)
	}

	// Empty existing dir → safe to write.
	empty := filepath.Join(base, "empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := dirHasFiles(empty); err != nil || got {
		t.Fatalf("empty dir: got (%v, %v), want (false, nil)", got, err)
	}

	// Non-empty dir (simulating an existing project) → must refuse.
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "package.xml"), []byte("<Package/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := dirHasFiles(proj); err != nil || !got {
		t.Fatalf("non-empty dir: got (%v, %v), want (true, nil)", got, err)
	}
	if err := ValidateCreateDestination(proj, false); err == nil {
		t.Fatal("non-empty destination was accepted without force")
	}
	if err := ValidateCreateDestination(proj, true); err != nil {
		t.Fatalf("forced destination rejected: %v", err)
	}
}

func TestCreateReplacesLeafSymlinksWithoutTouchingTargets(t *testing.T) {
	store, err := devproject.OpenPath(filepath.Join(t.TempDir(), "devprojects.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateDevProject(devproject.DevProject{ID: "p1", Name: "Test project"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddItem(devproject.Item{
		DevProjectID: "p1",
		OrgUser:      "test@example.invalid",
		Kind:         devproject.KindApexClass,
		Ref:          "01p000000000001",
		Name:         "ExampleClass",
	}); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"package.xml", "sfdx-project.json", "README.md"} {
		if err := os.Symlink(victim, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Create(store, CreateInput{
		ProjectID:   "p1",
		Path:        dir,
		OrgUser:     "test@example.invalid",
		FullProject: true,
		Force:       true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep me" {
		t.Fatalf("symlink target changed: %q", got)
	}
	for _, name := range []string{"package.xml", "sfdx-project.json", "README.md"} {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s remained a symlink", name)
		}
	}
}
