package usage

// Append-only JSONL trace of every API call this process makes.
// Enabled with SF_DECK_API_TRACE=1 (or any truthy value). Path defaults
// to ~/.sf-deck/log/api-trace-<ts>-<pid>.jsonl, override with
// SF_DECK_API_TRACE_PATH=/some/path.jsonl.
//
// Designed for one-shot audits: cold-launch with the env var set, walk
// a baseline journey, then read the file to attribute API traffic. The
// caller tag (from runtime.Callers in caller.go) gives "which fetcher",
// the args give "which endpoint", duration gives "how slow."
//
// File mode is append: re-running the binary with the same path keeps
// growing the file. For a clean run, truncate or remove the file first.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiTraceEnv     = "SF_DECK_API_TRACE"
	apiTracePathEnv = "SF_DECK_API_TRACE_PATH"
)

// apiTraceRecord is one JSONL line. Field names match the render
// trace's convention (lowercase + underscore) so consumers can grep
// across both files.
type apiTraceRecord struct {
	Event  string  `json:"event"`
	TS     string  `json:"ts"`
	Alias  string  `json:"alias,omitempty"`
	Method string  `json:"method,omitempty"` // GET / POST / PATCH / DELETE / "" for CLI
	Path   string  `json:"path,omitempty"`   // REST path (with query) or first CLI arg
	Args   string  `json:"args,omitempty"`   // full argv as space-joined, when relevant
	Caller string  `json:"caller,omitempty"` // attribution tag from captureCaller
	OK     bool    `json:"ok"`
	Err    string  `json:"err,omitempty"`
	DurMS  float64 `json:"dur_ms,omitempty"`
}

type apiTracer struct {
	mu   sync.Mutex
	file *os.File
}

// apiTrace is the process-wide tracer. nil = disabled (the env var
// wasn't set at startup).
var apiTrace *apiTracer

// init opens the trace file if the env var is set. Failure to open is
// silently ignored — tracing is a diagnostic, not load-bearing.
//
// Using package-init keeps the tracer wired without main.go needing to
// know about it; the env var alone toggles the feature.
func init() {
	if !envTruthy(os.Getenv(apiTraceEnv)) {
		return
	}
	path := os.Getenv(apiTracePathEnv)
	if path == "" {
		path = defaultAPITracePath()
	}
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	file, err := openAPITraceFile(path)
	if err != nil {
		return
	}
	apiTrace = &apiTracer{file: file}
	apiTrace.writeRaw(map[string]any{
		"event": "api_trace_start",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"path":  path,
		"pid":   os.Getpid(),
	})
}

func openAPITraceFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	_ = file.Chmod(0o600)
	return file, nil
}

func defaultAPITracePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	name := "api-trace-" + time.Now().Format("20060102-150405") + "-" + strconv.Itoa(os.Getpid()) + ".jsonl"
	return filepath.Join(home, ".sf-deck", "log", name)
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// write emits one JSONL record. Best-effort: errors are swallowed
// (trace is diagnostic; failing-to-log shouldn't break the TUI).
func (t *apiTracer) write(rec apiTraceRecord) {
	if t == nil || t.file == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = t.file.Write(b)
}

// writeRaw is for the start-marker. Same lock as write.
func (t *apiTracer) writeRaw(m map[string]any) {
	if t == nil || t.file == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = t.file.Write(b)
}

// traceCall fans out a recorded Bump to the JSONL trace if it's
// enabled. Cheap no-op when the tracer is nil.
func traceCall(c Call) {
	if apiTrace == nil {
		return
	}
	method, path := splitMethodPath(c.Args)
	rec := apiTraceRecord{
		Event:  "api_call",
		TS:     c.At.UTC().Format(time.RFC3339Nano),
		Alias:  c.Alias,
		Method: method,
		Path:   redactSOQL(path),
		Caller: c.Caller,
		OK:     c.OK,
		Err:    c.Err,
	}
	if c.Dur > 0 {
		rec.DurMS = float64(c.Dur.Microseconds()) / 1000.0
	}
	// For non-REST shell-out calls (sf data query, sf org list, …)
	// the full argv carries more meaning than the first token alone.
	// Redact `-q "<SOQL>"` payloads on `sf data query` so the trace
	// doesn't carry raw SELECT bodies (which can contain Id values,
	// field names, and filter literals that read as PII-adjacent).
	if method != "" && !isRESTVerb(method) {
		rec.Args = redactCLIArgs(c.Args)
	}
	apiTrace.write(rec)
}

// redactSOQL replaces the value of a `q=...` query parameter on a REST
// path with `<redacted>`, preserving the rest of the path so call
// attribution still works. SOQL bodies can include Id values, custom
// field names, and filter literals that read as PII-adjacent — they're
// useful for in-session debugging (the ring buffer + ctrl+a modal)
// but shouldn't sit on disk in a JSONL file the user might share.
func redactSOQL(p string) string {
	i := strings.Index(p, "?q=")
	if i < 0 {
		i = strings.Index(p, "&q=")
		if i < 0 {
			return p
		}
	}
	// Keep everything up to and including the "q=" marker, then look
	// for the next "&" (if any) and keep that tail.
	head := p[:i+3]
	tail := ""
	if amp := strings.IndexByte(p[i+3:], '&'); amp >= 0 {
		tail = p[i+3+amp:]
	}
	return head + "<redacted>" + tail
}

// redactCLIArgs walks an `sf` argv and replaces the value following a
// `-q` / `--query` flag with `<redacted>`. Other args (org alias,
// flags) pass through untouched so the trace still reads as a
// command shape.
func redactCLIArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	cleaned := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			cleaned = append(cleaned, "<redacted>")
			skip = false
			continue
		}
		if a == "-q" || a == "--query" {
			cleaned = append(cleaned, a)
			skip = true
			continue
		}
		cleaned = append(cleaned, a)
	}
	return strings.Join(cleaned, " ")
}

// splitMethodPath inspects the argv shape Bump was called with and
// pulls out (method, path). REST calls arrive as ["GET", "/services/.."];
// CLI calls as ["data", "query", "-q", "..."]. For CLI calls we return
// (firstToken, "") so the JSON record is still useful — the full argv
// also rides in rec.Args.
func splitMethodPath(args []string) (string, string) {
	if len(args) == 0 {
		return "", ""
	}
	if len(args) >= 2 && isRESTVerb(args[0]) {
		return args[0], args[1]
	}
	return args[0], ""
}
