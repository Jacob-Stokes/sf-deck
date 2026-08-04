package settings

import (
	"sync"
	"testing"
)

// TestResolveRaceWithSetOrg exercises the exact interleaving Codex
// flagged: IPC goroutines mutating safety via SetOrg/PinDefault while
// the TUI resolves gate decisions via Resolve/DefaultOrgUsername.
// Before the RWMutex fix these read s.Orgs without locking, so under
// -race this hit "concurrent map read and map write" (a hard runtime
// crash in production, not just a race warning). Run with -race to
// guard the fix.
func TestResolveRaceWithSetOrg(t *testing.T) {
	s := &Settings{Orgs: map[string]OrgConfig{"seed@x": {Safety: "records"}}}
	var wg sync.WaitGroup
	const n = 200
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			s.SetOrg("writer@x", SafetyMetadata, i%2 == 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_ = s.Resolve("writer@x", KindSandbox, "alias-x")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_ = s.DefaultOrgUsername()
			s.PinDefault("writer@x")
		}
	}()
	wg.Wait()
}

// TestResolveSafety covers Settings.Resolve — the per-org safety
// resolution chain: explicit per-username override, alias override,
// per-kind default, then the hardcoded safe fallback.
func TestResolveSafety(t *testing.T) {
	s := &Settings{
		Orgs: map[string]OrgConfig{
			"user@prod":     {Safety: "full"},
			"alias-sandbox": {Safety: "records"},
		},
		Defaults: Defaults{
			Production: "read_only",
			Sandbox:    "records",
		},
	}

	// 1. Explicit per-username override wins over everything.
	if got := s.Resolve("user@prod", KindProduction); got != SafetyFull {
		t.Errorf("username override = %v, want full", got)
	}
	// 2. Alias override when username has no config.
	if got := s.Resolve("no-cfg@x", KindProduction, "alias-sandbox"); got != SafetyRecords {
		t.Errorf("alias override = %v, want records", got)
	}
	// 3. Per-kind default when neither username nor alias is configured.
	if got := s.Resolve("someone@sbx", KindSandbox); got != SafetyRecords {
		t.Errorf("sandbox default = %v, want records", got)
	}
	// 4. Hardcoded fallback: production with no config/default is
	//    read-only (safe by default).
	empty := &Settings{}
	if got := empty.Resolve("x", KindProduction); got != SafetyReadOnly {
		t.Errorf("prod fallback = %v, want read_only", got)
	}
	// 5. Nil-safe: a nil *Settings resolves to the kind fallback.
	var nilS *Settings
	if got := nilS.Resolve("x", KindProduction); got != SafetyReadOnly {
		t.Errorf("nil settings prod = %v, want read_only", got)
	}
}

func TestResolveAfterClearDoesNotAssumeClearIsALower(t *testing.T) {
	s := &Settings{
		Orgs: map[string]OrgConfig{
			"scratch@example.com": {Safety: "read_only"},
			"scratch-alias":       {Safety: "metadata"},
		},
	}
	if got := s.ResolveAfterClear("scratch@example.com", KindScratch); got != SafetyFull {
		t.Fatalf("scratch clear = %v, want full hardcoded default", got)
	}
	if got := s.ResolveAfterClear("scratch@example.com", KindScratch, "scratch-alias"); got != SafetyMetadata {
		t.Fatalf("scratch clear with alias override = %v, want metadata", got)
	}
	if raw, ok := s.OrgSafetyOverride("scratch@example.com"); !ok || raw != "read_only" {
		t.Fatalf("override = (%q, %v), want (read_only, true)", raw, ok)
	}
}

