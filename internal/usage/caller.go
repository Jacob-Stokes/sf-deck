package usage

import (
	"runtime"
	"strings"
)

func captureCaller() string {
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

func tagOf(fn string) string {
	if fn == "" {
		return ""
	}
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

	short := fn
	if i := strings.LastIndex(fn, "/"); i >= 0 {
		short = fn[i+1:]
	}

	if strings.HasPrefix(short, "sf.") {
		method := short[len("sf."):]
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
		if strings.Contains(method, ".func") &&
			(strings.HasPrefix(method, "doOnce") ||
				strings.HasPrefix(method, "doWithRetry") ||
				strings.HasPrefix(method, "get")) {
			return ""
		}
	}

	if strings.HasPrefix(short, "tea.") ||
		strings.HasPrefix(short, "bubbletea.") ||
		strings.HasPrefix(short, "lipgloss.") {
		return ""
	}

	short = strings.ReplaceAll(short, "(*", "")
	short = strings.ReplaceAll(short, ")", "")
	return short
}
