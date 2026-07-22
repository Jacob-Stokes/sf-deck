package devproject

// migrateFromOrgProjects test — covers the legacy → flattened layout
// transformation. Builds a fresh DB with the OLD schema (dev_projects
// + org_projects + org_project_items), inserts a small dataset, runs
// migrateFromOrgProjects, then asserts items moved across with the
// right (DevProjectID, OrgUser) tuples and the legacy tables got
// renamed (not deleted).

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// legacySchema is the pre-flatten layout. Lifted verbatim from the
// store.go that shipped before this migration so the migration test
// runs against the actual previous shape.
const legacySchema = `
CREATE TABLE IF NOT EXISTS dev_projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    touched_at  INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS org_projects (
    id              TEXT PRIMARY KEY,
    dev_project_id  TEXT NOT NULL,
    org_user        TEXT NOT NULL,
    label           TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    touched_at      INTEGER NOT NULL,
    UNIQUE(dev_project_id, org_user)
);
CREATE TABLE IF NOT EXISTS org_project_items (
    org_project_id TEXT NOT NULL,
    kind           TEXT NOT NULL,
    ref            TEXT NOT NULL,
    type           TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL DEFAULT '',
    added_at       INTEGER NOT NULL,
    notes          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (org_project_id, kind, ref)
);
`

func TestMigrateFromOrgProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Lay down the legacy schema first.
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	// Layer the new schema on top — that's what Open() would do
	// before the migration runs.
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("new schema: %v", err)
	}

	// Populate: 1 DP with 2 OPs (different orgs), each with 2 items.
	if _, err := db.Exec(`
		INSERT INTO dev_projects (id, name, description, created_at, touched_at)
		VALUES ('dp1', 'Q2 migration', '', 1000, 1000);
		INSERT INTO org_projects (id, dev_project_id, org_user, label, created_at, touched_at)
		VALUES ('op1', 'dp1', 'dev@example.com', '', 1100, 1100),
		       ('op2', 'dp1', 'prod@example.com', 'Prod copy', 1200, 1200);
		INSERT INTO org_project_items (org_project_id, kind, ref, type, name, added_at, notes)
		VALUES ('op1', 'sobject', 'Account', '', 'Account', 1100, ''),
		       ('op1', 'flow', 'flow1', 'Flow', 'AccountRouter', 1110, ''),
		       ('op2', 'sobject', 'Account', '', 'Account', 1200, ''),
		       ('op2', 'apex_class', 'class1', 'ApexClass', 'AccountUtil', 1210, '');
	`); err != nil {
		t.Fatalf("populate: %v", err)
	}

	// Run the migration.
	if err := migrateFromOrgProjects(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Assert: dev_project_items has 4 rows, properly keyed.
	rows, err := db.Query(`
		SELECT dev_project_id, org_user, kind, ref, name
		FROM dev_project_items ORDER BY org_user, kind, ref`)
	if err != nil {
		t.Fatalf("query items: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var devID, org, kind, ref, name string
		if err := rows.Scan(&devID, &org, &kind, &ref, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, devID+"|"+org+"|"+kind+"|"+ref+"|"+name)
	}
	want := []string{
		"dp1|dev@example.com|flow|flow1|AccountRouter",
		"dp1|dev@example.com|sobject|Account|Account",
		"dp1|prod@example.com|apex_class|class1|AccountUtil",
		"dp1|prod@example.com|sobject|Account|Account",
	}
	if len(got) != len(want) {
		t.Fatalf("want %d items, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d: got %q, want %q", i, got[i], w)
		}
	}

	// Legacy tables should now be renamed (not dropped).
	if has, err := tableExists(db, "org_project_items_legacy"); err != nil || !has {
		t.Errorf("expected org_project_items_legacy to exist, has=%v err=%v", has, err)
	}
	if has, err := tableExists(db, "org_projects_legacy"); err != nil || !has {
		t.Errorf("expected org_projects_legacy to exist, has=%v err=%v", has, err)
	}
	if has, err := tableExists(db, "org_project_items"); err != nil || has {
		t.Errorf("expected org_project_items to be gone, has=%v err=%v", has, err)
	}

	// Idempotency: running again should be a no-op (legacy table no
	// longer exists, so migrateFromOrgProjects returns nil immediately).
	if err := migrateFromOrgProjects(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// TestMigrateNoLegacyTables covers the fresh-install case — no legacy
// tables means migrateFromOrgProjects must be a no-op without erroring.
func TestMigrateNoLegacyTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if err := migrateFromOrgProjects(db); err != nil {
		t.Fatalf("migrate on fresh: %v", err)
	}
}
