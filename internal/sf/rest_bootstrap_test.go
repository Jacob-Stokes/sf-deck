package sf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTBootstrapUsesExplicitTokenWhenDisplayRedacts(t *testing.T) {
	InvalidateRESTClients()
	defer InvalidateRESTClients()
	logPath := installRESTBootstrapFakeSF(t, `#!/bin/sh
echo "$*" >> "$SF_FAKE_LOG"
if [ "$1 $2" = "org display" ]; then
  printf '{"result":{"accessToken":"REDACTED","instanceUrl":"https://example.test","apiVersion":"65.0"}}'
  exit 0
fi
if [ "$1 $2 $3" = "org auth show-access-token" ]; then
  printf '{"result":{"accessToken":"EXPLICIT_TOKEN"}}'
  exit 0
fi
echo "unexpected sf command: $*" >&2
exit 1
`)

	c, err := RESTClient("dev")
	if err != nil {
		t.Fatal(err)
	}
	if c.accessToken != "EXPLICIT_TOKEN" {
		t.Fatalf("accessToken = %q, want EXPLICIT_TOKEN", c.accessToken)
	}
	if c.instanceURL != "https://example.test" {
		t.Fatalf("instanceURL = %q, want https://example.test", c.instanceURL)
	}
	logged := readFileString(t, logPath)
	if !containsLine(logged, "org auth show-access-token --target-org dev --no-prompt --json") {
		t.Fatalf("explicit token command was not called; log:\n%s", logged)
	}
}

func TestRESTBootstrapKeepsLegacyDisplayToken(t *testing.T) {
	InvalidateRESTClients()
	defer InvalidateRESTClients()
	logPath := installRESTBootstrapFakeSF(t, `#!/bin/sh
echo "$*" >> "$SF_FAKE_LOG"
if [ "$1 $2" = "org display" ]; then
  printf '{"result":{"accessToken":"DISPLAY_TOKEN","instanceUrl":"https://legacy.test","apiVersion":"62.0"}}'
  exit 0
fi
echo "unexpected sf command: $*" >&2
exit 1
`)

	c, err := RESTClient("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if c.accessToken != "DISPLAY_TOKEN" {
		t.Fatalf("accessToken = %q, want DISPLAY_TOKEN", c.accessToken)
	}
	logged := readFileString(t, logPath)
	if containsLine(logged, "org auth show-access-token --target-org legacy --no-prompt --json") {
		t.Fatalf("explicit token command should not be called for legacy token; log:\n%s", logged)
	}
}

func installRESTBootstrapFakeSF(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "sf")
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "sf.log")
	t.Setenv("SF_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func containsLine(s, want string) bool {
	for _, line := range strings.Split(s, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
