package ui

import (
	"charm.land/bubbles/v2/textarea"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type modelExec struct {
	// execInput is a multi-line text-area for anonymous Apex bodies.
	// Single-line textinput (which /soql uses) can't reasonably hold
	// the 5-50 line snippets people actually run. Users with serious
	// scripts use `e` to open $EDITOR; the inline area covers quick
	// one-liners and "just tweak that line and re-run" work.
	execInput textarea.Model

	execResult sf.ExecuteAnonymousResult

	// execErr is the most recent transport-level error (HTTP failure,
	// auth failure, etc.) — distinct from a compile or runtime
	// failure which lives on execResult.CompileProblem / Exception*.
	execErr error

	execRunning bool

	execEditing bool

	execCaptureLog bool

	execSubtabIdx int

	execEditingSavedID string

	execLogSearch *searchState
}
