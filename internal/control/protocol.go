// Package control implements the live-control IPC channel into a
// running sf-deck. It exposes a Unix-domain socket that speaks the
// same JSON envelope as the headless CLI, plus a handful of
// write/subscribe verbs that drive the running TUI.
package control

import "encoding/json"

// Request is the inbound envelope. One per line, terminated by \n.
//
// Command is the noun.verb identifier ("state.get", "tab.open",
// "chip.apply", ...). Args is verb-specific; unmarshal it inside
// each handler. ID is an optional client-supplied correlator —
// echoed back on the response so async / multi-request clients can
// pair requests with replies.
type Request struct {
	ID      string          `json:"id,omitempty"`
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args,omitempty"`
}

// Response is the outbound envelope. Mirrors `internal/headless.Response`
// for shape; we don't import that package to avoid creating a cycle
// between the UI/control layer and the headless layer.
type Response struct {
	ID      string         `json:"id,omitempty"`
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Changed bool           `json:"changed,omitempty"`
	Data    any            `json:"data,omitempty"`
	Error   *ResponseError `json:"error,omitempty"`
}

// ResponseError mirrors the headless error shape: a stable Code
// discriminator + human Message + arbitrary Details.
type ResponseError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error codes shared with the CLI's vocabulary.
const (
	ErrInvalidArgument      = "invalid_argument"
	ErrNotFound             = "not_found"
	ErrSafetyBlocked        = "safety_blocked"
	ErrAuthRequired         = "auth_required"
	ErrInstanceBusy         = "instance_busy"
	ErrConfirmationRequired = "confirmation_required"
	ErrMethodNotImplemented = "method_not_implemented"
	ErrInternal             = "internal_error"
)

func success(req Request, data any) Response {
	return Response{ID: req.ID, OK: true, Command: req.Command, Data: data}
}

func fail(req Request, code, message string, details map[string]any) Response {
	return Response{
		ID: req.ID, OK: false, Command: req.Command,
		Error: &ResponseError{Code: code, Message: message, Details: details},
	}
}
