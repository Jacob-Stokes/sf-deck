package devproject

// Bundles — on-disk sfdx project directories tied to a DevProject.
//
// Persistent counterpart to the transient bundle directories the
// `e` → "Full sfdx project + retrieve" export creates today. Once a
// DevProject has one or more linked bundles, the user can:
//
//   - Re-retrieve into the same dir (preserving git history)
//   - Deploy back to the org
//   - See a diff of org-vs-bundle via sf project retrieve preview
//   - Reveal in Finder / open in VS Code
//
// One-to-many: each Bundle belongs to exactly one DevProject; a
// DevProject can have any number of bundles (different orgs,
// different snapshots over time, etc.).
//
// The data is intentionally thin — we don't track the manifest's
// contents in SQLite. Source of truth for "what's in this bundle" is
// always package.xml on disk; SQLite just remembers the path + a few
// timestamps so the UI can render the bundle list without re-reading
// every package.xml on every paint.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Bundle is one on-disk sfdx project directory linked to a DevProject.
//
// Path is absolute. Stale bundles (the directory was moved/deleted
// out from under us) keep their row in SQLite — the UI flags them as
// stale rather than auto-deleting; the user might still want to
// remember the path.
type Bundle struct {
	ID              string
	DevProjectID    string
	Path            string
	DefaultOrgAlias string
	CreatedAt       time.Time
	LastRetrievedAt time.Time // zero value = never
	LastDeployedAt  time.Time // zero value = never
}

// Stale reports whether the bundle's on-disk directory still exists
// AND looks like a sfdx project (sfdx-project.json present). Returns
// true when either is missing — the UI displays stale bundles in a
// dim style with a "relocate / unlink" hint.
//
// Network-free; just stat()s. Cheap to call on every render.
func (b Bundle) Stale() bool {
	if b.Path == "" {
		return true
	}
	if info, err := os.Stat(b.Path); err != nil || !info.IsDir() {
		return true
	}
	manifestStat, err := os.Stat(filepath.Join(b.Path, "sfdx-project.json"))
	if err != nil || manifestStat.IsDir() {
		return true
	}
	return false
}

