package ui

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Drill-in stems track recordDetailReturnTab / triggerDetailReturnTab
// so the tab strip highlights the drill's parent, not the static
// Tab.stem() fallback. Separate test from the "transient drills don't
// pollute LastTabInStem" property below — both must hold together.
func TestTriggerDetailStemTracksReturnTab(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{
		Alias: "t", Username: "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected", LastUsed: time.Now().Format(time.RFC3339),
	}}
	_ = m.ensureOrgData("u@t.com")

	m.triggerDetailReturnTab = TabApex
	m.setTab(TabTriggerDetail)
	if got := m.stemForTab(TabTriggerDetail); got != TabApex {
		t.Fatalf("trigger detail stem from Apex = %v, want %v", got, TabApex)
	}

	m.triggerDetailReturnTab = TabObjectDetail
	m.setTab(TabTriggerDetail)
	if got := m.stemForTab(TabTriggerDetail); got != TabObjects {
		t.Fatalf("trigger detail stem from Object Detail = %v, want %v", got, TabObjects)
	}
}

func TestRecordDetailStemTracksReturnTab(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{
		Alias: "t", Username: "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected", LastUsed: time.Now().Format(time.RFC3339),
	}}
	_ = m.ensureOrgData("u@t.com")

	// Drill from /soql → /record. The tab strip should highlight
	// /soql, not /records (the old hard-coded behaviour).
	m.recordDetailReturnTab = TabSOQL
	m.setTab(TabRecordDetail)
	if got := m.stemForTab(TabRecordDetail); got != TabSOQL {
		t.Fatalf("record detail stem from SOQL = %v, want %v", got, TabSOQL)
	}

	// Drill from /reports → /record. Same property.
	m.recordDetailReturnTab = TabReportDetail
	m.setTab(TabRecordDetail)
	if got := m.stemForTab(TabRecordDetail); got != TabReports {
		t.Fatalf("record detail stem from Report Detail = %v, want %v", got, TabReports)
	}

	// No return tab set: falls through to the static stem so the
	// strip doesn't blow up when state's missing.
	// TabHome (== iota 0) is a valid drill origin — stem must
	// resolve to /home, not the static-fallback /records. Regression
	// for the ctrl+f-from-/home bug where the != 0 guard turned a
	// real TabHome into "unset" and silently overwrote to /records.
	m.recordDetailReturnTab = TabHome
	m.setTab(TabRecordDetail)
	if got := m.stemForTab(TabRecordDetail); got != TabHome {
		t.Fatalf("record detail stem with TabHome return = %v, want %v (TabHome is iota 0 but still valid)", got, TabHome)
	}
}

// Transient drill-ins (/record, /trigger, /report, etc.) must not
// pollute LastTabInStem — otherwise pressing the stem's number key
// later teleports the user into a stale per-row drill they thought
// they'd closed. Regression for the ctrl+f → pick record → press 9
// scenario where /records (or /reports) bounced the user back into
// the prior drill instead of the stem's list view.
func TestTransientDrillsDoNotPolluteLastTabInStem(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{
		Alias: "t", Username: "u@t.com",
		InstanceURL: "https://x.my.salesforce.com",
		Status:      "Connected", LastUsed: time.Now().Format(time.RFC3339),
	}}
	d := m.ensureOrgData("u@t.com")

	// Simulate: user on /home, opens record drill via ctrl+f.
	// Resolution under the home stem should still be /home.
	m.recordDetailReturnTab = TabHome
	m.setTab(TabRecordDetail)
	if got := m.resolveStem(TabHome); got != TabHome {
		t.Fatalf("after /record drill from /home, resolveStem(Home) = %v, want %v (drill must not pollute the stem)", got, TabHome)
	}

	// Same property for /reports: drilling into a /report row from
	// /reports must not make a later "press 9 for /reports" land
	// on the stale report-detail.
	d.LastTabInStem = nil
	m.setTab(TabReports)
	m.setTab(TabReportDetail)
	if got := m.resolveStem(TabReports); got != TabReports {
		t.Fatalf("after /report drill, resolveStem(Reports) = %v, want %v", got, TabReports)
	}

	// And for /trigger drills from /apex.
	d.LastTabInStem = nil
	m.setTab(TabApex)
	m.triggerDetailReturnTab = TabApex
	m.setTab(TabTriggerDetail)
	if got := m.resolveStem(TabApex); got != TabApex {
		t.Fatalf("after /trigger drill, resolveStem(Apex) = %v, want %v", got, TabApex)
	}

	// Per-entity drills (TabObjectDetail) SHOULD still be remembered:
	// the user might genuinely want to press 2 to return to the Account
	// schema they were inspecting. This is the feature, not the bug.
	d.LastTabInStem = nil
	m.setTab(TabObjectDetail)
	if got := m.resolveStem(TabObjects); got != TabObjectDetail {
		t.Fatalf("after /object-detail drill, resolveStem(Objects) = %v, want %v (per-entity drills must remain restorable)", got, TabObjectDetail)
	}
}
