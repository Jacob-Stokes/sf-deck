package sf

import (
	"strings"
	"testing"
)

func TestBuildGlobalSearchSOSL(t *testing.T) {
	got := buildGlobalSearchSOSL("acme", []GlobalSearchTarget{
		{Sobject: "Account", Secondary: "Industry"},
		{Sobject: "Contact", Secondary: "Email"},
		{Sobject: "Case", NameField: "Subject", Fields: []string{"CaseNumber"}, Secondary: "CaseNumber"},
	}, 50)

	// FIND clause + IN NAME FIELDS structure.
	if !strings.HasPrefix(got, "FIND {acme} IN NAME FIELDS RETURNING ") {
		t.Fatalf("missing FIND/RETURNING prefix: %q", got)
	}
	if !strings.HasSuffix(got, " LIMIT 50") {
		t.Fatalf("missing LIMIT suffix: %q", got)
	}

	// Each target carries Id implicitly + the requested fields.
	if !strings.Contains(got, "Account(Id, Name, Industry)") {
		t.Errorf("expected Account projection, got: %q", got)
	}
	if !strings.Contains(got, "Contact(Id, Name, Email)") {
		t.Errorf("expected Contact projection, got: %q", got)
	}
	// Case uses NameField=Subject and explicit Fields=CaseNumber +
	// Secondary=CaseNumber (deduped).
	if !strings.Contains(got, "Case(Id, Subject, CaseNumber)") {
		t.Errorf("expected Case projection, got: %q", got)
	}
}

func TestBuildGlobalSearchSOSLEscapesTerm(t *testing.T) {
	// SOSL braces in the term need escaping or the parser sees
	// nested {} and the query 400s server-side.
	got := buildGlobalSearchSOSL("a{b}c", []GlobalSearchTarget{
		{Sobject: "Account"},
	}, 10)
	if !strings.Contains(got, `{a\{b\}c}`) {
		t.Errorf("expected escaped braces in term, got: %q", got)
	}
}

func TestBuildGlobalSearchSOSLDeterministicOrder(t *testing.T) {
	// Rerunning with the same input must produce the same string
	// so per-term caches don't thrash on stable inputs.
	tgts := []GlobalSearchTarget{
		{Sobject: "Opportunity", Fields: []string{"StageName", "Amount"}, Secondary: "StageName"},
	}
	a := buildGlobalSearchSOSL("foo", tgts, 25)
	b := buildGlobalSearchSOSL("foo", tgts, 25)
	if a != b {
		t.Fatalf("non-deterministic SOSL:\n  a=%q\n  b=%q", a, b)
	}
}

func TestStringifyField(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
		// Nested relationship → prefer .Name.
		{map[string]any{"Name": "Acme", "Id": "001"}, "Acme"},
		{map[string]any{"Id": "001"}, "001"},
		{map[string]any{}, ""},
	}
	for _, tc := range tests {
		if got := stringifyField(tc.in); got != tc.want {
			t.Errorf("stringifyField(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSosjectTypeFromAttributes(t *testing.T) {
	row := map[string]any{
		"attributes": map[string]any{"type": "Account"},
		"Id":         "001AB",
	}
	if got := sosjectTypeFromAttributes(row); got != "Account" {
		t.Errorf("got %q, want Account", got)
	}

	// Missing attributes → "".
	if got := sosjectTypeFromAttributes(map[string]any{"Id": "x"}); got != "" {
		t.Errorf("missing attrs: got %q, want empty", got)
	}
}
