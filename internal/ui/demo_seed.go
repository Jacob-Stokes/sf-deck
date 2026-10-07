package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Demo is the ui-side demo flag: header badge, open-flash behaviour,
// and the deploy flip. Set from main alongside resource/sf DemoMode.
var Demo bool

// demoFlashMsg carries a status-bar flash out of a demo-mode tea.Cmd
// (cmds can't mutate the model; Update folds this into m.flash).
type demoFlashMsg struct{ text string }

// applyDemoSettings sets the safety ladder the tapes show off: prod
// read-only (the trust moment), uat metadata, dev full. Called by
// New() on the freshly-loaded (ephemeral) settings — the demo never
// touches the user's real settings.toml.
func applyDemoSettings(st *settings.Settings) {
	st.SetOrg(demoDev, settings.SafetyFull, false)
	st.SetOrg(demoUAT, settings.SafetyMetadata, false)
	st.SetOrg(demoProd, settings.SafetyReadOnly, false)
	// Pin the tabs the demo can actually back with seed data —
	// System (deploys/logs/audit/jobs) and Components (LWC + Aura)
	// replace Perms and Compare, which need live fetches the demo
	// refuses. (Records isn't pinnable — it lives as the object
	// drill's Records subtab.)
	st.SetPinnedTabs([]string{
		"home", "soql", "objects", "flows",
		"apex", "components", "users", "system",
	})
	for _, u := range []string{demoDev, demoUAT, demoProd} {
		st.SetRecentForOrg(u, demoRecentVisits())
	}
}

const (
	demoDev  = "dev@northwind.example"
	demoUAT  = "uat@northwind.example"
	demoProd = "ops@northwind.example"
)

func demoOrgs() []sf.Org {
	return []sf.Org{
		{Alias: "northwind-dev", Username: demoDev, InstanceURL: "https://northwind--dev.sandbox.my.salesforce.com",
			OrgID: "00DDM00000DEMODEV", IsSandbox: true, Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)},
		{Alias: "northwind-uat", Username: demoUAT, InstanceURL: "https://northwind--uat.sandbox.my.salesforce.com",
			OrgID: "00DDM00000DEMOUAT", IsSandbox: true, Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)},
		{Alias: "northwind", Username: demoProd, InstanceURL: "https://northwind.my.salesforce.com",
			OrgID: "00DDM00000DEMOPRD", Status: "Connected", LastUsed: time.Now().Format(time.RFC3339)},
	}
}

// SeedDemoCache writes the fixture world into the (throwaway) demo
// cache. Same payloads land for every demo org so org-hopping in the
// hero tape always has data behind it.
func SeedDemoCache(c *cache.Cache) error {
	orgs := demoOrgs()
	if err := c.PutJSON("global", "orgs", orgs); err != nil {
		return fmt.Errorf("seed orgs: %w", err)
	}
	if err := c.PutOrgs(orgsToRows(orgs)); err != nil {
		return fmt.Errorf("seed orgs table: %w", err)
	}
	return seedDemoOrgData(c, orgs)
}

