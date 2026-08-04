package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// newMetadataTestApp builds an app with three orgs spanning the
// safety ladder:
//   - prod   → read_only (default)            : blocks every write
//   - sand   → records (sandbox default)      : blocks metadata + apex
//   - scr    → full (scratch default)         : allows everything
//
// All four metadata writes share the same gate (WriteMetadata) so the
// distinguishing test is sand: a sandbox should NOT be able to do
// metadata writes without the user explicitly raising safety to
// "metadata" or "full".
func newMetadataTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "prod", Username: "boss@example.com"},
			{Alias: "sand", Username: "qa@x", IsSandbox: true},
			{Alias: "scr", Username: "s@x", IsScratch: true},
		},
	}
}

func runMetadataCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestMetadata_RejectsUnknownType(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "get",
		"--org", "prod", "--type", "Weirdness", "--id", "00Nabcd")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "unknown --type") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestMetadata_GetRequiresIDAndType(t *testing.T) {
	a := newMetadataTestApp()
	// Missing both.
	code, _ := runMetadataCLI(t, a, "--json", "metadata", "get", "--org", "prod")
	if code != headless.ExitInvalidArg {
		t.Errorf("missing both exit = %d", code)
	}
	// Missing id.
	code, _ = runMetadataCLI(t, a, "--json", "metadata", "get",
		"--org", "prod", "--type", "CustomField")
	if code != headless.ExitInvalidArg {
		t.Errorf("missing id exit = %d", code)
	}
}

func TestMetadataCreate_SafetyBlockedOnProd(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "prod", "--type", "ValidationRule",
		"--full-name", "Account.MyRule",
		"--patch", `{"active":true,"errorConditionFormula":"true"}`)
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	if details["required_write_kind"] != "metadata" {
		t.Errorf("details.required_write_kind = %v, want metadata",
			details["required_write_kind"])
	}
	if details["effective_safety"] != "read_only" {
		t.Errorf("details.effective_safety = %v, want read_only",
			details["effective_safety"])
	}
}

func TestMetadataCreate_SafetyBlockedOnSandbox(t *testing.T) {
	// Distinguishing test: sandbox at records-tier safety BLOCKS
	// metadata writes. The escalation from records → metadata is
	// what gates per-object schema changes.
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "sand", "--type", "ValidationRule",
		"--full-name", "Account.MyRule",
		"--patch", `{"active":true}`)
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	details, _ := errObj["details"].(map[string]any)
	if details["effective_safety"] != "records" {
		t.Errorf("details.effective_safety = %v, want records",
			details["effective_safety"])
	}
	if details["required_write_kind"] != "metadata" {
		t.Errorf("details.required_write_kind = %v, want metadata",
			details["required_write_kind"])
	}
}

