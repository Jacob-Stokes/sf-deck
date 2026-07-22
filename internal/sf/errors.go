package sf

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ErrorKind classifies a Salesforce error by what the user can do
// about it. The actionable distinction (retry helps vs user must fix
// something vs server problem) matters more than the raw code for UI
// purposes — flash banner vs sidebar panel vs retry hint vs giving up.
type ErrorKind int

const (
	// ErrUnknown — couldn't classify. Treat as server-ish.
	ErrUnknown ErrorKind = iota
	// ErrPermission — user's profile/perm sets don't allow the op.
	// Retry won't help; user needs Setup → Profiles work.
	ErrPermission
	// ErrValidation — user-input wrong (required field missing,
	// length exceeded, validation rule fired). Fixable in-place.
	ErrValidation
	// ErrSchema — wrong object shape, malformed ID, bad field name.
	// Usually a bug in the calling code or outdated cache.
	ErrSchema
	// ErrConflict — race or collision (row locked, duplicate, already
	// deleted). Retry later might help.
	ErrConflict
	// ErrSession — 401 / session expired. Re-auth needed.
	ErrSession
	// ErrServer — 5xx or network-layer error. Retry usually succeeds.
	ErrServer
)

// String names the ErrorKind for logs / debug output.
func (k ErrorKind) String() string {
	switch k {
	case ErrPermission:
		return "permission"
	case ErrValidation:
		return "validation"
	case ErrSchema:
		return "schema"
	case ErrConflict:
		return "conflict"
	case ErrSession:
		return "session"
	case ErrServer:
		return "server"
	}
	return "unknown"
}

// SFError is our typed representation of a Salesforce API error. We
// parse the JSON body of a 4xx/5xx once and keep the useful bits so
// callers never have to re-parse. Implements `error` so it drops in
// wherever callers already expect an error value.
type SFError struct {
	Kind     ErrorKind
	Code     string   // Salesforce's own errorCode, e.g. "REQUIRED_FIELD_MISSING"
	Message  string   // Raw message from Salesforce
	Fields   []string // Which fields the error points at (can be empty)
	HTTPCode int      // Underlying HTTP status (400, 403, 500 …)
	Hint     string   // Our human-authored "here's what to do" (can be empty)
}

