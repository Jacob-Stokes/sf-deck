package ui

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func newListTablePrefsTestModel(t *testing.T, c *cache.Cache) Model {
	t.Helper()

	m := New(c)
	m.width = 120
	m.height = 40
	m.orgs = []sf.Org{{
		Alias:       "test",
		Username:    "test@example.com",
		InstanceURL: "https://test.my.salesforce.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	m.selected = 0
	_ = m.ensureOrgData("test@example.com")
	m.setTab(TabObjects)
	return m
}

func TestListTableWidthPrefsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := newListTablePrefsTestModel(t, c)
	ctx := (&m).activeListTableContext()
	if ctx.Scope != "objects" {
		t.Fatalf("active table scope = %q, want objects (per-surface, chip-agnostic)", ctx.Scope)
	}
	ctx.State.UserWidths = map[string]int{"Name": 37}
	if cmd := m.saveListTableWidthsCmd(ctx); cmd == nil {
		t.Fatal("saveListTableWidthsCmd returned nil for persistable objects table")
	} else {
		_ = cmd()
	}

	reloaded := newListTablePrefsTestModel(t, c)
	ctx = (&reloaded).activeListTableContext()
	if got, want := ctx.State.UserWidths["Name"], 37; got != want {
		t.Fatalf("reloaded Name width = %d, want %d", got, want)
	}
}

// Width scopes are deliberately chip-AGNOSTIC on metadata surfaces:
// every chip shows the same columns (chips are row filters), so a
// width tweak follows the user across chips. Records keeps per-chip
// scoping (TestRecordsWidthScopeIncludesModeAndChip) because its SF
// list-view chips change the column set.
func TestListTableWidthPrefsObjectsScopeIsChipAgnostic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := newListTablePrefsTestModel(t, c)
	chips := m.stripRows(domainObjects, "*")
	if len(chips) < 2 {
		t.Fatalf("object chips = %d, want at least 2", len(chips))
	}

	m.setObjectsChipIdx(0)
	first := (&m).activeListTableContext().Scope
	m.setObjectsChipIdx(1)
	second := (&m).activeListTableContext().Scope

	if first != "objects" || second != "objects" {
		t.Fatalf("metadata width scopes should be per-surface: first=%q second=%q", first, second)
	}
}

func TestRecordsWidthScopeIncludesModeAndChip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := newListTablePrefsTestModel(t, c)
	d := m.data["test@example.com"]

	// Default chip on first visit is Recently viewed (__visited__) —
	// see selectedRecordsChip + the QA plan (vault, planning/qa-test-plan §1.x).  Width
	// scope therefore keys on that id, not the legacy "recent"
	// (Changed) default.
	if got, want := recordsWidthScope(d, "Account"), "records:Account:local:__visited__"; got != want {
		t.Fatalf("local records width scope = %q, want %q", got, want)
	}

	setChipMode(d, "Account", ChipModeSalesforce)
	d.ListViewCur["Account"] = "00Bxx0000001234"
	if got, want := recordsWidthScope(d, "Account"), "records:Account:salesforce:00Bxx0000001234"; got != want {
		t.Fatalf("salesforce records width scope = %q, want %q", got, want)
	}
}

func TestHandleColResetWidthsClearsPersistedScope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := newListTablePrefsTestModel(t, c)
	ctx := (&m).activeListTableContext()
	ctx.State.UserWidths = map[string]int{"Name": 41}
	if cmd := m.saveListTableWidthsCmd(ctx); cmd == nil {
		t.Fatal("saveListTableWidthsCmd returned nil for persistable objects table")
	} else {
		_ = cmd()
	}

	next, cmd, handled := m.handleColResetWidths()
	if !handled {
		t.Fatal("handleColResetWidths was not handled for objects table")
	}
	if len(next.data["test@example.com"].ObjectsTableState.UserWidths) != 0 {
		t.Fatalf("reset left widths in state: %#v", next.data["test@example.com"].ObjectsTableState.UserWidths)
	}
	if cmd == nil {
		t.Fatal("reset did not return a persistence command")
	}
	_ = cmd()

	var prefs listTableWidthPrefs
	if _, ok, err := c.GetJSON("test@example.com", listTableWidthPrefsKey, &prefs); err != nil {
		t.Fatalf("load prefs: %v", err)
	} else if !ok {
		t.Fatal("width prefs were not persisted")
	}
	if _, ok := prefs.Scopes["objects"]; ok {
		t.Fatalf("objects scope still persisted after reset: %#v", prefs.Scopes["objects"])
	}
}

// TestWidthScopeFoldsLegacyChipSuffix pins the per-surface width
// migration: scopes older builds wrote as "<base>:<chipID>" fold into
// the base, the base entry wins over folded variants, and records'
// genuinely per-chip scopes survive untouched.
func TestWidthScopeFoldsLegacyChipSuffix(t *testing.T) {
	in := listTableWidthPrefs{Scopes: map[string]map[string]int{
		"flows:active":                {"Label": 60},
		"flows":                       {"Label": 44},
		"apex:classes:tests":          {"Name": 30},
		"users:recent:somechip":       {"Name": 25},
		"records:Account:local:chipX": {"Phone": 18},
	}}
	out := normalizeListTableWidthPrefs(in)
	if got := out.Scopes["flows"]["Label"]; got != 44 {
		t.Errorf("base flows entry should win over chipped variant, got %d", got)
	}
	if _, ok := out.Scopes["flows:active"]; ok {
		t.Errorf("chipped flows scope should have folded away")
	}
	if got := out.Scopes["apex:classes"]["Name"]; got != 30 {
		t.Errorf("apex:classes:tests should fold to apex:classes, got %v", out.Scopes)
	}
	if got := out.Scopes["users:recent"]["Name"]; got != 25 {
		t.Errorf("users:recent:somechip should fold to users:recent (longest base), got %v", out.Scopes)
	}
	if got := out.Scopes["records:Account:local:chipX"]["Phone"]; got != 18 {
		t.Errorf("records per-chip scope must survive, got %v", out.Scopes)
	}
}
