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

func newRecordTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "dev", Username: "dev@example.com"},
		},
	}
}

func runRecordCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestRecordGet_RequiresID(t *testing.T) {
	a := newRecordTestApp()
	code, got := runRecordCLI(t, a, "--json", "record", "get", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--id") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecordGet_RejectsShortID(t *testing.T) {
	a := newRecordTestApp()
	code, got := runRecordCLI(t, a, "--json", "record", "get",
		"--org", "dev", "--id", "001abc")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "must be 15 or 18 chars") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecordRecent_RequiresObject(t *testing.T) {
	a := newRecordTestApp()
	code, got := runRecordCLI(t, a, "--json", "record", "recent", "--org", "dev")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--object") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecordRecent_RejectsNonPositiveLimit(t *testing.T) {
	a := newRecordTestApp()
	code, got := runRecordCLI(t, a, "--json", "record", "recent",
		"--org", "dev", "--object", "Account", "--limit", "0")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "positive") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestRecord_OrgNotFound(t *testing.T) {
	a := newRecordTestApp()
	code, got := runRecordCLI(t, a, "--json", "record", "get",
		"--org", "missing", "--id", "001gL00000lqv7BQAQ")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestWriteRecordErr_MissingRecordMapsNotFound(t *testing.T) {
	var stdout bytes.Buffer
	code := writeRecordErr("record.get", "dev@example.com", "Account", "001gL00000missing",
		errSimple("no Account with Id 001gL00000missing"), &stdout, headless.JSONMode)
	if code != headless.ExitNotFound {
		t.Fatalf("exit = %d, want %d", code, headless.ExitNotFound)
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout: %s", err, stdout.String())
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrNotFound {
		t.Errorf("error.code = %v, want %s", errObj["code"], headless.ErrNotFound)
	}
	details, _ := errObj["details"].(map[string]any)
	if details["object"] != "Account" || details["id"] != "001gL00000missing" {
		t.Errorf("details = %+v", details)
	}
}

func TestRecord_UnknownVerb(t *testing.T) {
	a := newRecordTestApp()
	code, _ := runRecordCLI(t, a, "--json", "record", "weird")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestRecord_DefaultsToRecentVerb(t *testing.T) {
	a := newRecordTestApp()
	// No verb → defaults to recent. The missing-object branch fires
	// because --object isn't passed; we just want to confirm the
	// command label is right.
	_, got := runRecordCLI(t, a, "--json", "record", "--org", "dev")
	if got["command"] != "record.recent" {
		t.Errorf("command = %v, want record.recent", got["command"])
	}
}
