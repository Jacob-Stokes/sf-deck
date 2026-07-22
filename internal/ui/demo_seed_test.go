package ui

// Seed integrity for `sf-deck --demo`. The TUI reads the demo world
// through the normal cache-first Resource path, so the contract is:
// every kv key a demo org's surfaces load on first visit exists and
// round-trips through the cache layer, and nothing in the fixture
// set references a real org (the whole point of hand-written
// fixtures).

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func demoTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.OpenPath(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("open demo cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := SeedDemoCache(c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return c
}

func TestSeedDemoCache_AllKeysRoundTrip(t *testing.T) {
	c := demoTestCache(t)

	var orgs []sf.Org
	if _, ok, err := c.GetJSON("global", "orgs", &orgs); err != nil || !ok {
		t.Fatalf("global/orgs missing: ok=%v err=%v", ok, err)
	}
	if len(orgs) != 3 {
		t.Fatalf("want 3 demo orgs, got %d", len(orgs))
	}

	for _, o := range orgs {
		for key, v := range map[string]any{
			"home":               &HomeData{},
			"org_info":           &sf.OrgInfo{},
			"sobjects_v5":        &[]sf.SObject{},
			"flows_v2":           &[]sf.Flow{},
			"apex_classes_v2":    &[]sf.ApexClassRow{},
			"deploys_v2":         &[]sf.DeployRow{},
			"setup_audit_v1":     &[]sf.SetupAuditRow{},
			"flow_interviews_v1": &[]sf.FlowInterviewRow{},
			"async_jobs_v1":      &[]sf.AsyncJobRow{},
			"scheduled_jobs_v1":  &[]sf.CronTriggerRow{},
			"active_users_v1":    &[]sf.ActiveUserRow{},
			"lwc_bundles_v2":     &[]sf.LWCBundle{},
			"aura_bundles_v2":    &[]sf.AuraBundle{},
		} {
			if _, ok, err := c.GetJSON(o.Username, key, v); err != nil || !ok {
				t.Errorf("%s/%s missing: ok=%v err=%v", o.Username, key, ok, err)
			}
		}
		// Code drill-downs: every class/trigger row must have a body,
		// every bundle its file set, every flow version a definition.
		for _, cl := range demoApexClasses() {
			var det sf.ApexClassDetail
			if _, ok, err := c.GetJSON(o.Username, "apex_class:"+cl.ID, &det); err != nil || !ok {
				t.Errorf("%s/apex_class:%s (%s) missing: ok=%v err=%v", o.Username, cl.ID, cl.Name, ok, err)
			} else if det.Body == "" || det.Name != cl.Name {
				t.Errorf("apex_class:%s body empty or name mismatch (%q)", cl.ID, det.Name)
			}
		}
		for _, tr := range demoTriggers() {
			var det sf.TriggerDetail
			if _, ok, err := c.GetJSON(o.Username, "triggerdetail:"+tr.ID, &det); err != nil || !ok {
				t.Errorf("%s/triggerdetail:%s (%s) missing: ok=%v err=%v", o.Username, tr.ID, tr.Name, ok, err)
			} else if det.Body == "" {
				t.Errorf("triggerdetail:%s body empty", tr.ID)
			}
		}
		for _, b := range demoLWCBundles() {
			var det sf.LWCBundleDetail
			if _, ok, err := c.GetJSON(o.Username, "lwc_bundle:"+b.ID, &det); err != nil || !ok {
				t.Errorf("%s/lwc_bundle:%s (%s) missing: ok=%v err=%v", o.Username, b.ID, b.DeveloperName, ok, err)
				continue
			}
			if len(det.Resources) < 4 {
				t.Errorf("lwc_bundle:%s has %d resources, want 4", b.ID, len(det.Resources))
			}
			for _, r := range det.Resources {
				if r.Source == "" {
					t.Errorf("lwc_bundle:%s resource %s has empty source", b.ID, r.FilePath)
				}
			}
		}
		for _, b := range demoAuraBundles() {
			var det sf.AuraBundleDetail
			if _, ok, err := c.GetJSON(o.Username, "aura_bundle:"+b.ID, &det); err != nil || !ok {
				t.Errorf("%s/aura_bundle:%s (%s) missing: ok=%v err=%v", o.Username, b.ID, b.DeveloperName, ok, err)
				continue
			}
			if len(det.Resources) < 3 {
				t.Errorf("aura_bundle:%s has %d resources, want 3", b.ID, len(det.Resources))
			}
		}
		for defID, versions := range demoFlowVersionsByDef() {
			for _, v := range versions {
				var def map[string]any
				if _, ok, err := c.GetJSON(o.Username, "flowversiondef:"+v.ID, &def); err != nil || !ok {
					t.Errorf("%s/flowversiondef:%s (def %s v%d) missing: ok=%v err=%v",
						o.Username, v.ID, defID, v.VersionNumber, ok, err)
				} else if len(def) == 0 {
					t.Errorf("flowversiondef:%s empty", v.ID)
				}
			}
		}
		// Every catalog object must have a describe (any drill works)
		// and every record fixture must round-trip.
		for _, so := range demoSObjects() {
			var d sf.SObjectDescribe
			if _, ok, err := c.GetJSON(o.Username, "describe_v3:"+so.Name, &d); err != nil || !ok {
				t.Errorf("%s/describe_v3:%s missing: ok=%v err=%v", o.Username, so.Name, ok, err)
			}
		}
		for _, sobject := range demoRecordObjects {
			var rl sf.RecordsList
			if _, ok, err := c.GetJSON(o.Username, "records:"+sobject, &rl); err != nil || !ok {
				t.Errorf("%s/records:%s missing: ok=%v err=%v", o.Username, sobject, ok, err)
			}
		}
		// FLS: a payload for every (object, permset) pair, and the
		// admin parent must grant edit somewhere (the tape toggles it).
		var picker []sf.FLSPickerEntry
		if _, ok, err := c.GetJSON(o.Username, "permsets", &picker); err != nil || !ok || len(picker) < 5 {
			t.Fatalf("%s/permsets missing or thin: ok=%v err=%v n=%d", o.Username, ok, err, len(picker))
		}
		for _, so := range demoSObjects() {
			for _, p := range picker {
				var rows []sf.FieldPermissionRow
				if _, ok, err := c.GetJSON(o.Username, "fls:"+so.Name+":"+p.ID, &rows); err != nil || !ok {
					t.Errorf("%s/fls:%s:%s missing: ok=%v err=%v", o.Username, so.Name, p.ID, ok, err)
				}
			}
		}
	}
}

func TestSeedDemoCache_FullPages(t *testing.T) {
	// The demo's whole point on tape is lists that fill pages and
	// scroll. Pin minimum volumes so fixture edits can't quietly
	// shrink the world back to a sparse-looking org.
	if n := len(demoSObjects()); n < 80 {
		t.Errorf("sObjects = %d, want >= 80", n)
	}
	if n := len(demoFlows()); n < 40 {
		t.Errorf("flows = %d, want >= 40", n)
	}
	if n := len(demoApexClasses()); n < 50 {
		t.Errorf("apex classes = %d, want >= 50", n)
	}
	if n := len(demoDeploys()); n != 25 {
		t.Errorf("deploys = %d, want exactly 25 (the list window cap)", n)
	}
	wantRecords := map[string]int{
		"Account": 100, "Contact": 80, "Opportunity": 60, "Case": 50,
		"Shipment__c": 100, "Carrier__c": 20, "Warehouse__c": 15,
	}
	for sobject, min := range wantRecords {
		if n := len(demoRecordLists()[sobject].Records); n < min {
			t.Errorf("records:%s = %d rows, want >= %d", sobject, n, min)
		}
	}
	// Every object's describe must hold more than the 8 system
	// fields, or a drill lands on a skeleton.
	for _, so := range demoSObjects() {
		if n := len(demoDescribeFor(so).Fields); n < 12 {
			t.Errorf("describe %s has %d fields, want >= 12", so.Name, n)
		}
	}
	// Operational surfaces added later: same "don't quietly shrink"
	// contract.
	if n := len(demoSetupAudit()); n < 15 {
		t.Errorf("setup audit rows = %d, want >= 15", n)
	}
	if n := len(demoFlowInterviews()); n < 6 {
		t.Errorf("flow interviews = %d, want >= 6", n)
	}
	if n := len(demoAsyncJobs()); n < 12 {
		t.Errorf("async jobs = %d, want >= 12", n)
	}
	if n := len(demoScheduledJobs()); n < 5 {
		t.Errorf("scheduled jobs = %d, want >= 5", n)
	}
	if n := len(demoActiveUsers()); n < 8 {
		t.Errorf("active users = %d, want >= 8", n)
	}
	if n := len(demoLWCBundles()); n < 8 {
		t.Errorf("lwc bundles = %d, want >= 8", n)
	}
	if n := len(demoAuraBundles()); n < 4 {
		t.Errorf("aura bundles = %d, want >= 4", n)
	}
	// Apex drill-downs must look like real code, not stubs.
	for _, c := range demoApexClasses() {
		if n := lineCount(demoApexBody(c.Name)); n < 20 {
			t.Errorf("apex body %s = %d lines, want >= 20", c.Name, n)
		}
	}
}

func TestSeedDemoCache_CrossReferences(t *testing.T) {
	// The fixtures cross-reference each other (job -> class, interview
	// -> flow, bundle JS -> apex controller). Pin the joins so a rename
	// in one catalog can't silently orphan another.
	classIDs := demoApexClassIDs()
	classNames := map[string]bool{}
	for name := range classIDs {
		classNames[name] = true
	}
	for _, j := range demoAsyncJobs() {
		if j.ApexClassName == "" {
			continue
		}
		if !classNames[j.ApexClassName] {
			t.Errorf("async job %s references unknown class %q", j.ID, j.ApexClassName)
		}
		if j.ApexClassID != classIDs[j.ApexClassName] {
			t.Errorf("async job %s class Id %q does not match seeded Id %q for %s",
				j.ID, j.ApexClassID, classIDs[j.ApexClassName], j.ApexClassName)
		}
	}
	flowLabels := map[string]bool{}
	for _, f := range demoFlows() {
		flowLabels[f.MasterLabel] = true
	}
	for _, iv := range demoFlowInterviews() {
		found := false
		for label := range flowLabels {
			if strings.HasPrefix(iv.Label, label+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("flow interview %s label %q does not reference a seeded flow", iv.ID, iv.Label)
		}
	}
	// Every LWC/Aura bundle's generated JS must call a seeded apex
	// class, so reading the source and drilling into the controller
	// both work.
	for _, s := range demoLWCSpecs() {
		if !classNames[s.apexClass] {
			t.Errorf("lwc %s references unknown apex class %q", s.dev, s.apexClass)
		}
	}
	for _, s := range demoAuraSpecs() {
		if !classNames[s.apexClass] {
			t.Errorf("aura %s references unknown apex class %q", s.dev, s.apexClass)
		}
	}
	// Trigger fixtures delegate to handler classes that must exist.
	for trigger, handler := range demoTriggerHandlers {
		if !classNames[handler] {
			t.Errorf("trigger %s delegates to unknown handler %q", trigger, handler)
		}
	}
	// Every trigger's Table must be a catalog object, or the
	// per-sObject trigger list seeding leaves an orphaned key.
	objNames := map[string]bool{}
	for _, o := range demoSObjects() {
		objNames[o.Name] = true
	}
	for _, tr := range demoTriggers() {
		if !objNames[tr.Table] {
			t.Errorf("trigger %s targets unknown object %q", tr.Name, tr.Table)
		}
	}
}

func TestSeedDemoCache_NothingRealLeaks(t *testing.T) {
	// The fixtures are hand-written, but pin it: no org identity may
	// reference anything outside the fictional Northwind world.
	for _, o := range demoOrgs() {
		if !strings.HasSuffix(o.Username, "@northwind.example") {
			t.Errorf("non-fictional username: %q", o.Username)
		}
		if !strings.Contains(o.InstanceURL, "northwind") {
			t.Errorf("non-fictional instance URL: %q", o.InstanceURL)
		}
	}
}

func TestDemoFlipInFlightDeploys(t *testing.T) {
	rows := demoDeploys()
	var hadInFlight bool
	for _, r := range rows {
		if r.InFlight() {
			hadInFlight = true
		}
	}
	if !hadInFlight {
		t.Fatal("demo deploys must include an in-flight row for the watch flip")
	}
	// Fresh rows (just started) must NOT flip yet — the tape needs to
	// catch the InProgress state on screen first.
	for _, r := range demoFlipInFlightDeploys(rows) {
		if r.ID == "0AfDM00000DEMO01" && !r.InFlight() {
			t.Error("in-flight row flipped immediately; want a delay before completion")
		}
	}
	// Backdate past the flip threshold → completes, counters filled.
	aged := make([]sf.DeployRow, len(rows))
	copy(aged, rows)
	for i := range aged {
		if aged[i].InFlight() {
			aged[i].StartDate = "2026-01-01T00:00:00.000+0000"
		}
	}
	for _, r := range demoFlipInFlightDeploys(aged) {
		if r.ID != "0AfDM00000DEMO01" {
			continue
		}
		if r.InFlight() {
			t.Error("aged in-flight row did not flip")
		}
		if r.Status != "Succeeded" || r.ComponentsDeployed != r.ComponentsTotal {
			t.Errorf("flip left row inconsistent: %+v", r)
		}
	}
}
