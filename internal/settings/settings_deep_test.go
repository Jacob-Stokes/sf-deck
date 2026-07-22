package settings

import (
	"errors"
	"testing"
	"time"
)

// --- Chips ---------------------------------------------------------------

func TestUpsertChip_AddAndReplace(t *testing.T) {
	s := &Settings{}
	c1 := ChipConfig{ID: "recent", Domain: "records", Label: "Recent"}
	c2 := ChipConfig{ID: "mine", Domain: "records", Label: "Mine"}
	s.UpsertChip(c1)
	s.UpsertChip(c2)
	if got := len(s.Chips()); got != 2 {
		t.Fatalf("got %d chips, want 2", got)
	}
	// Replace by (id, domain) — same id different domain = new entry.
	c1b := ChipConfig{ID: "recent", Domain: "records", Label: "Recent (updated)"}
	s.UpsertChip(c1b)
	if got := len(s.Chips()); got != 2 {
		t.Errorf("replace grew the slice: got %d, want 2", got)
	}
	for _, c := range s.Chips() {
		if c.ID == "recent" && c.Domain == "records" && c.Label != "Recent (updated)" {
			t.Errorf("replace didn't take: %+v", c)
		}
	}
}

func TestUpsertChip_DifferentDomainsCoexist(t *testing.T) {
	s := &Settings{}
	s.UpsertChip(ChipConfig{ID: "recent", Domain: "records"})
	s.UpsertChip(ChipConfig{ID: "recent", Domain: "objects"})
	if got := len(s.Chips()); got != 2 {
		t.Errorf("got %d, want 2 (same id different domain → distinct)", got)
	}
}

func TestDeleteChip_OnlyMatchingDomain(t *testing.T) {
	s := &Settings{}
	s.UpsertChip(ChipConfig{ID: "recent", Domain: "records"})
	s.UpsertChip(ChipConfig{ID: "recent", Domain: "objects"})
	s.DeleteChip("records", "recent")
	if got := len(s.Chips()); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	if s.Chips()[0].Domain != "objects" {
		t.Errorf("deleted the wrong chip: %+v", s.Chips()[0])
	}
}

func TestChipsForDomain_FiltersIn(t *testing.T) {
	s := &Settings{}
	s.UpsertChip(ChipConfig{ID: "a", Domain: "records"})
	s.UpsertChip(ChipConfig{ID: "b", Domain: "objects"})
	s.UpsertChip(ChipConfig{ID: "c", Domain: "records"})
	out := s.ChipsForDomain("records")
	if len(out) != 2 {
		t.Fatalf("got %d, want 2", len(out))
	}
	for _, c := range out {
		if c.Domain != "records" {
			t.Errorf("leak: %+v", c)
		}
	}
}

func TestChipFavouriteOverrides_PerDomain(t *testing.T) {
	s := &Settings{}
	s.SetChipFavouriteOverridesFor("records", map[string]bool{
		"recent": false,
		"mine":   true,
	})
	s.SetChipFavouriteOverridesFor("objects", map[string]bool{
		"customs": true,
	})
	rec := s.ChipFavouriteOverridesFor("records")
	if len(rec) != 2 {
		t.Errorf("records got %d, want 2: %v", len(rec), rec)
	}
	if rec["recent"] != false || rec["mine"] != true {
		t.Errorf("records values wrong: %v", rec)
	}
	obj := s.ChipFavouriteOverridesFor("objects")
	if len(obj) != 1 || obj["customs"] != true {
		t.Errorf("objects values wrong: %v", obj)
	}
	// Setting again replaces just the records prefix, leaves objects.
	s.SetChipFavouriteOverridesFor("records", map[string]bool{"only-one": true})
	if got := len(s.ChipFavouriteOverridesFor("records")); got != 1 {
		t.Errorf("after replace: records got %d, want 1", got)
	}
	if got := len(s.ChipFavouriteOverridesFor("objects")); got != 1 {
		t.Errorf("after replace: objects got %d, want 1 (untouched)", got)
	}
}

func TestSaveRejectsStaleConcurrentSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	initial, err := Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	initial.UpsertChip(ChipConfig{ID: "base", Domain: "records", Label: "Base"})
	if err := initial.Save(); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	first, err := Load()
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	second, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}

	first.UpsertChip(ChipConfig{ID: "first", Domain: "records", Label: "First"})
	if err := first.Save(); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	second.UpsertChip(ChipConfig{ID: "second", Domain: "records", Label: "Second"})
	err = second.Save()
	var conflict ErrConcurrentModification
	if !errors.As(err, &conflict) {
		t.Fatalf("second Save err = %T %v, want ErrConcurrentModification", err, err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Chips()) != 2 {
		t.Fatalf("len(chips) = %d, want base+first", len(reloaded.Chips()))
	}
	for _, c := range reloaded.Chips() {
		if c.ID == "second" {
			t.Fatalf("stale save leaked chip: %+v", c)
		}
	}
}

func TestSaveUpdatesDigestForSameProcessSaves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	st.UpsertChip(ChipConfig{ID: "one", Domain: "records", Label: "One"})
	if err := st.Save(); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	st.UpsertChip(ChipConfig{ID: "two", Domain: "records", Label: "Two"})
	if err := st.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Chips()) != 2 {
		t.Fatalf("len(chips) = %d, want 2", len(reloaded.Chips()))
	}
}

// --- ObjectFilters / FlowFilters / Lenses (legacy chip cousins) ---------

func TestUpsertObjectFilter_AddAndReplace(t *testing.T) {
	s := &Settings{}
	s.UpsertObjectFilter(FilterConfig{ID: "f1", Label: "Foo"})
	s.UpsertObjectFilter(FilterConfig{ID: "f2", Label: "Bar"})
	s.UpsertObjectFilter(FilterConfig{ID: "f1", Label: "Foo (renamed)"})
	got := s.ObjectFilters()
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, f := range got {
		if f.ID == "f1" && f.Label != "Foo (renamed)" {
			t.Errorf("replace didn't take: %+v", f)
		}
	}
}

func TestDeleteObjectFilter_NoOpOnMissing(t *testing.T) {
	s := &Settings{}
	s.UpsertObjectFilter(FilterConfig{ID: "f1"})
	s.DeleteObjectFilter("nonexistent")
	if got := len(s.ObjectFilters()); got != 1 {
		t.Errorf("delete-missing changed length: got %d, want 1", got)
	}
	s.DeleteObjectFilter("f1")
	if got := len(s.ObjectFilters()); got != 0 {
		t.Errorf("got %d, want 0 after delete", got)
	}
}

// --- Org groups ----------------------------------------------------------

func TestOrgGroupForUsername(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "prod", Name: "Production", Members: []string{"alice@prod"}},
		{ID: "qa", Name: "QA", Members: []string{"bob@qa", "carol@qa"}},
	})
	if got := s.OrgGroupForUsername("alice@prod"); got != "prod" {
		t.Errorf("alice → %q, want prod", got)
	}
	if got := s.OrgGroupForUsername("bob@qa"); got != "qa" {
		t.Errorf("bob → %q, want qa", got)
	}
	if got := s.OrgGroupForUsername("nobody@nope"); got != "" {
		t.Errorf("unknown user → %q, want empty", got)
	}
}

func TestPruneOrgGroupMembers_DropsLoggedOut(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "g1", Members: []string{"alice@x", "bob@x", "carol@x"}},
	})
	authed := map[string]bool{"alice@x": true, "carol@x": true}
	changed := s.PruneOrgGroupMembers(authed)
	if !changed {
		t.Error("PruneOrgGroupMembers reported no change despite dropping bob")
	}
	got := s.OrgGroups()[0].Members
	if len(got) != 2 {
		t.Fatalf("got %d members, want 2", len(got))
	}
	for _, m := range got {
		if m == "bob@x" {
			t.Error("bob was supposed to be pruned")
		}
	}
}

func TestPruneOrgGroupMembers_NoOpWhenAllPresent(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "g1", Members: []string{"alice@x"}},
	})
	authed := map[string]bool{"alice@x": true}
	if s.PruneOrgGroupMembers(authed) {
		t.Error("PruneOrgGroupMembers reported change when nothing changed")
	}
}

