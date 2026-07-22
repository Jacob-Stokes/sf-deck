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

// newObjectTestApp builds an app with a stub org. The actual network
// path (sf.ListSObjects / sf.Describe) requires a real org; tests
// here cover the parse + error-mapping layers.
func newObjectTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "dev", Username: "dev@example.com"},
		},
	}
}

func runObjectCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestObjectShow_RequiresName(t *testing.T) {
	a := newObjectTestApp()
	code, got := runObjectCLI(t, a, "--json", "object", "show", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--name") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestObjectFields_RequiresName(t *testing.T) {
	a := newObjectTestApp()
	code, _ := runObjectCLI(t, a, "--json", "object", "fields", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestObject_OrgNotFound(t *testing.T) {
	a := newObjectTestApp()
	code, got := runObjectCLI(t, a, "--json", "object", "show",
		"--org", "missing", "--name", "Account")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestObject_UnknownVerb(t *testing.T) {
	a := newObjectTestApp()
	code, _ := runObjectCLI(t, a, "--json", "object", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestObject_DefaultsToListVerb(t *testing.T) {
	// "object" with no verb routes to list — verb default. List will
	// fail at the network boundary in tests, but we can confirm the
	// command label is correct.
	a := newObjectTestApp()
	code, got := runObjectCLI(t, a, "--json", "object", "--org", "missing")
	// missing org makes this not_found; we just want the command
	// label to be object.list, confirming the default-verb route.
	_ = code
	if got["command"] != "object.list" {
		t.Errorf("command = %v, want object.list", got["command"])
	}
}

func TestWriteDescribeErr_NotFoundMapping(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want string
	}{
		{"NOT_FOUND uppercase", "NOT_FOUND: nope", headless.ErrNotFound},
		{"not found lowercase", "object not found", headless.ErrNotFound},
		{"404", "request failed: 404 Not Found", headless.ErrNotFound},
		{"unrelated", "connection reset", headless.ErrInternal},
		{"auth", "INVALID_SESSION_ID: expired", headless.ErrAuth},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout bytes.Buffer
			writeDescribeErr("object.show", "dev@x", "Account",
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

// errSimple is a tiny error helper. Already defined in soql_test.go;
// re-use it here.
