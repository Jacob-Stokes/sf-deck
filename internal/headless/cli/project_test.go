package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func newProjectTestApp(t *testing.T) *app.App {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dp, err := devproject.Open()
	if err != nil {
		t.Fatalf("devproject.Open: %v", err)
	}
	t.Cleanup(func() { _ = dp.Close() })
	return &app.App{
		Settings:     &settings.Settings{},
		Projects:     dp,
		SaveSettings: func() error { return nil },
	}
}

func runProjectCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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
		t.Fatalf("JSON unmarshal: %v\nstdout: %s", err, stdout.String())
	}
	return code, got
}

// projectIDFromCreate pulls data.project.id from a create response.
func projectIDFromCreate(t *testing.T, resp map[string]any) string {
	t.Helper()
	data, _ := resp["data"].(map[string]any)
	project, _ := data["project"].(map[string]any)
	id, ok := project["id"].(string)
	if !ok || id == "" {
		t.Fatalf("data.project.id missing: %v", project)
	}
	return id
}

func TestProjectCreate_JSONHappyPath(t *testing.T) {
	a := newProjectTestApp(t)
	code, got := runProjectCLI(t, a, "--json", "project", "create",
		"--name", "Q2 cleanup", "--description", "post-release")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if got["command"] != "project.create" {
		t.Errorf("command = %v", got["command"])
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	project, _ := data["project"].(map[string]any)
	if project["name"] != "Q2 cleanup" {
		t.Errorf("name = %v", project["name"])
	}
	if project["id"] == "" {
		t.Errorf("id empty")
	}
}

func TestProjectShow_NotFound(t *testing.T) {
	a := newProjectTestApp(t)
	code, got := runProjectCLI(t, a, "--json", "project", "show", "--id", "missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestProjectUpdate_PartialAndIdempotent(t *testing.T) {
	a := newProjectTestApp(t)
	_, created := runProjectCLI(t, a, "--json", "project", "create", "--name", "Old")
	id := projectIDFromCreate(t, created)

	// First update — Changed=true.
	code, got := runProjectCLI(t, a, "--json", "project", "update",
		"--id", id, "--name", "New")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}

	// Same name — Changed=false.
	code, got = runProjectCLI(t, a, "--json", "project", "update",
		"--id", id, "--name", "New")
	if code != headless.ExitOK {
		t.Fatalf("idempotent exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("idempotent changed = %v", changed)
	}
}

func TestProjectDelete_NotEmptyMapsInvalidArgWithDetails(t *testing.T) {
	a := newProjectTestApp(t)
	_, created := runProjectCLI(t, a, "--json", "project", "create", "--name", "X")
	id := projectIDFromCreate(t, created)

	// Add an item.
	_, _ = runProjectCLI(t, a, "--json", "project", "add-item",
		"--project-id", id, "--kind", "record", "--ref", "001A")

	// Delete without --force.
	code, got := runProjectCLI(t, a, "--json", "project", "delete", "--id", id)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Errorf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	if details["items"] != float64(1) {
		t.Errorf("details.items = %v, want 1", details["items"])
	}
	if details["force_flag"] != "--force" {
		t.Errorf("details.force_flag = %v", details["force_flag"])
	}

	// With --force succeeds.
	code, got = runProjectCLI(t, a, "--json", "project", "delete",
		"--id", id, "--force")
	if code != headless.ExitOK {
		t.Fatalf("force delete exit = %d (%+v)", code, got)
	}
}

func TestProjectAddItem_Roundtrip(t *testing.T) {
	a := newProjectTestApp(t)
	_, created := runProjectCLI(t, a, "--json", "project", "create", "--name", "X")
	id := projectIDFromCreate(t, created)

	// Add.
	code, got := runProjectCLI(t, a, "--json", "project", "add-item",
		"--project-id", id, "--kind", "field", "--ref", "Account.X__c",
		"--org-user", "dev@x", "--name", "X Field")
	if code != headless.ExitOK {
		t.Fatalf("add exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("add changed = %v", got["changed"])
	}

	// Re-add — idempotent.
	code, got = runProjectCLI(t, a, "--json", "project", "add-item",
		"--project-id", id, "--kind", "field", "--ref", "Account.X__c",
		"--org-user", "dev@x")
	if code != headless.ExitOK {
		t.Fatalf("re-add exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("re-add changed = %v", changed)
	}

	// Items list reflects.
	code, got = runProjectCLI(t, a, "--json", "project", "items", "--id", id)
	if code != headless.ExitOK {
		t.Fatalf("items exit = %d", code)
	}
	data, _ := got["data"].(map[string]any)
	if data["count"] != float64(1) {
		t.Errorf("count = %v, want 1", data["count"])
	}

	// Remove.
	code, got = runProjectCLI(t, a, "--json", "project", "remove-item",
		"--project-id", id, "--kind", "field", "--ref", "Account.X__c",
		"--org-user", "dev@x")
	if code != headless.ExitOK {
		t.Fatalf("remove exit = %d", code)
	}
	if got["changed"] != true {
		t.Errorf("remove changed = %v", got["changed"])
	}

	// Remove-again — Changed=false.
	code, got = runProjectCLI(t, a, "--json", "project", "remove-item",
		"--project-id", id, "--kind", "field", "--ref", "Account.X__c",
		"--org-user", "dev@x")
	if code != headless.ExitOK {
		t.Fatalf("re-remove exit = %d", code)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("re-remove changed = %v", changed)
	}
}

func TestProjectAddItem_BadKind(t *testing.T) {
	a := newProjectTestApp(t)
	_, created := runProjectCLI(t, a, "--json", "project", "create", "--name", "X")
	id := projectIDFromCreate(t, created)

	code, got := runProjectCLI(t, a, "--json", "project", "add-item",
		"--project-id", id, "--kind", "weird", "--ref", "x")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "weird") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestProjectAddItem_ProjectNotFound(t *testing.T) {
	a := newProjectTestApp(t)
	code, got := runProjectCLI(t, a, "--json", "project", "add-item",
		"--project-id", "missing", "--kind", "record", "--ref", "x")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestProjectList_DefaultsToListVerb(t *testing.T) {
	a := newProjectTestApp(t)
	code, got := runProjectCLI(t, a, "--json", "project")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if got["command"] != "project.list" {
		t.Errorf("command = %v", got["command"])
	}
}

func TestProject_NilProjectsRendersTypedError(t *testing.T) {
	a := &app.App{Settings: &settings.Settings{}} // no Projects
	code, got := runProjectCLI(t, a, "--json", "project", "list")
	if code != headless.ExitInternal {
		t.Errorf("exit = %d, want %d", code, headless.ExitInternal)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInternal {
		t.Errorf("error.code = %v", errObj["code"])
	}
}
