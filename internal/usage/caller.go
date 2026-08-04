package usage

import (
	"runtime"
	"strings"
)

// captureCaller walks up the current goroutine's stack and returns a
// short "pkg.func" tag for the highest-level frame that triggered the
// API call. Used to attribute each call to the fetcher / UI action that
// caused it ("fetchHome", "ensureActiveUsersChip", …) for the in-session
// API Call Log modal.
//
// Filtering rules:
//   - Skip frames in the runtime / net/http / sf REST plumbing / this
//     package itself — those are noise.
//   - Stop at the first frame that lives outside internal/sf's REST
//     transport layer. That frame is the caller we want to attribute to.
//   - If no good frame is found, return "" (the caller can fall back
//     to the bucketed path).
//
// Cost: one runtime.Callers + a handful of CallersFrames iterations.
// Cheap enough to run on every Bump.
func captureCaller() string {
	// Skip 0 = runtime.Callers, 1 = captureCaller, 2 = Bump caller frame.
	// Grab a generous slice; deep call stacks (cobra → cmd → ui → sf)
	// can easily reach 30 frames.
	var pcs [40]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])

	// Walk frames and return the FIRST non-noise frame. That's the
	// fetcher / UI helper closest to the REST call — which answers
	// the audit question "what specifically made this call?" better
	// than the highest-level frame (which is almost always
	// ui.(*Model).Update and tells you nothing).
	//
	// Higher frames (UI dispatchers, tea loop) are intentionally
	// skipped via the noise filter so they don't shadow the real
	// answer.
	for {
		f, more := frames.Next()
		if tag := tagOf(f.Function); tag != "" {
			return tag
		}
		if !more {
			break
		}
	}
	return ""
}

// tagOf turns a fully-qualified function name into a short attribution
// label, or returns "" when the frame is uninteresting noise.
//
// Examples:
//
//	"github.com/Jacob-Stokes/sf-deck/internal/sf.fetchHome"        → "sf.fetchHome"
//	"github.com/Jacob-Stokes/sf-deck/internal/ui.(*Model).Update"  → "ui.Model.Update"
//	"net/http.(*Client).Do"                                 → "" (noise)
//	"github.com/Jacob-Stokes/sf-deck/internal/sf.(*Client).get"    → "" (REST plumbing)
//	"github.com/Jacob-Stokes/sf-deck/internal/usage.(*Tracker).Bump"→ "" (self)
func tagOf(fn string) string {
	if fn == "" {
		return ""
	}
	// Drop noise: runtime, net/http, our own usage package, and the
	// sf-package REST transport plumbing (we want the *caller* of
	// the REST helper, not the helper itself).
	noisePrefixes := []string{
		"runtime.",
		"net/http.",
		"net.",
		"crypto/",
		"encoding/",
		"reflect.",
		"sync.",
		"testing.",
		"main.",
		"github.com/Jacob-Stokes/sf-deck/internal/usage.",
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(fn, p) {
			return ""
		}
	}

	// Strip everything before the last "/" so the project path doesn't
	// dominate; we're left with "pkg.func" or "pkg.(*Type).method".
	short := fn
	if i := strings.LastIndex(fn, "/"); i >= 0 {
		short = fn[i+1:]
	}

	// Drop sf REST transport frames (c.get / c.doOnce / fireOnCall /
	// QueryREST / doWithRetry / etc.) — these are all the plumbing
	// every call goes through. We want the *business* caller above.
	if strings.HasPrefix(short, "sf.") {
		method := short[len("sf."):]
		// Strip "(*Client)." receiver prefix if present.
		method = strings.TrimPrefix(method, "(*Client).")
		method = strings.TrimPrefix(method, "(*Tracker).")
		switch method {
		case "get", "getWithAccept", "getWithAcceptTimeout",
			"doOnceWithAccept", "doOnce", "doWithRetry",
			"patch", "post", "delete", "postMultipart",
			"doOnceMultipart", "fireOnCall", "RESTClient",
			"bootstrap", "QueryREST", "QueryRESTCapped",
			"Exec", "ExecWithStderr", "ExecJSON":
			return ""
		}
		// Anonymous closures inside REST plumbing: e.g.
		// "sf.(*Client).doOnce.func1" — drop those too.
		if strings.Contains(method, ".func") &&
			(strings.HasPrefix(method, "doOnce") ||
				strings.HasPrefix(method, "doWithRetry") ||
				strings.HasPrefix(method, "get")) {
			return ""
		}
	}

	// Drop tea / bubbletea / lipgloss internals if any leak in.
	if strings.HasPrefix(short, "tea.") ||
		strings.HasPrefix(short, "bubbletea.") ||
		strings.HasPrefix(short, "lipgloss.") {
		return ""
	}

	// Strip "(*Type)." receivers so the tag reads as "pkg.Type.method"
	// rather than "pkg.(*Type).method".
	short = strings.ReplaceAll(short, "(*", "")
	short = strings.ReplaceAll(short, ")", "")
	return short
}
