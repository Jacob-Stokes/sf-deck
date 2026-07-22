package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// newWriteTestApp returns an App with two stub orgs: a production org
// that defaults to read_only (safety gate WILL block writes) and a
// scratch org that defaults to full (safety gate will let writes
// through to the network — those still fail at the boundary in tests,
// but they fail *after* the gate, which is what we want to verify).
func newWriteTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "prod", Username: "boss@example.com"},    // → read_only
			{Alias: "scr", Username: "s@x", IsScratch: true}, // → full
		},
	}
}

func runWriteCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestRecordUpdate_RequiresID(t *testing.T) {
	a := newWriteTestApp()
	code, got := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "scr", "--field", "Name=X")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--id") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecordUpdate_RequiresFields(t *testing.T) {
	a := newWriteTestApp()
	code, got := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "scr", "--id", "001gL00000lqv7BQAQ")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--field") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecordUpdate_RejectsShortID(t *testing.T) {
	a := newWriteTestApp()
	code, _ := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "scr", "--id", "001abc", "--field", "Name=X")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestRecordUpdate_RejectsBadFieldSyntax(t *testing.T) {
	a := newWriteTestApp()
	// "Name" without = should be rejected at flag.Set time.
	code, _ := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "scr", "--id", "001gL00000lqv7BQAQ", "--field", "JustName")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestRecordUpdate_SafetyBlockedOnProd(t *testing.T) {
	// Production org defaults to SafetyReadOnly. A WriteRecord call
	// must hit the typed safety_blocked envelope BEFORE any network
	// call.
	a := newWriteTestApp()
	code, got := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "prod", "--object", "Account",
		"--id", "001gL00000lqv7BQAQ", "--field", "Name=Boom")
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	if got["ok"] != false {
		t.Errorf("ok = %v", got["ok"])
	}
	if got["command"] != "record.update" {
		t.Errorf("command = %v", got["command"])
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v, want %s", errObj["code"], headless.ErrSafetyBlocked)
	}
	// Shape matches docs/headless-mode-plan.md example.
	details, _ := errObj["details"].(map[string]any)
	if details["required_write_kind"] != "records" {
		t.Errorf("details.required_write_kind = %v", details["required_write_kind"])
	}
	if details["effective_safety"] != "read_only" {
		t.Errorf("details.effective_safety = %v", details["effective_safety"])
	}
	if details["target"] != "prod" {
		t.Errorf("details.target = %v", details["target"])
	}
}

func TestRecordUpdate_SafetyBlockBeforeNetwork(t *testing.T) {
	// Defensive regression: even without --object the safety gate
	// must fire BEFORE we try to resolve the sObject via
	// sf.ListSObjects (which would shell out). If the gate is wrong
	// this test would hang or fail with a network error instead of
	// returning safety_blocked.
	a := newWriteTestApp()
	code, got := runWriteCLI(t, a, "--json", "record", "update",
		"--org", "prod",
		"--id", "001gL00000lqv7BQAQ", "--field", "Name=Boom")
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want safety_blocked", code)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestKVFlag_Parses(t *testing.T) {
	k := &kvFlag{}
	if err := k.Set("Name=Acme"); err != nil {
		t.Fatalf("Set name: %v", err)
	}
	if err := k.Set("Industry=Tech"); err != nil {
		t.Fatalf("Set industry: %v", err)
	}
	if err := k.Set("Description=Multi=word=value"); err != nil {
		// = inside the value is fine — we split on the first one.
		t.Fatalf("Set with embedded =: %v", err)
	}
	if k.values["Name"] != "Acme" {
		t.Errorf("Name = %q", k.values["Name"])
	}
	if k.values["Description"] != "Multi=word=value" {
		t.Errorf("Description = %q", k.values["Description"])
	}
}

func TestKVFlag_RejectsBadInput(t *testing.T) {
	cases := []string{"NoEquals", "=NoKey", ""}
	for _, c := range cases {
		k := &kvFlag{}
		if err := k.Set(c); err == nil {
			t.Errorf("Set(%q) returned nil, want error", c)
		}
	}
}

func TestWriteSafetyBlocked_EnvelopeShape(t *testing.T) {
	// Build the canonical BlockedError + render it. Pins the wire
	// shape independently of which CLI verb produced it.
	be := app.BlockedError{
		Target:   "prod",
		Username: "boss@x",
		Required: settings.WriteMetadata,
		Actual:   settings.SafetyReadOnly,
	}
	var stdout bytes.Buffer
	code := writeSafetyBlocked("bundle.deploy", "boss@x", be, &stdout, headless.JSONMode)
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("code = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, stdout.String())
	}
	if got["command"] != "bundle.deploy" {
		t.Errorf("command = %v", got["command"])
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	if details["required_write_kind"] != "metadata" {
		t.Errorf("details.required_write_kind = %v", details["required_write_kind"])
	}
	if details["effective_safety"] != "read_only" {
		t.Errorf("details.effective_safety = %v", details["effective_safety"])
	}
}

func TestWriteKindString(t *testing.T) {
	cases := []struct {
		k    settings.WriteKind
		want string
	}{
		{settings.WriteRecord, "records"},
		{settings.WriteMetadata, "metadata"},
		{settings.WriteAnonymous, "full"},
		{settings.WriteKind(99), "unknown"},
	}
	for _, c := range cases {
		if got := writeKindString(c.k); got != c.want {
			t.Errorf("writeKindString(%d) = %q, want %q", c.k, got, c.want)
		}
	}
}
