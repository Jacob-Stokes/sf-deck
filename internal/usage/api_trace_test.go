package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/redact"
)

func TestRedactSOQL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/services/data/v62.0/sobjects/Account/describe", "/services/data/v62.0/sobjects/Account/describe"},
		{
			"/services/data/v62.0/query?q=SELECT+Id+FROM+Account+WHERE+Id+%3D+%270015%27",
			"/services/data/v62.0/query?q=<redacted>",
		},
		{
			"/services/data/v62.0/query?q=SELECT+Id&pretty=true",
			"/services/data/v62.0/query?q=<redacted>&pretty=true",
		},
		{
			"/x?other=1&q=SELECT",
			"/x?other=1&q=<redacted>",
		},
		{"/services/data/v62.0/query/0r8xx5S5xkLFptKACT-2000", "/services/data/v62.0/query/0r8xx5S5xkLFptKACT-2000"},
	}
	for _, c := range cases {
		got := redactSOQL(c.in)
		if got != c.want {
			t.Errorf("redactSOQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAPITraceWriteRedactsRegisteredSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api-trace.jsonl")
	file, err := openAPITraceFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := "api-trace-runtime-secret-redaction-test"
	redact.RegisterSecret(secret)
	tracer := &apiTracer{file: file}
	tracer.write(apiTraceRecord{Event: "api_call", Err: "Bearer " + secret})
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("trace retained registered secret: %s", body)
	}
}

func TestRedactCLIArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{
			[]string{"data", "query", "-q", "SELECT Id FROM Account", "-o", "acme-test", "--json"},
			"data query -q <redacted> -o acme-test --json",
		},
		{
			[]string{"data", "query", "--query", "SELECT Id FROM Account"},
			"data query --query <redacted>",
		},
		{
			[]string{"org", "list", "--json"},
			"org list --json",
		},
		{
			[]string{"data", "query", "-q"},
			"data query -q",
		},
	}
	for _, c := range cases {
		got := redactCLIArgs(c.in)
		if got != c.want {
			t.Errorf("redactCLIArgs(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOpenAPITraceFileTightensPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api-trace.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed trace file: %v", err)
	}

	f, err := openAPITraceFile(path)
	if err != nil {
		t.Fatalf("openAPITraceFile: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close trace file: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trace file: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("trace file mode = %o, want %o", got, want)
	}
}
