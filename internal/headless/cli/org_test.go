package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// newOrgTestApp builds an app with a stubbed org list + a save hook
// that captures whether (and how often) it was called. No real disk
// touching.
func newOrgTestApp() (*app.App, *int) {
	saveCalls := 0
	st := &settings.Settings{Orgs: map[string]settings.OrgConfig{}}
	return &app.App{
		Settings: st,
		Orgs: []sf.Org{
			{Alias: "prod", Username: "boss@example.com"},
			{Alias: "sand", Username: "qa@example.com.sandbox", IsSandbox: true},
			{Username: "scratch1@example.com", IsScratch: true},
		},
		SaveSettings: func() error {
			saveCalls++
			return nil
		},
	}, &saveCalls
}

func runOrgCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestOrgList_ReturnsAllWithResolvedSafety(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org", "list")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	data, _ := got["data"].(map[string]any)
	if data["count"] != float64(3) {
		t.Errorf("count = %v, want 3", data["count"])
	}
	orgs, _ := data["orgs"].([]any)
	if len(orgs) != 3 {
		t.Fatalf("orgs len = %d", len(orgs))
	}
	// Each org carries a resolved safety level.
	for _, o := range orgs {
		m, _ := o.(map[string]any)
		if m["safety"] == nil || m["safety"] == "" {
			t.Errorf("org missing safety: %+v", m)
		}
	}
	// Prod resolves to read_only by default.
	prod, _ := orgs[0].(map[string]any)
	if prod["safety"] != "read_only" {
		t.Errorf("prod safety = %v, want read_only", prod["safety"])
	}
}

func TestOrgShow_DefaultsToFirstOrg(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org", "show")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if got["target"] != "prod" {
		t.Errorf("target = %v, want prod", got["target"])
	}
}

func TestOrgShow_NotFound(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org", "show", "--org", "missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	if details["target"] != "missing" {
		t.Errorf("details.target = %v", details["target"])
	}
}

func TestOrgSafetyGet_DefaultsToImplicit(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "get", "--org", "prod")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	data, _ := got["data"].(map[string]any)
	if data["safety"] != "read_only" {
		t.Errorf("safety = %v", data["safety"])
	}
	if data["explicit"] != false {
		t.Errorf("explicit = %v, want false (kind default)", data["explicit"])
	}
	if data["source"] != "default" {
		t.Errorf("source = %v, want default", data["source"])
	}
}

func TestOrgSafetySet_Roundtrip(t *testing.T) {
	a, saves := newOrgTestApp()

	// Set to records.
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "prod", "--level", "records")
	if code != headless.ExitOK {
		t.Fatalf("set exit = %d (%+v)", code, got)
	}
	if got["changed"] != true {
		t.Errorf("changed = %v, want true", got["changed"])
	}
	data, _ := got["data"].(map[string]any)
	if data["safety"] != "records" {
		t.Errorf("safety = %v", data["safety"])
	}
	if data["prior_safety"] != "read_only" {
		t.Errorf("prior_safety = %v", data["prior_safety"])
	}
	if *saves != 1 {
		t.Errorf("save calls = %d, want 1", *saves)
	}

	// Get reflects the change.
	code, got = runOrgCLI(t, a, "--json", "org", "safety", "get", "--org", "prod")
	if code != headless.ExitOK {
		t.Fatalf("get exit = %d", code)
	}
	data, _ = got["data"].(map[string]any)
	if data["safety"] != "records" {
		t.Errorf("post-set safety = %v", data["safety"])
	}
	if data["explicit"] != true {
		t.Errorf("explicit = %v, want true", data["explicit"])
	}
	if data["source"] != "override" {
		t.Errorf("source = %v, want override", data["source"])
	}
}

func TestOrgSafetySet_Clear(t *testing.T) {
	a, _ := newOrgTestApp()
	// Set, then clear.
	_, _ = runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "prod", "--level", "records")
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "prod", "--clear")
	if code != headless.ExitOK {
		t.Fatalf("clear exit = %d (%+v)", code, got)
	}
	data, _ := got["data"].(map[string]any)
	if data["cleared"] != true {
		t.Errorf("cleared = %v", data["cleared"])
	}
	if data["safety"] != "read_only" {
		t.Errorf("after-clear safety = %v, want read_only (kind default)", data["safety"])
	}
}

func TestOrgSafetySet_RejectsBothLevelAndClear(t *testing.T) {
	a, _ := newOrgTestApp()
	code, _ := runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "prod", "--level", "records", "--clear")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestOrgSafetySet_RejectsNeither(t *testing.T) {
	a, _ := newOrgTestApp()
	code, _ := runOrgCLI(t, a, "--json", "org", "safety", "set", "--org", "prod")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestOrgSafetySet_RejectsBogusLevel(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "prod", "--level", "yolo")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestOrgSafetySet_IdempotentReportsChangedFalse(t *testing.T) {
	a, _ := newOrgTestApp()
	// Sandbox defaults to records; set to records again — no change.
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "set",
		"--org", "sand", "--level", "records")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if changed, ok := got["changed"]; ok && changed == true {
		t.Errorf("no-op set reported Changed=true")
	}
}

func TestOrgSafetyGet_DefaultsToGet(t *testing.T) {
	a, _ := newOrgTestApp()
	// No subverb — defaults to get.
	code, got := runOrgCLI(t, a, "--json", "org", "safety", "--org", "prod")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d (%+v)", code, got)
	}
	if got["command"] != "org.safety.get" {
		t.Errorf("command = %v, want org.safety.get", got["command"])
	}
}

func TestOrgList_DefaultsToListVerb(t *testing.T) {
	a, _ := newOrgTestApp()
	code, got := runOrgCLI(t, a, "--json", "org")
	if code != headless.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if got["command"] != "org.list" {
		t.Errorf("command = %v", got["command"])
	}
}

func TestOrg_UnknownVerb(t *testing.T) {
	a, _ := newOrgTestApp()
	code, _ := runOrgCLI(t, a, "--json", "org", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestOrg_UnknownSafetySubverb(t *testing.T) {
	a, _ := newOrgTestApp()
	code, _ := runOrgCLI(t, a, "--json", "org", "safety", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}
