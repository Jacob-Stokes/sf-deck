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

// newSOQLTestApp builds an app with a single fake org. The actual
// query against Salesforce is mocked at the boundary: sf.Query goes
// through RESTClient → falls back to runSF, both of which would fail
// in tests. These tests cover the *parser* and the *error-mapping*
// layers, not the network call.
func newSOQLTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "dev", Username: "dev@example.com"},
		},
	}
}

func runSOQLCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestSOQLRun_RequiresQuery(t *testing.T) {
	a := newSOQLTestApp()
	code, got := runSOQLCLI(t, a, "--json", "soql", "run", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrInvalidArgument {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestSOQLRun_RejectsBothQueryAndFile(t *testing.T) {
	a := newSOQLTestApp()
	code, _ := runSOQLCLI(t, a, "--json", "soql", "run", "--org", "dev",
		"--query", "SELECT Id FROM Account",
		"--query-file", "/tmp/nope.soql")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestSOQLRun_OrgNotFound(t *testing.T) {
	a := newSOQLTestApp()
	code, got := runSOQLCLI(t, a, "--json", "soql", "run",
		"--org", "missing", "--query", "SELECT Id FROM Account")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestResolveSOQL_Inline(t *testing.T) {
	got, err := resolveSOQL("SELECT Id FROM Account", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "SELECT Id FROM Account" {
		t.Errorf("got = %q", got)
	}
}

func TestResolveSOQL_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.soql")
	want := "SELECT Id, Name FROM Contact"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := resolveSOQL("", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != want {
		t.Errorf("got = %q", got)
	}
}

func TestResolveSOQL_FileMissing(t *testing.T) {
	_, err := resolveSOQL("", "/nonexistent/path/q.soql")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSOQL_BothRejected(t *testing.T) {
	_, err := resolveSOQL("SELECT 1", "/tmp/q.soql")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSOQL_EmptyOK(t *testing.T) {
	// Both empty returns "" + nil — caller decides what to do.
	got, err := resolveSOQL("", "")
	if err != nil || got != "" {
		t.Errorf("got=%q err=%v", got, err)
	}
}

func TestWriteSOQLErr_AuthMappedToExit5(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want string
	}{
		{"INVALID_SESSION_ID", "INVALID_SESSION_ID: bad token", headless.ErrAuth},
		{"401 status", "request failed: 401 Unauthorized", headless.ErrAuth},
		{"MALFORMED_QUERY", "MALFORMED_QUERY: unexpected token x", headless.ErrInvalidArgument},
		{"INVALID_FIELD", "INVALID_FIELD: No such column Bogus", headless.ErrInvalidArgument},
		{"INVALID_TYPE", "INVALID_TYPE: sObject type Account__c not supported", headless.ErrInvalidArgument},
		{"other goes internal", "connection refused", headless.ErrInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout bytes.Buffer
			writeSOQLErr("soql.run", "dev@x",
				errSimple(c.msg), &stdout, headless.JSONMode)
			var got map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			errObj, _ := got["error"].(map[string]any)
			if errObj["code"] != c.want {
				t.Errorf("error.code = %v, want %v", errObj["code"], c.want)
			}
		})
	}
}

func TestSOQLRun_UnknownVerb(t *testing.T) {
	a := newSOQLTestApp()
	code, _ := runSOQLCLI(t, a, "--json", "soql", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestSOQLRun_DefaultsToRunVerb(t *testing.T) {
	a := newSOQLTestApp()
	// No verb + no query → should hit `run` verb's missing-query
	// branch, not "unknown verb".
	code, got := runSOQLCLI(t, a, "--json", "soql", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
	if got["command"] != "soql.run" {
		t.Errorf("command = %v, want soql.run", got["command"])
	}
}

// errSimple is a tiny error type for the writeSOQLErr table test.
type errSimple string

func (e errSimple) Error() string { return string(e) }

// Verify JSON-mode output keys are stable for soql.run (caller-visible
// contract — agents grep these). Uses the same input-validation
// fail-path that doesn't touch the network.
func TestSOQLRun_FailureEnvelopeKeys(t *testing.T) {
	a := newSOQLTestApp()
	var stdout, stderr bytes.Buffer
	args := Parse([]string{"--json", "soql", "run", "--org", "dev"})
	Dispatch(a, args, &stdout, &stderr)
	for _, want := range []string{`"ok":`, `"command":`, `"error":`, `"code":`, `"message":`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output missing %s\n%s", want, stdout.String())
		}
	}
}
