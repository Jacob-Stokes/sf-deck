package sf

import (
	"encoding/json"
	"testing"
)

func TestExecuteAnonymousResult_JSONShape(t *testing.T) {
	// Compile success + run success.
	raw := `{"compiled":true,"compileProblem":null,"success":true,"exceptionMessage":null,"exceptionStackTrace":null,"line":-1,"column":-1}`
	var r ExecuteAnonymousResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal happy: %v", err)
	}
	if !r.Compiled || !r.Success {
		t.Fatalf("happy result not parsed as success: %+v", r)
	}

	// Compile failure shape.
	raw = `{"compiled":false,"compileProblem":"Unexpected token '}'","success":false,"exceptionMessage":null,"exceptionStackTrace":null,"line":3,"column":12}`
	r = ExecuteAnonymousResult{}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal compile-fail: %v", err)
	}
	if r.Compiled {
		t.Fatalf("compile-failed result said Compiled=true")
	}
	if r.CompileProblem == "" {
		t.Fatalf("missing CompileProblem")
	}
	if r.Line != 3 || r.Column != 12 {
		t.Fatalf("line/col not parsed: got %d:%d", r.Line, r.Column)
	}

	// Runtime exception shape.
	raw = `{"compiled":true,"compileProblem":null,"success":false,"exceptionMessage":"System.NullPointerException: Attempt to de-reference a null object","exceptionStackTrace":"AnonymousBlock: line 4, column 1","line":4,"column":1}`
	r = ExecuteAnonymousResult{}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal runtime-fail: %v", err)
	}
	if !r.Compiled {
		t.Fatalf("runtime-fail result said Compiled=false")
	}
	if r.Success {
		t.Fatalf("runtime-fail result said Success=true")
	}
	if r.ExceptionMessage == "" || r.ExceptionStack == "" {
		t.Fatalf("missing exception fields: %+v", r)
	}
}