// TestStartupAndLimitAccessors round-trips the startup + limit
// getters/setters added for the layout / reconcile work, covering both
// the nil-safe path and the set→get path.
func TestStartupAndLimitAccessors(t *testing.T) {
	var nilS *Settings
	// Nil-safe getters return their fallbacks, not panics.
	_ = nilS.StartupAutoLayout()
	_ = nilS.StartupAutoLayoutMinWidth()
	_ = nilS.StartupSidebarStacked(false)
	_ = nilS.StartupQueryLineVisible(true)
	_ = nilS.StartupLeftRailOpen(false)

	s := &Settings{}
	s.SetStartupAutoLayout(true)
	if !s.StartupAutoLayout() {
		t.Error("SetStartupAutoLayout(true) not reflected")
	}
	s.SetStartupAutoLayoutMinWidth(175)
	if s.StartupAutoLayoutMinWidth() != 175 {
		t.Errorf("min width = %d, want 175", s.StartupAutoLayoutMinWidth())
	}
	s.SetStartupSidebarStacked(true)
	if !s.StartupSidebarStacked(false) {
		t.Error("SetStartupSidebarStacked(true) not reflected")
	}
	s.SetStartupQueryLineVisible(false)
	if s.StartupQueryLineVisible(true) {
		t.Error("SetStartupQueryLineVisible(false) not reflected")
	}
	s.SetStartupLeftRailOpen(true)
	if !s.StartupLeftRailOpen(false) {
		t.Error("SetStartupLeftRailOpen(true) not reflected")
	}
}

// TestFlowVersionEnterOpens covers the flow-version Enter toggle: it
// defaults to true (open Flow Builder) and round-trips both ways.
func TestFlowVersionEnterOpens(t *testing.T) {
	var nilS *Settings
	if !nilS.FlowVersionEnterOpens() {
		t.Error("nil Settings should default to true (open)")
	}
	s := &Settings{}
	if !s.FlowVersionEnterOpens() {
		t.Error("unset should default to true (open Flow Builder)")
	}
	s.SetFlowVersionEnterOpens(false)
	if s.FlowVersionEnterOpens() {
		t.Error("SetFlowVersionEnterOpens(false) not reflected")
	}
	s.SetFlowVersionEnterOpens(true)
	if !s.FlowVersionEnterOpens() {
		t.Error("SetFlowVersionEnterOpens(true) not reflected")
	}
}

// TestSidebarPosition covers the sidebar-position enum: default rhs,
// round-trips the three values, and coerces unknown input to rhs.
func TestSidebarPosition(t *testing.T) {
	var nilS *Settings
	if nilS.SidebarPosition() != SidebarPositionRHS {
		t.Error("nil Settings should default to rhs")
	}
	s := &Settings{}
	if s.SidebarPosition() != SidebarPositionRHS {
		t.Error("unset should default to rhs")
	}
	for _, pos := range []string{SidebarPositionBottom, SidebarPositionAuto, SidebarPositionRHS} {
		s.SetSidebarPosition(pos)
		if s.SidebarPosition() != pos {
			t.Errorf("SetSidebarPosition(%q) not reflected", pos)
		}
	}
	s.SetSidebarPosition("garbage")
	if s.SidebarPosition() != SidebarPositionRHS {
		t.Error("unknown value should coerce to rhs")
	}
}

// TestRecentExcludedSFTypes covers the RecentlyViewed sObject-type
// exclusion list: built-in defaults until set, round-trips a custom
// list, and an explicit empty list means "filter nothing".
func TestRecentExcludedSFTypes(t *testing.T) {
	var nilS *Settings
	if len(nilS.RecentExcludedSFTypes()) == 0 {
		t.Error("nil Settings should return the built-in defaults")
	}
	s := &Settings{}
	def := s.RecentExcludedSFTypes()
	if len(def) == 0 {
		t.Fatal("unset should return the built-in defaults")
	}
	found := false
	for _, tp := range def {
		if tp == "FlowRecordElement" {
			found = true
		}
	}
	if !found {
		t.Error("defaults should include FlowRecordElement (the reported noise)")
	}

	s.SetRecentExcludedSFTypes([]string{"MyType__x", "AnotherType"})
	got := s.RecentExcludedSFTypes()
	if len(got) != 2 || got[0] != "MyType__x" {
		t.Errorf("custom list not reflected: %v", got)
	}

	// Explicit empty → filter nothing (distinct from unset = defaults).
	s.SetRecentExcludedSFTypes(nil)
	if got := s.RecentExcludedSFTypes(); len(got) != 0 {
		t.Errorf("explicit empty should filter nothing, got %v", got)
	}
}
