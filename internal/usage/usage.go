package usage

// Local API-call counter. Tracks how many times sf-deck has shelled out
// to the sf CLI, broken down by day (local time). The TUI reads the
// today-total from Today() and displays it in the header. Persisted to
// SQLite so it survives restarts.
//
// Scope: this is sf-deck's *self-reported* call count. It isn't the same
// as the org's DailyApiRequests (Salesforce-side), which counts every
// API call across every tool/user against the org.

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// isRESTVerb reports whether the command tag is one of the HTTP verbs
// the REST client emits (vs the sf CLI subcommand shape).
func isRESTVerb(s string) bool {
	switch s {
	case "GET", "POST", "PATCH", "DELETE", "PUT":
		return true
	}
	return false
}

// bucketPath reduces a Salesforce REST path to a stable, low-cardinality
// string so the calls table groups by endpoint shape rather than per-Id.
// Examples:
//
//	/services/data/v62.0/sobjects/Account/describe
//	  → sobjects/Account/describe
//	/services/data/v62.0/tooling/query?q=…
//	  → tooling/query
//	/services/data/v62.0/sobjects/Account/listviews/00B…/results
//	  → sobjects/Account/listviews/<id>/results
//
// The (sobject, listview-id, record-id) variants get squashed to "<id>"
// so a screen that hammers one ID isn't double-counted across two
// distinct rows.
func bucketPath(p string) string {
	q := strings.IndexByte(p, '?')
	if q >= 0 {
		p = p[:q]
	}
	// Strip the /services/data/vXX.0/ prefix.
	if i := strings.Index(p, "/services/data/"); i >= 0 {
		rest := p[i+len("/services/data/"):]
		// rest = "v62.0/sobjects/..."
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			rest = rest[j+1:]
		}
		p = rest
	}
	p = strings.TrimPrefix(p, "/")
	// Replace 15/18-char Salesforce IDs with <id>.
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if isSalesforceID(seg) {
			parts[i] = "<id>"
		}
	}
	return strings.Join(parts, "/")
}

// isSalesforceID reports whether s looks like a 15/18-char SF Id.
func isSalesforceID(s string) bool {
	if len(s) != 15 && len(s) != 18 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !alnum {
			return false
		}
	}
	return true
}

// Call is one API call as captured by the ring buffer. Used by the
// in-session "recent calls" debug view; NOT persisted.
type Call struct {
	At      time.Time
	Alias   string
	Command string   // first argv token (data / org / sobject / REST method)
	Args    []string // full argv, trimmed to something renderable
	OK      bool
	Err     string // err.Error() when OK=false
	// Caller is the short "pkg.func" tag of the highest-level project
	// frame that triggered the call (e.g. "sf.fetchHome",
	// "ui.ensureActiveUsersChip"). Captured from the goroutine stack at
	// Bump time; "" when no useful frame was found. Used by the API
	// Call Log modal to attribute API traffic to fetchers / UI actions.
	Caller string
	// Dur is wall-clock latency from request start to fireOnCall, as
	// measured at the REST/CLI call site. Zero when the call site
	// didn't supply a duration.
	Dur time.Duration
}

// recentBufferSize caps the ring-buffer length for the debug view.
// 500 is enough to cover "what did the app just do" without eating
// memory.
const recentBufferSize = 500

// Tracker is the on-disk counter. Safe for concurrent Bump() calls.
type Tracker struct {
	db     *sql.DB
	path   string
	mu     sync.Mutex
	recent []Call // ring buffer, oldest first
}

// Open returns a Tracker backed by ~/.sf-deck/usage.db. The caller is
// responsible for calling Close() on shutdown.
func Open() (*Tracker, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return openAt(filepath.Join(dir, "usage.db"))
}

// openAt opens (or creates) a tracker DB at an explicit path. Used by
// Open() with the default location and by tests with a temp file.
func openAt(path string) (*Tracker, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single connection + busy_timeout, mirroring the cache store, so a
	// write can't race a second pooled connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Tracker{db: db, path: path}, nil
}

