package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
)

func TestSOQLExport_RequiresQuery(t *testing.T) {
	a := newSOQLTestApp()
	code, got := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "dev", "--output", "/tmp/x.csv")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--query") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestSOQLExport_RequiresOutput(t *testing.T) {
	a := newSOQLTestApp()
	code, got := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "dev", "--query", "SELECT Id FROM Account")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--output") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestSOQLExport_RejectsBulkAndTooling(t *testing.T) {
	a := newSOQLTestApp()
	code, got := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "dev", "--query", "SELECT Id FROM ApexClass",
		"--output", "/tmp/x.csv", "--bulk", "--tooling")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "mutually exclusive") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestSOQLExport_RejectsBadFormat(t *testing.T) {
	a := newSOQLTestApp()
	tmp := filepath.Join(t.TempDir(), "x.txt") // unknown extension
	code, got := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "dev", "--query", "SELECT Id FROM Account",
		"--output", tmp)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "infer format") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestSOQLExport_RejectsBadExplicitFormat(t *testing.T) {
	a := newSOQLTestApp()
	tmp := filepath.Join(t.TempDir(), "x.csv")
	code, _ := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "dev", "--query", "SELECT Id FROM Account",
		"--output", tmp, "--format", "yaml")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestPickExportFormat_FromExtension(t *testing.T) {
	cases := []struct {
		path string
		want exporters.Format
	}{
		{"/tmp/x.csv", exporters.FormatCSV},
		{"/tmp/x.json", exporters.FormatJSON},
		{"/tmp/x.xlsx", exporters.FormatXLSX},
	}
	for _, c := range cases {
		got, err := pickExportFormat(c.path, "")
		if err != nil {
			t.Errorf("path=%s err=%v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("path=%s got=%v want=%v", c.path, got, c.want)
		}
	}
}

func TestPickExportFormat_ExplicitWins(t *testing.T) {
	got, err := pickExportFormat("/tmp/x.csv", "json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != exporters.FormatJSON {
		t.Errorf("got = %v, want JSON", got)
	}
}

func TestPickExportFormat_RejectsUnknown(t *testing.T) {
	if _, err := pickExportFormat("/tmp/x.csv", "yaml"); err == nil {
		t.Error("expected error for yaml")
	}
}

func TestPickExportFormat_RejectsBundleFormat(t *testing.T) {
	// package-xml, sfdx-project are bundle formats — they aren't
	// supported by the SOQL export path. pickExportFormat doesn't
	// currently enumerate them; if the user passes "package-xml" via
	// --format we want a clear error.
	_, err := pickExportFormat("/tmp/x.csv", "package-xml")
	if err == nil {
		t.Error("expected error for package-xml")
	}
}

func TestSOQLExport_OrgNotFound(t *testing.T) {
	a := newSOQLTestApp()
	tmp := filepath.Join(t.TempDir(), "x.csv")
	code, _ := runSOQLCLI(t, a, "--json", "soql", "export",
		"--org", "missing", "--query", "SELECT Id FROM Account",
		"--output", tmp)
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
}
