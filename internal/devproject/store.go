package devproject

// SQLite-backed storage for Dev Projects + Items.
//
// File: ~/.sf-deck/devprojects.db (separate from cache.db so cache
// rebuilds / migrations don't risk wiping the user's hand-curated
// project state).
//
// Earlier versions of sf-deck split items across a per-(DevProject,
// org) "OrgProject" intermediate. This file used to manage all three
// (dev_projects, org_projects, org_project_items). The migration
// flattens that to two tables — dev_projects + dev_project_items —
// where each item carries the originating org_user directly. See
// migrateFromOrgProjects below for the one-time data move; the
// org_projects + org_project_items tables are renamed to
// org_projects_legacy + org_project_items_legacy after a successful
// migration so the data isn't lost if the user wants to inspect it.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotEmpty is returned by DeleteDevProject when force=false and
// the project still has items. The UI catches this and prompts the
// user to either empty the project first or pass force.
var ErrNotEmpty = errors.New("devproject: project is not empty")

const schema = `
CREATE TABLE IF NOT EXISTS dev_projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    touched_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dev_project_items (
    dev_project_id TEXT NOT NULL,
    org_user       TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL,
    ref            TEXT NOT NULL,
    type           TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL DEFAULT '',
    added_at       INTEGER NOT NULL,
    notes          TEXT NOT NULL DEFAULT '',
    namespace      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (dev_project_id, org_user, kind, ref)
);

CREATE INDEX IF NOT EXISTS idx_items_kind ON dev_project_items(kind);
CREATE INDEX IF NOT EXISTS idx_items_org  ON dev_project_items(org_user);
CREATE INDEX IF NOT EXISTS idx_items_dev  ON dev_project_items(dev_project_id);

-- Tags. Personal annotation layer over metadata + records, orthogonal
-- to projects. Definitions are global (one tag namespace per user);
-- bindings are per-(item kind, item ref, org) so the same tag can be
-- applied across orgs without conflict.
CREATE TABLE IF NOT EXISTS tags (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE COLLATE NOCASE,
    color       TEXT NOT NULL DEFAULT '',
    icon        TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tag_bindings (
    tag_id     INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    item_kind  TEXT NOT NULL,
    item_ref   TEXT NOT NULL,
    org_user   TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (tag_id, item_kind, item_ref, org_user)
);

CREATE INDEX IF NOT EXISTS idx_tagbind_item ON tag_bindings(item_kind, item_ref, org_user);
CREATE INDEX IF NOT EXISTS idx_tagbind_tag  ON tag_bindings(tag_id);

-- Bundles. On-disk sfdx project directories linked back to the
-- DevProject they were created from. One DevProject can have many
-- bundles; each bundle belongs to exactly one DevProject. Path is
-- the absolute filesystem location; we re-stat it on each read to
-- detect "stale" bundles (user moved/deleted the directory).
CREATE TABLE IF NOT EXISTS bundles (
    id                 TEXT PRIMARY KEY,
    dev_project_id     TEXT NOT NULL REFERENCES dev_projects(id) ON DELETE CASCADE,
    path               TEXT NOT NULL,
    default_org_alias  TEXT NOT NULL DEFAULT '',
    created_at         INTEGER NOT NULL,
    last_retrieved_at  INTEGER NOT NULL DEFAULT 0,
    last_deployed_at   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_bundles_dev ON bundles(dev_project_id);

-- Saved SOQL queries. First-class user-authored objects, distinct
-- from one-shot history rows. Org-agnostic by default (a saved query
-- can run against any org); pinning to a DevProject + tagging both
-- reuse the existing dev_project_items + tag_bindings tables with
-- kind='soql_query', ref=saved_queries.id.
CREATE TABLE IF NOT EXISTS saved_queries (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_saved_queries_updated ON saved_queries(updated_at DESC);

-- SOQL history. Every executed query lands here, scoped to the org
-- it ran against. Tail-truncated periodically to keep the table from
-- growing unboundedly (see TrimSOQLHistory).
CREATE TABLE IF NOT EXISTS soql_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    org_user    TEXT NOT NULL,
    body        TEXT NOT NULL,
    executed_at INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    row_count   INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_soql_history_org_time ON soql_history(org_user, executed_at DESC);

-- Saved Apex snippets — user-authored anonymous Apex bodies. Same
-- shape as saved_queries (id / name / description / body / timestamps)
-- so the Library subtab on /exec mirrors the Library on /soql without
-- per-table special cases. Org-agnostic; pinnable / taggable via
-- dev_project_items + tag_bindings with kind='apex_snippet'.
CREATE TABLE IF NOT EXISTS saved_apex (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_saved_apex_updated ON saved_apex(updated_at DESC);

-- Apex execution history. Every executeAnonymous run lands here.
-- Status fields mirror the Tooling-API result: compiled / success /
-- compile_problem / exception_message + the canonical line:column on
-- failure. Trimmed periodically via TrimApexHistory.
CREATE TABLE IF NOT EXISTS apex_history (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    org_user          TEXT NOT NULL,
    body              TEXT NOT NULL,
    executed_at       INTEGER NOT NULL,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    compiled          INTEGER NOT NULL DEFAULT 0,
    success           INTEGER NOT NULL DEFAULT 0,
    compile_problem   TEXT NOT NULL DEFAULT '',
    exception_message TEXT NOT NULL DEFAULT '',
    line              INTEGER NOT NULL DEFAULT -1,
    column_           INTEGER NOT NULL DEFAULT -1,
    log_id            TEXT NOT NULL DEFAULT '',
    log_body          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_apex_history_org_time ON apex_history(org_user, executed_at DESC);

-- Saved metadata comparisons — a full org-to-org compare RESULT (not
-- just a template): source/target/scope/method metadata PLUS a gzipped
-- JSON blob of the retrieved snapshots + inventory, so the user can
-- reopen and act on the comparison offline after a restart. Lives here
-- (devprojects.db, not cache.db) so cache-clear never wipes it.
CREATE TABLE IF NOT EXISTS saved_comparisons (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    target      TEXT NOT NULL DEFAULT '',
    scope       TEXT NOT NULL DEFAULT '',  -- comma-joined type labels
    method      TEXT NOT NULL DEFAULT '',
    blob_gz     BLOB,                      -- gzipped JSON: {snapA, snapB, inventory}
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_saved_comparisons_updated ON saved_comparisons(updated_at DESC);

-- Notes. One free-text note per (item kind, item ref, org) -- same
-- identity triple as tag_bindings, so anything collectable/taggable
-- is notable. Independent of projects: a note needs no collect.
CREATE TABLE IF NOT EXISTS notes (
    item_kind  TEXT NOT NULL,
    item_ref   TEXT NOT NULL,
    org_user   TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (item_kind, item_ref, org_user)
);
`

