package cli

import (
	"testing"
)

func TestProjectImportBundle_MissingArgs(t *testing.T) {
	a := newTestAppWithStore(t)
	cases := []struct {
		name string
		argv []string
	}{
		{"missing both", []string{"project", "import-bundle", "--json"}},
		{"missing path", []string{"project", "import-bundle", "--project-id", "p1", "--json"}},
		{"missing project-id", []string{"project", "import-bundle", "--path", "/tmp/x", "--json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out, _ := runCLI(t, a, c.argv...)
			if code == 0 {
				t.Fatalf("expected non-zero exit: %+v", out)
			}
			errEnv := out["error"].(map[string]any)
			if errEnv["code"] != "invalid_argument" {
				t.Errorf("code = %v, want invalid_argument", errEnv["code"])
			}
		})
	}
}

func TestProjectImportBundle_NonexistentPathFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "project", "import-bundle",
		"--project-id", "anything",
		"--path", "/nonexistent/path/to/package.xml",
		"--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if out["ok"] != false {
		t.Errorf("ok = %v", out["ok"])
	}
}
