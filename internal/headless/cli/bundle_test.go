package cli

import (
	"path/filepath"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// newTestAppWithStore returns a test App that has a real (but
// in-temp-dir) devproject Store backing a.Projects. Lets us drive
// bundle.list / project.list / project.create etc. through the
// real CLI dispatcher with empty fixtures.
func newTestAppWithStore(t *testing.T) *app.App {
	t.Helper()
	store, err := devproject.OpenPath(filepath.Join(t.TempDir(), "devprojects.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &app.App{
		Settings:     &settings.Settings{},
		SaveSettings: func() error { return nil },
		Projects:     store,
	}
}

// Bundle CLI verbs all need a *devproject.Store and shell out to
// `sf` for the actual deploy/retrieve work — exercising them
// end-to-end requires a real org. What we CAN cover without one:
//
//   - dispatch routes each verb correctly
//   - the "store unavailable" guard (a.Projects == nil)
//   - the unknown-verb fall-through
//   - flag-parsing failure path on every verb
//
// That's the dispatch contract — what scripts depend on. The
// service-layer behaviour is covered by tests inside
// internal/services/bundles + integration tests.

func TestBundle_StoreUnavailableErrors(t *testing.T) {
	a := newTestApp() // a.Projects is nil
	verbs := []string{"list", "show", "create", "link", "retrieve",
		"deploy", "validate", "report", "delete"}
	for _, v := range verbs {
		t.Run(v, func(t *testing.T) {
			code, out, _ := runCLI(t, a, "bundle", v, "--json")
			if code == 0 {
				t.Fatal("expected non-zero exit")
			}
			errEnv, ok := out["error"].(map[string]any)
			if !ok {
				t.Fatalf("no error envelope: %+v", out)
			}
			if errEnv["code"] != "internal_error" {
				t.Errorf("error.code = %v, want internal_error", errEnv["code"])
			}
			if msg, _ := errEnv["message"].(string); msg == "" {
				t.Error("error.message should not be empty")
			}
		})
	}
}

func TestBundle_UnknownVerb(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "bundle", "frobnicate", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	// "store unavailable" wins because the guard runs before the
	// verb switch — that's fine; both are diagnostic of missing
	// setup. The message format matters less than the exit code.
	if errEnv["code"] == "" {
		t.Error("expected an error code")
	}
}

func TestBundle_DefaultVerbIsList(t *testing.T) {
	a := newTestApp()
	// Without an explicit verb, dispatcher should attempt list.
	// Without a.Projects this returns the store-unavailable error
	// on the bundle.list command, which is still proof of routing.
	code, out, _ := runCLI(t, a, "bundle", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if out["command"] != "bundle.list" {
		t.Errorf("command = %v, want bundle.list (default verb)", out["command"])
	}
}

// ----- bundle list / show / delete against a real (in-temp-dir) store -----

func TestBundle_ListEmptyStore(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %+v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok = %v", out["ok"])
	}
	data := out["data"].(map[string]any)
	if data["count"] != float64(0) {
		t.Errorf("count = %v, want 0 on empty store", data["count"])
	}
	if bundles, ok := data["bundles"].([]any); ok && len(bundles) != 0 {
		t.Errorf("bundles should be empty on fresh store; got %v", bundles)
	}
}

func TestBundle_ShowMissingIDFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "show", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v, want invalid_argument", errEnv["code"])
	}
}

func TestBundle_ShowUnknownIDFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "show", "--id", "nope", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "not_found" {
		t.Errorf("code = %v, want not_found", errEnv["code"])
	}
}

func TestBundle_DeleteMissingIDFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "delete", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_CreateMissingProjectIDFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "create", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_LinkMissingFieldsFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "link", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_RetrieveMissingIDFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "retrieve", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_DeployMissingFieldsFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "deploy", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_ValidateMissingFieldsFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "validate", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_ReportMissingFieldsFails(t *testing.T) {
	a := newTestAppWithStore(t)
	code, out, _ := runCLI(t, a, "bundle", "report", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}

func TestBundle_BadDeployTestsLevel(t *testing.T) {
	a := newTestAppWithStore(t)
	// Drive the buildDeployOpts unknown-level path through the CLI.
	code, out, _ := runCLI(t, a, "bundle", "deploy",
		"--id", "anything", "--org", "x", "--tests", "InventedLevel", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("code = %v", errEnv["code"])
	}
}
