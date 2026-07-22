package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Cache struct {
	db   *sql.DB
	path string
}

// Open returns a Cache backed by ~/.sf-deck/cache.db. If an older
// ~/.salesforce-deck/ directory exists (from the pre-rename era) and
// ~/.sf-deck/ does not, we transparently rename it — existing caches
// are preserved across the rename.
func Open() (*Cache, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return OpenPath(filepath.Join(dir, "cache.db"))
}

// OpenPath opens (creating if needed) a cache at an explicit path.
// Used by `--demo` for its throwaway seeded cache and by tests.
func OpenPath(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := configureDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	// sqlite creates the db file 0644; tighten to owner-only. The cache
	// holds queried org data + list snapshots that shouldn't be readable
	// by other local users. Best-effort — the 0700 base dir is the
	// primary guard, this is defense in depth.
	_ = os.Chmod(path, 0o600)
	migrateOrgsTable(db)
	return &Cache{db: db, path: path}, nil
}

// migrateOrgsTable applies additive ALTER TABLE migrations to the
// orgs table. Best-effort: "duplicate column name" on an already-
// migrated db is the expected steady state, and a failed migration
// only degrades the new columns (PutOrgs would then error loudly).
func migrateOrgsTable(db *sql.DB) {
	for _, stmt := range []string{
		`ALTER TABLE orgs ADD COLUMN expiration_date TEXT DEFAULT ''`,
		`ALTER TABLE orgs ADD COLUMN is_default INTEGER DEFAULT 0`,
		`ALTER TABLE orgs ADD COLUMN is_default_devhub INTEGER DEFAULT 0`,
	} {
		_, _ = db.Exec(stmt)
	}
}

func (c *Cache) Close() error { return c.db.Close() }
func (c *Cache) Path() string { return c.path }

