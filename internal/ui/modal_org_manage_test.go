package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestCycleSafetyOverride pins the ladder used by `s` in the org-
// manage modal. Changing this cycle is a UX change — the test
// catches accidental re-orderings.
func TestCycleSafetyOverride(t *testing.T) {
	cases := []struct {
		name        string
		override    string
		hasOverride bool
		wantLevel   settings.SafetyLevel
		wantClear   bool
	}{
		{
			name:        "no override starts at read_only",
			override:    "",
			hasOverride: false,
			wantLevel:   settings.SafetyReadOnly,
			wantClear:   false,
		},
		{
			name:        "read_only → records",
			override:    "read_only",
			hasOverride: true,
			wantLevel:   settings.SafetyRecords,
		},
		{
			name:        "records → metadata",
			override:    "records",
			hasOverride: true,
			wantLevel:   settings.SafetyMetadata,
		},
		{
			name:        "metadata → full",
			override:    "metadata",
			hasOverride: true,
			wantLevel:   settings.SafetyFull,
		},
		{
			name:        "full → clear (inherit)",
			override:    "full",
			hasOverride: true,
			wantLevel:   settings.SafetyReadOnly,
			wantClear:   true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLevel, gotClear := cycleSafetyOverride(c.override, c.hasOverride)
			if gotLevel != c.wantLevel {
				t.Errorf("level = %v, want %v", gotLevel, c.wantLevel)
			}
			if gotClear != c.wantClear {
				t.Errorf("clear = %v, want %v", gotClear, c.wantClear)
			}
		})
	}
}

func TestCycleSafetySaveFailureRollsBackAndKeepsErrorVisible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Change the file after Load so Save deterministically reports a
	// concurrent modification.
	dir := filepath.Join(home, ".sf-deck")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("# changed elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := sf.Org{Alias: "prod", Username: "prod@example.com"}
	m := Model{
		modelOrgs:     modelOrgs{orgs: []sf.Org{o}},
		modelServices: modelServices{settings: st},
	}
	m.orgManageModal = &orgManageModalState{Cursor: 0}
	m.cycleSafetyManageCursoredOrg()
	if _, ok := st.OrgSafetyOverride(o.Username); ok {
		t.Fatal("failed save left an unpersisted safety override active")
	}
	if !strings.Contains(m.banner, "settings save failed") {
		t.Fatalf("banner = %q, want save failure", m.banner)
	}
}
