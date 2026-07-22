package devproject

// Saved SOQL queries — user-authored, org-agnostic, taggable, and
// pinnable to DevProjects. The data model deliberately reuses the
// tag_bindings + dev_project_items tables with kind='soql_query',
// so all the existing tag and project machinery works for queries
// without per-kind shims.
//
// Why SQLite (rather than ~/.sf-deck/queries/*.toml):
//   - Tag bindings reference items by (kind, ref); cross-table
//     joins work only if queries live in the same store.
//   - DevProject items are SQL-rooted; pinning a query is the
//     same row shape as pinning any other Kind.
//   - One persistence paradigm rather than two.
//   - History needs SQL anyway; queries piggyback on the same
//     transactions.
//
// Identifiers: "sq_" + base32-of-12-random-bytes. Short, sortable
// roughly by creation order, easy to recognise as ours in logs.

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSavedQueryNotFound is returned when a lookup references an
// unknown saved-query id.
var ErrSavedQueryNotFound = errors.New("devproject: saved query not found")

// ErrSavedQueryEmpty is returned when CreateSavedQuery / UpdateSavedQuery
// gets a name or body that's blank after trimming. We refuse to store
// these because they'd render as invisible rows in the Library view.
var ErrSavedQueryEmpty = errors.New("devproject: saved query name and body are required")

// SavedQuery is one user-authored SOQL query.
//
// Description is freeform — typically one paragraph. Body is the
// raw SOQL with $ME / $ORG / $TODAY tokens preserved (substitution
// happens at load time, not at save time, so the on-disk form stays
// portable across orgs and stable as the resolved values change).
type SavedQuery struct {
	ID          string
	Name        string
	Description string
	Body        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Field implements query.Row so chip predicates can filter the
// saved-queries list. Mirrors the column shape rendered in the
// Saved subtab + a couple of derived fields ("HasDescription",
// "HasTags") that user-authored chips can predicate against.
//
// "HasTags" / "PinnedToProject" are derived state and require a
// store lookup, which the chip predicate engine can't do here —
// they're left out for now and surfaced via dedicated builtin
// chips that the renderer pre-filters before evaluation. See the
// Saved subtab's chip strip in tab_soql_library.go.
func (q SavedQuery) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return q.ID, true
	case "Name":
		return q.Name, true
	case "Description":
		return q.Description, true
	case "Body":
		return q.Body, true
	case "HasDescription":
		return q.Description != "", true
	case "CreatedAt":
		return q.CreatedAt, true
	case "UpdatedAt":
		return q.UpdatedAt, true
	}
	return nil, false
}

// NewSavedQueryID mints a fresh "sq_..." id. Exported so callers
// that want to construct a SavedQuery before insert (e.g. import
// flows) can stamp it without round-tripping through the store.
func NewSavedQueryID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "sq_" + strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b[:]), "="))
}

// CreateSavedQuery inserts a new saved query and returns it with
// its assigned id + timestamps. Name and body are trimmed; an empty
// trimmed value yields ErrSavedQueryEmpty.
func (s *Store) CreateSavedQuery(name, description, body string) (SavedQuery, error) {
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	if name == "" || body == "" {
		return SavedQuery{}, ErrSavedQueryEmpty
	}
	now := time.Now()
	q := SavedQuery{
		ID:          NewSavedQueryID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		Body:        body,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.Exec(`
		INSERT INTO saved_queries (id, name, description, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		q.ID, q.Name, q.Description, q.Body,
		q.CreatedAt.Unix(), q.UpdatedAt.Unix())
	if err != nil {
		return SavedQuery{}, fmt.Errorf("insert saved_query: %w", err)
	}
	return q, nil
}

// UpdateSavedQuery overwrites the editable fields of an existing
// saved query and bumps updated_at. Returns ErrSavedQueryNotFound
// if id doesn't exist; ErrSavedQueryEmpty if name/body are blank.
func (s *Store) UpdateSavedQuery(id, name, description, body string) error {
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	if name == "" || body == "" {
		return ErrSavedQueryEmpty
	}
	res, err := s.db.Exec(`
		UPDATE saved_queries
		   SET name = ?, description = ?, body = ?, updated_at = ?
		 WHERE id = ?`,
		name, strings.TrimSpace(description), body, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update saved_query: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedQueryNotFound
	}
	return nil
}

// DeleteSavedQuery removes the saved query AND cascades through the
// tag_bindings + dev_project_items tables so dangling tag/project
// references don't survive the delete. Returns ErrSavedQueryNotFound
// when id doesn't exist (so callers can disambiguate "deleted ok"
// from "tried to delete a ghost").
func (s *Store) DeleteSavedQuery(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM saved_queries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete saved_query: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedQueryNotFound
	}
	if _, err := tx.Exec(`DELETE FROM tag_bindings WHERE item_kind = ? AND item_ref = ?`,
		string(KindSOQLQuery), id); err != nil {
		return fmt.Errorf("cascade tag_bindings: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM dev_project_items WHERE kind = ? AND ref = ?`,
		string(KindSOQLQuery), id); err != nil {
		return fmt.Errorf("cascade dev_project_items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Cascaded into tag_bindings + dev_project_items, so gutter
	// caches must invalidate.
	s.touch()
	return nil
}

// GetSavedQuery returns one saved query by id.
func (s *Store) GetSavedQuery(id string) (SavedQuery, error) {
	var q SavedQuery
	var createdSec, updatedSec int64
	err := s.db.QueryRow(`
		SELECT id, name, description, body, created_at, updated_at
		  FROM saved_queries
		 WHERE id = ?`, id).
		Scan(&q.ID, &q.Name, &q.Description, &q.Body, &createdSec, &updatedSec)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedQuery{}, ErrSavedQueryNotFound
	}
	if err != nil {
		return SavedQuery{}, err
	}
	q.CreatedAt = time.Unix(createdSec, 0)
	q.UpdatedAt = time.Unix(updatedSec, 0)
	return q, nil
}

// ListSavedQueries returns every saved query ordered by most-recent
// updated first. The whole list is small (typical user has dozens,
// power user maybe low hundreds) so we don't paginate.
func (s *Store) ListSavedQueries() ([]SavedQuery, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, body, created_at, updated_at
		  FROM saved_queries
		 ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedQuery
	for rows.Next() {
		var q SavedQuery
		var createdSec, updatedSec int64
		if err := rows.Scan(&q.ID, &q.Name, &q.Description, &q.Body, &createdSec, &updatedSec); err != nil {
			return nil, err
		}
		q.CreatedAt = time.Unix(createdSec, 0)
		q.UpdatedAt = time.Unix(updatedSec, 0)
		out = append(out, q)
	}
	return out, rows.Err()
}

// TouchSavedQuery bumps updated_at without changing content. Used
// when a query is run from the Library so "most recent" ordering
// reflects "most recently used," not just "most recently edited."
func (s *Store) TouchSavedQuery(id string) error {
	res, err := s.db.Exec(`UPDATE saved_queries SET updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedQueryNotFound
	}
	return nil
}