func configureDB(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err := db.Exec(`PRAGMA busy_timeout = 5000`)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS orgs (
  username     TEXT PRIMARY KEY,
  alias        TEXT,
  instance_url TEXT,
  org_id       TEXT,
  is_sandbox   INTEGER,
  is_scratch   INTEGER,
  is_devhub    INTEGER,
  status       TEXT,
  last_used    TEXT,
  expiration_date   TEXT DEFAULT '',
  is_default        INTEGER DEFAULT 0,
  is_default_devhub INTEGER DEFAULT 0,
  cached_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS kv (
  org_username TEXT NOT NULL,
  key          TEXT NOT NULL,
  value        TEXT NOT NULL,
  cached_at    INTEGER NOT NULL,
  PRIMARY KEY (org_username, key)
);
`

// --- org list cache -------------------------------------------------------

type OrgRow struct {
	Username    string
	Alias       string
	InstanceURL string
	OrgID       string
	IsSandbox   bool
	IsScratch   bool
	IsDevHub    bool
	Status      string
	LastUsed    string
	// ExpirationDate (scratch only) + the sf CLI default markers —
	// see sf.Org for semantics.
	ExpirationDate  string
	IsDefault       bool
	IsDefaultDevHub bool
	CachedAt        time.Time
}

func (c *Cache) PutOrgs(orgs []OrgRow) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM orgs"); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO orgs
		(username, alias, instance_url, org_id, is_sandbox, is_scratch, is_devhub, status, last_used,
		 expiration_date, is_default, is_default_devhub, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, o := range orgs {
		if _, err := stmt.Exec(
			o.Username, o.Alias, o.InstanceURL, o.OrgID,
			boolToInt(o.IsSandbox), boolToInt(o.IsScratch), boolToInt(o.IsDevHub),
			o.Status, o.LastUsed,
			o.ExpirationDate, boolToInt(o.IsDefault), boolToInt(o.IsDefaultDevHub), now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetOrgs returns all cached orgs and the newest cached_at timestamp
// (zero time if the table is empty).
func (c *Cache) GetOrgs() ([]OrgRow, time.Time, error) {
	rows, err := c.db.Query(`SELECT username, alias, instance_url, org_id,
		is_sandbox, is_scratch, is_devhub, status, last_used,
		expiration_date, is_default, is_default_devhub, cached_at
		FROM orgs ORDER BY last_used DESC`)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	var out []OrgRow
	var newest int64
	for rows.Next() {
		var o OrgRow
		var isSandbox, isScratch, isDevHub, isDefault, isDefaultHub int
		var cachedAt int64
		if err := rows.Scan(&o.Username, &o.Alias, &o.InstanceURL, &o.OrgID,
			&isSandbox, &isScratch, &isDevHub,
			&o.Status, &o.LastUsed,
			&o.ExpirationDate, &isDefault, &isDefaultHub, &cachedAt); err != nil {
			return nil, time.Time{}, err
		}
		o.IsSandbox = isSandbox != 0
		o.IsScratch = isScratch != 0
		o.IsDevHub = isDevHub != 0
		o.IsDefault = isDefault != 0
		o.IsDefaultDevHub = isDefaultHub != 0
		o.CachedAt = time.Unix(cachedAt, 0)
		if cachedAt > newest {
			newest = cachedAt
		}
		out = append(out, o)
	}
	var t time.Time
	if newest > 0 {
		t = time.Unix(newest, 0)
	}
	return out, t, rows.Err()
}

// --- generic per-org JSON kv ---------------------------------------------
//
// Use for anything else we want to cache per org — sobject lists, field
// descriptions, counts, recent deploys. Keeps the schema thin while we're
// still figuring out what's worth storing relationally.

func (c *Cache) PutJSON(orgUsername, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`INSERT INTO kv (org_username, key, value, cached_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(org_username, key) DO UPDATE SET value=excluded.value, cached_at=excluded.cached_at`,
		orgUsername, key, string(b), time.Now().Unix())
	return err
}

func (c *Cache) GetJSON(orgUsername, key string, dst any) (time.Time, bool, error) {
	var value string
	var cachedAt int64
	err := c.db.QueryRow(
		`SELECT value, cached_at FROM kv WHERE org_username = ? AND key = ?`,
		orgUsername, key).Scan(&value, &cachedAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if err := json.Unmarshal([]byte(value), dst); err != nil {
		return time.Time{}, false, fmt.Errorf("unmarshal %s/%s: %w", orgUsername, key, err)
	}
	return time.Unix(cachedAt, 0), true, nil
}

// DeleteKeyPrefix deletes every kv row where key starts with the given
// prefix. Used at startup to purge caches that should never have been
// persisted (e.g. legacy record data before we marked it NoCache).
// DeleteScope removes every cached payload for one org (all kv rows with
// the given org_username). Used to purge an injected demo org's cache
// namespace on removal — its rows are cleanly keyed by the demo username,
// so this touches nothing belonging to a real org.
func (c *Cache) DeleteScope(orgUsername string) (int, error) {
	res, err := c.db.Exec(`DELETE FROM kv WHERE org_username = ?`, orgUsername)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (c *Cache) DeleteKeyPrefix(prefix string) (int, error) {
	res, err := c.db.Exec(`DELETE FROM kv WHERE key LIKE ? || '%'`, prefix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ClearAll empties the response cache (the kv table — every cached API
// payload across all orgs, which is the bulk of cache.db) and VACUUMs
// so the freed pages are returned to the filesystem rather than left as
// free-list slack. Returns the number of kv rows deleted.
//
// The orgs table is deliberately left intact: it's the org directory
// the running UI holds live references into, it re-pulls on its own
// short TTL, and it's tiny. "Clear cache" means "drop the fetched data
// so everything re-fetches", not "forget which orgs exist".
func (c *Cache) ClearAll() (int, error) {
	res, err := c.db.Exec(`DELETE FROM kv`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	// VACUUM can't run inside a transaction; db has one open conn so this
	// is safe to issue directly. Reclaims the on-disk space the deleted
	// blobs occupied (otherwise the file stays ~37MB of free pages).
	if _, err := c.db.Exec(`VACUUM`); err != nil {
		// Rows are already gone; a failed VACUUM only means the file
		// didn't shrink. Report it but don't pretend nothing cleared.
		return int(n), fmt.Errorf("cache cleared (%d rows) but VACUUM failed: %w", n, err)
	}
	return int(n), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	newDir := filepath.Join(home, ".sf-deck")
	oldDir := filepath.Join(home, ".salesforce-deck")
	// One-time migration from the pre-rename directory. Best-effort: if
	// the rename fails (e.g. permissions), we fall through and the new
	// dir gets created fresh, which is also fine — users lose the cache
	// but not any real data.
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		if _, err := os.Stat(oldDir); err == nil {
			_ = os.Rename(oldDir, newDir)
		}
	}
	return newDir, nil
}