// CreateBundle inserts a new bundle row, returning the populated
// struct (with ID + CreatedAt set). devProjectID must be a valid
// existing DevProject — the FK constraint will fail otherwise.
//
// path is normalized to absolute. defaultOrgAlias may be "" when
// unknown (the UI then prompts on first retrieve/deploy).
func (s *Store) CreateBundle(devProjectID, path, defaultOrgAlias string) (Bundle, error) {
	if s == nil || s.db == nil {
		return Bundle{}, fmt.Errorf("devproject: store closed")
	}
	if devProjectID == "" {
		return Bundle{}, fmt.Errorf("devproject: bundle requires a dev project id")
	}
	if path == "" {
		return Bundle{}, fmt.Errorf("devproject: bundle requires a path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Bundle{}, fmt.Errorf("normalize path: %w", err)
	}
	id := newBundleID()
	now := time.Now()
	_, err = s.db.Exec(`INSERT INTO bundles
		(id, dev_project_id, path, default_org_alias, created_at, last_retrieved_at, last_deployed_at)
		VALUES (?, ?, ?, ?, ?, 0, 0)`,
		id, devProjectID, abs, defaultOrgAlias, now.Unix())
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{
		ID:              id,
		DevProjectID:    devProjectID,
		Path:            abs,
		DefaultOrgAlias: defaultOrgAlias,
		CreatedAt:       now,
	}, nil
}

// ListBundlesFor returns all bundles linked to the given DevProject,
// most-recently-touched first (max of last retrieve/deploy timestamps,
// falling back to created_at when neither has fired). Empty slice
// when the project has no bundles.
func (s *Store) ListBundlesFor(devProjectID string) ([]Bundle, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("devproject: store closed")
	}
	rows, err := s.db.Query(`SELECT id, dev_project_id, path, default_org_alias,
		created_at, last_retrieved_at, last_deployed_at
		FROM bundles WHERE dev_project_id = ?
		ORDER BY MAX(last_retrieved_at, last_deployed_at, created_at) DESC`,
		devProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bundle
	for rows.Next() {
		var b Bundle
		var created, retrieved, deployed int64
		if err := rows.Scan(&b.ID, &b.DevProjectID, &b.Path, &b.DefaultOrgAlias,
			&created, &retrieved, &deployed); err != nil {
			return nil, err
		}
		b.CreatedAt = time.Unix(created, 0)
		if retrieved > 0 {
			b.LastRetrievedAt = time.Unix(retrieved, 0)
		}
		if deployed > 0 {
			b.LastDeployedAt = time.Unix(deployed, 0)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListAllBundles returns every bundle in the store across all
// DevProjects, most-recently-touched first. Used by the top-level
// /dev-projects → Bundles subtab so users can scan their on-disk
// artifacts without drilling into each project.
func (s *Store) ListAllBundles() ([]Bundle, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("devproject: store closed")
	}
	rows, err := s.db.Query(`SELECT id, dev_project_id, path, default_org_alias,
		created_at, last_retrieved_at, last_deployed_at
		FROM bundles
		ORDER BY MAX(last_retrieved_at, last_deployed_at, created_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bundle
	for rows.Next() {
		var b Bundle
		var created, retrieved, deployed int64
		if err := rows.Scan(&b.ID, &b.DevProjectID, &b.Path, &b.DefaultOrgAlias,
			&created, &retrieved, &deployed); err != nil {
			return nil, err
		}
		b.CreatedAt = time.Unix(created, 0)
		if retrieved > 0 {
			b.LastRetrievedAt = time.Unix(retrieved, 0)
		}
		if deployed > 0 {
			b.LastDeployedAt = time.Unix(deployed, 0)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBundle returns a single bundle by ID, or sql.ErrNoRows when
// missing.
func (s *Store) GetBundle(id string) (Bundle, error) {
	if s == nil || s.db == nil {
		return Bundle{}, fmt.Errorf("devproject: store closed")
	}
	var b Bundle
	var created, retrieved, deployed int64
	row := s.db.QueryRow(`SELECT id, dev_project_id, path, default_org_alias,
		created_at, last_retrieved_at, last_deployed_at
		FROM bundles WHERE id = ?`, id)
	if err := row.Scan(&b.ID, &b.DevProjectID, &b.Path, &b.DefaultOrgAlias,
		&created, &retrieved, &deployed); err != nil {
		return Bundle{}, err
	}
	b.CreatedAt = time.Unix(created, 0)
	if retrieved > 0 {
		b.LastRetrievedAt = time.Unix(retrieved, 0)
	}
	if deployed > 0 {
		b.LastDeployedAt = time.Unix(deployed, 0)
	}
	return b, nil
}

// MarkRetrieved sets last_retrieved_at on the bundle. Called after a
// successful `sf project retrieve` against the bundle dir.
func (s *Store) MarkRetrieved(bundleID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("devproject: store closed")
	}
	_, err := s.db.Exec(`UPDATE bundles SET last_retrieved_at = ? WHERE id = ?`,
		time.Now().Unix(), bundleID)
	return err
}

// MarkDeployed sets last_deployed_at on the bundle.
func (s *Store) MarkDeployed(bundleID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("devproject: store closed")
	}
	_, err := s.db.Exec(`UPDATE bundles SET last_deployed_at = ? WHERE id = ?`,
		time.Now().Unix(), bundleID)
	return err
}

// SetDefaultOrgAlias updates which org this bundle retrieves from /
// deploys to by default. Empty string clears the preference.
func (s *Store) SetDefaultOrgAlias(bundleID, alias string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("devproject: store closed")
	}
	_, err := s.db.Exec(`UPDATE bundles SET default_org_alias = ? WHERE id = ?`,
		alias, bundleID)
	return err
}

// SetBundlePath updates the on-disk location for a bundle. Used when
// the user moves a bundle out from under sf-deck and wants to relink
// it (the "relocate" gesture in the bundle list UI).
func (s *Store) SetBundlePath(bundleID, path string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("devproject: store closed")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("normalize path: %w", err)
	}
	_, err = s.db.Exec(`UPDATE bundles SET path = ? WHERE id = ?`, abs, bundleID)
	return err
}

// DeleteBundle removes the row. The on-disk directory is NOT touched;
// the user can either re-link it later or delete it manually with
// their file manager. Conservative — losing a row is recoverable;
// nuking force-app/ from the TUI would be devastating.
func (s *Store) DeleteBundle(bundleID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("devproject: store closed")
	}
	_, err := s.db.Exec(`DELETE FROM bundles WHERE id = ?`, bundleID)
	return err
}

// newBundleID generates a 12-byte hex ID — long enough to avoid
// collisions, short enough to type if the user ever needs to. Same
// shape used elsewhere in the package (see DevProject IDs).
func newBundleID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to a timestamp-based id; collisions are still
		// vanishingly rare with second granularity + a single-process
		// usage pattern.
		return fmt.Sprintf("b%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// nullTime is a tiny adapter so we can scan nullable INTEGER columns
// from SQLite into time.Time. Currently unused since we encode "never"
// as 0 directly; kept in case future migrations add proper NULLs.
var _ = sql.NullInt64{}
