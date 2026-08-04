package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRedactSOQL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// No q= param: passes through.
		{"/services/data/v62.0/sobjects/Account/describe", "/services/data/v62.0/sobjects/Account/describe"},
		// q= with full SOQL: body replaced.
		{
			"/services/data/v62.0/query?q=SELECT+Id+FROM+Account+WHERE+Id+%3D+%270015%27",
			"/services/data/v62.0/query?q=<redacted>",
		},
		// q= followed by more params: body redacted, params kept.
		{
			"/services/data/v62.0/query?q=SELECT+Id&pretty=true",
			"/services/data/v62.0/query?q=<redacted>&pretty=true",
		},
		// q= mid-string after & : also handled.
		{
			"/x?other=1&q=SELECT",
			"/x?other=1&q=<redacted>",
		},
		// Cursor follow has no q=: passes through.
		{"/services/data/v62.0/query/0r8xx5S5xkLFptKACT-2000", "/services/data/v62.0/query/0r8xx5S5xkLFptKACT-2000"},
	}
	for _, c := range cases {
		got := redactSOQL(c.in)
		if got != c.want {
			t.Errorf("redactSOQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRedactCLIArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		// Standard query shell-out.
		{
			[]string{"data", "query", "-q", "SELECT Id FROM Account", "-o", "acme-test", "--json"},
			"data query -q <redacted> -o acme-test --json",
		},
		// --query long form.
		{
			[]string{"data", "query", "--query", "SELECT Id FROM Account"},
			"data query --query <redacted>",
		},
		// No -q: untouched.
		{
			[]string{"org", "list", "--json"},
			"org list --json",
		},
		// Edge: -q at the end with no value — still emits the flag.
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