// TestPruneOrgGroupMembers_EmptySetIsNoOp guards the bug where clearing
// the cache wiped every group's membership: an empty authed set means
// "we don't currently know which orgs exist" (transient: cache clear,
// startup before the org list lands, a failed `sf org list`), NOT "all
// orgs logged out". Pruning then must be a no-op and leave members.
func TestPruneOrgGroupMembers_EmptySetIsNoOp(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "g1", Members: []string{"alice@x", "bob@x"}},
		{ID: "g2", Members: []string{"carol@x"}},
	})
	if s.PruneOrgGroupMembers(map[string]bool{}) {
		t.Error("prune reported change on empty authed set — must be a no-op")
	}
	if s.PruneOrgGroupMembers(nil) {
		t.Error("prune reported change on nil authed set — must be a no-op")
	}
	groups := s.OrgGroups()
	if len(groups[0].Members) != 2 || len(groups[1].Members) != 1 {
		t.Fatalf("members wiped by empty-set prune: %+v", groups)
	}
}

// TestPruneOrgGroupMembers_MultiGroupOnlyTouchesChanged guards the
// aliasing bug: a removal in an earlier group must not corrupt a later
// group that lost nothing. (The old shared `changed` flag + g.Members[:0]
// reuse rewrote unchanged groups.)
func TestPruneOrgGroupMembers_MultiGroupOnlyTouchesChanged(t *testing.T) {
	s := &Settings{}
	s.SetOrgGroups([]OrgGroupConfig{
		{ID: "g1", Members: []string{"alice@x", "gone@x"}}, // loses gone@x
		{ID: "g2", Members: []string{"bob@x", "carol@x"}},  // loses nothing
	})
	authed := map[string]bool{"alice@x": true, "bob@x": true, "carol@x": true}
	if !s.PruneOrgGroupMembers(authed) {
		t.Fatal("expected change (gone@x removed)")
	}
	groups := s.OrgGroups()
	if len(groups[0].Members) != 1 || groups[0].Members[0] != "alice@x" {
		t.Errorf("g1 = %+v, want [alice@x]", groups[0].Members)
	}
	if len(groups[1].Members) != 2 || groups[1].Members[0] != "bob@x" || groups[1].Members[1] != "carol@x" {
		t.Errorf("g2 corrupted = %+v, want [bob@x carol@x]", groups[1].Members)
	}
}

// --- TreeChip ------------------------------------------------------------

func TestTreeChipForOrg_RoundTrip(t *testing.T) {
	s := &Settings{}
	cfg := TreeChipConfig{
		Pins:     []string{"folder-a", "folder-b"},
		LastPath: []string{"root", "sub"},
	}
	s.SetTreeChipForOrg("alice@x", "reports", cfg)
	got := s.TreeChipForOrg("alice@x", "reports")
	if len(got.Pins) != 2 || got.Pins[0] != "folder-a" {
		t.Errorf("Pins lost in round-trip: %+v", got)
	}
	if len(got.LastPath) != 2 || got.LastPath[0] != "root" {
		t.Errorf("LastPath lost in round-trip: %+v", got)
	}
}

func TestTreeChipForOrg_DistinctByOrgAndDomain(t *testing.T) {
	s := &Settings{}
	s.SetTreeChipForOrg("a@x", "reports", TreeChipConfig{Pins: []string{"r1"}})
	s.SetTreeChipForOrg("b@x", "reports", TreeChipConfig{Pins: []string{"r2"}})
	s.SetTreeChipForOrg("a@x", "dashboards", TreeChipConfig{Pins: []string{"d1"}})
	if got := s.TreeChipForOrg("a@x", "reports").Pins[0]; got != "r1" {
		t.Errorf("got %q, want r1", got)
	}
	if got := s.TreeChipForOrg("b@x", "reports").Pins[0]; got != "r2" {
		t.Errorf("got %q, want r2", got)
	}
	if got := s.TreeChipForOrg("a@x", "dashboards").Pins[0]; got != "d1" {
		t.Errorf("got %q, want d1", got)
	}
}

// --- Recent --------------------------------------------------------------

func TestRecentForOrg_RoundTrip(t *testing.T) {
	s := &Settings{}
	now := time.Now()
	list := []RecentConfig{
		{Kind: "record", ID: "001A", Name: "Acme", Type: "Account", VisitedAt: now},
		{Kind: "sobject", ID: "Account", VisitedAt: now.Add(-time.Hour)},
	}
	s.SetRecentForOrg("alice@x", list)
	got := s.RecentForOrg("alice@x")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Name != "Acme" || got[0].Type != "Account" {
		t.Errorf("first entry lost fields: %+v", got[0])
	}
}

