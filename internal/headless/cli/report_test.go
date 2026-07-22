package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func newReportTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "dev", Username: "dev@example.com"},
		},
	}
}

func runReportCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
	t.Helper()
	args := Parse(argv)
	if !args.IsHeadless() {
		t.Fatalf("argv %v not recognized as headless", argv)
	}
	var stdout, stderr bytes.Buffer
	code := Dispatch(a, args, &stdout, &stderr)
	if !args.JSON {
		return code, nil
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, stdout.String())
	}
	return code, got
}

func TestReportRun_RequiresID(t *testing.T) {
	a := newReportTestApp()
	code, got := runReportCLI(t, a, "--json", "report", "run", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--id") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestReportExport_RequiresIDAndOutput(t *testing.T) {
	a := newReportTestApp()
	// Missing id.
	code, _ := runReportCLI(t, a, "--json", "report", "export",
		"--org", "dev", "--output", "/tmp/x.xlsx")
	if code != headless.ExitInvalidArg {
		t.Errorf("missing id exit = %d", code)
	}
	// Missing output.
	code, _ = runReportCLI(t, a, "--json", "report", "export",
		"--org", "dev", "--id", "00OabcdEF")
	if code != headless.ExitInvalidArg {
		t.Errorf("missing output exit = %d", code)
	}
}

func TestReportExport_RejectsNonXLSXExtension(t *testing.T) {
	a := newReportTestApp()
	tmp := filepath.Join(t.TempDir(), "x.csv")
	code, got := runReportCLI(t, a, "--json", "report", "export",
		"--org", "dev", "--id", "00OabcdEF", "--output", tmp)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "xlsx") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestReportExport_RejectsBadView(t *testing.T) {
	a := newReportTestApp()
	tmp := filepath.Join(t.TempDir(), "x.xlsx")
	code, got := runReportCLI(t, a, "--json", "report", "export",
		"--org", "dev", "--id", "00OabcdEF", "--output", tmp,
		"--view", "raw")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--view") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestReport_OrgNotFound(t *testing.T) {
	a := newReportTestApp()
	code, got := runReportCLI(t, a, "--json", "report", "list", "--org", "missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestReport_UnknownVerb(t *testing.T) {
	a := newReportTestApp()
	code, _ := runReportCLI(t, a, "--json", "report", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestReport_DefaultsToList(t *testing.T) {
	a := newReportTestApp()
	_, got := runReportCLI(t, a, "--json", "report", "--org", "missing")
	if got["command"] != "report.list" {
		t.Errorf("command = %v, want report.list", got["command"])
	}
}