// Store is the SQLite handle. Open returns one rooted at
// ~/.sf-deck/devprojects.db; Close on app shutdown.
type Store struct {
	db   *sql.DB
	path string

	// generation increments on every write that affects tag bindings
	// or project membership. Read-only consumers (the per-render
	// bulk gutter caches in internal/ui/orgData) compare it against
	// the value they captured at cache-fill time to decide whether
	// to reuse or rebuild. Bumped via touch(); zero is the initial
	// "never mutated" state.
	//
	// atomic.Int64 because some Store mutations execute inside
	// tea.Cmd closures that run on background goroutines (modal
	// save handlers, project create/delete from action sidebars).
	// Pre-atomic this was a torn-write hazard between the cmd
	// goroutine bumping `generation` and the main loop reading it
	// during Filtered() / BuildRenderModel.
	generation atomic.Int64
}

// Generation returns the current mutation counter. Cache consumers
// capture this at fill time and re-check on read; a mismatch means
// "data changed underneath you, rebuild."
func (s *Store) Generation() int {
	if s == nil {
		return 0
	}
	return int(s.generation.Load())
}

// touch bumps the mutation counter. Called from every write method
// that affects tag_bindings or dev_project_items so cache consumers
// invalidate correctly.
func (s *Store) touch() {
	if s == nil {
		return
	}
	s.generation.Add(1)
}

