package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenPath_DBFileIsOwnerOnly verifies OpenPath tightens the cache
// db to 0600. sqlite creates the file 0644 by default; the cache holds
// queried org data + list snapshots that must not be world-readable on
// a multi-user machine. Exercises the real OpenPath (not the test
// helper, which bypasses it).
func TestOpenPath_DBFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("cache db mode = %o, want 600", got)
	}
}

// openTestCache returns a fresh Cache backed by a temp file. Bypasses
// Open() so the user's real ~/.sf-deck/cache.db is never touched.
func openTestCache(t *testing.T) *Cache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := configureDB(db); err != nil {
		_ = db.Close()
		t.Fatalf("configureDB: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		t.Fatalf("schema exec: %v", err)
	}
	c := &Cache{db: db, path: path}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestPutGetOrgs_RoundTrip(t *testing.T) {
	c := openTestCache(t)
	orgs := []OrgRow{
		{
			Username:    "alice@example.com",
			Alias:       "dev-org",
			InstanceURL: "https://dev-org.my.salesforce.com",
			OrgID:       "00D000000000001",
			IsSandbox:   true,
			IsScratch:   false,
			IsDevHub:    false,
			Status:      "Connected",
			LastUsed:    "2026-05-11",
		},
		{
			Username: "bob@example.com",
			Alias:    "prod",
			OrgID:    "00D000000000002",
			Status:   "Connected",
		},
	}
	if err := c.PutOrgs(orgs); err != nil {
		t.Fatalf("PutOrgs: %v", err)
	}
	got, newest, err := c.GetOrgs()
	if err != nil {
		t.Fatalf("GetOrgs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d orgs, want 2", len(got))
	}
	if newest.IsZero() {
		t.Error("newest timestamp should be set")
	}
	// Verify boolean round-trip survives the INTEGER↔bool encoding.
	var alice *OrgRow
	for i, o := range got {
		if o.Username == "alice@example.com" {
			alice = &got[i]
		}
	}
	if alice == nil {
		t.Fatal("alice@ row missing from result")
	}
	if !alice.IsSandbox {
		t.Error("alice IsSandbox lost in round-trip")
	}
	if alice.IsScratch || alice.IsDevHub {
		t.Errorf("alice scratch/devhub flags wrong: %+v", alice)
	}
}

func TestPutOrgs_ReplacesPrior(t *testing.T) {
	c := openTestCache(t)
	first := []OrgRow{{Username: "a@example.com", Status: "Connected"}}
	if err := c.PutOrgs(first); err != nil {
		t.Fatal(err)
	}
	second := []OrgRow{
		{Username: "b@example.com", Status: "Connected"},
		{Username: "c@example.com", Status: "Connected"},
	}
	if err := c.PutOrgs(second); err != nil {
		t.Fatal(err)
	}
	got, _, err := c.GetOrgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d after replace, want 2 (PutOrgs should DELETE before INSERT)", len(got))
	}
	for _, o := range got {
		if o.Username == "a@example.com" {
			t.Error("stale row a@ survived a replace")
		}
	}
}

func TestGetOrgs_EmptyReturnsZeroTime(t *testing.T) {
	c := openTestCache(t)
	rows, ts, err := c.GetOrgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows on empty table", len(rows))
	}
	if !ts.IsZero() {
		t.Errorf("got %v newest on empty table, want zero", ts)
	}
}

func TestPutGetJSON_RoundTrip(t *testing.T) {
	c := openTestCache(t)
	type payload struct {
		Name  string   `json:"name"`
		Items []string `json:"items"`
	}
	in := payload{Name: "describe:Account", Items: []string{"Id", "Name", "Industry"}}
	if err := c.PutJSON("alice@example.com", "describe:Account", in); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}
	var out payload
	cachedAt, ok, err := c.GetJSON("alice@example.com", "describe:Account", &out)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !ok {
		t.Fatal("ok=false for round-tripped key")
	}
	if cachedAt.IsZero() {
		t.Error("cachedAt should be set")
	}
	if out.Name != in.Name || len(out.Items) != len(in.Items) {
		t.Errorf("payload mismatch: got %+v, want %+v", out, in)
	}
}