func TestRecentForOrg_IsolatedPerOrg(t *testing.T) {
	s := &Settings{}
	s.SetRecentForOrg("a@x", []RecentConfig{{Kind: "record", ID: "1"}})
	s.SetRecentForOrg("b@x", []RecentConfig{{Kind: "record", ID: "2"}})
	if got := s.RecentForOrg("a@x")[0].ID; got != "1" {
		t.Errorf("a got %q, want 1", got)
	}
	if got := s.RecentForOrg("b@x")[0].ID; got != "2" {
		t.Errorf("b got %q, want 2", got)
	}
}

// --- LoadedDevProject ---------------------------------------------------

func TestLoadedDevProjectForOrg_RoundTrip(t *testing.T) {
	s := &Settings{}
	s.SetLoadedDevProjectForOrg("alice@x", "proj-123")
	if got := s.LoadedDevProjectForOrg("alice@x"); got != "proj-123" {
		t.Errorf("got %q, want proj-123", got)
	}
	// Empty string clears.
	s.SetLoadedDevProjectForOrg("alice@x", "")
	if got := s.LoadedDevProjectForOrg("alice@x"); got != "" {
		t.Errorf("clear failed: got %q, want empty", got)
	}
}

// --- Tunables (wheel + recent + boost) ----------------------------------

func TestTunables_DefaultsWhenUnset(t *testing.T) {
	s := &Settings{}
	if got := s.WheelQuietGapMs(); got != DefaultWheelQuietGapMs {
		t.Errorf("WheelQuietGapMs default: got %d, want %d", got, DefaultWheelQuietGapMs)
	}
	if got := s.WheelMinIntervalMs(); got != DefaultWheelMinIntervalMs {
		t.Errorf("WheelMinIntervalMs default: got %d, want %d", got, DefaultWheelMinIntervalMs)
	}
	if got := s.WheelMaxStep(); got != DefaultWheelMaxStep {
		t.Errorf("WheelMaxStep default: got %d, want %d", got, DefaultWheelMaxStep)
	}
	if got := s.RecentMaxEntries(); got != DefaultRecentMaxEntries {
		t.Errorf("RecentMaxEntries default: got %d, want %d", got, DefaultRecentMaxEntries)
	}
	if got := s.ExportHistoryMax(); got != DefaultExportHistoryMax {
		t.Errorf("ExportHistoryMax default: got %d, want %d", got, DefaultExportHistoryMax)
	}
}

func TestTunables_RoundTrip(t *testing.T) {
	s := &Settings{}
	s.SetWheelQuietGapMs(123)
	s.SetWheelMinIntervalMs(45)
	s.SetWheelMaxStep(10)
	s.SetRecentMaxEntries(99)
	s.SetExportHistoryMax(500)
	if s.WheelQuietGapMs() != 123 {
		t.Errorf("WheelQuietGapMs: got %d", s.WheelQuietGapMs())
	}
	if s.WheelMinIntervalMs() != 45 {
		t.Errorf("WheelMinIntervalMs: got %d", s.WheelMinIntervalMs())
	}
	if s.WheelMaxStep() != 10 {
		t.Errorf("WheelMaxStep: got %d", s.WheelMaxStep())
	}
	if s.RecentMaxEntries() != 99 {
		t.Errorf("RecentMaxEntries: got %d", s.RecentMaxEntries())
	}
	if s.ExportHistoryMax() != 500 {
		t.Errorf("ExportHistoryMax: got %d", s.ExportHistoryMax())
	}
}

// --- Theme favourites --------------------------------------------------

func TestThemeFavourites_ToggleAddRemove(t *testing.T) {
	s := &Settings{}
	if s.IsThemeFavourite("dracula") {
		t.Error("preconditions: should not be a favourite")
	}
	s.ToggleThemeFavourite("dracula")
	if !s.IsThemeFavourite("dracula") {
		t.Error("toggle didn't add")
	}
	s.ToggleThemeFavourite("dracula")
	if s.IsThemeFavourite("dracula") {
		t.Error("toggle didn't remove")
	}
}

// --- Pinned tabs --------------------------------------------------------

func TestPinnedTabs_RoundTrip(t *testing.T) {
	s := &Settings{}
	want := []string{"home", "objects", "records"}
	s.SetPinnedTabs(want)
	got := s.PinnedTabs()
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("[%d]: got %q, want %q", i, v, want[i])
		}
	}
}
