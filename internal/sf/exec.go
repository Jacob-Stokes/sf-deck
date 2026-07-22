package sf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ErrSFNotFound is returned by every shell-out helper when the `sf`
// binary isn't on $PATH. Distinct from "sf returned an error" so the
// UI can render an install-CLI panel rather than a generic error.
var ErrSFNotFound = errors.New("sf CLI not found on PATH")

// DefaultRunSFTimeout caps how long any single `sf` invocation can run
// before being killed. The `sf` CLI is usually responsive within
// seconds, but a stuck Node process can hang a goroutine indefinitely
// — this floor turns "TUI never updates" into "TUI shows an error,
// user can retry."
//
// Override per-call via runSFCtx when a known-slow operation needs
// longer (deploy / retrieve / report-export). The current callers
// in internal/sf are all metadata reads that fit comfortably in the
// default; if a specific call is consistently timing out, route it
// through runSFCtx with a wider deadline rather than bumping the
// default for everyone.
const DefaultRunSFTimeout = 30 * time.Second

// runSF runs `sf <args...>` with a default 30s timeout. Most callers
// want this — it's the same blocking semantics as the original
// implementation, just with a process kill if `sf` hangs.
//
// Use runSFCtx when you have a context.Context to thread through
// (e.g. cancellation tied to a tea.Cmd lifecycle) or when the
// operation legitimately needs longer than 30s.
func runSF(args ...string) ([]byte, error) {
	if DemoMode {
		return nil, errors.New("demo mode: sf CLI calls are disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfgCLITimeout())
	defer cancel()
	return runSFCtx(ctx, args...)
}

// runSFCtx is the context-aware variant. The caller's deadline +
// cancellation propagate to the spawned `sf` process tree (see
// newSFCommand — the whole process group is killed, not just the direct
// child, so a wedged node grandchild can't hold the call open past the
// deadline). On timeout the returned error wraps context.DeadlineExceeded
// so callers can detect "sf hung" vs. "sf failed normally" without
// parsing strings.
//
// Returns the same shape as runSF: stdout bytes on success, *SFError
// (or a plain error) on failure. The usage hook fires regardless
// of outcome since SF counts API attempts, not successes.
// newSFCommand builds an `sf` command wired so a ctx cancel/timeout
// tears down the WHOLE process tree, not just the direct child. `sf`
// launches node + further children that inherit the stdout pipe; the
// stock exec.CommandContext only SIGKILLs the direct child, leaving
// those grandchildren holding the pipe open so cmd.Output() blocks past
// the deadline. Putting the command in its own process group and killing
// the group on cancel (with WaitDelay as a portable backstop) makes the
// timeout real. Used by every `sf` shell-out so the two call paths can't
// drift. See procgroup_*.go for setProcessGroup/killProcessGroup.
func newSFCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sf", args...)
	cmd.Env = quietEnv(os.Environ())
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

func runSFCtx(ctx context.Context, args ...string) ([]byte, error) {
	if DemoMode {
		return nil, errors.New("demo mode: sf CLI calls are disabled")
	}
	cmd := newSFCommand(ctx, args...)
	start := time.Now()
	out, err := cmd.Output()
	fireOnCall(aliasFromArgs(args), args, err, time.Since(start))
	if err == nil {
		return out, nil
	}
	// `sf` binary missing on $PATH — classified specifically so the UI
	// can render a "install the Salesforce CLI" onboarding panel
	// instead of a generic error.
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrSFNotFound
	}
	// Distinguish timeout from a normal failure. exec.CommandContext
	// returns os.ErrProcessDone or similar when the context is killed;
	// the cleanest signal is ctx.Err() being non-nil.
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("sf %s timed out after %s", argsLabel(args), timeoutLabel(ctx))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, fmt.Errorf("sf %s cancelled", argsLabel(args))
	}
	// Normal sf failure path — parse JSON body, then stderr fallback.
	if typed := parseCLIError(out); typed != nil {
		return nil, typed
	}
	if msg := parseStructuredError(out); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		cleaned := cleanStderr(string(ee.Stderr))
		if cleaned == "" {
			// Stderr was nothing but Node noise; surface a generic
			// message keyed off the exit code so callers don't get
			// an empty error.
			return nil, fmt.Errorf("sf %s exited with code %d",
				argsLabel(args), ee.ExitCode())
		}
		return nil, errors.New(cleaned)
	}
	return nil, err
}

// argsLabel returns a short string describing what command timed out
// (e.g. "sobject describe", "org display"). Skips flag values to
// keep error messages tight.
func argsLabel(args []string) string {
	parts := []string{}
	for i := 0; i < len(args) && i < 3; i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			break
		}
		parts = append(parts, a)
	}
	if len(parts) == 0 {
		return "command"
	}
	return strings.Join(parts, " ")
}

// timeoutLabel renders the context's effective deadline as a short
// human duration ("30s") for the error message. Falls back to
// DefaultRunSFTimeout's label when the deadline isn't readable.
func timeoutLabel(ctx context.Context) string {
	deadline, ok := ctx.Deadline()
	if !ok {
		return DefaultRunSFTimeout.String()
	}
	d := time.Until(deadline)
	if d < 0 {
		d = 0
	}
	// Round to whole seconds — sub-second resolution adds noise.
	return d.Round(time.Second).String()
}