func TestMetadataCreate_RequiresPatch(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "scr", "--type", "ValidationRule",
		"--full-name", "Account.MyRule")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--patch") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestMetadataCreate_RequiresFullName(t *testing.T) {
	a := newMetadataTestApp()
	code, _ := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "scr", "--type", "ValidationRule",
		"--patch", `{"active":true}`)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestMetadataCreate_RejectsBadJSON(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "scr", "--type", "ValidationRule",
		"--full-name", "Account.MyRule",
		"--patch", `not-json`)
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "invalid JSON") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestMetadataCreate_RejectsBothPatchAndFile(t *testing.T) {
	a := newMetadataTestApp()
	code, _ := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "scr", "--type", "ValidationRule",
		"--full-name", "Account.MyRule",
		"--patch", `{}`, "--patch-file", "/tmp/x")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestMetadataCreate_PatchFromFile(t *testing.T) {
	a := newMetadataTestApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "patch.json")
	if err := os.WriteFile(path, []byte(`{"active":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// scratch is full-tier so the gate lets us through; the test
	// then fails at the network boundary which fine — we just want
	// to know the patch resolved cleanly.
	code, got := runMetadataCLI(t, a, "--json", "metadata", "create",
		"--org", "scr", "--type", "ValidationRule",
		"--full-name", "Account.MyRule",
		"--patch-file", path)
	// We expect anything OTHER than invalid_argument with a patch-
	// resolution message — the gate would not catch this, and a
	// network call would fail. Either internal or invalid_argument
	// from the SF side is acceptable.
	if code == headless.ExitSafetyBlocked {
		t.Errorf("scratch should not be safety_blocked")
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj != nil {
		msg, _ := errObj["message"].(string)
		if strings.Contains(msg, "--patch") || strings.Contains(msg, "invalid JSON") {
			t.Errorf("expected patch resolved cleanly, got message: %s", msg)
		}
	}
}

func TestMetadataUpdate_SafetyBlockedOnProd(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "update",
		"--org", "prod", "--type", "CustomField",
		"--id", "00Nxxx00000abc",
		"--patch", `{"description":"new"}`)
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestMetadataDelete_SafetyBlockedOnSandbox(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "delete",
		"--org", "sand", "--type", "ValidationRule",
		"--id", "03Dxxx")
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	details, _ := errObj["details"].(map[string]any)
	// Delete requires full (WriteAnonymous), not metadata — destructive
	// operations sit one tier above non-destructive create/update.
	if details["required_write_kind"] != "full" {
		t.Errorf("details.required_write_kind = %v, want full",
			details["required_write_kind"])
	}
}

func TestMetadataDelete_BlockedAtMetadataTier(t *testing.T) {
	// Distinguishing test for the destructive escalation: an org
	// explicitly raised to "metadata" can create/update metadata but
	// CANNOT delete. Pins P1 so a future refactor that drops the
	// escalation to WriteMetadata gets caught.
	st := &settings.Settings{
		Orgs: map[string]settings.OrgConfig{
			"sand": {Safety: "metadata"}, // explicit override to metadata
		},
	}
	a := &app.App{
		Settings: st,
		Orgs: []sf.Org{
			{Alias: "sand", Username: "qa@x", IsSandbox: true},
		},
	}
	// Update is allowed at metadata tier.
	code, _ := runMetadataCLI(t, a, "--json", "metadata", "update",
		"--org", "sand", "--type", "CustomField",
		"--id", "00Nxxx", "--patch", `{"description":"x"}`)
	if code == headless.ExitSafetyBlocked {
		t.Errorf("metadata update on metadata-tier org was blocked; should be allowed")
	}
	// Delete must still be blocked.
	code, got := runMetadataCLI(t, a, "--json", "metadata", "delete",
		"--org", "sand", "--type", "CustomField", "--id", "00Nxxx")
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("delete on metadata-tier org exit = %d, want safety_blocked", code)
	}
	errObj, _ := got["error"].(map[string]any)
	details, _ := errObj["details"].(map[string]any)
	if details["effective_safety"] != "metadata" {
		t.Errorf("effective_safety = %v", details["effective_safety"])
	}
	if details["required_write_kind"] != "full" {
		t.Errorf("required_write_kind = %v, want full", details["required_write_kind"])
	}
}

func TestMetadata_SafetyBlockBeforeNetwork(t *testing.T) {
	// Defensive regression: every write gates before the network
	// call. If a future refactor moved the gate after the call this
	// test would fail with a network error.
	cases := []struct {
		name string
		argv []string
	}{
		{"create", []string{"--json", "metadata", "create",
			"--org", "prod", "--type", "ValidationRule",
			"--full-name", "X.Y", "--patch", `{}`}},
		{"update", []string{"--json", "metadata", "update",
			"--org", "prod", "--type", "CustomField",
			"--id", "00N", "--patch", `{}`}},
		{"delete", []string{"--json", "metadata", "delete",
			"--org", "prod", "--type", "ValidationRule", "--id", "03D"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newMetadataTestApp()
			code, _ := runMetadataCLI(t, a, c.argv...)
			if code != headless.ExitSafetyBlocked {
				t.Errorf("exit = %d, want safety_blocked", code)
			}
		})
	}
}

func TestMetadata_OrgNotFound(t *testing.T) {
	a := newMetadataTestApp()
	code, got := runMetadataCLI(t, a, "--json", "metadata", "get",
		"--org", "missing", "--type", "CustomField", "--id", "00N")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestMetadata_UnknownVerb(t *testing.T) {
	a := newMetadataTestApp()
	code, _ := runMetadataCLI(t, a, "--json", "metadata", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestMetadata_DefaultsToGet(t *testing.T) {
	a := newMetadataTestApp()
	_, got := runMetadataCLI(t, a, "--json", "metadata",
		"--org", "missing", "--type", "CustomField", "--id", "00N")
	if got["command"] != "metadata.get" {
		t.Errorf("command = %v, want metadata.get", got["command"])
	}
}

func TestKnownToolingTypesList_Sorted(t *testing.T) {
	got := knownToolingTypesList()
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("knownToolingTypesList not sorted: %v", got)
			break
		}
	}
	if !knownToolingTypes["CustomField"] {
		t.Error("CustomField must be in the closed set")
	}
}

func TestResolvePatch_Inline(t *testing.T) {
	got, err := resolvePatch(`{"active":true}`, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got["active"] != true {
		t.Errorf("got = %v", got)
	}
}

func TestResolvePatch_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	if err := os.WriteFile(path, []byte(`{"n":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolvePatch("", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got["n"] != float64(42) {
		t.Errorf("got = %v", got)
	}
}

func TestResolvePatch_BothRejected(t *testing.T) {
	if _, err := resolvePatch(`{}`, "/tmp/x"); err == nil {
		t.Error("expected error")
	}
}

func TestResolvePatch_EmptyOK(t *testing.T) {
	got, err := resolvePatch("", "")
	if err != nil || got != nil {
		t.Errorf("got = %v, err = %v", got, err)
	}
}

func TestResolvePatch_BadJSON(t *testing.T) {
	if _, err := resolvePatch("not json", ""); err == nil {
		t.Error("expected error")
	}
}

func TestWriteMetadataErr_Classification(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"NOT_FOUND: validation rule", headless.ErrNotFound},
		{"the row was not found", headless.ErrNotFound},
		{"FIELD_INTEGRITY_EXCEPTION: bad field", headless.ErrInvalidArgument},
		{"INVALID_FIELD: x", headless.ErrInvalidArgument},
		{"DUPLICATE_VALUE: name taken", headless.ErrInvalidArgument},
		{"malformed request body", headless.ErrInvalidArgument},
		{"missing_argument: full name", headless.ErrInvalidArgument},
		{"connection reset by peer", headless.ErrInternal},
		{"INVALID_SESSION_ID", headless.ErrAuth},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			var stdout bytes.Buffer
			writeMetadataErr("metadata.create", "dev@x", "ref",
				errSimple(c.msg), &stdout, headless.JSONMode)
			var got map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v\nout: %s", err, stdout.String())
			}
			errObj, _ := got["error"].(map[string]any)
			if errObj["code"] != c.want {
				t.Errorf("error.code = %v, want %v", errObj["code"], c.want)
			}
		})
	}
}
