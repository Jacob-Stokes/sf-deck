package ui

// Session-scoped anonymous-Apex editor + result state. Mirrors
// modelSOQL's shape (editor body, busy flag, last result, current
// saved-snippet id, sticky search buffer for the output viewer)
// without trying to share types — Apex results and SOQL results
// have nothing in common past the "user pressed enter, here's
// what came back" envelope.

import (
	"charm.land/bubbles/v2/textarea"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// modelExec owns session-scoped editor + result state. Embedded into
// Model so callers say `m.execInput`, `m.execResult`, etc. as if the
// fields were declared inline.
type modelExec struct {
	// execInput is a multi-line text-area for anonymous Apex bodies.
	// Single-line textinput (which /soql uses) can't reasonably hold
	// the 5-50 line snippets people actually run. Users with serious
	// scripts use `e` to open $EDITOR; the inline area covers quick
	// one-liners and "just tweak that line and re-run" work.
	execInput textarea.Model

	// execResult is the most recent run's result envelope (compile +
	// runtime status + the body that ran + optional debug-log body).
	// Cleared on a fresh run; populated when execResultMsg lands.
	execResult sf.ExecuteAnonymousResult

	// execErr is the most recent transport-level error (HTTP failure,
	// auth failure, etc.) — distinct from a compile or runtime
	// failure which lives on execResult.CompileProblem / Exception*.
	execErr error

	// execRunning is the in-flight flag. Set when the user hits Enter,
	// cleared when execResultMsg lands. Drives the "running…" copy
	// in the editor body.
	execRunning bool

	// execEditing is true while the textarea has focus. The dispatcher
	// in handleExecKey routes keys into the widget when this is set.
	execEditing bool

	// execCaptureLog toggles whether we fetch the most recent ApexLog
	// after a successful run. On by default — most users want to see
	// the debug output. Toggled via Keys.ExecToggleLog (ctrl+d).
	execCaptureLog bool

	// execSubtabIdx selects between Editor (default), Output, Saved,
	// and History on /exec. Saved/History lists live per-org on
	// orgData (ExecSavedList, ExecHistoryList) so the standard
	// listSurface plumbing works without Model-side branches.
	execSubtabIdx int

	// execEditingSavedID, when non-empty, is the id of the saved
	// snippet the editor currently holds. Set when the user loads a
	// row via Enter from the Saved subtab; reset when the editor is
	// cleared or saved as new. Drives the "S updates in place vs
	// creates new" decision. Same pattern as soqlEditingSavedID.
	execEditingSavedID string

	// execLogSearch is the per-session sticky search buffer for the
	// Output subtab. `/` opens it; the log viewer narrows visible
	// lines to substring matches. Pointer so the same state
	// survives the value-Model copy through Update / render.
	execLogSearch *searchState
}