// Open creates / loads the dev-project store. Idempotent — running
// twice on the same machine reuses the same file. Migrates legacy
// (DevProject + OrgProject + Items) layouts to the flat (DevProject
// + Items-with-org_user) layout on first run after the upgrade.
func Open() (*Store, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "devprojects.db")
	st, err := openAt(path)
	if err == nil {
		// Healthy open: refresh the rolling backup so a future
		// corruption has something recent to fall back to. Best-
		// effort — a failed copy never blocks startup.
		backupDB(path)
		return st, nil
	}
	// Open or integrity check failed. The store holds projects,
	// tags, and saved queries — losing it silently (the previous
	// behaviour: features just turned off) is worse than recovering
	// a slightly stale backup. Quarantine the corrupt file and try
	// the backup; if there is none, start fresh.
	quarantine := path + ".corrupt-" + time.Now().Format("20060102-150405")
	_ = os.Rename(path, quarantine)
	if _, berr := os.Stat(path + ".bak"); berr == nil {
		if cerr := copyFile(path+".bak", path); cerr == nil {
			if st, rerr := openAt(path); rerr == nil {
				return st, fmt.Errorf("devprojects.db was corrupt (saved to %s); RESTORED FROM BACKUP — recent changes may be missing: %w", quarantine, errRecoveredFromBackup)
			}
		}
		_ = os.Remove(path)
	}
	st, ferr := openAt(path)
	if ferr != nil {
		return nil, fmt.Errorf("devprojects.db corrupt (saved to %s) and recovery failed: %w", quarantine, ferr)
	}
	return st, fmt.Errorf("devprojects.db was corrupt (saved to %s); STARTED FRESH: %w", quarantine, errRecoveredFromBackup)
}

// OpenPath opens a store at an explicit path, skipping Open()'s
// backup/recovery wrapper. Used by `sf-deck --demo` for a throwaway
// store under a temp dir; a corrupt throwaway has nothing worth
// recovering.
func OpenPath(path string) (*Store, error) {
	return openAt(path)
}

// errRecoveredFromBackup marks the soft-error shape Open returns
// when it recovered: the Store is USABLE (non-nil) but the caller
// should surface the data-loss warning to the user.
var errRecoveredFromBackup = errors.New("devproject store recovered")

// RecoveredFromBackup reports whether an Open error is the soft
// recovered-but-usable signal rather than a fatal failure.
func RecoveredFromBackup(err error) bool {
	return errors.Is(err, errRecoveredFromBackup)
}

// openAt opens + migrates + integrity-checks one path. Also the
// test seam — tests open temp paths directly, skipping Open()'s
// backup/recovery wrapper.
func openAt(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Pin to a single connection + a busy_timeout, mirroring the cache
	// store. database/sql can otherwise open a 2nd physical connection
	// (e.g. a write firing while *sql.Rows is mid-iteration); on a 2nd
	// connection FK enforcement isn't guaranteed and writes can hit
	// "database is locked" instead of waiting. One connection serializes
	// access and keeps the PRAGMA settings (incl. foreign_keys) bound.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Enable foreign keys before applying schema so the tag_bindings
	// FK constraint is enforced (SQLite defaults FK enforcement OFF).
	if err := ensureForeignKeysOn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// quick_check is the cheap corruption probe (vs full
	// integrity_check) — catches torn pages from a kill-9 mid-commit
	// without scanning every index.
	var verdict string
	if err := db.QueryRow("PRAGMA quick_check(1)").Scan(&verdict); err != nil || verdict != "ok" {
		_ = db.Close()
		if err == nil {
			err = fmt.Errorf("quick_check: %s", verdict)
		}
		return nil, fmt.Errorf("integrity: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateFromOrgProjects(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate org_projects: %w", err)
	}
	if err := addItemNamespaceColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate dev_project_items.namespace: %w", err)
	}
	if err := migrateRecordItemRefs(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate record item refs: %w", err)
	}
	// sqlite creates the db file 0644; tighten to owner-only. This store
	// holds SOQL history, saved Apex, tags, and project membership — not
	// world-readable data. Best-effort, defense in depth behind the 0700
	// base dir.
	_ = os.Chmod(path, 0o600)
	return &Store{db: db, path: path}, nil
}

// backupDB copies the db file to <path>.bak. Called after every
// healthy open — at-most-one-session stale, which bounds the loss a
// restore can incur. Plain file copy is safe at this point: the
// connection has just been opened and nothing writes during startup.
func backupDB(path string) {
	_ = copyFile(path, path+".bak")
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// addItemNamespaceColumn is an idempotent ALTER TABLE migration that
// adds the namespace column to dev_project_items for stores created
// before the column was part of the schema. CREATE TABLE IF NOT
// EXISTS won't add a column to an existing table — this function
// fills the gap.
//
// The column defaults to ” so existing rows naturally classify as
// non-managed. New collects populate it from the source query.
func addItemNamespaceColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(dev_project_items)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "namespace" {
			return nil // already present
		}
	}
	_, err = db.Exec(`ALTER TABLE dev_project_items ADD COLUMN namespace TEXT NOT NULL DEFAULT ''`)
	return err
}