// migrate brings older DBs forward to the current schema. CREATE TABLE
// IF NOT EXISTS is a no-op when an old table exists with a different
// PK, so we detect and rebuild when the current table's columns /
// primary-key don't match what we expect.
func migrate(db *sql.DB) error {
	// Check if the calls table has an `alias` column AND whether
	// (day, alias, command, ok) is the composite primary key. A
	// pre-v2 DB has no alias column; a partial-migration DB has the
	// column but the old PK without it.
	hasAlias := false
	rows, err := db.Query(`PRAGMA table_info(calls)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "alias" {
			hasAlias = true
			if pk > 0 {
				// Column exists AND is part of PK — current schema.
				rows.Close()
				return nil
			}
		}
	}
	rows.Close()

	// Rebuild the table into the new shape. SQLite can't ALTER PK in
	// place, so we build a new table, copy rows over with alias=''
	// for the legacy ones, drop the old, rename.
	stmts := []string{
		`CREATE TABLE calls_new (
			day     TEXT NOT NULL,
			alias   TEXT NOT NULL DEFAULT '',
			command TEXT NOT NULL,
			ok      INTEGER NOT NULL,
			count   INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (day, alias, command, ok)
		)`,
	}
	if hasAlias {
		stmts = append(stmts,
			`INSERT INTO calls_new (day, alias, command, ok, count)
			 SELECT day, alias, command, ok, count FROM calls`)
	} else {
		stmts = append(stmts,
			`INSERT INTO calls_new (day, alias, command, ok, count)
			 SELECT day, '', command, ok, count FROM calls`)
	}
	stmts = append(stmts,
		`DROP TABLE calls`,
		`ALTER TABLE calls_new RENAME TO calls`,
		`CREATE INDEX IF NOT EXISTS idx_calls_day ON calls(day)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_alias ON calls(alias)`,
	)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (t *Tracker) Close() error { return t.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS calls (
  day     TEXT NOT NULL,       -- YYYY-MM-DD local date
  alias   TEXT NOT NULL DEFAULT '',  -- org alias the call targeted; '' for org-agnostic
  command TEXT NOT NULL,       -- first sf subcommand, e.g. "data" / "org" / "sobject"
  ok      INTEGER NOT NULL,    -- 1 = success, 0 = error
  count   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (day, alias, command, ok)
);
CREATE INDEX IF NOT EXISTS idx_calls_day ON calls(day);
CREATE INDEX IF NOT EXISTS idx_calls_alias ON calls(alias);
`

// Bump records one API call. alias is the org the call targeted
// (empty string for org-agnostic calls like sf org list). args is
// the argv; we record the first token as a rough command tag
// (data / org / sobject / apex / deploy / …) so future reports can
// group by subcommand. dur is wall-clock latency (0 if the call site
// didn't measure). Safe to call from any goroutine.
func (t *Tracker) Bump(alias string, args []string, callErr error, dur time.Duration) {
	// Capture the caller *before* taking the lock so the stack walk
	// reflects the actual fetcher's goroutine, not the lock-acquisition
	// path.
	caller := captureCaller()

	t.mu.Lock()
	defer t.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	// For REST calls (GET / POST / PATCH / DELETE) the second arg is
	// the request path. We bucket the path down to a stable shape
	// ("sobjects/Account/describe", "tooling/query", …) so the calls
	// table groups by endpoint, which is the diagnosis question we
	// keep needing to answer ("which surface is hammering?"). Falls
	// back to the bare verb when args don't fit the REST shape.
	if len(args) >= 2 && isRESTVerb(cmd) {
		cmd = cmd + " " + bucketPath(args[1])
	}
	ok := 1
	if callErr != nil {
		ok = 0
	}
	_, _ = t.db.Exec(`
		INSERT INTO calls (day, alias, command, ok, count) VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(day, alias, command, ok) DO UPDATE SET count = count + 1`,
		day, alias, cmd, ok)

	// Ring buffer for the recent-calls debug view. Memory only; fine
	// to drop on process exit since this is a diagnostic aid.
	entry := Call{
		At:      time.Now(),
		Alias:   alias,
		Command: cmd,
		Args:    append([]string(nil), args...),
		OK:      callErr == nil,
		Caller:  caller,
		Dur:     dur,
	}
	if callErr != nil {
		entry.Err = callErr.Error()
	}
	t.recent = append(t.recent, entry)
	if len(t.recent) > recentBufferSize {
		t.recent = t.recent[len(t.recent)-recentBufferSize:]
	}

	// Stream the call to the JSONL trace if SF_DECK_API_TRACE was set.
	// No-op when tracing is disabled. Cheap enough to call under the
	// lock; the tracer has its own mutex but contention is fine since
	// every call already serializes on t.mu anyway.
	traceCall(entry)
}

// Recent returns a copy of the ring buffer, newest first. Capped at
// recentBufferSize entries. Safe to call from any goroutine.
func (t *Tracker) Recent() []Call {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Call, len(t.recent))
	// Reverse-copy so index 0 is the newest call.
	for i, c := range t.recent {
		out[len(t.recent)-1-i] = c
	}
	return out
}

// Today returns the total number of API calls made today across all
// orgs (including org-agnostic calls).
func (t *Tracker) Today() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	day := time.Now().Format("2006-01-02")
	var n int
	_ = t.db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM calls WHERE day = ?`, day).Scan(&n)
	return n
}

// TodayForOrg returns today's call count for one specific org alias.
// Org-agnostic calls (alias = "") are not included — they're counted
// by Today() but don't belong to any single org.
func (t *Tracker) TodayForOrg(alias string) int {
	return t.TodayForOrgKeys(alias)
}

// TodayForOrgKeys sums today's calls across SEVERAL keys for the same org
// and counts each underlying row once. The same org is referred to by
// BOTH its short alias (e.g. "acme-test") and its username
// (e.g. "user@org.test") in different code paths — fireOnCall records
// under whichever the caller used. Passing both keys here reconciles them
// so the header counter reflects ALL of an org's calls (this is what made
// the /compare counter appear flat: it records under the username while
// the header looked up the alias). Empty keys are ignored.
func (t *Tracker) TodayForOrgKeys(aliases ...string) int {
	seen := map[string]bool{}
	var keys []string
	for _, a := range aliases {
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		keys = append(keys, a)
	}
	if len(keys) == 0 {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	day := time.Now().Format("2006-01-02")
	placeholders := make([]string, len(keys))
	args := make([]any, 0, len(keys)+1)
	args = append(args, day)
	for i, k := range keys {
		placeholders[i] = "?"
		args = append(args, k)
	}
	q := `SELECT COALESCE(SUM(count), 0) FROM calls WHERE day = ? AND alias IN (` +
		strings.Join(placeholders, ",") + `)`
	var n int
	_ = t.db.QueryRow(q, args...).Scan(&n)
	return n
}

// TodayByCommand returns today's totals keyed by subcommand. Summed
// across all orgs.
func (t *Tracker) TodayByCommand() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	day := time.Now().Format("2006-01-02")
	rows, err := t.db.Query(`SELECT command, SUM(count) FROM calls WHERE day = ? GROUP BY command`, day)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err == nil && c != "" {
			out[c] = n
		}
	}
	return out
}

// Path returns the on-disk location of the usage DB, useful for debug
// output and `rm` during testing.
func (t *Tracker) Path() string { return t.path }

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sf-deck"), nil
}
