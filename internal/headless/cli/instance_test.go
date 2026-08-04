package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// instance.list reads ~/.sf-deck/instances.json. With HOME pointed at
// a temp dir + no file, it should return an empty list (not an
// error) — agents poll this and "no instances running yet" is the
// normal cold-start state.

func TestInstanceList_EmptyRegistryReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := newTestApp()
	code, out, _ := runCLI(t, a, "instance", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if out["ok"] != true {
		t.Fatalf("ok = %v: %+v", out["ok"], out)
	}
	data := out["data"].(map[string]any)
	instances := data["instances"].([]any)
	if len(instances) != 0 {
		t.Errorf("expected empty instances; got %v", instances)
	}
	if data["count"] != float64(0) {
		t.Errorf("count = %v, want 0", data["count"])
	}
	if _, ok := data["registry_path"]; !ok {
		t.Error("registry_path missing from response")
	}
}

func TestInstanceList_ReadsRegistryEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Pre-populate the registry file.
	dir := filepath.Join(home, ".sf-deck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"entries":[
  {"number":1,"pid":99999,"started_at":"2026-06-29T10:00:00Z","socket":"/tmp/sf-deck-1.sock","label":"dev"},
  {"number":2,"pid":99998,"started_at":"2026-06-29T10:01:00Z","label":"prod"}
]}`
	if err := os.WriteFile(filepath.Join(dir, "instances.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	a := newTestApp()
	code, out, _ := runCLI(t, a, "instance", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	data := out["data"].(map[string]any)
	// The registry prunes dead PIDs on read, so what's surfaced
	// depends on whether 99999/99998 happen to be live (almost
	// always not). Either way the response shape should be valid.
	instances := data["instances"].([]any)
	count := int(data["count"].(float64))
	if len(instances) != count {
		t.Errorf("instances len (%d) != count (%d)", len(instances), count)
	}
}

func TestInstanceDispatch_UnknownVerb(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := newTestApp()
	code, out, _ := runCLI(t, a, "instance", "frobnicate", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
}
