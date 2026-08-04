package sf

import (
	"strings"
	"testing"
)

// TestDemoModeBlocksProjectShellOuts locks in the backstop guarantee:
// in --demo mode, NO sf CLI call may spawn a real subprocess, including
// project-level deploy/validate paths through runSFInDirWithTimeout.
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
