package sf

import (
	"testing"
)

func TestParseFieldErrors_TypicalRejection(t *testing.T) {
	// Real-shape rejection: SF returns an array of objects with
	// errorCode, message, and a fields slice.
	raw := []byte(`[
		{"fields":["Name"],"message":"Required fields are missing: [Name]","errorCode":"REQUIRED_FIELD_MISSING"},
		{"fields":["Amount"],"message":"Amount must be positive","errorCode":"INVALID_FIELD"}
	]`)
	errs := parseFieldErrors(raw)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Fields[0] != "Name" || errs[0].ErrorCode != "REQUIRED_FIELD_MISSING" {
		t.Errorf("first error wrong: %+v", errs[0])
	}
	if !contains(errs[0].String(), "Name") || !contains(errs[0].String(), "Required") {
		t.Errorf("String() lost content: %q", errs[0].String())
	}
}

func TestParseFieldErrors_RecordLevel(t *testing.T) {
	// Some errors don't carry a fields array (validation rule that
	// references multiple fields, record-locked errors).
	raw := []byte(`[{"message":"Custom validation rule failed","errorCode":"FIELD_CUSTOM_VALIDATION_EXCEPTION"}]`)
	errs := parseFieldErrors(raw)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if len(errs[0].Fields) != 0 {
		t.Errorf("expected no fields, got %v", errs[0].Fields)
	}
	if !contains(errs[0].String(), "FIELD_CUSTOM_VALIDATION_EXCEPTION") {
		t.Errorf("record-level error String() should include code: %q", errs[0].String())
	}
}

func TestParseFieldErrors_Empty(t *testing.T) {
	if got := parseFieldErrors(nil); got != nil {
		t.Errorf("nil body should parse to nil, got %v", got)
	}
	if got := parseFieldErrors([]byte("")); got != nil {
		t.Errorf("empty body should parse to nil, got %v", got)
	}
	if got := parseFieldErrors([]byte("not json")); got != nil {
		t.Errorf("garbage body should parse to nil, got %v", got)
	}
}

func TestEscapeSOSLBraces(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"plain":      "plain",
		"a {b} c":    `a \{b\} c`,
		`back\slash`: `back\\slash`,
	}
	for in, want := range cases {
		if got := escapeSOSLBraces(in); got != want {
			t.Errorf("escapeSOSLBraces(%q) = %q, want %q", in, got, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
