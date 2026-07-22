package sf

import (
	"strings"
	"testing"
)

// TestDemoModeBlocksProjectShellOuts locks in the backstop guarantee:
// in --demo mode, NO sf CLI call may spawn a real subprocess. The
// project-level deploy/validate path runs through runSFInDirWithTimeout,
// which previously had no DemoMode check (only runSF did), so a demo
// deploy/validate could execute for real against a colliding alias.
func TestDemoModeBlocksProjectShellOuts(t *testing.T) {
	orig := DemoMode
	DemoMode = true
	defer func() { DemoMode = orig }()

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"DeployProject", func() error { _, err := DeployProject(t.TempDir(), "any-alias", DeployOpts{}); return err }},
		{"ValidateDeployProject", func() error { _, err := ValidateDeployProject(t.TempDir(), "any-alias", DeployOpts{}); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("demo mode should refuse the sf call, got nil error")
			}
			if !strings.Contains(err.Error(), "demo mode") {
				t.Errorf("expected a demo-mode refusal, got: %v", err)
			}
		})
	}
}
