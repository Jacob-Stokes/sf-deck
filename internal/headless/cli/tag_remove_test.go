package cli

import (
	"testing"
)

// tag remove needs a valid --id + --kind + --ref. The validation
// path is reachable without a real org or any tag fixtures.

func TestTagRemove_MissingArgs(t *testing.T) {
	a := newTestAppWithStore(t)
	cases := [][]string{
		{"tag", "remove", "--json"},
		{"tag", "remove", "--id", "1", "--json"},
		{"tag", "remove", "--id", "1", "--kind", "flow", "--json"},
		{"tag", "remove", "--kind", "flow", "--ref", "F", "--json"},
	}
	for _, argv := range cases {
		t.Run(argv[len(argv)-2], func(t *testing.T) {
			code, out, _ := runCLI(t, a, argv...)
			if code == 0 {
				t.Fatalf("expected non-zero exit; got %+v", out)
			}
			errEnv := out["error"].(map[string]any)
			if errEnv["code"] != "invalid_argument" {
				t.Errorf("code = %v", errEnv["code"])
			}
		})
	}
}
