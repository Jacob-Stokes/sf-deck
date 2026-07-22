package devproject

// Tags — personal annotation layer over metadata + records.
//
// Tags are orthogonal to projects: a project is "what ships together,"
// a tag is "what I'm thinking about." A single field can carry several
// tags AND belong to several projects at once. Tags are user-scoped
// (one namespace shared across orgs), bindings are per-(item, org).
//
// Schema lives in store.go's `schema` constant (tags + tag_bindings
// tables). This file is just the API surface.
//
// Two-table layout rationale:
//   - `tags`         = the definition (name, color, icon). One row per
//                      tag, ever. Renaming = update; deleting cascades
//                      all bindings via FK ON DELETE CASCADE.
//   - `tag_bindings` = (tag, item_kind, item_ref, org_user). One row
//                      per item-tag pair. Querying "all items tagged
//                      X" is a join; querying "all tags on item Y" is
//                      a join the other direction. Both are indexed.
//
// Cross-org behavior: the same tag id binds to different items across
// orgs. The tag definition lives once; the binding rows multiply per
// org. This is the right shape for "tag `account-merge` means the
// same conceptual thing across all 6 client orgs even though it
// resolves to different concrete flow Ids in each."

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTagExists is returned when CreateTag or RenameTag would collide
// with an existing tag name. Names are unique case-insensitive.
var ErrTagExists = errors.New("devproject: tag with that name exists")

// ErrTagNotFound is returned when an operation references a tag id
// that doesn't exist (deleted, or never created). UI surfaces this as
// "tag is gone" rather than a generic error.
var ErrTagNotFound = errors.New("devproject: tag not found")

// Tag is one entry in the global tag namespace.
//
// Color is a theme color name ("blue", "purple", etc.) rather than a
// hex string so tags re-theme automatically when the user switches
// themes. Empty string = use a default fallback color.
//
// Icon is an optional unicode glyph (single grapheme, typically an
// emoji) prepended to the tag name in pills + chips. Empty string =
// no icon prefix. Stored as a string because graphemes can be
// multiple bytes (emoji + variation selectors etc.).
type Tag struct {
	ID        int64
	Name      string
	Color     string
	Icon      string
	CreatedAt time.Time
}

// Binding is one (tag, item) attachment. UI typically resolves
// bindings via TagsFor / ItemsWithTag rather than working with
// Bindings directly, but the type is exposed for store-level
// inspection.
type Binding struct {
	TagID     int64
	ItemKind  ItemKind
	ItemRef   string
	OrgUser   string
	CreatedAt time.Time
}

// TagUsage is Tag + how many bindings reference it. Used by the
// management view ("how many things are tagged with X?") and to
// determine whether a tag chip should appear in a surface's strip
// (zero usage = not shown).
type TagUsage struct {
	Tag
	Count int
}

// CreateTag inserts a new tag. Returns ErrTagExists if the name is
// already taken (case-insensitive). The returned Tag has its ID
// populated.
func (s *Store) CreateTag(name, color, icon string) (Tag, error) {
	if s == nil || s.db == nil {
		return Tag{}, errors.New("devproject: store closed")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Tag{}, errors.New("devproject: tag name is required")
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO tags (name, color, icon, created_at) VALUES (?, ?, ?, ?)`,
		name, color, icon, now.Unix(),
	)
	if err != nil {
		// SQLite UNIQUE constraint surfaces with a recognisable text
		// fragment; convert to our sentinel so callers can distinguish.
		if isUniqueConstraint(err) {
			return Tag{}, ErrTagExists
		}
		return Tag{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Tag{}, err
	}
	s.touch()
	return Tag{ID: id, Name: name, Color: color, Icon: icon, CreatedAt: now}, nil
}

// UpdateTag patches a tag's name / color / icon. Returns
// ErrTagExists if the new name collides with another tag, or
// ErrTagNotFound if the id doesn't exist.
func (s *Store) UpdateTag(id int64, name, color, icon string) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("devproject: tag name is required")
	}
	res, err := s.db.Exec(
		`UPDATE tags SET name = ?, color = ?, icon = ? WHERE id = ?`,
		name, color, icon, id,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return ErrTagExists
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTagNotFound
	}
	s.touch()
	return nil
}

// DeleteTag removes the tag and all bindings that reference it (via
// ON DELETE CASCADE). Idempotent — deleting a non-existent tag is a
// no-op. Returns ErrTagNotFound only if the caller cares about
// validating prior existence (we report rows-affected).
func (s *Store) DeleteTag(id int64) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	res, err := s.db.Exec(`DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.touch()
	}
	return nil
}

