package devproject

// Saved anonymous-Apex snippets — user-authored, org-agnostic,
// taggable + pinnable to DevProjects. Same shape as saved_queries
// (deliberately) so the /exec Library subtab can mirror /soql's
// Library without per-table special cases.
//
// Identifiers: "ax_" + base32-of-12-random-bytes. Short, sortable
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

// ErrSavedApexNotFound is returned when a lookup references an
// unknown saved-apex id.
var ErrSavedApexNotFound = errors.New("devproject: saved apex not found")

// ErrSavedApexEmpty is returned when CreateSavedApex / UpdateSavedApex
// gets a name or body that's blank after trimming.
var ErrSavedApexEmpty = errors.New("devproject: saved apex name and body are required")

// SavedApex is one user-authored anonymous-Apex snippet. Mirrors
// SavedQuery so the Library renderer can treat them uniformly.
type SavedApex struct {
	ID          string
	Name        string
	Description string
	Body        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Field implements query.Row so chip predicates can filter the
// Saved subtab list. Mirrors SavedQuery.Field.
func (a SavedApex) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return a.ID, true
	case "Name":
		return a.Name, true
	case "Description":
		return a.Description, true
	case "Body":
		return a.Body, true
	case "HasDescription":
		return a.Description != "", true
	case "CreatedAt":
		return a.CreatedAt, true
	case "UpdatedAt":
		return a.UpdatedAt, true
	}
	return nil, false
}

// NewSavedApexID mints a fresh "ax_..." id.
func NewSavedApexID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "ax_" + strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b[:]), "="))
}

// CreateSavedApex inserts a new snippet and returns it with its
// assigned id + timestamps.
func (s *Store) CreateSavedApex(name, description, body string) (SavedApex, error) {
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	if name == "" || body == "" {
		return SavedApex{}, ErrSavedApexEmpty
	}
	now := time.Now()
	a := SavedApex{
		ID:          NewSavedApexID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		Body:        body,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.Exec(`
		INSERT INTO saved_apex (id, name, description, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Description, a.Body,
		a.CreatedAt.Unix(), a.UpdatedAt.Unix())
	if err != nil {
		return SavedApex{}, fmt.Errorf("insert saved_apex: %w", err)
	}
	return a, nil
}

// UpdateSavedApex overwrites the editable fields of an existing
// snippet and bumps updated_at.
func (s *Store) UpdateSavedApex(id, name, description, body string) error {
	name = strings.TrimSpace(name)
	body = strings.TrimSpace(body)
	if name == "" || body == "" {
		return ErrSavedApexEmpty
	}
	res, err := s.db.Exec(`
		UPDATE saved_apex
		   SET name = ?, description = ?, body = ?, updated_at = ?
		 WHERE id = ?`,
		name, strings.TrimSpace(description), body, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update saved_apex: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedApexNotFound
	}
	return nil
}

// DeleteSavedApex removes the snippet and cascades through the
// tag_bindings + dev_project_items tables.
func (s *Store) DeleteSavedApex(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM saved_apex WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete saved_apex: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedApexNotFound
	}
	if _, err := tx.Exec(`DELETE FROM tag_bindings WHERE item_kind = ? AND item_ref = ?`,
		string(KindApexSnippet), id); err != nil {
		return fmt.Errorf("cascade tag_bindings: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM dev_project_items WHERE kind = ? AND ref = ?`,
		string(KindApexSnippet), id); err != nil {
		return fmt.Errorf("cascade dev_project_items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.touch()
	return nil
}

// GetSavedApex returns one snippet by id.
func (s *Store) GetSavedApex(id string) (SavedApex, error) {
	var a SavedApex
	var createdSec, updatedSec int64
	err := s.db.QueryRow(`
		SELECT id, name, description, body, created_at, updated_at
		  FROM saved_apex
		 WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Description, &a.Body, &createdSec, &updatedSec)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedApex{}, ErrSavedApexNotFound
	}
	if err != nil {
		return SavedApex{}, err
	}
	a.CreatedAt = time.Unix(createdSec, 0)
	a.UpdatedAt = time.Unix(updatedSec, 0)
	return a, nil
}

// ListSavedApex returns every snippet ordered by most-recently-updated.
func (s *Store) ListSavedApex() ([]SavedApex, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, body, created_at, updated_at
		  FROM saved_apex
		 ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedApex
	for rows.Next() {
		var a SavedApex
		var createdSec, updatedSec int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Body, &createdSec, &updatedSec); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(createdSec, 0)
		a.UpdatedAt = time.Unix(updatedSec, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}

// TouchSavedApex bumps updated_at without changing content.
func (s *Store) TouchSavedApex(id string) error {
	res, err := s.db.Exec(`UPDATE saved_apex SET updated_at = ? WHERE id = ?`,
		time.Now().Unix(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSavedApexNotFound
	}
	return nil
}
