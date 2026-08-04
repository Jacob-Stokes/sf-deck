package sf

// Table-driven tests for the error-classification layer — the June
// review's biggest flagged coverage gap. Every branch of error
// classification is user-facing (it decides whether the UI says
// "fix your input" / "you lack permission" / "re-auth" / "retry"),
// so misclassification is a UX bug even when nothing crashes.

import (
	"errors"
	"strings"
	"testing"
)

func TestParseHTTPError_ArrayShape(t *testing.T) {
	he := &sfHTTPError{Status: 400, Body: []byte(
		`[{"message":"Required fields are missing: [Name]","errorCode":"REQUIRED_FIELD_MISSING","fields":["Name"]}]`)}
	e := AsSFError(he)
	if e == nil {
		t.Fatal("nil SFError")
	}
	if e.Code != "REQUIRED_FIELD_MISSING" || e.Kind != ErrValidation {
		t.Fatalf("code=%q kind=%v", e.Code, e.Kind)
	}
	if len(e.Fields) != 1 || e.Fields[0] != "Name" {
		t.Fatalf("fields=%v", e.Fields)
	}
	if e.HTTPCode != 400 {
		t.Fatalf("http=%d", e.HTTPCode)
	}
}

func TestParseHTTPError_SingleObjectShape(t *testing.T) {
	he := &sfHTTPError{Status: 404, Body: []byte(
		`{"message":"The requested resource does not exist","errorCode":"NOT_FOUND"}`)}
	e := AsSFError(he)
	if e == nil || e.Code != "NOT_FOUND" {
		t.Fatalf("e=%+v", e)
	}
}

func TestParseHTTPError_NonJSONBody(t *testing.T) {
	he := &sfHTTPError{Status: 502, Body: []byte("<html>Bad Gateway</html>")}
	e := AsSFError(he)
	if e == nil {
		t.Fatal("nil SFError")
	}
	if e.Kind != ErrServer {
		t.Fatalf("kind=%v want ErrServer for 5xx", e.Kind)
	}
	if !strings.Contains(e.Message, "Bad Gateway") {
		t.Fatalf("message=%q should carry the raw body", e.Message)
	}
}

func TestAsSFError_PassThroughAndNil(t *testing.T) {
	if AsSFError(nil) != nil {
		t.Fatal("nil error should map to nil")
	}
	typed := &SFError{Kind: ErrConflict, Code: "DUPLICATE_VALUE"}
	if got := AsSFError(typed); got != typed {
		t.Fatal("already-typed error should pass through unchanged")
	}
	if AsSFError(errors.New("dial tcp: connection refused")) != nil {
		t.Fatal("non-SF errors should map to nil, not a fake SFError")
	}
}

func TestClassifyErrorCode_Table(t *testing.T) {
	cases := []struct {
		code   string
		status int
		want   ErrorKind
	}{
		// One representative per classification bucket + the
		// codes that have bitten in the field.
		{"INSUFFICIENT_ACCESS_OR_READONLY", 403, ErrPermission},
		{"CANNOT_MODIFY_MANAGED_OBJECT", 400, ErrPermission},
		{"REQUIRED_FIELD_MISSING", 400, ErrValidation},
		{"FIELD_CUSTOM_VALIDATION_EXCEPTION", 400, ErrValidation},
		{"STRING_TOO_LONG", 400, ErrValidation},
		{"MALFORMED_ID", 400, ErrSchema},
		{"INVALID_FIELD", 400, ErrSchema},
		{"INVALID_TYPE", 400, ErrSchema},
		{"UNABLE_TO_LOCK_ROW", 400, ErrConflict},
		{"DUPLICATES_DETECTED", 400, ErrConflict},
		{"ENTITY_IS_DELETED", 404, ErrConflict},
		{"INVALID_SESSION_ID", 401, ErrSession},
		{"INVALID_LOGIN", 401, ErrSession},
		// No recognizable code → HTTP-status fallback ladder.
		{"", 401, ErrSession},
		{"", 403, ErrPermission},
		{"", 500, ErrServer},
		{"", 503, ErrServer},
		{"", 400, ErrValidation},
		{"", 0, ErrUnknown},
		// Unknown code falls through to the status ladder too.
		{"SOME_FUTURE_CODE", 403, ErrPermission},
	}
	for _, c := range cases {
		if got := classifyErrorCode(c.code, c.status); got != c.want {
			t.Errorf("classify(%q, %d) = %v, want %v", c.code, c.status, got, c.want)
		}
	}
}

func TestSFError_ErrorString(t *testing.T) {
	cases := []struct {
		e    *SFError
		want string
	}{
		{nil, "<nil sf error>"},
		{&SFError{Code: "X", Message: "boom"}, "X: boom"},
		{&SFError{Message: "just text"}, "just text"},
		{&SFError{HTTPCode: 418}, "HTTP 418"},
	}
	for _, c := range cases {
		if got := c.e.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}
}

func TestIsSessionExpired(t *testing.T) {
	if !isSessionExpired(&sfHTTPError{Status: 401, Body: []byte(
		`[{"errorCode":"INVALID_SESSION_ID","message":"Session expired or invalid"}]`)}) {
		t.Error("INVALID_SESSION_ID should read as session-expired")
	}
	if isSessionExpired(&sfHTTPError{Status: 400, Body: []byte(
		`[{"errorCode":"MALFORMED_QUERY","message":"unexpected token"}]`)}) {
		t.Error("MALFORMED_QUERY should NOT read as session-expired")
	}
	if isSessionExpired(nil) {
		t.Error("nil is not session-expired")
	}
}
