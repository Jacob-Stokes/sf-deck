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

func newNotificationTestApp() *app.App {
	return &app.App{
		Settings: &settings.Settings{},
		Orgs: []sf.Org{
			{Alias: "prod", Username: "boss@example.com"},    // → read_only
			{Alias: "scr", Username: "s@x", IsScratch: true}, // → full
		},
	}
}

func runNotifCLI(t *testing.T, a *app.App, argv ...string) (int, map[string]any) {
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

func TestNotificationList_RejectsBadLimit(t *testing.T) {
	a := newNotificationTestApp()
	code, _ := runNotifCLI(t, a, "--json", "notification", "list",
		"--org", "prod", "--limit", "0")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
}

func TestNotificationMarkRead_RequiresIDOrAll(t *testing.T) {
	a := newNotificationTestApp()
	code, got := runNotifCLI(t, a, "--json", "notification", "mark-read",
		"--org", "prod")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "--id or --all") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestNotificationMarkRead_RejectsBothIDAndAll(t *testing.T) {
	a := newNotificationTestApp()
	code, got := runNotifCLI(t, a, "--json", "notification", "mark-read",
		"--org", "prod", "--id", "abc", "--all")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d, want %d", code, headless.ExitInvalidArg)
	}
	errObj, _ := got["error"].(map[string]any)
	if !strings.Contains(errObj["message"].(string), "mutually exclusive") {
		t.Errorf("error.message = %v", errObj["message"])
	}
}

func TestNotificationMarkRead_SafetyBlockedOnProd(t *testing.T) {
	// Production org is read_only by default — mark-read writes a
	// per-user state flag and is gated at WriteRecord. The gate must
	// fire BEFORE the network call.
	a := newNotificationTestApp()
	code, got := runNotifCLI(t, a, "--json", "notification", "mark-read",
		"--org", "prod", "--id", "0M0xxx")
	if code != headless.ExitSafetyBlocked {
		t.Fatalf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
	details, _ := errObj["details"].(map[string]any)
	if details["required_write_kind"] != "records" {
		t.Errorf("details.required_write_kind = %v", details["required_write_kind"])
	}
}

func TestNotificationMarkRead_SafetyBlockedAllVariant(t *testing.T) {
	// --all on prod should also be blocked.
	a := newNotificationTestApp()
	code, got := runNotifCLI(t, a, "--json", "notification", "mark-read",
		"--org", "prod", "--all")
	if code != headless.ExitSafetyBlocked {
		t.Errorf("exit = %d, want %d", code, headless.ExitSafetyBlocked)
	}
	errObj, _ := got["error"].(map[string]any)
	if errObj["code"] != headless.ErrSafetyBlocked {
		t.Errorf("error.code = %v", errObj["code"])
	}
}

func TestNotification_OrgNotFound(t *testing.T) {
	a := newNotificationTestApp()
	code, _ := runNotifCLI(t, a, "--json", "notification", "list",
		"--org", "missing")
	if code != headless.ExitNotFound {
		t.Errorf("exit = %d, want %d", code, headless.ExitNotFound)
	}
}

func TestNotification_UnknownVerb(t *testing.T) {
	a := newNotificationTestApp()
	code, _ := runNotifCLI(t, a, "--json", "notification", "weird",
		"--org", "prod")
	if code != headless.ExitInvalidArg {
		t.Errorf("exit = %d", code)
	}
}

func TestNotification_DefaultsToList(t *testing.T) {
	a := newNotificationTestApp()
	_, got := runNotifCLI(t, a, "--json", "notification", "--org", "missing")
	if got["command"] != "notification.list" {
		t.Errorf("command = %v, want notification.list", got["command"])
	}
}
