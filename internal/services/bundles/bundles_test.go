package bundles

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDirHasFiles guards the overwrite-protection check that keeps
// bundle create from truncating package.xml / sfdx-project.json /
// README.md inside a user's existing sfdx project (a mistyped --path).
// The three cases the guard must distinguish: missing dir (safe to
// create), empty dir (safe to write), non-empty dir (refuse without
// --force).
func TestDirHasFiles(t *testing.T) {
	base := t.TempDir()

	// Missing path → not "has files", no error (MkdirAll will create it).
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
