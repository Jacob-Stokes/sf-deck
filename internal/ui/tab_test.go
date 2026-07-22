package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestStemMappings pins every drill-tab -> family mapping that the old
// hand-written stem() switch encoded, now that stem() reads the TabSpec
// registry. If a registry entry's Stem drifts, this catches it.
func TestStemMappings(t *testing.T) {
	want := map[Tab]Tab{
		TabObjectDetail:      TabObjects,
		TabFieldDetail:       TabObjects,
		TabValidationDetail:  TabObjects,
		TabRecordTypeDetail:  TabObjects,
		TabTriggerDetail:     TabObjects,
		TabFlowDetail:        TabFlows,
		TabPermParentDetail:  TabPerms,
		TabQueueDetail:       TabPerms,
		TabPublicGroupDetail: TabPerms,
		TabReportDetail:      TabReports,
		TabRecordDetail:      TabRecords,
		TabDevProjectDetail:  TabDevProjects,
		TabBundleDetail:      TabDevProjects,
		TabApexDetail:        TabApex,
		TabLWCDetail:         TabLWC,
		TabUserDetail:        TabUsers,
	}
	for drill, family := range want {
		if got := drill.stem(); got != family {
			t.Errorf("stem(%v) = %v, want %v", drill, got, family)
		}
	}
	// Top-level tabs stem to themselves; spec-less TabRecent via the
	// identity fallback.
	for _, top := range []Tab{TabHome, TabSOQL, TabObjects, TabFlows, TabApex, TabCompare, TabRecent} {
		if got := top.stem(); got != top {
			t.Errorf("stem(%v) = %v, want identity", top, got)
		}
	}
}

// TestRegistryStemsAreSane: Stem's zero value is TabHome (Tab is an int
// enum), so a registry entry that forgets to set Stem silently stems to
// /home. No tab legitimately stems to Home except Home itself — fail
// loudly on any other entry reading as TabHome.
func TestRegistryStemsAreSane(t *testing.T) {
	for tab, spec := range tabSpecs() {
		if tab != TabHome && spec.Stem == TabHome {
			t.Errorf("registry entry %v has Stem == TabHome — forgot to set Stem?", tab)
		}
	}
}

// TestEveryTabHasSlugAndSpec keeps the tabSlugs table and the TabSpec
// registry in lockstep: every registered tab has a slug and every slug
// has a registry entry. Adding a Tab constant without both shows up
// here instead of as a "?" breadcrumb or an unpinnable tab.
func TestEveryTabHasSlugAndSpec(t *testing.T) {
	specs := tabSpecs()
	for tab := range specs {
		if _, ok := tabSlugs[tab]; !ok {
			t.Errorf("registry entry %d has no tabSlugs entry (String() would return %q)", int(tab), tab.String())
		}
	}
	// TabRecent is the one legitimate spec-less tab: the surface moved
	// to a /home subtab and the constant survives only so old persisted
	// references keep resolving a slug. It must never grow a spec
	// without a Renderer (TestTabRegistryCompleteness enforces that).
	for tab, slug := range tabSlugs {
		if tab == TabRecent {
			continue
		}
		if specs[tab] == nil {
			t.Errorf("tabSlugs[%q] has no registry entry", slug)
		}
	}
	// Slugs must be unique — they're persisted settings ids.
	seen := map[string]Tab{}
	for tab, slug := range tabSlugs {
		if prev, dup := seen[slug]; dup {
			t.Errorf("slug %q used by both %v and %v", slug, prev, tab)
		}
		seen[slug] = tab
	}
}

// TestTabSlugs pins the persisted slug strings — these are saved in
// settings (pinned tabs) and must never drift.
func TestTabSlugs(t *testing.T) {
	want := map[Tab]string{
		TabHome: "home", TabSOQL: "soql", TabObjects: "objects",
		TabObjectDetail: "object", TabFieldDetail: "field",
		TabValidationDetail: "validation", TabRecordTypeDetail: "recordtype",
		TabTriggerDetail: "trigger", TabFlows: "flows", TabFlowDetail: "flow",
		TabRecords:  "records",
		TabPackages: "packages", TabProjects: "projects", TabSetup: "setup",
		TabPerms: "perms", TabPermParentDetail: "perm", TabReports: "reports",
		TabReportDetail: "report", TabRecent: "recent", TabRecordDetail: "record",
		TabSystem: "system", TabDevProjects: "dev-projects",
		TabDevProjectDetail: "dev-project", TabBundleDetail: "bundle",
		TabApex: "apex", TabApexDetail: "apex-class", TabLWC: "components",
		TabLWCDetail: "component", TabMeta: "meta", TabTags: "tags",
		TabQueueDetail: "queue", TabPublicGroupDetail: "public-group",
		TabUsers: "users", TabUserDetail: "user-detail", TabExec: "exec",
		TabCompare: "compare",
	}
	for tab, slug := range want {
		if got := tab.String(); got != slug {
			t.Errorf("Tab(%d).String() = %q, want %q", int(tab), got, slug)
		}
	}
	if got := Tab(9999).String(); got != "?" {
		t.Errorf("unknown tab String() = %q, want ?", got)
	}
}

