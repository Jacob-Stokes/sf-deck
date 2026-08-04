package qchip

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// chipWithShare is shorthand for a registry-resident user chip with the
// share field populated — keeps the test cases readable.
func chipWithShare(id string, sh settings.ChipShare) Chip {
	return Chip{
		ID:     id,
		Scope:  "*",
		Origin: OriginUser,
		Share:  sh,
	}
}

// fakeGroups makes a tiny groupMembers closure for tests: "u@a" and
// "u@b" live in group "g1"; "u@c" lives in "g2".
func fakeGroups(groupID, username string) bool {
	mem := map[string]map[string]bool{
		"g1": {"u@a": true, "u@b": true},
		"g2": {"u@c": true},
	}
	return mem[groupID][username]
}

func TestRegistryFiltersByShareSingleOrg(t *testing.T) {
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{
		chipWithShare("only-a", settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@a"}}),
		chipWithShare("only-b", settings.ChipShare{Kind: settings.ChipShareOrg, Orgs: []string{"u@b"}}),
	})

	r.SetActiveOrg("u@a")
	got := chipIDs(r.ChipsFor("*"))
	if !contains(got, "only-a") || contains(got, "only-b") {
		t.Errorf("u@a should see only-a but not only-b; got %v", got)
	}

	r.SetActiveOrg("u@b")
	got = chipIDs(r.ChipsFor("*"))
	if !contains(got, "only-b") || contains(got, "only-a") {
		t.Errorf("u@b should see only-b but not only-a; got %v", got)
	}
}

func TestRegistryFiltersByShareMultiOrg(t *testing.T) {
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{
		chipWithShare("shared-ab", settings.ChipShare{
			Kind: settings.ChipShareOrgs,
			Orgs: []string{"u@a", "u@b"},
		}),
	})

	for _, org := range []string{"u@a", "u@b"} {
		r.SetActiveOrg(org)
		if !contains(chipIDs(r.ChipsFor("*")), "shared-ab") {
			t.Errorf("org %s in the list should see the chip", org)
		}
	}
	r.SetActiveOrg("u@c")
	if contains(chipIDs(r.ChipsFor("*")), "shared-ab") {
		t.Error("u@c (not in list) should NOT see the chip")
	}
}

func TestRegistryFiltersByShareGroup(t *testing.T) {
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{
		chipWithShare("g1-chip", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"}),
	})
	r.SetGroupMembers(fakeGroups)

	// u@a and u@b are in g1 → see it.
	for _, org := range []string{"u@a", "u@b"} {
		r.SetActiveOrg(org)
		if !contains(chipIDs(r.ChipsFor("*")), "g1-chip") {
			t.Errorf("org %s in g1 should see g1-chip", org)
		}
	}
	// u@c is in g2 → does not see g1's chip.
	r.SetActiveOrg("u@c")
	if contains(chipIDs(r.ChipsFor("*")), "g1-chip") {
		t.Error("u@c (g2 member) should NOT see g1-chip")
	}
}

func TestRegistryGroupChipFailsClosedWithoutMembersResolver(t *testing.T) {
	// No SetGroupMembers call → resolver is nil → group-shared chips
	// must NOT appear, matching the registry's documented contract.
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{
		chipWithShare("g1-chip", settings.ChipShare{Kind: settings.ChipShareGroup, Group: "g1"}),
	})
	r.SetActiveOrg("u@a")
	if contains(chipIDs(r.ChipsFor("*")), "g1-chip") {
		t.Error("group-shared chip should fail closed when no members resolver is set")
	}
}

func TestRegistryGlobalShareAppearsForEveryOrg(t *testing.T) {
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{
		chipWithShare("everywhere", settings.ChipShare{Kind: settings.ChipShareGlobal}),
	})
	for _, org := range []string{"u@a", "u@b", "u@c", ""} {
		r.SetActiveOrg(org)
		if !contains(chipIDs(r.ChipsFor("*")), "everywhere") {
			t.Errorf("global chip should appear for org %q", org)
		}
	}
}

// TestRegistryLegacyOrgUserStillHonoured covers chips that haven't been
// migrated yet (Share zero, OrgUser populated) — the registry's
// fallback branch should behave exactly as it did before the refactor.
func TestRegistryLegacyOrgUserStillHonoured(t *testing.T) {
	r := NewRegistry("records", nil)
	r.SetUser([]Chip{{
		ID:      "legacy",
		Scope:   "*",
		Origin:  OriginUser,
		OrgUser: "u@a",
		// Share intentionally zero (pre-migration shape).
	}})
	r.SetActiveOrg("u@a")
	if !contains(chipIDs(r.ChipsFor("*")), "legacy") {
		t.Error("legacy OrgUser chip should appear for matching org")
	}
	r.SetActiveOrg("u@b")
	if contains(chipIDs(r.ChipsFor("*")), "legacy") {
		t.Error("legacy OrgUser chip should NOT appear for other orgs")
	}
}

// chipIDs is defined in qchip_test.go (package-scoped helper).

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
