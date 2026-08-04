// Package headless is the wire contract for sf-deck's CLI / agent
// surface — the JSON envelope, error codes, and exit-code policy
// every headless command renders into.
//
// No command implementations live here. Domain logic for chips,
// records, projects, etc. belongs in internal/services/<X> packages
// (added in later phases of docs/headless-mode-plan.md). headless
// just owns the shape every command's output takes.
//
// The shape is fixed because:
//
//   - agent / skill code parses it
//   - shell scripts read .ok / .data / .error.code fields
//   - exit codes drive CI / script behaviour without re-parsing JSON
//
// Adding a new field is OK as long as it's optional (nil/empty).
// Removing or renaming a field is a contract break.
package headless

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Response is the standard envelope every headless command renders.
// Mirrors the shape documented in docs/headless-mode-plan.md.
type Response struct {
	// OK is the success flag. False when Error is non-nil.
	OK bool `json:"ok"`

	// Command is the verb that produced this response (e.g.
	// "chip.create", "record.update"). Helps agents correlate
	// stdout output with the request they issued, and helps logs
	// stay scannable.
	Command string `json:"command"`

	// Org is the target org's alias-or-username (whichever the
	// caller passed via --org). Empty when the command isn't
	// org-scoped (e.g. tag CRUD).
	Org string `json:"org,omitempty"`

	// Target is the value passed to sf -o (alias preferred, falls
	// back to username). Mirrors Org for now but kept separate so
	// future routing can disambiguate when alias resolution gets
	// more complex.
	Target string `json:"target,omitempty"`

	// Changed reports whether the command mutated any state.
	// Useful for idempotent commands that no-op when the target is
	// already in the desired state — skills can decide whether to
	// surface a "changed N items" line based on this.
	Changed bool `json:"changed,omitempty"`

	// Warnings is a list of non-fatal advisories. Skills may
	// surface them; scripts can ignore.
	Warnings []string `json:"warnings,omitempty"`

	// Data is the command-specific payload. Shape varies per
	// command; each service package documents its own.
	Data any `json:"data,omitempty"`

	// Error is set when OK is false. Carries a typed code so
	// callers can branch without string-matching the message.
	Error *Error `json:"error,omitempty"`
}

// Error is the typed failure envelope.
type Error struct {
	// Code is the machine-readable error tag. Stable across versions
	// — callers branch on this, not on Message.
	Code string `json:"code"`

	// Message is the human-readable summary. Format is free-form;
	// agents should display verbatim.
	Message string `json:"message"`

	// Details carries structured context (the required safety
	// level, the conflicting field name, etc.). Shape varies per
	// code; documented next to each constant below.
	Details map[string]any `json:"details,omitempty"`
}

// Error implements the error interface so command bodies can return
// *Error directly and have the marshal path use the typed shape.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Standard error codes. Stable strings — adding new codes is
// allowed, renaming existing ones is a contract break.
const (
	// ErrInvalidArgument — caller passed unparseable input. Maps
	// to exit code 2.
	ErrInvalidArgument = "invalid_argument"

	// ErrSafetyBlocked — the configured safety policy refuses
	// this write on this org. Details carry required_write_kind
	// and effective_safety. Maps to exit code 3.
	ErrSafetyBlocked = "safety_blocked"

	// ErrNotFound — the referenced record / chip / project /
	// org doesn't exist. Maps to exit code 4.
	ErrNotFound = "not_found"

	// ErrAuth — Salesforce auth/session issue (expired token,
	// disconnected org). Maps to exit code 5.
	ErrAuth = "auth_required"

	// ErrPartial — multi-item operation that succeeded for some
	// items + failed for others. Details carry per-item status.
	// Maps to exit code 6.
	ErrPartial = "partial_success"

	// ErrInternal — anything else (network failure, programming
	// bug, unexpected state). Maps to exit code 1.
	ErrInternal = "internal_error"
)

// Exit code policy. Headless commands call os.Exit(ExitCodeFor(r))
// or its equivalent so script callers can branch on $?.
const (
	ExitOK             = 0
	ExitInternal       = 1
	ExitInvalidArg     = 2
	ExitSafetyBlocked  = 3
	ExitNotFound       = 4
	ExitAuthRequired   = 5
	ExitPartialSuccess = 6
)

// ExitCodeFor returns the appropriate process exit code for a
// completed Response. Success paths return 0; error responses route
// through their Code.
func ExitCodeFor(r *Response) int {
	if r == nil {
		return ExitInternal
	}
	if r.OK {
		return ExitOK
	}
	if r.Error == nil {
		return ExitInternal
	}
	switch r.Error.Code {
	case ErrInvalidArgument:
		return ExitInvalidArg
	case ErrSafetyBlocked:
		return ExitSafetyBlocked
	case ErrNotFound:
		return ExitNotFound
	case ErrAuth:
		return ExitAuthRequired
	case ErrPartial:
		return ExitPartialSuccess
	}
	return ExitInternal
}

// Write renders the response to w. JSON mode emits a single
// pretty-printed JSON object followed by a newline; text mode emits
// a short summary line. Each command decides which mode to use
// based on the user's --json flag.
type WriteMode int

const (
	// JSONMode emits the Response as pretty-printed JSON.
	// Standard for skill / agent / script consumption.
	JSONMode WriteMode = iota
	// TextMode emits a one-line human summary. Default for
	// interactive shell use.
	TextMode
)

// Write renders r to w according to mode. Returns the io.Writer
// error if any; never modifies r.
func (r *Response) Write(w io.Writer, mode WriteMode) error {
	if w == nil {
		w = os.Stdout
	}
	switch mode {
	case JSONMode:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case TextMode:
		if r.OK {
			if r.Changed {
				fmt.Fprintf(w, "ok · %s · changed\n", r.Command)
			} else {
				fmt.Fprintf(w, "ok · %s\n", r.Command)
			}
			return nil
		}
		if r.Error != nil {
			fmt.Fprintf(w, "error · %s · %s · %s\n",
				r.Command, r.Error.Code, r.Error.Message)
		} else {
			fmt.Fprintf(w, "error · %s\n", r.Command)
		}
		return nil
	}
	return fmt.Errorf("unknown write mode: %d", mode)
}

// Success is a convenience constructor for happy-path responses.
func Success(command, org, target string, changed bool, data any) *Response {
	return &Response{
		OK:      true,
		Command: command,
		Org:     org,
		Target:  target,
		Changed: changed,
		Data:    data,
	}
}

// Fail is the convenience constructor for typed errors.
func Fail(command, org string, code, message string, details map[string]any) *Response {
	return &Response{
		OK:      false,
		Command: command,
		Org:     org,
		Error: &Error{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}
