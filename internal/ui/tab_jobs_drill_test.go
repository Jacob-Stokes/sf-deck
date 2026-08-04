package ui

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestAsyncJobDrillReturnsToJobsSubtab: Enter on an async-job row with an
// Apex class drills into TabApexDetail AND stamps DrillReturnTab so esc
// returns to /system (with the jobs subtab preserved, since SystemSubtab
// is untouched). No-op flash when the job has no class.
func TestAsyncJobDrillSetsReturnTab(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	m.orgs = []sf.Org{{Alias: "d", Username: "d@d.test", InstanceURL: "https://d.my.salesforce.com", Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)}}
	d := m.ensureOrgData("d@d.test")
	m.setTab(TabSystem)
	// Land on the Async Jobs subtab.
	subs := systemSubtabs()
	for i, s := range subs {
		if s.ID == SubtabSystemAsyncJobs {
			m.setSystemSubtab(i)
		}
	}
	subBefore := m.systemSubtab()

	// Seed one async job WITH an apex class under the cursor.
	d.AsyncJobList.Set([]sf.AsyncJobRow{{ID: "707x", Status: "Completed", ApexClassID: "01px", ApexClassName: "MyBatch"}})

	(&m).activateAsyncJob()

	if m.tab() != TabApexDetail {
		t.Fatalf("expected drill into TabApexDetail, got %v", m.tab())
	}
	if d.ApexCur != "01px" {
		t.Errorf("expected ApexCur=01px, got %q", d.ApexCur)
	}
	if back := d.DrillReturnTab[TabApexDetail]; back != TabSystem {
		t.Errorf("expected DrillReturnTab[TabApexDetail]=TabSystem for esc-back, got %v", back)
	}
	// The subtab index is untouched → esc returning to /system lands on
	// the same (Async Jobs) subtab.
	if m.systemSubtab() != subBefore {
		t.Errorf("drill must not change the system subtab index (was %d, now %d)", subBefore, m.systemSubtab())
	}
}

// TestAsyncJobDrillNoClass: a job with no Apex class doesn't drill.
func TestAsyncJobDrillNoClass(t *testing.T) {
	c, _ := cache.Open()
	defer c.Close()
	m := New(c)
	m.orgs = []sf.Org{{Alias: "d", Username: "d@d.test", InstanceURL: "https://d.my.salesforce.com", Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)}}
	d := m.ensureOrgData("d@d.test")
	m.setTab(TabSystem)
	d.AsyncJobList.Set([]sf.AsyncJobRow{{ID: "707x", Status: "Completed", JobType: "Future"}}) // no class
	(&m).activateAsyncJob()
	if m.tab() != TabSystem {
		t.Errorf("no-class job should not drill; tab = %v", m.tab())
	}
}