// ListTags returns every tag, sorted by name (case-insensitive).
// Empty slice when no tags exist.
func (s *Store) ListTags() ([]Tag, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	rows, err := s.db.Query(
		`SELECT id, name, color, icon, created_at FROM tags ORDER BY name COLLATE NOCASE`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		var ts int64
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTagsWithUsage returns tags + binding counts. Useful for the
// management view and for deciding whether to surface a tag in a
// chip strip (skip zero-usage tags so the strip stays clean).
func (s *Store) ListTagsWithUsage() ([]TagUsage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	rows, err := s.db.Query(`
		SELECT t.id, t.name, t.color, t.icon, t.created_at,
		       COALESCE(COUNT(b.tag_id), 0) AS usage
		FROM tags t
		LEFT JOIN tag_bindings b ON b.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagUsage
	for rows.Next() {
		var u TagUsage
		var ts int64
		if err := rows.Scan(&u.ID, &u.Name, &u.Color, &u.Icon, &ts, &u.Count); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, u)
	}
	return out, rows.Err()
}

// FindTagByName looks up a tag by case-insensitive name match.
// Returns (Tag{}, false) when no match — distinct from a lookup
// error, which is returned via the error.
func (s *Store) FindTagByName(name string) (Tag, bool, error) {
	if s == nil || s.db == nil {
		return Tag{}, false, errors.New("devproject: store closed")
	}
	row := s.db.QueryRow(
		`SELECT id, name, color, icon, created_at FROM tags WHERE name = ? COLLATE NOCASE`,
		strings.TrimSpace(name),
	)
	var t Tag
	var ts int64
	if err := row.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tag{}, false, nil
		}
		return Tag{}, false, err
	}
	t.CreatedAt = time.Unix(ts, 0).UTC()
	return t, true, nil
}

// ApplyTag binds a tag to an item. Idempotent — re-applying an
// existing binding is a no-op (PRIMARY KEY collision swallowed).
// Returns ErrTagNotFound if the tag id doesn't exist.
func (s *Store) ApplyTag(tagID int64, kind ItemKind, ref, orgUser string) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	if ref == "" || kind == "" {
		return errors.New("devproject: ApplyTag requires kind + ref")
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO tag_bindings (tag_id, item_kind, item_ref, org_user, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		tagID, string(kind), ref, orgUser, time.Now().UTC().Unix(),
	)
	if err != nil {
		// Foreign-key violation = tag doesn't exist. SQLite reports
		// this as "FOREIGN KEY constraint failed" so we map it to a
		// recognisable sentinel for the UI.
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return ErrTagNotFound
		}
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.touch()
	}
	return nil
}

// RemoveTag unbinds a tag from an item. Idempotent — removing a
// non-existent binding is a no-op.
func (s *Store) RemoveTag(tagID int64, kind ItemKind, ref, orgUser string) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	res, err := s.db.Exec(
		`DELETE FROM tag_bindings
		 WHERE tag_id = ? AND item_kind = ? AND item_ref = ? AND org_user = ?`,
		tagID, string(kind), ref, orgUser,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.touch()
	}
	return nil
}

// BoundItem is one distinct (kind, ref, org) that has at least one tag
// binding — the unit reconcile scans for staleness / ref-normalisation.
type BoundItem struct {
	Kind    ItemKind
	Ref     string
	OrgUser string
}

// ListBoundItems returns every distinct item that carries a tag,
// regardless of which tag. Used by the reconcile pass to detect stale
// bindings (resource deleted in the org) and non-canonical refs.
func (s *Store) ListBoundItems() ([]BoundItem, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	rows, err := s.db.Query(
		`SELECT DISTINCT item_kind, item_ref, org_user FROM tag_bindings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BoundItem
	for rows.Next() {
		var b BoundItem
		var kind string
		if err := rows.Scan(&kind, &b.Ref, &b.OrgUser); err != nil {
			return nil, err
		}
		b.Kind = ItemKind(kind)
		out = append(out, b)
	}
	return out, rows.Err()
}

// TagBindingDelete removes ALL tag bindings for one item (its resource
// no longer exists). Shape mirrors ItemDelete.
type TagBindingDelete struct {
	Kind    ItemKind
	Ref     string
	OrgUser string
}

// TagBindingRewrite re-keys every binding on an item from a non-
// canonical ref to the canonical one (e.g. a flow DeveloperName ->
// DefinitionId). If the target ref already has bindings, the two tag
// sets are merged (INSERT OR IGNORE) and the source rows dropped.
type TagBindingRewrite struct {
	Kind    ItemKind
	OrgUser string
	FromRef string
	ToRef   string
}

// ReconcileTagBindings applies a batch of stale-binding deletes + ref
// rewrites in one transaction. Mirrors Store.ApplyReconcile for dev-
// project items. No-op (no write, no touch) when both lists are empty,
// so it's safe on every project touch. Returns rows removed + merged.
func (s *Store) ReconcileTagBindings(deletes []TagBindingDelete, rewrites []TagBindingRewrite) (removed, merged int, err error) {
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

	for _, d := range deletes {
		res, e := tx.Exec(
			`DELETE FROM tag_bindings WHERE item_kind=? AND item_ref=? AND org_user=?`,
			string(d.Kind), d.Ref, d.OrgUser)
		if e != nil {
			err = e
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed += int(n)
		}
	}

	for _, rw := range rewrites {
		if rw.ToRef == "" || rw.ToRef == rw.FromRef {
			continue
		}
		// Merge source tag ids into the target ref (ignore dup PKs),
		// then drop the source rows. Handles both "target has no
		// bindings" (pure move) and "target already tagged" (merge).
		if _, e := tx.Exec(
			`INSERT OR IGNORE INTO tag_bindings (tag_id, item_kind, item_ref, org_user, created_at)
			 SELECT tag_id, item_kind, ?, org_user, created_at
			 FROM tag_bindings WHERE item_kind=? AND item_ref=? AND org_user=?`,
			rw.ToRef, string(rw.Kind), rw.FromRef, rw.OrgUser); e != nil {
			err = e
			return 0, 0, err
		}
		res, e := tx.Exec(
			`DELETE FROM tag_bindings WHERE item_kind=? AND item_ref=? AND org_user=?`,
			string(rw.Kind), rw.FromRef, rw.OrgUser)
		if e != nil {
			err = e
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			merged += int(n)
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

// SetTagsFor replaces the full tag set on an item to exactly the
// given tag ids. Convenience for the tag-picker modal which presents
// a multi-select and commits the diff in one shot.
//
// Wrapped in a transaction so a partial failure doesn't leave the
// item in a half-applied state. Idempotent.
func (s *Store) SetTagsFor(kind ItemKind, ref, orgUser string, tagIDs []int64) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	if ref == "" || kind == "" {
		return errors.New("devproject: SetTagsFor requires kind + ref")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`DELETE FROM tag_bindings
		 WHERE item_kind = ? AND item_ref = ? AND org_user = ?`,
		string(kind), ref, orgUser,
	); err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	for _, id := range tagIDs {
		if _, err := tx.Exec(
			`INSERT INTO tag_bindings (tag_id, item_kind, item_ref, org_user, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			id, string(kind), ref, orgUser, now,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.touch()
	return nil
}

// TagsFor returns every tag bound to a single item. Empty slice when
// untagged. The slice is sorted by tag name for stable rendering.
func (s *Store) TagsFor(kind ItemKind, ref, orgUser string) ([]Tag, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	rows, err := s.db.Query(`
		SELECT t.id, t.name, t.color, t.icon, t.created_at
		FROM tags t
		JOIN tag_bindings b ON b.tag_id = t.id
		WHERE b.item_kind = ? AND b.item_ref = ? AND b.org_user = ?
		ORDER BY t.name COLLATE NOCASE
	`, string(kind), ref, orgUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		var ts int64
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, t)
	}
	return out, rows.Err()
}

// ItemsWithTag returns every binding that carries a given tag.
// Caller filters by org if needed (most chip-strip use cases want
// "items in current org tagged X" — pass orgUser there).
//
// orgUser="" means "any org." This is the right default for cross-
// org tag management views.
func (s *Store) ItemsWithTag(tagID int64, orgUser string) ([]Binding, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	q := `SELECT tag_id, item_kind, item_ref, org_user, created_at
	      FROM tag_bindings WHERE tag_id = ?`
	args := []any{tagID}
	if orgUser != "" {
		q += ` AND org_user = ?`
		args = append(args, orgUser)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Binding
	for rows.Next() {
		var b Binding
		var ts int64
		var kind string
		if err := rows.Scan(&b.TagID, &kind, &b.ItemRef, &b.OrgUser, &ts); err != nil {
			return nil, err
		}
		b.ItemKind = ItemKind(kind)
		b.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, b)
	}
	return out, rows.Err()
}

// TagsForItems is the bulk variant of TagsFor — given a slice of
// (kind, ref) pairs in the same org, returns a map keyed by
// "kind:ref" → tag list. Used by list renderers that need to know
// "which of these N rows have any tags?" without N round-trips.
//
// Implementation: load every binding for the org in one query, then
// filter client-side against the wanted set. The earlier OR-chain
// approach hit SQLite's expression-tree depth limit (1000 nodes)
// on big lists like /objects which can have 1000+ sObjects. Loading
// the full bindings slice instead is cheap because the bindings
// table is small in practice (typically <1000 rows per user even
// with heavy tag use across many orgs).
func (s *Store) TagsForItems(orgUser string, items []TagLookupKey) (map[string][]Tag, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("devproject: store closed")
	}
	if len(items) == 0 {
		return map[string][]Tag{}, nil
	}
	wanted := make(map[string]struct{}, len(items))
	for _, it := range items {
		wanted[string(it.Kind)+":"+it.Ref] = struct{}{}
	}
	rows, err := s.db.Query(`
		SELECT b.item_kind, b.item_ref, t.id, t.name, t.color, t.icon, t.created_at
		FROM tag_bindings b JOIN tags t ON t.id = b.tag_id
		WHERE b.org_user = ?
		ORDER BY t.name COLLATE NOCASE
	`, orgUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Tag{}
	for rows.Next() {
		var kind, ref string
		var t Tag
		var ts int64
		if err := rows.Scan(&kind, &ref, &t.ID, &t.Name, &t.Color, &t.Icon, &ts); err != nil {
			return nil, err
		}
		key := kind + ":" + ref
		if _, want := wanted[key]; !want {
			continue
		}
		t.CreatedAt = time.Unix(ts, 0).UTC()
		out[key] = append(out[key], t)
	}
	return out, rows.Err()
}

// TagLookupKey is the (kind, ref) pair for TagsForItems. Exposed as
// a named type so callers don't pass two parallel string slices.
type TagLookupKey struct {
	Kind ItemKind
	Ref  string
}

// isUniqueConstraint pattern-matches SQLite's UNIQUE error so we can
// surface a meaningful sentinel to the UI. modernc.org/sqlite
// returns the error as a plain text message; checking the substring
// is the canonical workaround.
func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

// ensureForeignKeysOn turns on FK enforcement for this connection.
// SQLite defaults to OFF for backwards compatibility, but we depend
// on ON DELETE CASCADE for tag_bindings cleanup.
//
// Called from Open() — kept here next to the tag schema since the
// only FK in the database belongs to tag_bindings.
func ensureForeignKeysOn(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

// BulkApplyRemoveTags applies and removes tag sets across many items
// of one kind in a single transaction — the "tag everything visible"
// path. add/remove are tag ids; refs are the item refs. Removing a
// binding that doesn't exist or adding one that already does is a
// no-op per item (INSERT OR IGNORE / plain DELETE), so partially-
// tagged sets converge without errors.
func (s *Store) BulkApplyRemoveTags(kind ItemKind, orgUser string, refs []string, add, remove []int64) error {
	if s == nil || s.db == nil {
		return errors.New("devproject: store closed")
	}
	if len(refs) == 0 || (len(add) == 0 && len(remove) == 0) {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ins, err := tx.Prepare(`INSERT OR IGNORE INTO tag_bindings (tag_id, item_kind, item_ref, org_user, created_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	del, err := tx.Prepare(`DELETE FROM tag_bindings WHERE tag_id = ? AND item_kind = ? AND item_ref = ? AND org_user = ?`)
	if err != nil {
		return err
	}
	defer del.Close()
	now := time.Now().UTC().Unix()
	for _, ref := range refs {
		for _, id := range add {
			if _, err := ins.Exec(id, string(kind), ref, orgUser, now); err != nil {
				return err
			}
		}
		for _, id := range remove {
			if _, err := del.Exec(id, string(kind), ref, orgUser); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.touch()
	return nil
}