// migrateRecordItemRefs rewrites legacy KindRecord refs from the bare
// "<Id>" shape (sObject only in the type column) to the canonical
// "<sObject>:<Id>" every tag/project lookup keys by. Items written
// before the fix never matched a PROJECTS-gutter lookup — collected
// records showed no pill. Idempotent: migrated rows contain ':' and
// are skipped on later opens.
//
// Twin-safe: UPDATE OR IGNORE skips any row whose canonical twin
// already exists (PK collision on dev_project_id/org_user/kind/ref —
// possible if the same record was re-collected after the fix); the
// leftover legacy rows are then dropped as duplicates.
func migrateRecordItemRefs(db *sql.DB) error {
	if _, err := db.Exec(`UPDATE OR IGNORE dev_project_items
		SET ref = type || ':' || ref
		WHERE kind = 'record' AND type <> '' AND instr(ref, ':') = 0`); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM dev_project_items
		WHERE kind = 'record' AND type <> '' AND instr(ref, ':') = 0`)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("user home not resolvable")
	}
	return filepath.Join(home, ".sf-deck"), nil
}

// migrateFromOrgProjects flattens any pre-existing OrgProject layout
// into the new dev_project_items table. No-op when the legacy tables
// don't exist (fresh install) or are already empty (already migrated).
//
// Strategy:
//   - For every (org_project_id, kind, ref) row in the legacy item
//     table, look up the org_project's (dev_project_id, org_user)
//     and INSERT OR IGNORE into dev_project_items with the same
//     metadata.
//   - Rename the legacy tables to *_legacy so the data isn't gone
//     forever — users can inspect via sqlite3 if anything's missed.
//
// Idempotent: running twice is safe; the second pass sees the legacy
// tables under their renamed names and does nothing.
func migrateFromOrgProjects(db *sql.DB) error {
	hasLegacy, err := tableExists(db, "org_project_items")
	if err != nil {
		return err
	}
	if !hasLegacy {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT op.dev_project_id, op.org_user, i.kind, i.ref, i.type, i.name, i.added_at, i.notes
		FROM org_project_items i
		JOIN org_projects op ON op.id = i.org_project_id`)
	if err != nil {
		// org_projects table missing but org_project_items present —
		// data is already orphaned, nothing to copy. Continue with
		// rename to clean up.
		if !strings.Contains(err.Error(), "no such table") {
			return err
		}
	} else {
		for rows.Next() {
			var devID, orgUser, kind, ref, typ, name, notes string
			var addedAt int64
			if err := rows.Scan(&devID, &orgUser, &kind, &ref, &typ, &name, &addedAt, &notes); err != nil {
				rows.Close()
				return err
			}
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO dev_project_items
				(dev_project_id, org_user, kind, ref, type, name, added_at, notes)
				VALUES (?,?,?,?,?,?,?,?)`,
				devID, orgUser, kind, ref, typ, name, addedAt, notes); err != nil {
				rows.Close()
				return err
			}
		}
		rows.Close()
	}

	if _, err := tx.Exec(`ALTER TABLE org_project_items RENAME TO org_project_items_legacy`); err != nil {
		return err
	}
	if has, _ := tableExists(db, "org_projects"); has {
		if _, err := tx.Exec(`ALTER TABLE org_projects RENAME TO org_projects_legacy`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tableExists(db *sql.DB, name string) (bool, error) {
	row := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name=?`, name)
	var n int
	if err := row.Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateDevProject inserts a new dev project. ID is caller-provided
// (typically a small UUID/random string) so the call site can persist
// the ID before the row commits if needed.
func (s *Store) CreateDevProject(p DevProject) error {
	if s == nil {
		return nil
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.TouchedAt.IsZero() {
		p.TouchedAt = p.CreatedAt
	}
	_, err := s.db.Exec(
		`INSERT INTO dev_projects (id, name, description, created_at, touched_at) VALUES (?,?,?,?,?)`,
		p.ID, p.Name, p.Description, p.CreatedAt.Unix(), p.TouchedAt.Unix())
	if err == nil {
		s.touch()
	}
	return err
}

// UpdateDevProject mutates the name / description and bumps touched_at.
func (s *Store) UpdateDevProject(id, name, description string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE dev_projects SET name=?, description=?, touched_at=? WHERE id=?`,
		name, description, time.Now().Unix(), id)
	if err == nil {
		s.touch()
	}
	return err
}

// DeleteDevProject removes a dev project + every item belonging to
// it. When force=false and the project still has items, returns
// ErrNotEmpty without touching state — the UI surfaces a "force to
// cascade" hint. When force=true, cascades in one transaction.
func (s *Store) DeleteDevProject(id string, force bool) error {
	if s == nil {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !force {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM dev_project_items WHERE dev_project_id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrNotEmpty
		}
	}
	if _, err := tx.Exec(`DELETE FROM dev_project_items WHERE dev_project_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM dev_projects WHERE id=?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.touch()
	return nil
}

// GetDevProject returns the dev project with the given id, or
// (nil, nil) when none exists. Used by the create wizards + edit
// flows to resolve a freshly-created or just-edited row.
func (s *Store) GetDevProject(id string) (*DevProject, error) {
	if s == nil {
		return nil, nil
	}
	row := s.db.QueryRow(
		`SELECT id, name, description, created_at, touched_at
		 FROM dev_projects WHERE id=?`, id)
	var p DevProject
	var c, t int64
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &c, &t); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.CreatedAt = time.Unix(c, 0)
	p.TouchedAt = time.Unix(t, 0)
	return &p, nil
}

// ListDevProjects returns every dev project, most-recently-touched first.
func (s *Store) ListDevProjects() ([]DevProject, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT id, name, description, created_at, touched_at FROM dev_projects ORDER BY touched_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DevProject
	for rows.Next() {
		var p DevProject
		var c, t int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &c, &t); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(c, 0)
		p.TouchedAt = time.Unix(t, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddItem inserts an item into a dev project. PRIMARY KEY collision
// (same project + org + kind + ref) is treated as "already there"
// and reported via the boolean return; no error.
func (s *Store) AddItem(item Item) (added bool, err error) {
	if s == nil {
		return false, nil
	}
	if item.DevProjectID == "" {
		return false, errors.New("devproject.AddItem: missing DevProjectID")
	}
	if item.AddedAt.IsZero() {
		item.AddedAt = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO dev_project_items
		 (dev_project_id, org_user, kind, ref, type, name, added_at, notes, namespace)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		item.DevProjectID, item.OrgUser, string(item.Kind), item.Ref, item.Type,
		item.Name, item.AddedAt.Unix(), item.Notes, item.Namespace)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows > 0 {
		_, _ = s.db.Exec(
			`UPDATE dev_projects SET touched_at=? WHERE id=?`,
			time.Now().Unix(), item.DevProjectID)
		s.touch()
	}
	return rows > 0, nil
}

// RemoveItem deletes one item from a dev project. Org-scoped: the
// same (kind, ref) collected from two different orgs are two rows
// and removed independently.
func (s *Store) RemoveItem(devProjectID, orgUser string, kind ItemKind, ref string) error {
	if s == nil {
		return nil
	}
	res, err := s.db.Exec(
		`DELETE FROM dev_project_items
		 WHERE dev_project_id=? AND org_user=? AND kind=? AND ref=?`,
		devProjectID, orgUser, string(kind), ref)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.touch()
	}
	return nil
}

// ItemDelete identifies one item row to remove (its full primary key).
type ItemDelete struct {
	DevProjectID string
	OrgUser      string
	Kind         ItemKind
	Ref          string
}

// ItemRewrite re-keys an item to a canonical ref (and optionally fills
// Name/Type), used by reconcile to normalise e.g. a flow stored under
// its DeveloperName to its stable DefinitionId. From is the current
// (kind, ref); ToRef is the canonical ref. If a row already exists at
// the target ref, the From row is dropped (the target wins) rather than
// colliding on the primary key.
type ItemRewrite struct {
	DevProjectID string
	OrgUser      string
	Kind         ItemKind
	FromRef      string
	ToRef        string
	Name         string // set when non-empty
	Type         string // set when non-empty
}

// ApplyReconcile executes a batch of deletes + ref-rewrites in a single
// transaction. Idempotent and safe to call on every dev-project touch:
// with nothing to change it's a no-op (no write, no touch()). Returns
// the number of rows removed and rewritten so the caller can flash a
// summary only when something actually changed.
func (s *Store) ApplyReconcile(deletes []ItemDelete, rewrites []ItemRewrite) (removed, merged int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, nil
	}
	if len(deletes) == 0 && len(rewrites) == 0 {
		return 0, 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	delItem, err := tx.Prepare(`DELETE FROM dev_project_items
		WHERE dev_project_id=? AND org_user=? AND kind=? AND ref=?`)
	if err != nil {
		return 0, 0, err
	}
	defer delItem.Close()

	for _, d := range deletes {
		res, e := delItem.Exec(d.DevProjectID, d.OrgUser, string(d.Kind), d.Ref)
		if e != nil {
			err = e
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed++
		}
	}

	// For a rewrite: if the target ref already exists, drop the source
	// row (merge into target). Otherwise UPDATE the source row's ref
	// (and name/type when provided).
	existsAt := func(rw ItemRewrite, ref string) (bool, error) {
		var n int
		e := tx.QueryRow(`SELECT COUNT(1) FROM dev_project_items
			WHERE dev_project_id=? AND org_user=? AND kind=? AND ref=?`,
			rw.DevProjectID, rw.OrgUser, string(rw.Kind), ref).Scan(&n)
		return n > 0, e
	}
	for _, rw := range rewrites {
		if rw.ToRef == "" || rw.ToRef == rw.FromRef {
			continue
		}
		targetExists, e := existsAt(rw, rw.ToRef)
		if e != nil {
			err = e
			return 0, 0, err
		}
		if targetExists {
			// Canonical row already there — drop the duplicate source.
			res, e := delItem.Exec(rw.DevProjectID, rw.OrgUser, string(rw.Kind), rw.FromRef)
			if e != nil {
				err = e
				return 0, 0, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				merged++
			}
			continue
		}
		// Re-key the source row to the canonical ref, filling name/type.
		res, e := tx.Exec(`UPDATE dev_project_items
			SET ref=?,
			    name=CASE WHEN ?<>'' THEN ? ELSE name END,
			    type=CASE WHEN ?<>'' THEN ? ELSE type END
			WHERE dev_project_id=? AND org_user=? AND kind=? AND ref=?`,
			rw.ToRef, rw.Name, rw.Name, rw.Type, rw.Type,
			rw.DevProjectID, rw.OrgUser, string(rw.Kind), rw.FromRef)
		if e != nil {
			err = e
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			merged++
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	if removed > 0 || merged > 0 {
		s.touch()
	}
	return removed, merged, nil
}

// ListItems returns every item in a dev project, ordered by org_user
// then kind then by added-at (oldest first within a kind so the
// user's mental "what did I add when" timeline is preserved).
//
// orgUser="" returns items from every org. Pass an explicit org_user
// to filter to that org's contributions only — used by the per-org
// Scope hydrator and by the rail's "items in active org" view.
func (s *Store) ListItems(devProjectID, orgUser string) ([]Item, error) {
	if s == nil {
		return nil, nil
	}
	q := `SELECT dev_project_id, org_user, kind, ref, type, name, added_at, notes, namespace
	      FROM dev_project_items WHERE dev_project_id=?`
	args := []any{devProjectID}
	if orgUser != "" {
		q += ` AND org_user=?`
		args = append(args, orgUser)
	}
	q += ` ORDER BY org_user, kind, added_at`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		var ts int64
		var kind string
		if err := rows.Scan(&it.DevProjectID, &it.OrgUser, &kind, &it.Ref, &it.Type, &it.Name, &ts, &it.Notes, &it.Namespace); err != nil {
			return nil, err
		}
		it.Kind = ItemKind(kind)
		it.AddedAt = time.Unix(ts, 0)
		out = append(out, it)
	}
	return out, rows.Err()
}

// CountsForDev aggregates item + distinct-org counts for a dev project.
// Used in the /dev-projects list view to render the per-project summary
// line.
func (s *Store) CountsForDev(devProjectID string) (Counts, error) {
	c := Counts{ByKind: map[ItemKind]int{}}
	if s == nil {
		return c, nil
	}
	row := s.db.QueryRow(
		`SELECT COUNT(DISTINCT org_user) FROM dev_project_items
		 WHERE dev_project_id=? AND org_user<>''`,
		devProjectID)
	if err := row.Scan(&c.Orgs); err != nil {
		return c, err
	}
	rows, err := s.db.Query(
		`SELECT kind, COUNT(*) FROM dev_project_items
		 WHERE dev_project_id=?
		 GROUP BY kind`, devProjectID)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return c, err
		}
		c.ByKind[ItemKind(kind)] = n
		c.Items += n
	}
	return c, rows.Err()
}

// CountsForDevInOrg is the per-(project, org) variant — items
// contributed from one specific org. Used by the per-org hydration
// path so the rail can show "23 items in this org / 5 in other orgs."
func (s *Store) CountsForDevInOrg(devProjectID, orgUser string) (Counts, error) {
	c := Counts{Orgs: 1, ByKind: map[ItemKind]int{}}
	if s == nil {
		return c, nil
	}
	rows, err := s.db.Query(
		`SELECT kind, COUNT(*) FROM dev_project_items
		 WHERE dev_project_id=? AND org_user=? GROUP BY kind`,
		devProjectID, orgUser)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return c, err
		}
		c.ByKind[ItemKind(kind)] = n
		c.Items += n
	}
	return c, rows.Err()
}

