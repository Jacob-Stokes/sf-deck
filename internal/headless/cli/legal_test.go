package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/headless"
	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
)

func TestLegalAcceptAndStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	code, got, _ := runCLI(t, nil, "legal", "status", "--json")
	if code != 0 {
		t.Fatalf("initial status code = %d", code)
	}
	data := got["data"].(map[string]any)
	if data["accepted"] != false {
		t.Fatalf("initial accepted = %#v", data["accepted"])
	}

	code, got, _ = runCLI(t, nil, "legal", "accept", "--yes", "--json")
	if code != 0 || got["changed"] != true {
		t.Fatalf("accept code=%d response=%#v", code, got)
	}
	data = got["data"].(map[string]any)
	if data["policy_version"] != productlegal.PolicyVersion || data["accepted"] != true {
		t.Fatalf("accepted data = %#v", data)
	}

	code, got, _ = runCLI(t, nil, "legal", "status", "--json")
	if code != 0 {
		t.Fatalf("final status code = %d", code)
	}
	data = got["data"].(map[string]any)
	if data["accepted"] != true {
		t.Fatalf("final accepted = %#v", data["accepted"])
	}
}

func TestLegalAcceptRequiresExplicitYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	code, got, _ := runCLI(t, nil, "legal", "accept", "--json")
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	errBody := got["error"].(map[string]any)
	if errBody["code"] != "invalid_argument" {
		t.Fatalf("error = %#v", errBody)
	}
}

func TestLegalAndDataTextAndErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if code, _, out := runCLI(t, nil, "legal", "status"); code != 0 || !strings.Contains(out, "not accepted") {
		t.Fatalf("legal status: code=%d out=%q", code, out)
	}
	if code, got, _ := runCLI(t, nil, "legal", "wat", "--json"); code != 2 || got["ok"] != false {
		t.Fatalf("unknown legal: code=%d response=%#v", code, got)
	}
	if code, _, out := runCLI(t, nil, "data", "inspect"); code != 0 || !strings.Contains(out, "record payloads persisted: no") {
		t.Fatalf("data inspect: code=%d out=%q", code, out)
	}
	if code, got, _ := runCLI(t, nil, "data", "erase", "--json"); code != 2 || got["ok"] != false {
		t.Fatalf("unconfirmed erase: code=%d response=%#v", code, got)
	}
	if code, got, _ := runCLI(t, nil, "data", "wat", "--json"); code != 2 || got["ok"] != false {
		t.Fatalf("unknown data: code=%d response=%#v", code, got)
	}

	var out bytes.Buffer
	code := WriteLegalRequired("org.list", &out, headless.JSONMode)
	if code != 2 || !strings.Contains(out.String(), "accept_command") {
		t.Fatalf("legal-required response: code=%d out=%q", code, out.String())
	}
}

func TestDataInspectAndErase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	appDir := filepath.Join(home, ".sf-deck")
	bundlesDir := filepath.Join(home, "sf-deck-bundles")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "settings.toml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bundlesDir, 0o700); err != nil {
		t.Fatal(err)
	}

	code, got, _ := runCLI(t, nil, "data", "inspect", "--json")
	if code != 0 {
		t.Fatalf("inspect code = %d", code)
	}
	if got["data"].(map[string]any)["record_payloads_persisted"] != false {
		t.Fatalf("inspect data = %#v", got["data"])
	}

	code, _, _ = runCLI(t, nil, "data", "erase", "--yes", "--json")
	if code != 0 {
		t.Fatalf("erase code = %d", code)
	}
	if _, err := os.Stat(appDir); !os.IsNotExist(err) {
		t.Fatalf("app dir still exists: %v", err)
	}
	if _, err := os.Stat(bundlesDir); err != nil {
		t.Fatalf("bundles should remain without --include-bundles: %v", err)
	}
}

func TestDataEraseCanIncludeDefaultBundles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	appDir := filepath.Join(home, ".sf-deck")
	bundlesDir := filepath.Join(home, "sf-deck-bundles")
	for _, dir := range []string{appDir, bundlesDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	code, _, _ := runCLI(t, nil, "data", "erase", "--yes", "--include-bundles", "--json")
	if code != 0 {
		t.Fatalf("erase code = %d", code)
	}
	for _, dir := range []string{appDir, bundlesDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", dir, err)
		}
	}
}

func TestOrgLogoutRequiresTargetAndConfirmation(t *testing.T) {
	a := newTestApp()
	if code, got, _ := runCLI(t, a, "org", "logout", "--json"); code != 2 || got["ok"] != false {
		t.Fatalf("missing org: code=%d response=%#v", code, got)
	}
	if code, got, _ := runCLI(t, a, "org", "logout", "--org", "dev", "--json"); code != 2 || got["ok"] != false {
		t.Fatalf("missing confirmation: code=%d response=%#v", code, got)
	}
}
