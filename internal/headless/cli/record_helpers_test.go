package cli

import (
	"sort"
	"strings"
	"testing"
)

// kvFlag is the repeated --field K=V flag handler shared by
// record.create / record.update. Pure value type — test in
// isolation.

func TestKVFlag_AcceptsPairs(t *testing.T) {
	var k kvFlag
	for _, s := range []string{"Name=Acme", "Industry=Logistics", "Phone=555-0100"} {
		if err := k.Set(s); err != nil {
			t.Fatalf("Set(%q): %v", s, err)
		}
	}
	if len(k.values) != 3 {
		t.Errorf("values len = %d, want 3", len(k.values))
	}
	if k.values["Name"] != "Acme" || k.values["Industry"] != "Logistics" {
		t.Errorf("values = %+v", k.values)
	}
}

func TestKVFlag_RejectsMalformed(t *testing.T) {
	var k kvFlag
	bads := []string{"", "Name", "=Acme", "  =Acme"}
	for _, s := range bads {
		if err := k.Set(s); err == nil {
			t.Errorf("Set(%q) should fail", s)
		}
	}
}

func TestKVFlag_AllowsEmptyValue(t *testing.T) {
	var k kvFlag
	if err := k.Set("Phone="); err != nil {
		t.Fatalf("Phone= should succeed: %v", err)
	}
	if v, ok := k.values["Phone"]; !ok || v != "" {
		t.Errorf("values[Phone] = %q (ok=%v), want \"\"", v, ok)
	}
}

func TestKVFlag_RepeatedKeyOverwrites(t *testing.T) {
	var k kvFlag
	_ = k.Set("Name=First")
	_ = k.Set("Name=Second")
	if k.values["Name"] != "Second" {
		t.Errorf("Name = %q, want Second (repeated key should overwrite)", k.values["Name"])
	}
}

func TestKVFlag_ValueWithEquals(t *testing.T) {
	// Values may contain '=' (e.g. base64 padding) — we split on the FIRST '='.
	var k kvFlag
	if err := k.Set("Token=abc=def="); err != nil {
		t.Fatal(err)
	}
	if k.values["Token"] != "abc=def=" {
		t.Errorf("Token = %q, want 'abc=def='", k.values["Token"])
	}
}

func TestKVFlag_StringStable(t *testing.T) {
	var k kvFlag
	if got := k.String(); got != "" {
		t.Errorf("zero kvFlag.String() = %q, want empty", got)
	}
	_ = k.Set("A=1")
	_ = k.Set("B=2")
	got := strings.Split(k.String(), ",")
	sort.Strings(got)
	want := []string{"A=1", "B=2"}
	if len(got) != len(want) {
		t.Fatalf("String() = %q (split %v)", k.String(), got)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("[%d] = %q, want %q", i, got[i], p)
		}
	}
}

func TestKVFlag_KeysExtractor(t *testing.T) {
	var k kvFlag
	_ = k.Set("A=1")
	_ = k.Set("B=2")
	keys := kvFlagKeys(&k)
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "A" || keys[1] != "B" {
		t.Errorf("keys = %v", keys)
	}
}

// ----- record CLI validation -----

func TestRecordUpdate_MissingIDFails(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "record", "update", "--field", "Name=X", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
}

func TestRecordCreate_MissingObjectFails(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "record", "create", "--field", "Name=X", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
}

func TestRecordDelete_MissingIDFails(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "record", "delete", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
}

func TestRecordDelete_ShortIDFails(t *testing.T) {
	a := newTestApp()
	code, out, _ := runCLI(t, a, "record", "delete", "--id", "abc", "--json")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	errEnv := out["error"].(map[string]any)
	if errEnv["code"] != "invalid_argument" {
		t.Errorf("error.code = %v", errEnv["code"])
	}
	msg, _ := errEnv["message"].(string)
	if !strings.Contains(msg, "15 or 18") {
		t.Errorf("expected '15 or 18' in error message; got %q", msg)
	}
}