func TestGetJSON_MissingKey(t *testing.T) {
	c := openTestCache(t)
	var out struct{}
	_, ok, err := c.GetJSON("alice@example.com", "missing:key", &out)
	if err != nil {
		t.Fatalf("GetJSON on missing should return nil err, got %v", err)
	}
	if ok {
		t.Error("ok=true for missing key")
	}
}

func TestPutJSON_UpsertReplaces(t *testing.T) {
	c := openTestCache(t)
	type v struct {
		N int `json:"n"`
	}
	if err := c.PutJSON("a@x", "k", v{N: 1}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := c.PutJSON("a@x", "k", v{N: 2}); err != nil {
		t.Fatal(err)
	}
	var got v
	_, ok, err := c.GetJSON("a@x", "k", &got)
	if err != nil || !ok {
		t.Fatalf("second get: err=%v ok=%v", err, ok)
	}
	if got.N != 2 {
		t.Errorf("upsert didn't replace value: got %d, want 2", got.N)
	}
}

func TestDeleteKeyPrefix(t *testing.T) {
	c := openTestCache(t)
	type v struct{ X int }
	// Mix of records:* keys + other keys.
	if err := c.PutJSON("a@x", "records:Account", v{X: 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("a@x", "records:Opportunity", v{X: 2}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("a@x", "describe:Account", v{X: 3}); err != nil {
		t.Fatal(err)
	}
	n, err := c.DeleteKeyPrefix("records:")
	if err != nil {
		t.Fatalf("DeleteKeyPrefix: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted %d rows, want 2", n)
	}
	// describe:Account should survive.
	var d v
	_, ok, _ := c.GetJSON("a@x", "describe:Account", &d)
	if !ok {
		t.Error("non-matching key was incorrectly deleted")
	}
	// records: keys should be gone.
	_, ok, _ = c.GetJSON("a@x", "records:Account", &d)
	if ok {
		t.Error("records:Account survived prefix-delete")
	}
}

func TestDeleteScopeOnlyRemovesSelectedOrg(t *testing.T) {
	c := openTestCache(t)
	type v struct{ X int }
	if err := c.PutJSON("remove@example.com", "describe:Account", v{X: 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("remove@example.com", "flows", v{X: 2}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("keep@example.com", "describe:Account", v{X: 3}); err != nil {
		t.Fatal(err)
	}

	n, err := c.DeleteScope("remove@example.com")
	if err != nil {
		t.Fatalf("DeleteScope: %v", err)
	}
	if n != 2 {
		t.Fatalf("DeleteScope removed %d rows, want 2", n)
	}

	var got v
	if _, ok, err := c.GetJSON("remove@example.com", "describe:Account", &got); err != nil || ok {
		t.Fatalf("removed org entry remains: ok=%v err=%v", ok, err)
	}
	if _, ok, err := c.GetJSON("keep@example.com", "describe:Account", &got); err != nil || !ok {
		t.Fatalf("other org entry was removed: ok=%v err=%v", ok, err)
	}
}

func TestClearAll(t *testing.T) {
	c := openTestCache(t)
	type v struct{ X int }
	// Seed kv entries across two orgs + an orgs-table row.
	if err := c.PutJSON("a@x", "describe:Account", v{X: 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("a@x", "records:Opportunity", v{X: 2}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutJSON("b@y", "flows", v{X: 3}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutOrgs([]OrgRow{{Username: "a@x", Status: "Connected"}}); err != nil {
		t.Fatal(err)
	}

	n, err := c.ClearAll()
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if n != 3 {
		t.Errorf("cleared %d kv rows, want 3", n)
	}

	// Every kv entry is gone.
	var d v
	for _, key := range []string{"describe:Account", "records:Opportunity"} {
		if _, ok, _ := c.GetJSON("a@x", key, &d); ok {
			t.Errorf("kv key %q survived ClearAll", key)
		}
	}
	if _, ok, _ := c.GetJSON("b@y", "flows", &d); ok {
		t.Error("kv key b@y/flows survived ClearAll")
	}

	// The orgs table is deliberately preserved.
	orgs, _, err := c.GetOrgs()
	if err != nil {
		t.Fatalf("GetOrgs after ClearAll: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("orgs table has %d rows after ClearAll, want 1 (must be preserved)", len(orgs))
	}
}

func TestPath(t *testing.T) {
	c := openTestCache(t)
	if c.Path() == "" {
		t.Error("Path() returned empty string")
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) != 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) != 0")
	}
}