// OrgsForDev returns every distinct org_user that has contributed
// items to a dev project, most-recent first. Used by the detail
// view's "this project touches N orgs" header.
func (s *Store) OrgsForDev(devProjectID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT org_user FROM dev_project_items
		 WHERE dev_project_id=? AND org_user<>''
		 GROUP BY org_user
		 ORDER BY MAX(added_at) DESC`, devProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ProjectsForItem returns every DevProject that contains the given
// item in the given org. Empty slice when the item isn't in any
// project. Sorted by project name (case-insensitive) for stable
// rendering in pills.
func (s *Store) ProjectsForItem(kind ItemKind, ref, orgUser string) ([]DevProject, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.description, p.created_at, p.touched_at
		FROM dev_projects p
		JOIN dev_project_items i ON i.dev_project_id = p.id
		WHERE i.kind = ? AND i.ref = ? AND i.org_user = ?
		ORDER BY p.name COLLATE NOCASE
	`, string(kind), ref, orgUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DevProject
	for rows.Next() {
		var p DevProject
		var c, t int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &c, &t); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(c, 0).UTC()
		p.TouchedAt = time.Unix(t, 0).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// ItemInProject reports whether the given item (kind, ref, org) is
// already collected into the specified project. Used by the collect
// toggle (ctrl+K) so a second press on an already-collected item
// removes it rather than re-adding.
func (s *Store) ItemInProject(devProjectID string, kind ItemKind, ref, orgUser string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("devproject: store closed")
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM dev_project_items
		 WHERE dev_project_id=? AND kind=? AND ref=? AND org_user=?`,
		devProjectID, string(kind), ref, orgUser,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ProjectsForItems is the bulk variant of ProjectsForItem — given a
// slice of (kind, ref) pairs in the same org, returns a map keyed
// by "kind:ref" → []DevProject. Mirrors TagsForItems' shape +
// rationale: load all per-org bindings once and filter client-side
// to dodge SQLite's expression-tree depth limit on big lists.
func (s *Store) ProjectsForItems(orgUser string, items []TagLookupKey) (map[string][]DevProject, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	if len(items) == 0 {
		return map[string][]DevProject{}, nil
	}
	wanted := make(map[string]struct{}, len(items))
	for _, it := range items {
		wanted[string(it.Kind)+":"+it.Ref] = struct{}{}
	}
	rows, err := s.db.Query(`
		SELECT i.kind, i.ref,
		       p.id, p.name, p.description, p.created_at, p.touched_at
		FROM dev_project_items i
		JOIN dev_projects p ON p.id = i.dev_project_id
		WHERE i.org_user = ?
		ORDER BY p.name COLLATE NOCASE
	`, orgUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]DevProject{}
	for rows.Next() {
		var kind, ref string
		var p DevProject
		var c, t int64
		if err := rows.Scan(&kind, &ref, &p.ID, &p.Name, &p.Description, &c, &t); err != nil {
			return nil, err
		}
		key := kind + ":" + ref
		if _, want := wanted[key]; !want {
			continue
		}
		p.CreatedAt = time.Unix(c, 0).UTC()
		p.TouchedAt = time.Unix(t, 0).UTC()
		out[key] = append(out[key], p)
	}
	return out, rows.Err()
}