func seedDemoOrgData(c *cache.Cache, orgs []sf.Org) error {
	objs := demoSObjects()
	apexDetails := demoApexClassDetails()
	triggerDetails := demoTriggerDetails()
	lwcDetails := demoLWCBundleDetails()
	auraDetails := demoAuraBundleDetails()
	flowVersionDefs := demoFlowVersionDefs()
	triggersByTable := demoTriggersByTable(objs)
	entries := make([]cache.JSONEntry, 0, 512)
	for _, o := range orgs {
		u := o.Username
		seed := func(key string, v any) error {
			entries = append(entries, cache.JSONEntry{OrgUsername: u, Key: key, Value: v})
			return nil
		}
		if err := firstErr(
			seed("home", demoHome(o)),
			seed("org_info", demoOrgInfo(o)),
			seed("sobjects_v5", objs),
			seed("flows_v2", demoFlows()),
			seed("apex_classes_v2", demoApexClasses()),
			seed("apex_triggers_flat_v2", demoTriggers()),
			seed("apexlogs", demoApexLogs()),
			seed("deploys_v2", demoDeploys()),
			seed("notifications", demoNotifications()),
			seed("recently_viewed", demoRecentlyViewed()),
			seed("setup_audit_v1", demoSetupAudit()),
			seed("flow_interviews_v1", demoFlowInterviews()),
			seed("async_jobs_v1", demoAsyncJobs()),
			seed("scheduled_jobs_v1", demoScheduledJobs()),
			seed("active_users_v1", demoActiveUsers()),
			seed("lwc_bundles_v2", demoLWCBundles()),
			seed("aura_bundles_v2", demoAuraBundles()),
		); err != nil {
			return err
		}
		if err := seed("permsets", demoPermsetPicker()); err != nil {
			return err
		}
		for defID, versions := range demoFlowVersionsByDef() {
			if err := seed("flowversions:"+defID, versions); err != nil {
				return err
			}
		}
		for verID, def := range flowVersionDefs {
			if err := seed("flowversiondef:"+verID, def); err != nil {
				return err
			}
		}
		for id, det := range apexDetails {
			if err := seed("apex_class:"+id, det); err != nil {
				return err
			}
		}
		for id, det := range triggerDetails {
			if err := seed("triggerdetail:"+id, det); err != nil {
				return err
			}
		}
		for id, det := range lwcDetails {
			if err := seed("lwc_bundle:"+id, det); err != nil {
				return err
			}
		}
		for id, det := range auraDetails {
			if err := seed("aura_bundle:"+id, det); err != nil {
				return err
			}
		}
		for sobject, list := range triggersByTable {
			if err := seed("triggers:"+sobject, list); err != nil {
				return err
			}
		}
		picker := demoPermsetPicker()
		for _, so := range objs {
			desc := demoDescribeFor(so)
			if err := firstErr(
				seed("describe_v3:"+so.Name, desc),
				seed("object_baseline:"+so.Name, demoBaselineFor(so)),
			); err != nil {
				return err
			}
			for pi, p := range picker {
				rows := demoFLSRows(so.Name, desc.Fields, pi, len(picker), p.ID)
				if err := seed("fls:"+so.Name+":"+p.ID, rows); err != nil {
					return err
				}
			}
		}
		ord := 0
		for sobject, list := range demoRecordLists() {
			ord++
			if err := firstErr(
				seed("records:"+sobject, list),
				seed("listviews:"+sobject, demoListViews(sobject, ord)),
			); err != nil {
				return err
			}
			for chipID, slice := range demoChipSlices(list) {
				if err := seed("chiprecords:"+sobject+":"+chipID, slice); err != nil {
					return err
				}
			}
		}
		for sobject, list := range demoVisitedChipRecords() {
			if err := seed("chiprecords:"+sobject+":"+recentlyViewedChipID, list); err != nil {
				return err
			}
		}
	}
	if err := c.PutJSONBatch(entries); err != nil {
		return fmt.Errorf("seed demo cache: %w", err)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func demoOrgInfo(o sf.Org) sf.OrgInfo {
	typ := "Enterprise Edition"
	if o.IsSandbox {
		typ = "Enterprise Edition (Sandbox)"
	}
	return sf.OrgInfo{Name: "Northwind Trading", OrganizationType: typ, InstanceName: "EU45"}
}

func demoHome(o sf.Org) HomeData {
	return HomeData{
		APIVersion:  "v62.0",
		Username:    o.Username,
		UserID:      "005DM000000DEMOAAA",
		InstanceURL: o.InstanceURL,
		KeyLimits: []KeyLimit{
			{Name: "DailyApiRequests", Max: 100000, Remaining: 87231},
			{Name: "DailyBulkApiBatches", Max: 15000, Remaining: 14910},
			{Name: "DataStorageMB", Max: 10240, Remaining: 6105},
			{Name: "FileStorageMB", Max: 20480, Remaining: 18920},
		},
		RecentDeploys: demoDeploys()[:3],
		Users: sf.UserSummary{
			TotalActive: 34, TotalInactive: 7,
			RecentLogins: demoRecentLogins(),
		},
		AsyncJobs: demoAsyncJobs(),
	}
}

func demoRecordHits(term string) []sf.GlobalSearchHit {
	t := strings.ToLower(strings.TrimSpace(term))
	if t == "" {
		return nil
	}
	var hits []sf.GlobalSearchHit
	for _, sobject := range demoRecordObjects {
		list := demoRecordLists()[sobject]
		for _, rec := range list.Records {
			name, _ := rec["Name"].(string)
			if !strings.Contains(strings.ToLower(name), t) {
				continue
			}
			id, _ := rec["Id"].(string)
			hits = append(hits, sf.GlobalSearchHit{
				Sobject: list.SObject, ID: id, Name: name, Fields: rec,
			})
			if len(hits) >= 20 {
				return hits
			}
		}
	}
	return hits
}

func demoFlipInFlightDeploys(rows []sf.DeployRow) []sf.DeployRow {
	out := make([]sf.DeployRow, len(rows))
	copy(out, rows)
	for i, r := range out {
		if !r.InFlight() {
			continue
		}
		started, err := time.Parse("2006-01-02T15:04:05.000+0000", r.StartDate)
		if err != nil || time.Since(started) < 12*time.Second {
			continue
		}
		r.Status = "Succeeded"
		r.CompletedDate = time.Now().UTC().Format("2006-01-02T15:04:05.000+0000")
		r.ComponentsDeployed = r.ComponentsTotal
		r.TestsCompleted = r.TestsTotal
		out[i] = r
	}
	return out
}
