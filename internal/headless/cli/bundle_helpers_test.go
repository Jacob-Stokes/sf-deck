package cli

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Pure-function helpers in bundle.go are easy to test without an
// org. These cover the parser/validator surface so future edits
// don't silently break the agent contract (the IDs/statuses they
// extract are what scripts poll on).

// ----- buildDeployOpts ---------------------------------------------

func TestBuildDeployOpts_EmptyTestsLevel(t *testing.T) {
	got, err := buildDeployOpts("", "")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if got.TestLevel != "" {
		t.Errorf("empty TestLevel expected, got %q", got.TestLevel)
	}
}

func TestBuildDeployOpts_TestClassesWithoutLevel(t *testing.T) {
	_, err := buildDeployOpts("", "MyTest")
	if err == nil || !strings.Contains(err.Error(), "RunSpecifiedTests") {
		t.Errorf("expected 'requires RunSpecifiedTests' err, got %v", err)
	}
}

func TestBuildDeployOpts_NormalisesLevelAliases(t *testing.T) {
	cases := []struct {
		input string
		want  sf.DeployTestLevel
	}{
		{"NoTestRun", sf.TestLevelNoTestRun},
		{"no-test-run", sf.TestLevelNoTestRun},
		{"NOTESTRUN", sf.TestLevelNoTestRun},
		{"RunLocalTests", sf.TestLevelRunLocalTests},
		{"run-local", sf.TestLevelRunLocalTests},
		{"local", sf.TestLevelRunLocalTests},
		{"RunAllTestsInOrg", sf.TestLevelRunAllTestsInOrg},
		{"run-all", sf.TestLevelRunAllTestsInOrg},
		{"all", sf.TestLevelRunAllTestsInOrg},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got, err := buildDeployOpts(c.input, "")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got.TestLevel != c.want {
				t.Errorf("TestLevel = %q, want %q", got.TestLevel, c.want)
			}
		})
	}
}

func TestBuildDeployOpts_UnknownLevel(t *testing.T) {
	_, err := buildDeployOpts("InventedLevel", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error mention 'unknown'; got %v", err)
	}
}

func TestBuildDeployOpts_RunSpecifiedRequiresClasses(t *testing.T) {
	_, err := buildDeployOpts("RunSpecifiedTests", "")
	if err == nil {
		t.Fatal("expected requires-test-classes error")
	}
	if !strings.Contains(err.Error(), "test-classes") {
		t.Errorf("error mention 'test-classes'; got %v", err)
	}
}

func TestBuildDeployOpts_RunSpecifiedSplitsClasses(t *testing.T) {
	opts, err := buildDeployOpts("RunSpecifiedTests", "FooTest, BarTest ,BazTest")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FooTest", "BarTest", "BazTest"}
	if len(opts.TestClasses) != len(want) {
		t.Fatalf("classes len = %d, want %d (%v)", len(opts.TestClasses), len(want), opts.TestClasses)
	}
	for i, c := range want {
		if opts.TestClasses[i] != c {
			t.Errorf("class[%d] = %q, want %q", i, opts.TestClasses[i], c)
		}
	}
}

func TestBuildDeployOpts_ClassesOnNonSpecified(t *testing.T) {
	_, err := buildDeployOpts("NoTestRun", "FooTest")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "only meaningful") {
		t.Errorf("expected 'only meaningful' err; got %v", err)
	}
}

// ----- parseDeployJobID -------------------------------------------

func TestParseDeployJobID_ExtractsID(t *testing.T) {
	out := []byte(`{"result":{"id":"0Af0X00000Wxyz","status":"InProgress"}}`)
	if id := parseDeployJobID(out); id != "0Af0X00000Wxyz" {
		t.Errorf("id = %q, want 0Af0X00000Wxyz", id)
	}
}

func TestParseDeployJobID_MalformedReturnsEmpty(t *testing.T) {
	if id := parseDeployJobID([]byte(`{not-json`)); id != "" {
		t.Errorf("expected empty id on bad JSON; got %q", id)
	}
}

func TestParseDeployJobID_MissingIDReturnsEmpty(t *testing.T) {
	out := []byte(`{"result":{"status":"InProgress"}}`)
	if id := parseDeployJobID(out); id != "" {
		t.Errorf("expected empty id when missing; got %q", id)
	}
}

// ----- parseDeployStatus ------------------------------------------

func TestParseDeployStatus_ExtractsAllFields(t *testing.T) {
	out := []byte(`{"result":{
        "status":"Succeeded",
        "done":true,
        "success":true,
        "numberComponentsTotal":42,
        "numberComponentErrors":0,
        "numberTestErrors":0,
        "checkOnly":true
    }}`)
	status, ok := parseDeployStatus(out)
	if !ok {
		t.Fatal("expected ok=true on valid JSON")
	}
	if status["status"] != "Succeeded" {
		t.Errorf("status = %v", status["status"])
	}
	if status["done"] != true {
		t.Errorf("done = %v", status["done"])
	}
	if status["number_components_total"] != 42 {
		t.Errorf("components_total = %v", status["number_components_total"])
	}
	if status["check_only"] != true {
		t.Errorf("check_only = %v", status["check_only"])
	}
}

func TestParseDeployStatus_MalformedReturnsNotOk(t *testing.T) {
	if _, ok := parseDeployStatus([]byte(`{bad`)); ok {
		t.Error("expected ok=false on bad JSON")
	}
}

// ----- jsonUnmarshalLenient ---------------------------------------

func TestJSONUnmarshalLenient_EmptyErrors(t *testing.T) {
	var v map[string]any
	if err := jsonUnmarshalLenient(nil, &v); err == nil {
		t.Error("expected error on empty input")
	}
	if err := jsonUnmarshalLenient([]byte{}, &v); err == nil {
		t.Error("expected error on zero-len input")
	}
}

func TestJSONUnmarshalLenient_PropagatesParseError(t *testing.T) {
	var v map[string]any
	if err := jsonUnmarshalLenient([]byte(`{nope`), &v); err == nil {
		t.Error("expected parse error")
	}
}

func TestJSONUnmarshalLenient_HappyPath(t *testing.T) {
	var v map[string]any
	if err := jsonUnmarshalLenient([]byte(`{"a":1}`), &v); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v["a"] != float64(1) {
		t.Errorf("a = %v, want 1", v["a"])
	}
}

// ----- parseBool ---------------------------------------------------

func TestParseBool_AcceptsCommonForms(t *testing.T) {
	trues := []string{"true", "True", "TRUE", "t", "T", "1", "yes", "Y", "y"}
	for _, s := range trues {
		got, err := parseBool(s)
		if err != nil || !got {
			t.Errorf("parseBool(%q) = (%v, %v); want (true, nil)", s, got, err)
		}
	}
	falses := []string{"false", "False", "FALSE", "f", "F", "0", "no", "N", "n"}
	for _, s := range falses {
		got, err := parseBool(s)
		if err != nil || got {
			t.Errorf("parseBool(%q) = (%v, %v); want (false, nil)", s, got, err)
		}
	}
	// Whitespace tolerated
	got, err := parseBool(" true ")
	if err != nil || !got {
		t.Errorf("parseBool(\" true \") = (%v, %v); want (true, nil)", got, err)
	}
}

func TestParseBool_RejectsGarbage(t *testing.T) {
	bads := []string{"", "maybe", "2", "True!", "yesyes"}
	for _, s := range bads {
		if _, err := parseBool(s); err == nil {
			t.Errorf("parseBool(%q) accepted; expected error", s)
		}
	}
}
