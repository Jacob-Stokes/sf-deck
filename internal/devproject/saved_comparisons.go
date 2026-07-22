package devproject

// Saved metadata comparisons — a persisted org-to-org compare RESULT.
//
// Unlike a comparison TEMPLATE (just source/target/scope/method, stored
// in settings.toml), a saved comparison carries the full retrieved
// snapshots + inventory as an opaque gzipped blob, so the user can
// reopen it offline and act on the diff list after a restart.
//
// The store treats the payload as opaque bytes (Blob) — the UI layer
// owns the JSON shape (snapshots + inventory), keeping internal/devproject
// free of any internal/diff dependency. Lives in devprojects.db (not
// cache.db) so the cache-clear action never destroys saved comparisons.
//
// Identifiers: "cmp_" + base32-of-12-random-bytes.

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrSavedComparisonNotFound is returned for an unknown id.
	ErrSavedComparisonNotFound = errors.New("devproject: saved comparison not found")
	// ErrSavedComparisonEmpty is returned when name is blank after trim.
	ErrSavedComparisonEmpty = errors.New("devproject: saved comparison name is required")
)

// SavedComparison is one persisted comparison result. Blob is the
// opaque (gzipped JSON) payload the UI serializes; the store neither
// inspects nor validates it. The scalar columns drive the Saved-subtab
// list without unzipping the blob.
type SavedComparison struct {
	ID        string
	Name      string
	Source    string
	Target    string
	Scope     string // comma-joined type labels
	Method    string
	Blob      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSavedComparisonID mints a fresh "cmp_..." id.
func NewSavedComparisonID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "cmp_" + strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b[:]), "="))
}

// SaveComparison inserts a new saved comparison and returns it with its
// assigned id + timestamps. Name is trimmed; blank → ErrSavedComparisonEmpty.
func (s *Store) SaveComparison(c SavedComparison) (SavedComparison, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return SavedComparison{}, ErrSavedComparisonEmpty
	}
	now := time.Now()
	c.ID = NewSavedComparisonID()
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO saved_comparisons (id, name, source, target, scope, method, blob_gz, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Source, c.Target, c.Scope, c.Method, c.Blob,
		c.CreatedAt.Unix(), c.UpdatedAt.Unix())
	if err != nil {
		return SavedComparison{}, fmt.Errorf("insert saved_comparison: %w", err)
	}
	return c, nil
}

// RenameSavedComparison changes only the display name.
func (s *Store) RenameSavedComparison(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrSavedComparisonEmpty
	}
	res, err := s.db.Exec(`UPDATE saved_comparisons SET name = ?, updated_at = ? WHERE id = ?`,
		name, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("rename saved_comparison: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedComparisonNotFound
	}
	return nil
}

// UpdateComparison overwrites an existing saved comparison's content
// (scope/method/blob) and bumps updated_at — used by the "overwrite
// original" save path after a rerun. Name is left unchanged (rename has
// its own path). Returns ErrSavedComparisonNotFound for unknown id.
func (s *Store) UpdateComparison(id, source, target, scope, method string, blob []byte) error {
	res, err := s.db.Exec(`
		UPDATE saved_comparisons
		   SET source = ?, target = ?, scope = ?, method = ?, blob_gz = ?, updated_at = ?
		 WHERE id = ?`,
		source, target, scope, method, blob, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update saved_comparison: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedComparisonNotFound
	}
	return nil
}

// DeleteSavedComparison removes a saved comparison (blob included).
func (s *Store) DeleteSavedComparison(id string) error {
	res, err := s.db.Exec(`DELETE FROM saved_comparisons WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete saved_comparison: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSavedComparisonNotFound
	}
	return nil
}

// GetSavedComparison returns one saved comparison INCLUDING its blob.
func (s *Store) GetSavedComparison(id string) (SavedComparison, error) {
	var c SavedComparison
	var createdSec, updatedSec int64
	err := s.db.QueryRow(`
		SELECT id, name, source, target, scope, method, blob_gz, created_at, updated_at
		  FROM saved_comparisons WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Source, &c.Target, &c.Scope, &c.Method, &c.Blob, &createdSec, &updatedSec)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedComparison{}, ErrSavedComparisonNotFound
	}
	if err != nil {
		return SavedComparison{}, err
	}
	c.CreatedAt = time.Unix(createdSec, 0)
	c.UpdatedAt = time.Unix(updatedSec, 0)
	return c, nil
}

// ListSavedComparisons returns metadata for every saved comparison
// (most-recent first), WITHOUT the blob — the list view only needs the
// scalar columns, and blobs can be large. Use GetSavedComparison to
// load one's blob on open.
func (s *Store) ListSavedComparisons() ([]SavedComparison, error) {
	rows, err := s.db.Query(`
		SELECT id, name, source, target, scope, method, created_at, updated_at
		  FROM saved_comparisons ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedComparison
	for rows.Next() {
		var c SavedComparison
		var createdSec, updatedSec int64
		if err := rows.Scan(&c.ID, &c.Name, &c.Source, &c.Target, &c.Scope, &c.Method, &createdSec, &updatedSec); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(createdSec, 0)
		c.UpdatedAt = time.Unix(updatedSec, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}
