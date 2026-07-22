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

// newSavedQueryTestApp opens devproject under a temp HOME. soql.saved
// is local-only — no orgs needed.
func newSavedQueryTestApp(t *testing.T) *app.App {
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

func runSavedCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestSavedQuery_FullLifecycle(t *testing.T) {
	a := newSavedQueryTestApp(t)

	// Create.
	code, got := runSavedCLI(t, a, "--json", "soql", "saved", "create",
		"--name", "recent-accounts",
		"--query", "SELECT Id, Name FROM Account LIMIT 10")
	if code != headless.ExitOK {
		t.Fatalf("create exit = %d", code)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	q, _ := data["query"].(map[string]any)
	id, _ := q["id"].(string)
	if !strings.HasPrefix(id, "sq_") {
		t.Errorf("id = %q, want sq_ prefix", id)
	}

	// List.
	code, got = runSavedCLI(t, a, "--json", "soql", "saved", "list")
	if code != headless.ExitOK {
		t.Fatalf("list exit = %d", code)
	}
	if got["data"].(map[string]any)["count"] != float64(1) {
		t.Errorf("list count = %v", got["data"].(map[string]any)["count"])
	}

	// Update.
	code, got = runSavedCLI(t, a, "--json", "soql", "saved", "update",
		"--id", id, "--name", "renamed")
	if code != headless.ExitOK {
		t.Fatalf("update exit = %d (%+v)", code, got)
	}

	// Delete.
	code, _ = runSavedCLI(t, a, "--json", "soql", "saved", "delete",
		"--id", id)
	if code != headless.ExitOK {
		t.Fatalf("delete exit = %d", code)
	}

	// Show now not_found.
	code, _ = runSavedCLI(t, a, "--json", "soql", "saved", "show", "--id", id)
	if code != headless.ExitNotFound {
		t.Errorf("post-delete show exit = %d, want %d", code, headless.ExitNotFound)
	}
}

func TestSavedQuery_CreateRequiresNameAndQuery(t *testing.T) {
	a := newSavedQueryTestApp(t)
	code, _ := runSavedCLI(t, a, "--json", "soql", "saved", "create",
		"--name", "x")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
	code, _ = runSavedCLI(t, a, "--json", "soql", "saved", "create",
		"--query", "SELECT Id FROM Account")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestSavedQuery_ShowMissing(t *testing.T) {
	a := newSavedQueryTestApp(t)
	code, got := runSavedCLI(t, a, "--json", "soql", "saved", "show",
		"--id", "sq_missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestSavedQuery_UpdateNoFields(t *testing.T) {
	a := newSavedQueryTestApp(t)
	_, got := runSavedCLI(t, a, "--json", "soql", "saved", "create",
		"--name", "x", "--query", "SELECT Id FROM Account")
	id := got["data"].(map[string]any)["query"].(map[string]any)["id"].(string)
	code, _ := runSavedCLI(t, a, "--json", "soql", "saved", "update", "--id", id)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestSavedQuery_ListOmitsBody(t *testing.T) {
	// Regression for P2a (soql saved variant). Listing the saved
	// SOQL library must not dump the body of every query.
	a := newSavedQueryTestApp(t)
	_, _ = runSavedCLI(t, a, "--json", "soql", "saved", "create",
		"--name", "secret-query",
		"--query", "SELECT Id FROM Account WHERE InternalNote__c LIKE 'top-secret%'")

	_, got := runSavedCLI(t, a, "--json", "soql", "saved", "list")
	data, _ := got["data"].(map[string]any)
	queries, _ := data["queries"].([]any)
	if len(queries) != 1 {
		t.Fatalf("len(queries) = %d", len(queries))
	}
	m, _ := queries[0].(map[string]any)
	if _, hasBody := m["body"]; hasBody {
		t.Errorf("list response contains body field; should be summary-only: %+v", m)
	}
	if m["body_chars"] == nil || m["body_sha256"] == nil {
		t.Errorf("summary missing body_chars/body_sha256: %+v", m)
	}
}

func TestSavedQuery_DefaultsToList(t *testing.T) {
	a := newSavedQueryTestApp(t)
	_, got := runSavedCLI(t, a, "--json", "soql", "saved")
	if got["command"] != "soql.saved.list" {
		t.Errorf("command = %v", got["command"])
	}
}

func TestSavedQuery_NilProjects(t *testing.T) {
	a := &app.App{Settings: &settings.Settings{}}
	_ = devproject.ErrSavedQueryNotFound // pin import
	code, got := runSavedCLI(t, a, "--json", "soql", "saved", "list")
	if code != headless.ExitInternal {
		t.Errorf("exit = %d, want %d", code, headless.ExitInternal)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInternal {
		t.Errorf("error.code = %v", errObj["code"])
	}
}
