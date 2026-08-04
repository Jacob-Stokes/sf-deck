package settings

import "testing"

// alwaysIn / neverIn are tiny group-membership stubs so the share
// resolver can be tested without a real Settings instance.
func alwaysIn(_, _ string) bool { return true }
func neverIn(_, _ string) bool  { return false }

func TestChipShareAllows(t *testing.T) {
	cases := []struct {
		name    string
		share   ChipShare
		org     string
		members func(string, string) bool
		want    bool
	}{
		{"global allows any org", ChipShare{Kind: ChipShareGlobal}, "a@b", neverIn, true},
		{"global allows empty org too", ChipShare{Kind: ChipShareGlobal}, "", neverIn, true},
		{"org matches one entry", ChipShare{Kind: ChipShareOrg, Orgs: []string{"u@a"}}, "u@a", neverIn, true},
		{"org rejects mismatch", ChipShare{Kind: ChipShareOrg, Orgs: []string{"u@a"}}, "u@b", neverIn, false},
		{"orgs list match", ChipShare{Kind: ChipShareOrgs, Orgs: []string{"u@a", "u@b"}}, "u@b", neverIn, true},
		{"orgs list miss", ChipShare{Kind: ChipShareOrgs, Orgs: []string{"u@a"}}, "u@b", neverIn, false},
		{"orgs rejects empty querying org", ChipShare{Kind: ChipShareOrg, Orgs: []string{"u@a"}}, "", neverIn, false},
		{"group hits when members says yes", ChipShare{Kind: ChipShareGroup, Group: "g1"}, "u@a", alwaysIn, true},
		{"group misses when members says no", ChipShare{Kind: ChipShareGroup, Group: "g1"}, "u@a", neverIn, false},
		{"group with empty id never matches", ChipShare{Kind: ChipShareGroup, Group: ""}, "u@a", alwaysIn, false},
		{"group with nil members never matches", ChipShare{Kind: ChipShareGroup, Group: "g1"}, "u@a", nil, false},
		{"unknown kind fails closed (don't show)", ChipShare{Kind: "future-kind"}, "u@a", alwaysIn, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.share.Allows(c.org, c.members); got != c.want {
				t.Errorf("Allows = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEffectiveShareLegacyMigration(t *testing.T) {
	// Legacy ChipConfig with only OrgUser set should read as ChipShareOrg.
	c := ChipConfig{OrgUser: "user@org.test"}
	got := c.EffectiveShare()
	if got.Kind != ChipShareOrg || len(got.Orgs) != 1 || got.Orgs[0] != "user@org.test" {
		t.Errorf("legacy OrgUser did not migrate to ChipShareOrg: got %+v", got)
	}

	// Built-in / pre-OrgUser chip (no OrgUser, no Share) reads as global.
	if got := (ChipConfig{}).EffectiveShare(); got.Kind != ChipShareGlobal {
		t.Errorf("empty chip should be global, got %+v", got)
	}

	// When Share IS populated, it takes precedence over OrgUser.
	c2 := ChipConfig{
		OrgUser: "legacy@org",
		Share:   ChipShare{Kind: ChipShareGlobal},
	}
	if got := c2.EffectiveShare(); got.Kind != ChipShareGlobal {
		t.Errorf("Share should override OrgUser, got %+v", got)
	}
}

func TestNormaliseShareCollapsesLegacyField(t *testing.T) {
	c := ChipConfig{OrgUser: "u@a"}
	c.NormaliseShare()
	if c.OrgUser != "" {
		t.Errorf("OrgUser should be cleared after migration, got %q", c.OrgUser)
	}
	if c.Share.Kind != ChipShareOrg || len(c.Share.Orgs) != 1 || c.Share.Orgs[0] != "u@a" {
		t.Errorf("Share not populated correctly: %+v", c.Share)
	}

	// Already-modern chip is left alone.
	modern := ChipConfig{Share: ChipShare{Kind: ChipShareGroup, Group: "g1"}}
	modern.NormaliseShare()
	if modern.Share.Kind != ChipShareGroup || modern.Share.Group != "g1" {
		t.Errorf("modern chip mutated: %+v", modern.Share)
	}
}

func TestUpsertChipNormalisesShareOnWrite(t *testing.T) {
	s := &Settings{}
	// A legacy-shape chip going through Upsert should land on disk as Share.
	s.UpsertChip(ChipConfig{ID: "x", Domain: "records", OrgUser: "u@a"})
	got := s.Chips()
	if len(got) != 1 {
		t.Fatalf("want 1 chip, got %d", len(got))
	}
	if got[0].OrgUser != "" {
		t.Errorf("OrgUser should be cleared by UpsertChip, got %q", got[0].OrgUser)
	}
	if got[0].Share.Kind != ChipShareOrg || len(got[0].Share.Orgs) != 1 {
		t.Errorf("Share not normalised: %+v", got[0].Share)
	}

	// Round-trip: SetChips with legacy entries also normalises.
	s2 := &Settings{}
	s2.SetChips([]ChipConfig{{ID: "y", Domain: "records", OrgUser: "u@b"}})
	if s2.Chips()[0].OrgUser != "" || s2.Chips()[0].Share.Kind != ChipShareOrg {
		t.Errorf("SetChips did not normalise: %+v", s2.Chips()[0])
	}
}

// TestChipShareIntegratesWithOrgGroup proves the share resolver lines up
// with the real OrgGroupForUsername helper — i.e. the adapter the UI
// will use (groupMembers := s.OrgGroupForUsername(u) == groupID) gives
// correct allow/deny against actual group config.
func TestChipShareIntegratesWithOrgGroup(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "acme", Name: "Acme", Members: []string{"prod@acme", "test@acme"}},
		{ID: "ext", Name: "Ext", Members: []string{"other@ext"}},
	})
	groupMembers := func(groupID, username string) bool {
		return s.OrgGroupForUsername(username) == groupID
	}
	share := ChipShare{Kind: ChipShareGroup, Group: "acme"}
	if !share.Allows("prod@acme", groupMembers) {
		t.Error("Acme-grouped chip should allow prod@acme")
	}
	if !share.Allows("test@acme", groupMembers) {
		t.Error("Acme-grouped chip should allow test@acme")
	}
	if share.Allows("other@ext", groupMembers) {
		t.Error("Acme-grouped chip should NOT allow other@ext")
	}
	if share.Allows("unknown@x", groupMembers) {
		t.Error("Acme-grouped chip should NOT allow unknown@x")
	}
}

func TestChipShareIsShared(t *testing.T) {
	cases := []struct {
		name  string
		share ChipShare
		want  bool
	}{
		{"global is shared", ChipShare{Kind: ChipShareGlobal}, true},
		{"group is shared", ChipShare{Kind: ChipShareGroup, Group: "g1"}, true},
		{"orgs with >1 entry is shared", ChipShare{Kind: ChipShareOrgs, Orgs: []string{"a", "b"}}, true},
		{"orgs with 1 entry is NOT shared", ChipShare{Kind: ChipShareOrgs, Orgs: []string{"a"}}, false},
		{"single-org is NOT shared", ChipShare{Kind: ChipShareOrg, Orgs: []string{"a"}}, false},
		{"zero share is NOT shared (legacy chip)", ChipShare{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.share.IsShared(); got != c.want {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}