// TestTabOverflowHints pins the overflow-modal hints now that they live
// on TabSpec.OverflowHint, including the slug fallback for tabs with no
// hint of their own.
func TestTabOverflowHints(t *testing.T) {
	if got := tabOverflowHint(TabPerms); got != "permsets / PSGs / profiles / queues / public groups" {
		t.Errorf("TabPerms hint = %q", got)
	}
	if got := tabOverflowHint(TabCompare); got != "compare" {
		t.Errorf("hint fallback for TabCompare = %q, want slug", got)
	}
}

// TestTransientDrills pins which drills are excluded from LastTabInStem
// now that the flag lives on TabSpec.TransientDrill. Getting this wrong
// makes number-key nav teleport into stale rows (transient marked
// persistent) or forget the user's entity drill (the reverse).
func TestTransientDrills(t *testing.T) {
	transient := []Tab{TabRecordDetail, TabFieldDetail, TabValidationDetail, TabRecordTypeDetail, TabTriggerDetail, TabReportDetail}
	for _, tab := range transient {
		if !isTransientDrill(tab) {
			t.Errorf("isTransientDrill(%v) = false, want true", tab)
		}
	}
	persistent := []Tab{TabObjectDetail, TabFlowDetail, TabApexDetail, TabUserDetail, TabPermParentDetail, TabLWCDetail, TabObjects, TabHome}
	for _, tab := range persistent {
		if isTransientDrill(tab) {
			t.Errorf("isTransientDrill(%v) = true, want false", tab)
		}
	}
}

// TestMigratedOpenSurfacesWired pins the registry wiring for the nine
// surfaces migrated out of cursorOpenable's legacy tab switch. If an
// entry loses its Open pointer, o/ctrl+o silently dies on that tab.
func TestMigratedOpenSurfacesWired(t *testing.T) {
	wantOpen := []Tab{TabHome, TabReports, TabReportDetail, TabFlowDetail, TabRecords, TabPackages, TabSetup, TabPermParentDetail}
	for _, tab := range wantOpen {
		spec := lookupTabSpec(tab)
		if spec == nil || spec.Open == nil || spec.Open.Openable == nil {
			t.Errorf("%v: no tab-level Open.Openable wired", tab)
		}
	}
	// TabSOQL's Open lives on its Editor subtab (o targets result
	// rows, which only exist there).
	if spec := lookupTabSpec(TabSOQL); spec != nil {
		found := false
		for _, sub := range spec.Subtabs {
			if sub.ID == SubtabSOQLEditor && sub.Open != nil && sub.Open.Openable != nil {
				found = true
			}
		}
		if !found {
			t.Errorf("TabSOQL editor subtab: no Open.Openable wired")
		}
	}
	// The object drill's Records subtab carries its own Open.
	for _, sub := range objectDrillSubtabSpecs() {
		if sub.ID == SubtabRecords {
			if sub.Open == nil || sub.Open.Openable == nil {
				t.Errorf("object drill Records subtab: no Open.Openable wired")
			}
			return
		}
	}
	t.Errorf("object drill has no Records subtab?")
}

// TestNoTabSwitchesOutsideAllowlist is the migration ratchet. Per-tab
// behaviour belongs on the TabSpec registry, not in switch statements
// scattered through the package. The files below hold the residual
// small contextual checks (2-3 cases each) that aren't dispatch; the
// counts are ceilings — adding a new `case Tab` anywhere else, or
// growing one of these, fails here. Shrink freely.
func TestNoTabSwitchesOutsideAllowlist(t *testing.T) {
	allow := map[string]int{
		"sidebar.go":               3,
		"records_export.go":        3,
		"chip_helpers.go":          3,
		"update_search_keys.go":    2,
		"tab_records_dashboard.go": 2,
		"tab_bundles.go":           2,
		"report_export.go":         2,
		"listtable_keys.go":        2,
		"tab_registry.go":          1,
		"devproject_export.go":     1,
		"chip_strip.go":            1,
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\bcase Tab`)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		n := len(re.FindAll(src, -1))
		if n == 0 {
			continue
		}
		ceil, ok := allow[name]
		if !ok {
			t.Errorf("%s: %d new `case Tab` switch case(s) — per-tab behaviour belongs on TabSpec, not a switch", name, n)
		} else if n > ceil {
			t.Errorf("%s: `case Tab` count grew to %d (ceiling %d) — move the new behaviour onto TabSpec", name, n, ceil)
		}
	}
}

// TestEveryChipDomainHasWizardCatalogue: every domain whose manager
// offers "New view…" must have a field catalogue, or the wizard's
// "+ Add filter" silently does nothing (the /apex and /users bug).
// domainRecords is exercised separately — its catalogue is built from
// the live describe.
func TestEveryChipDomainHasWizardCatalogue(t *testing.T) {
	var m Model
	for _, def := range chipDomainDefs() {
		if def.Builtins == nil {
			t.Errorf("domain %q: nil Builtins", def.Domain)
		}
		switch def.Domain {
		case domainSchemaFields:
			continue // no wizard by design
		case domainRecords:
			continue // catalogue derives from the live describe
		case domainActiveUsers:
			continue // fixed built-in lenses (All / No MFA / Recent / API); no custom-chip wizard over derived session rows
		}
		if def.WizardFields == nil {
			t.Errorf("domain %q: no WizardFields — + Add filter would dead-end", def.Domain)
			continue
		}
		if fields := m.wizardFieldsFor(def.Domain, "*"); len(fields) == 0 {
			t.Errorf("domain %q: empty wizard catalogue — + Add filter would dead-end", def.Domain)
		}
	}
}
