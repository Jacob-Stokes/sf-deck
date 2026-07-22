package applog

// Per-session structured log.
//
// One file per session at ~/.sf-deck/log/<timestamp>-<pid>.log. Always
// on (no env var gate) — the value of having a record of what happened
// is high and the cost is microscopic. Older sessions stay on disk so
// post-mortem debugging is possible after sf-deck has exited.
//
// Two surfaces:
//   - Info / Warn / Error / Debug for operations.
//   - Dump for binary blobs (HTML responses, xlsx that didn't parse,
//     etc.) — written to a sibling file under ~/.sf-deck/log/dumps/
//     so log greps stay readable.
//
// Logging is fire-and-forget: a write failure (disk full, permissions)
// shouldn't break the operation that triggered it. Errors from inside
// the logger are silently dropped.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level is the severity tag stamped on each line.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

var (
	mu      sync.Mutex
	file    *os.File
	dumpDir string
	logPath string
)

// Init opens the per-session log file. Idempotent — first call wins;
// subsequent calls are no-ops. Returns the absolute path of the log
// file so callers can surface it ("session log: …") if desired.
//
// Errors during init don't return as errors — they're surfaced once
// in the log file we *did* manage to open, or silently dropped if we
// couldn't open one at all. The point of the logger is durability,
// not correctness; a logger that errors out makes its callers worse.
func Init() string {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		return logPath
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	base := filepath.Join(home, ".sf-deck")
	dir := filepath.Join(base, "log")
	dumpDir = filepath.Join(dir, "dumps")
	if err := os.MkdirAll(dumpDir, 0o700); err != nil {
		return ""
	}
	// Tighten the base ~/.sf-deck dir to owner-only. MkdirAll only
	// sets the mode on dirs it CREATES, so existing installs (created
	// 0755 by older builds) need an explicit chmod. A 0700 base dir
	// blocks other local users from traversing in, which protects
	// every file inside — cache.db, devprojects.db (SOQL history),
	// instances.json, deploy.log — regardless of their own 0644 modes.
	// applog.Init runs early in startup, so this is the natural choke
	// point. Best-effort: a chmod failure shouldn't stop the logger.
	_ = os.Chmod(base, 0o700)
	// Prune older session logs + dumps so the directory doesn't grow
	// unbounded. We keep the most recent N of each so post-mortem
	// debugging still works for any recent session; older entries
	// drop. Errors here are silently ignored — the logger's job is
	// to start logging, not to enforce hygiene.
	pruneOldFiles(dir, "*.log", logKeep)
	pruneOldFiles(dumpDir, "*", dumpKeep)

	ts := time.Now().Format("20060102-150405")
	logPath = filepath.Join(dir, fmt.Sprintf("%s-%d.log", ts, os.Getpid()))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return ""
	}
	file = f
	// Header so each session's start is greppable.
	fmt.Fprintf(file, "[%s] [INFO] session_start pid=%d\n",
		time.Now().Format(time.RFC3339Nano), os.Getpid())
	return logPath
}

// Keep the last logKeep session logs and the last dumpKeep dump
// files. Constants rather than settings hooks: the user's not going
// to want a knob for this and adding one is just more surface.
// Numbers chosen to leave ample room for post-mortem ("when did I
// last open sf-deck and what happened?") without growing unbounded.
const (
	logKeep  = 50
	dumpKeep = 100
)

// pruneOldFiles removes the oldest files matching pattern in dir,
// keeping the keep most recent (by file mtime). Errors are ignored
// — the prune is best-effort and never blocks startup.
func pruneOldFiles(dir, pattern string, keep int) {
	if dir == "" || keep < 1 {
		return
	}
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) <= keep {
		return
	}
	type entry struct {
		path string
		mod  time.Time
	}
	entries := make([]entry, 0, len(matches))
	for _, p := range matches {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		entries = append(entries, entry{path: p, mod: fi.ModTime()})
	}
	if len(entries) <= keep {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mod.After(entries[j].mod)
	})
	for _, e := range entries[keep:] {
		_ = os.Remove(e.path)
	}
}

// Close flushes + closes the file. Safe to call multiple times.
// sf-deck's main calls this on shutdown.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		fmt.Fprintf(file, "[%s] [INFO] session_end pid=%d\n",
			time.Now().Format(time.RFC3339Nano), os.Getpid())
		_ = file.Close()
		file = nil
	}
}

// Log writes one structured line. Format:
//
//	[2026-04-26T15:37:01.234567Z] [LEVEL] event key=val key2=val2 …
//
// Values are stringified naïvely for readability; complex values
// should pass through Errorf-style formatting before reaching here.
// Keys are sorted by name so two lines with the same fields look the
// same regardless of map iteration order.
func Log(level Level, event string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return
	}
	var sb strings.Builder
	sb.WriteByte('[')
	sb.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	sb.WriteString("] [")
	sb.WriteString(string(level))
	sb.WriteString("] ")
	sb.WriteString(event)
	if len(fields) > 0 {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteByte(' ')
			sb.WriteString(k)
			sb.WriteByte('=')
			sb.WriteString(formatValue(fields[k]))
		}
	}
	sb.WriteByte('\n')
	_, _ = file.WriteString(sb.String())
}

// Info / Warn / Error are convenience wrappers over Log.
func Info(event string, fields map[string]any)  { Log(LevelInfo, event, fields) }
func Warn(event string, fields map[string]any)  { Log(LevelWarn, event, fields) }
func Error(event string, fields map[string]any) { Log(LevelError, event, fields) }

// Dump writes a binary blob to a separate file under ~/.sf-deck/log/dumps/
// and logs a pointer to it. Use for HTML error responses, malformed
// xlsx bytes, large API payloads — anything you'd want to inspect later
// without polluting the main log. Returns the dump file path so callers
// can include it in user-facing errors.
//
// Keys come from `tags`, used in the filename so dumps are
// self-describing: e.g. "20260426-153701-export-html-r.html" for a
// failed report export. Empty tags get a generic "dump" label.
func Dump(tags []string, ext string, body []byte) string {
	mu.Lock()
	dir := dumpDir
	mu.Unlock()
	if dir == "" {
		return ""
	}
	if ext == "" {
		ext = "bin"
	}
	label := "dump"
	if len(tags) > 0 {
		label = strings.Join(tags, "-")
		// Sanitise — file names mustn't carry surprising chars.
		label = sanitiseLabel(label)
	}
	ts := time.Now().Format("20060102-150405.000")
	name := fmt.Sprintf("%s-%s.%s", ts, label, ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return ""
	}
	Info("dump", map[string]any{"path": path, "bytes": len(body), "tags": tags})
	return path
}

// formatValue renders one field value for the log line. Strings get
// quoted only when they contain spaces or quotes; everything else uses
// JSON marshalling so structured values stay machine-readable.
func formatValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		if strings.ContainsAny(t, " \t\n\"=") {
			return strconvQuote(t)
		}
		return t
	case error:
		return strconvQuote(t.Error())
	case fmt.Stringer:
		return strconvQuote(t.String())
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
	return string(b)
}

// strconvQuote is fmt.Sprintf("%q", s) — the Go-quoted form. Inlined
// to avoid pulling strconv into the import list when only this is
// needed (and to keep escape-quoting consistent across versions).
func strconvQuote(s string) string {
	return fmt.Sprintf("%q", s)
}

func sanitiseLabel(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-', c == '_', c == '.':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