// parseCLIError turns the JSON that `sf --json` emits on non-zero
// exit into an *SFError. Returns nil if the body isn't parseable or
// doesn't carry an error payload.
//
// Shape varies a bit by command: some emit {status, name, message},
// some include a nested `.result.errors[]` array (data manipulation),
// some include `data` with one of the structures above. We try each.
func parseCLIError(b []byte) *SFError {
	// Top-level {status, name, message, data?}.
	var top struct {
		Status  int             `json:"status"`
		Name    string          `json:"name"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(b, &top); err != nil {
		return nil
	}
	if top.Status == 0 && top.Name == "" && top.Message == "" {
		return nil
	}

	// Look for a nested errors[] array (DML failures via `sf data`).
	for _, blob := range [][]byte{top.Data, top.Result} {
		if len(blob) == 0 {
			continue
		}
		var nested struct {
			Errors []struct {
				Message   string   `json:"message"`
				ErrorCode string   `json:"errorCode"`
				Fields    []string `json:"fields"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(blob, &nested); err == nil && len(nested.Errors) > 0 {
			first := nested.Errors[0]
			return &SFError{
				Kind:    classifyErrorCode(first.ErrorCode, 0),
				Code:    first.ErrorCode,
				Message: first.Message,
				Fields:  first.Fields,
				Hint:    hintForCode(first.ErrorCode),
			}
		}
	}

	// No nested errors[]. Fall back to the top-level name as the code.
	// `sf` often uses the sfdx error name (e.g. "MissingOrgError"),
	// which isn't a Salesforce errorCode but is a useful classifier.
	code := top.Name
	return &SFError{
		Kind:    classifyErrorCode(code, 0),
		Code:    code,
		Message: strings.TrimSpace(top.Message),
		Hint:    hintForCode(code),
	}
}

// quietEnv appends env vars that suppress the orange update-available
// warning so it doesn't leak into parse errors. Harmless if already set.
func quietEnv(env []string) []string {
	return append(env,
		"SF_AUTOUPDATE_DISABLE=true",
		"SF_SUPPRESS_UPDATE_WARNING=true",
		"SFDX_SUPPRESS_UPDATE_WARNING=true",
		// Suppress Node runtime warnings (deprecation, experimental
		// feature warnings, file-handle GC noise). sf still exits 0
		// for these; they just clutter stderr.
		"NODE_NO_WARNINGS=1",
	)
}

// parseStructuredError extracts `.message` from a JSON error body emitted
// by `sf --json` on failure. Returns "" if not parseable.
func parseStructuredError(b []byte) string {
	var e struct {
		Status  int    `json:"status"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &e); err != nil || e.Status == 0 {
		return ""
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return e.Name
	}
	if e.Name != "" {
		return fmt.Sprintf("%s: %s", e.Name, msg)
	}
	return msg
}

// cleanStderr strips ANSI color, sf's update-available warning, Node
// runtime deprecation noise, and leading whitespace so error text
// surfaced to the UI is readable.
//
// Why filter Node deprecations: `sf` is a Node app, and Node emits
// DeprecationWarning messages to stderr at process exit when certain
// internal cleanup paths run during GC. They look like
//
//	(node:12345) [DEP0137] DeprecationWarning: ...
//	<anonymous_script>:0; [Error: A FileHandle object was closed during
//	garbage collection. This used to be allowed with a deprecation warning...]
//
// These are not failures — sf still exits 0 and the JSON on stdout is
// valid. Without filtering, every `sf` call surfaces the deprecation
// as if it were a real error. Strip them so the UI only flashes
// actual errors.
func cleanStderr(s string) string {
	s = ansiRE.ReplaceAllString(s, "")
	s = updateWarningRE.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	var kept []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if isNodeNoise(t) {
			continue
		}
		kept = append(kept, t)
	}
	return strings.TrimSpace(strings.Join(kept, "; "))
}

// isNodeNoise reports whether a stderr line looks like a Node-runtime
// deprecation / process-exit warning rather than a real sf error.
// These are non-fatal — sf still exits 0 and stdout is valid — but
// they surface to the UI as if they were failures. Strip them.
func isNodeNoise(line string) bool {
	if nodeWarningRE.MatchString(line) {
		return true
	}
	if anonymousScriptRE.MatchString(line) {
		return true
	}
	// Trailing fragments of multi-line DEP messages — short bracketed
	// hints, "Use node --trace-deprecation ...", "(rejection id: ...)".
	low := strings.ToLower(line)
	if strings.Contains(low, "deprecationwarning") ||
		strings.Contains(low, "trace-deprecation") ||
		strings.Contains(low, "filehandle object was closed") ||
		strings.Contains(low, "experimentalwarning") {
		return true
	}
	return false
}

var (
	ansiRE            = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	updateWarningRE   = regexp.MustCompile(`(?i)Warning:\s*@salesforce/cli update available[^\n]*`)
	nodeWarningRE     = regexp.MustCompile(`^\(node:\d+\)\s*\[?[A-Z0-9]*\]?\s*(Deprecation|Experimental|Pending)?Warning`)
	anonymousScriptRE = regexp.MustCompile(`^<anonymous_script>:`)
)
