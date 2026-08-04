package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// makeSettings is a tiny constructor that drops a fixed set of chips
// + org groups into a fresh Settings — keeps the test cases short.
func makeSettings(t *testing.T, chips []settings.ChipConfig, groups []settings.OrgGroupConfig) *settings.Settings {
	t.Helper()
	s := &settings.Settings{}
	if groups != nil {
		s.SetOrgGroups(groups)
	}
	for _, c := range chips {
		s.UpsertChip(c)
	}
	return s
}

func TestChipScopeApplies(t *testing.T) {
	cases := []struct {
		chipScope, querScope string
		want                 bool
	}{
		{"", "Account", true},        // empty matches anything
		{"*", "Account", true},       // wildcard matches anything
		{"Account", "Account", true}, // exact match
		{"Account", "Contact", false},
		{"Account", "", false}, // empty query scope doesn't match a specific chip
	}
	for _, c := range cases {
		if got := chipScopeApplies(c.chipScope, c.querScope); got != c.want {
			t.Errorf("chipScopeApplies(%q,%q) = %v, want %v",
				c.chipScope, c.querScope, got, c.want)
		}
	}
}

func TestChipOriginOrgFromShare(t *testing.T) {
	cases := []struct {
		name  string
		share settings.ChipShare
		want  string
	}{
		{"single org", settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}}, "u@a"},
		{"orgs list first", settings.ChipShare{Kind: settings.ChipShareOrgs, Orgs: []string{"u@a", "u@b"}}, "u@a"},
		{"group returns group id", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"}, "g1"},
		{"global has no representative origin", settings.ChipShare{Kind: settings.ChipShareGlobal}, ""},
		{"empty share", settings.ChipShare{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chipOriginOrgFromShare(c.share); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestChipsFromOtherOrgsExcludesAllowed: a chip whose share INCLUDES
// the active org should NOT appear in the cross-org list (it's already
// rendered in the main "available/pinned" sections).
func TestChipsFromOtherOrgsExcludesAllowed(t *testing.T) {
	s := makeSettings(t, []settings.ChipConfig{
		{ID: "a", Domain: "objects", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareOrg, Orgs: []string{"u@active"}}},
		{ID: "b", Domain: "objects", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareOrg, Orgs: []string{"u@other"}}},
		{ID: "c", Domain: "objects", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareGlobal}}, // already visible everywhere
	}, nil)
	m := Model{
		modelServices: modelServices{settings: s},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0, // u@active
		},
	}
	got := m.chipsFromOtherOrgs(domainObjects, "*")
	if len(got) != 1 || got[0].ID != "b" {
		ids := make([]string, len(got))
		for i, r := range got {
			ids[i] = r.ID
		}
		t.Fatalf("expected only 'b' in other-orgs, got %v", ids)
	}
	if got[0].OriginOrgUser != "u@other" {
		t.Errorf("origin org wrong: %q", got[0].OriginOrgUser)
	}
}

// TestChipsFromOtherOrgsScopeFilter: only chips matching the current
// view's scope ('Account' here) should appear, not ones for other
// sObjects.
func TestChipsFromOtherOrgsScopeFilter(t *testing.T) {
	s := makeSettings(t, []settings.ChipConfig{
		{ID: "acc", Domain: "records", Scope: "Account", Share: settings.ChipShare{
			Kind: settings.ChipShareOrg, Orgs: []string{"u@other"}}},
		{ID: "con", Domain: "records", Scope: "Contact", Share: settings.ChipShare{
			Kind: settings.ChipShareOrg, Orgs: []string{"u@other"}}},
		{ID: "any", Domain: "records", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareOrg, Orgs: []string{"u@other"}}},
	}, nil)
	m := Model{
		modelServices: modelServices{settings: s},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0,
		},
	}
	got := m.chipsFromOtherOrgs(domainRecords, "Account")
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if !ids["acc"] || !ids["any"] {
		t.Errorf("Account-scope or wildcard should appear; got %v", ids)
	}
	if ids["con"] {
		t.Errorf("Contact-scope chip should NOT appear when current scope is Account; got %v", ids)
	}
}

// TestChipsFromOtherOrgsGroupResolved: a chip shared with a group the
// active org is IN should NOT appear in the other-orgs list (it's
// already visible). A chip shared with a group the active org is NOT
// in should appear.
func TestChipsFromOtherOrgsGroupResolved(t *testing.T) {
	s := makeSettings(t, []settings.ChipConfig{
		{ID: "in-group", Domain: "objects", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareGroup, Group: "g1"}},
		{ID: "out-group", Domain: "objects", Scope: "*", Share: settings.ChipShare{
			Kind: settings.ChipShareGroup, Group: "g2"}},
	}, []settings.OrgGroupConfig{
		{ID: "g1", Name: "Mine", Members: []string{"u@active"}},
		{ID: "g2", Name: "Other", Members: []string{"u@elsewhere"}},
	})
	m := Model{
		modelServices: modelServices{settings: s},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}},
			selected: 0,
		},
	}
	got := m.chipsFromOtherOrgs(domainObjects, "*")
	if len(got) != 1 || got[0].ID != "out-group" {
		ids := make([]string, len(got))
		for i, r := range got {
			ids[i] = r.ID
		}
		t.Errorf("expected only 'out-group' (active org NOT in g2); got %v", ids)
	}
}

// TestChipsFromOtherOrgsHonoursLegacyOrgUser: a chip stored with only
// the legacy OrgUser field (no Share) must still be considered owned
// by that org for the cross-org list.
func TestChipsFromOtherOrgsHonoursLegacyOrgUser(t *testing.T) {
	// UpsertChip normalises OrgUser into Share — bypass it for this test
	// by writing the chips via SetChips with the legacy field, then
	// resetting them to the pre-normalisation shape.
	s := &settings.Settings{}
	s.UpsertChip(settings.ChipConfig{ID: "legacy", Domain: "objects", Scope: "*", OrgUser: "u@other"})
	// UpsertChip should have migrated OrgUser → Share; verify and then
	// check the cross-org selector still classifies it as another org's.
	chips := s.Chips()
	if len(chips) != 1 || chips[0].OrgUser != "" || chips[0].Share.Kind != settings.ChipShareOrg {
		t.Fatalf("UpsertChip did not normalise OrgUser as expected: %+v", chips[0])
	}
	m := Model{
		modelServices: modelServices{settings: s},
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0,
		},
	}
	got := m.chipsFromOtherOrgs(domainObjects, "*")
	if len(got) != 1 || got[0].ID != "legacy" {
		t.Errorf("legacy OrgUser-only chip should classify as 'other org', got %+v", got)
	}
}
