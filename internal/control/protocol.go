// Package control implements the live-control IPC channel into a
// running sf-deck. It exposes a Unix-domain socket that speaks the
// same JSON envelope as the headless CLI, plus a handful of
// write/subscribe verbs that drive the running TUI.
//
// Default off. Enable with sf-deck --control (or SF_DECK_CONTROL=1).
// On startup, the listener claims an instance number via package
// internal/instance, mints a per-instance socket path
// (~/.sf-deck/control-<N>.sock), and starts accepting line-delimited
// JSON requests. On clean shutdown, it removes the socket file and
// releases the instance number.
//
// The wire format is one JSON object per line for both requests and
// responses, matching `internal/headless` conventions so consumers
// don't learn a new schema:
//
//	-> {"command":"state.get"}
//	<- {"ok":true,"command":"state.get","data":{...}}
//
// Errors carry a typed `error.code` (same vocabulary as the CLI),
// plus IPC-specific codes:
//   - instance_busy           — another writer holds the channel
//   - confirmation_required   — destructive op needs a human keypress
//   - method_not_implemented  — verb is reserved but not yet handled
//
// Safety: this layer does NOT widen what's possible. Writes still
// pass through the existing safety gates; destructive ops still
// require the same human confirmation flow they always have.
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

// success builds an OK response.
func success(req Request, data any) Response {
	return Response{ID: req.ID, OK: true, Command: req.Command, Data: data}
}

// fail builds a failure response with a typed code + message.
func fail(req Request, code, message string, details map[string]any) Response {
	return Response{
		ID: req.ID, OK: false, Command: req.Command,
		Error: &ResponseError{Code: code, Message: message, Details: details},
	}
}