func (e *SFError) Error() string {
	if e == nil {
		return "<nil sf error>"
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP %d", e.HTTPCode)
}

// AsSFError tries to coerce any error into *SFError. Handles nil, an
// already-typed *SFError, and the internal *sfHTTPError (which it
// parses on-demand). Returns nil if the error isn't recognisable as a
// Salesforce error (e.g. connection refused at the TCP layer).
func AsSFError(err error) *SFError {
	if err == nil {
		return nil
	}
	if s, ok := err.(*SFError); ok {
		return s
	}
	if he, ok := err.(*sfHTTPError); ok {
		return parseHTTPError(he)
	}
	return nil
}

// parseHTTPError turns a raw non-2xx HTTP response body into a typed
// *SFError. Salesforce usually returns an array of {message,errorCode,
// fields} objects; we use the first one for the top-level error and
// best-effort classify by code.
func parseHTTPError(he *sfHTTPError) *SFError {
	out := &SFError{HTTPCode: he.Status}

	// Common shape: array of error objects.
	var arr []struct {
		Message   string   `json:"message"`
		ErrorCode string   `json:"errorCode"`
		Fields    []string `json:"fields"`
	}
	if err := json.Unmarshal(he.Body, &arr); err == nil && len(arr) > 0 {
		out.Code = arr[0].ErrorCode
		out.Message = arr[0].Message
		out.Fields = arr[0].Fields
	} else {
		// Some endpoints return a single object instead of an array.
		var single struct {
			Message   string   `json:"message"`
			ErrorCode string   `json:"errorCode"`
			Fields    []string `json:"fields"`
		}
		if err := json.Unmarshal(he.Body, &single); err == nil && single.ErrorCode != "" {
			out.Code = single.ErrorCode
			out.Message = single.Message
			out.Fields = single.Fields
		} else {
			out.Message = strings.TrimSpace(string(he.Body))
		}
	}

	out.Kind = classifyErrorCode(out.Code, he.Status)
	out.Hint = hintForCode(out.Code)
	return out
}

// classifyErrorCode maps Salesforce errorCodes to our ErrorKind. The
// HTTP status is a fallback when there's no recognizable code (e.g.
// network-layer 502s).
func classifyErrorCode(code string, httpStatus int) ErrorKind {
	switch code {
	// Permission — user's profile / FLS / sharing rules block the op.
	case "INSUFFICIENT_ACCESS_OR_READONLY",
		"INSUFFICIENT_ACCESS_ON_CROSS_REFERENCE_ENTITY",
		"CANNOT_MODIFY_MANAGED_OBJECT",
		"FIELD_INTEGRITY_EXCEPTION",
		"METHOD_NOT_ALLOWED":
		return ErrPermission

	// Validation — user-fixable input problem.
	case "REQUIRED_FIELD_MISSING",
		"STRING_TOO_LONG",
		"NUMBER_OUTSIDE_VALID_RANGE",
		"FIELD_CUSTOM_VALIDATION_EXCEPTION",
		"INVALID_EMAIL_ADDRESS",
		"INVALID_DATE",
		"INVALID_TYPE_ON_FIELD_IN_RECORD":
		return ErrValidation

	// Schema — wrong object shape, bad IDs, outdated describe.
	case "MALFORMED_ID",
		"INVALID_FIELD",
		"INVALID_FIELD_FOR_INSERT_UPDATE",
		"INVALID_TYPE",
		"INVALID_QUERY_FILTER_OPERATOR":
		return ErrSchema

	// Conflict — concurrent edit, dup, already-deleted.
	case "UNABLE_TO_LOCK_ROW",
		"DUPLICATE_VALUE",
		"DUPLICATES_DETECTED",
		"DELETE_FAILED",
		"ENTITY_IS_DELETED",
		"INVALID_CROSS_REFERENCE_KEY":
		return ErrConflict

	// Session/auth.
	case "INVALID_SESSION_ID", "INVALID_LOGIN":
		return ErrSession
	}

	// Fallback to HTTP status.
	switch {
	case httpStatus == 401:
		return ErrSession
	case httpStatus == 403:
		return ErrPermission
	case httpStatus >= 500:
		return ErrServer
	case httpStatus >= 400:
		return ErrValidation
	}
	return ErrUnknown
}

// hintForCode returns a short, human-authored suggestion for fixing
// the error. Empty when we don't have one yet — the message alone will
// have to do. Callers decide whether to show the hint (e.g. sidebar
// panel yes, flash banner no — too much text).
func hintForCode(code string) string {
	switch code {
	case "INSUFFICIENT_ACCESS_OR_READONLY":
		return "Your profile or FLS blocks this. Check Setup → Profiles → Object Permissions."
	case "INSUFFICIENT_ACCESS_ON_CROSS_REFERENCE_ENTITY":
		return "You don't have read access to the referenced parent record."
	case "CANNOT_MODIFY_MANAGED_OBJECT":
		return "This is managed-package metadata — edit in the package or via unlocked extension."
	case "REQUIRED_FIELD_MISSING":
		return "Required field is blank. Fill it in and retry."
	case "STRING_TOO_LONG":
		return "Value exceeds the field's length limit."
	case "FIELD_CUSTOM_VALIDATION_EXCEPTION":
		return "A validation rule rejected this. The message above is the rule's own text."
	case "DUPLICATE_VALUE":
		return "Another record already has this value on a unique field."
	case "DUPLICATES_DETECTED":
		return "Salesforce's duplicate-detection rule flagged this. Check + confirm."
	case "UNABLE_TO_LOCK_ROW":
		return "Row is locked — another user or process is editing it. Press r to retry."
	case "ENTITY_IS_DELETED":
		return "Record is in the recycle bin. Undelete first, or operate on a live record."
	case "MALFORMED_ID":
		return "Not a valid 15/18-char Salesforce ID."
	case "INVALID_FIELD":
		return "Field name not recognised on this sObject — cached describe may be stale (r to refresh)."
	case "INVALID_CROSS_REFERENCE_KEY":
		return "Referenced record doesn't exist or you can't see it."
	case "DELETE_FAILED":
		return "Can't delete — this record has dependents that must go first."
	case "INVALID_SESSION_ID":
		return "Session expired. Run `sf org login web --alias <alias>` to re-auth."
	}
	return ""
}
