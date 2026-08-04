package devproject

// Notes — one free-text note per (kind, ref, org) item. Keyed by the
// same identity triple as tag_bindings, so every kind FromOpenable can
// address is notable, and notes need no project membership. The UI
// shows the note in the sidebar for the cursored item and edits it via
// the q-n chord.

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// NoteFor returns the note body for an item, or "" when the item has
// no note. A missing row is not an error.
func (s *Store) NoteFor(kind ItemKind, ref, orgUser string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("devproject: store closed")
	}
	var body string
	err := s.db.QueryRow(`
		SELECT body FROM notes
		WHERE item_kind = ? AND item_ref = ? AND org_user = ?
	`, string(kind), ref, orgUser).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// SetNote upserts the item's note. A blank body (empty or whitespace-
// only) deletes the note instead — "save empty to remove" is the one
// removal gesture, so there's no separate delete API. Bumps the store
// generation either way so render-side memos invalidate.
func (s *Store) SetNote(kind ItemKind, ref, orgUser, body string) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	if strings.TrimSpace(body) == "" {
		_, err := s.db.Exec(`
			DELETE FROM notes
			WHERE item_kind = ? AND item_ref = ? AND org_user = ?
		`, string(kind), ref, orgUser)
		if err == nil {
			s.touch()
		}
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO notes (item_kind, item_ref, org_user, body, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(item_kind, item_ref, org_user)
		DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at
	`, string(kind), ref, orgUser, body, time.Now().Unix())
	if err == nil {
		s.touch()
	}
	return err
}
