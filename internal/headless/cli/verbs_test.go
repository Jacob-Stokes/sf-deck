package cli

import (
	"testing"
)

// `verbs list` is the registry-driven introspection surface agents
// use to discover what sf-deck can do. Robust tests here matter
// because changing the JSON shape silently breaks agent code.

func TestVerbsList_BasicShape(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "verbs", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if out["ok"] != true {
		t.Fatalf("ok = %v", out["ok"])
	}
	if out["command"] != "verbs.list" {
		t.Errorf("command = %v", out["command"])
	}
	data := out["data"].(map[string]any)
	if data["surface"] != "" {
		t.Errorf("surface filter should be empty by default, got %v", data["surface"])
	}
	if _, ok := data["count"].(float64); !ok {
		t.Errorf("count missing or wrong type: %v", data["count"])
	}
	verbs := data["verbs"].([]any)
	if len(verbs) == 0 {
		t.Fatal("expected at least one verb in the registry")
	}
	// Spot-check the shape of one verb.
	first := verbs[0].(map[string]any)
	for _, k := range []string{"noun", "verb", "qualified", "summary", "stability"} {
		if _, ok := first[k]; !ok {
			t.Errorf("verb missing key %q: %+v", k, first)
		}
	}
}

func TestVerbsList_FilterByCLI(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "verbs", "list", "--surface", "cli", "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	data := out["data"].(map[string]any)
	if data["surface"] != "cli" {
		t.Errorf("surface = %v, want cli", data["surface"])
	}
	verbs := data["verbs"].([]any)
	for _, v := range verbs {
		m := v.(map[string]any)
		if _, ok := m["cli"]; !ok {
			t.Errorf("verb %v lacks cli binding even with --surface=cli", m["qualified"])
		}
	}
}

func TestVerbsList_FilterByIPC(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "verbs", "list", "--surface", "ipc", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	data := out["data"].(map[string]any)
	verbs := data["verbs"].([]any)
	for _, v := range verbs {
		m := v.(map[string]any)
		if _, ok := m["ipc"]; !ok {
			t.Errorf("verb %v lacks ipc binding under --surface=ipc", m["qualified"])
		}
	}
}

func TestVerbsList_UnknownSurfaceErrors(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "verbs", "list", "--surface", "frob", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if out["ok"] != false {
		t.Errorf("ok should be false: %+v", out)
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v, want invalid_argument", errEnv["code"])
	}
}

func TestVerbsDispatch_UnknownVerb(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "verbs", "frobnicate", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
}
