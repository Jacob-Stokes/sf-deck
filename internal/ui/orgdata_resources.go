package ui

// orgData Resource initialisation.
//
// Extracted from newOrgData in model.go — these are the 20 top-level
// Resource[T] fields that get the same shape of "Scope/Key/TTL/Fetch"
// wiring on org-data construction. Lifting them into their own
// function keeps newOrgData readable (the constructor was 632 lines)
// without losing co-location.
//
// The ttl closure is passed in so the same settings.CacheTTL fallback
// machinery applies as before — caller threads its st.CacheTTL
// wrapper. Same alias/username are threaded as the closures still
// capture them.

import (
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// initOrgDataResources wires every top-level Resource on orgData.
// Called once per org from newOrgData. Per-key sub-Resources
// (Records, ChipRecords, FlowVersions, etc.) are still lazy via
// the Ensure* methods on orgData.
func initOrgDataResources(d *orgData, username, alias string, st *settings.Settings, ttl func(string, time.Duration) time.Duration) {
	d.PermissionSets = Resource[[]sf.FLSPickerEntry]{
		Scope: username, Key: "permsets", TTL: ttl("permsets", 2*time.Hour), NoCache: true,
		Fetch: func() ([]sf.FLSPickerEntry, error) {
			return sf.ListFLSPickerEntries(alias)
		},
	}
	d.Home = Resource[HomeData]{
		Scope: username, Key: "home", TTL: ttl("home", 10*time.Minute),
		Fetch: func() (HomeData, error) {
			// loginLimit was historically borrowed from the async-job
			// setting (so /users · Recent silently capped at 10) —
			// dedicated setting since 2026-06-12.
			return fetchHome(alias, username,
				st.LimitRecentLogins(), st.LimitAsyncJobHistory(), st.LimitDeployHistory())
		},
	}
	// Organization singleton — separate Resource so the cloud-banner
	// header on the Home tab can render before the heavier Home payload
	// (limits + users + jobs + …) lands. Long TTL — Org name and edition
	// don't change often.
	d.OrgInfo = Resource[sf.OrgInfo]{
		Scope: username, Key: "org_info", TTL: ttl("org_info", 24*time.Hour),
		Fetch: func() (sf.OrgInfo, error) { return sf.FetchOrgInfo(alias) },
	}
	d.SObjects = Resource[[]sf.SObject]{
		Scope: username, Key: "sobjects_v5", TTL: ttl("sobjects", 4*time.Hour),
		Fetch: func() ([]sf.SObject, error) { return sf.ListSObjects(alias) },
	}
	d.ApexLogs = Resource[[]sf.ApexLogRow]{
		Scope: username, Key: "apexlogs", TTL: ttl("apex_logs", 30*time.Second),
		Fetch: func() ([]sf.ApexLogRow, error) { return sf.ApexLogs(alias) },
	}
	d.SetupAudit = Resource[[]sf.SetupAuditRow]{
		Scope: username, Key: "setup_audit_v1", TTL: ttl("setup_audit", 2*time.Minute),
		Fetch: func() ([]sf.SetupAuditRow, error) { return sf.SetupAuditTrail(alias, 1000) },
	}
	d.FlowInterviews = Resource[[]sf.FlowInterviewRow]{
		Scope: username, Key: "flow_interviews_v1", TTL: ttl("flow_interviews", 1*time.Minute),
		Fetch: func() ([]sf.FlowInterviewRow, error) { return sf.FlowInterviews(alias, 1000) },
	}
	d.ActiveUsers = Resource[[]sf.ActiveUserRow]{
		Scope: username, Key: "active_users_v1", TTL: ttl("active_users", 1*time.Minute),
		Fetch: func() ([]sf.ActiveUserRow, error) { return sf.ActiveUsers(alias, time.Now()) },
	}
	// Async apex jobs are live/operational — short TTL so the Jobs
	// subtab reflects in-flight executions. Browse limit is generous
	// (200) since the default AsyncJobs cap is tuned for the Home
	// summary, not a scrollable list.
	d.AsyncJobs = Resource[[]sf.AsyncJobRow]{
		Scope: username, Key: "async_jobs_v1", TTL: ttl("async_jobs", 1*time.Minute),
		Fetch: func() ([]sf.AsyncJobRow, error) { return sf.AsyncJobs(alias, 200) },
	}
	d.ScheduledJobs = Resource[[]sf.CronTriggerRow]{
		Scope: username, Key: "scheduled_jobs_v1", TTL: ttl("scheduled_jobs", 2*time.Minute),
		Fetch: func() ([]sf.CronTriggerRow, error) { return sf.ScheduledJobs(alias, 200) },
	}
	d.Community = Resource[[]sf.CommunityRow]{
		Scope: username, Key: "communities_v1", TTL: ttl("communities", 10*time.Minute),
		Fetch: func() ([]sf.CommunityRow, error) { return sf.ListCommunities(alias) },
	}
	d.Deploys = Resource[[]sf.DeployRow]{
		Scope: username, Key: "deploys_v2", TTL: ttl("deploys", 2*time.Minute),
		// Delta-refresh: on second+ fetch, we only pull rows newer than
		// the most recent row we have and merge client-side. Full
		// re-pull happens only when the cached slice is empty.
		FetchWithExisting: func(existing []sf.DeployRow) ([]sf.DeployRow, error) {
			return fetchDeploysDelta(alias, existing, st.LimitDeployHistory())
		},
	}
	d.Packages = Resource[[]sf.InstalledPackage]{
		Scope: username, Key: "packages", TTL: ttl("packages", 2*time.Hour),
		Fetch: func() ([]sf.InstalledPackage, error) { return sf.InstalledPackages(alias) },
	}
	d.Flows = Resource[[]sf.Flow]{
		Scope: username, Key: "flows_v2", TTL: ttl("flows", 15*time.Minute),
		Fetch: func() ([]sf.Flow, error) { return sf.ListFlows(alias) },
	}
	d.Reports = Resource[[]sf.ReportSummary]{
		Scope: username, Key: "reports", TTL: ttl("reports", 1*time.Hour),
		Fetch: func() ([]sf.ReportSummary, error) { return sf.ListAllReports(alias) },
	}
	d.Dashboards = Resource[[]sf.DashboardRow]{
		Scope: username, Key: "dashboards_v1", TTL: ttl("dashboards", 30*time.Minute),
		Fetch: func() ([]sf.DashboardRow, error) { return sf.ListDashboards(alias) },
	}
	d.ReportTypes = Resource[[]sf.ReportTypeRow]{
		Scope: username, Key: "report_types_v1", TTL: ttl("report_types", 4*time.Hour),
		Fetch: func() ([]sf.ReportTypeRow, error) { return sf.ListReportTypes(alias) },
	}
	d.MetaTypes = Resource[[]sf.MetadataTypeInfo]{
		Scope: username, Key: "metatypes_v1", TTL: ttl("meta_types", 24*time.Hour),
		Fetch: func() ([]sf.MetadataTypeInfo, error) { return sf.DescribeMetadataTypes(alias) },
	}
	d.CustomLabels = Resource[[]sf.CustomLabelRow]{
		Scope: username, Key: "custom_labels_v1", TTL: ttl("custom_labels", 1*time.Hour),
		Fetch: func() ([]sf.CustomLabelRow, error) { return sf.ListCustomLabels(alias) },
	}
	d.CMTTypes = Resource[[]sf.MetaEntityRow]{
		Scope: username, Key: "cmt_types_v1", TTL: ttl("cmt_types", 4*time.Hour),
		Fetch: func() ([]sf.MetaEntityRow, error) { return sf.ListCustomMetadataTypes(alias) },
	}
	d.CustomSettings = Resource[[]sf.MetaEntityRow]{
		Scope: username, Key: "custom_settings_v1", TTL: ttl("custom_settings", 4*time.Hour),
		Fetch: func() ([]sf.MetaEntityRow, error) { return sf.ListCustomSettings(alias) },
	}
	d.StaticResources = Resource[[]sf.StaticResourceRow]{
		Scope: username, Key: "static_resources_v1", TTL: ttl("static_resources", 1*time.Hour),
		Fetch: func() ([]sf.StaticResourceRow, error) { return sf.ListStaticResources(alias) },
	}
	d.NamedCreds = Resource[[]sf.NamedCredentialRow]{
		Scope: username, Key: "named_creds_v1", TTL: ttl("named_creds", 4*time.Hour),
		Fetch: func() ([]sf.NamedCredentialRow, error) { return sf.ListNamedCredentials(alias) },
	}
	d.RemoteSites = Resource[[]sf.RemoteSiteRow]{
		Scope: username, Key: "remote_sites_v1", TTL: ttl("remote_sites", 4*time.Hour),
		Fetch: func() ([]sf.RemoteSiteRow, error) { return sf.ListRemoteSites(alias) },
	}
	d.ApexClasses = Resource[[]sf.ApexClassRow]{
		Scope: username, Key: "apex_classes_v2", TTL: ttl("apex_classes", 30*time.Minute),
		Fetch: func() ([]sf.ApexClassRow, error) { return sf.ListApexClasses(alias) },
	}
	d.LWCBundles = Resource[[]sf.LWCBundle]{
		Scope: username, Key: "lwc_bundles_v2", TTL: ttl("lwc_bundles", 30*time.Minute),
		Fetch: func() ([]sf.LWCBundle, error) { return sf.ListLWCBundles(alias) },
	}
	d.AuraBundles = Resource[[]sf.AuraBundle]{
		Scope: username, Key: "aura_bundles_v2", TTL: ttl("aura_bundles", 30*time.Minute),
		Fetch: func() ([]sf.AuraBundle, error) { return sf.ListAuraBundles(alias) },
	}
	d.Queues = Resource[[]sf.QueueRow]{
		Scope: username, Key: "queues_v2", TTL: ttl("queues", 30*time.Minute),
		Fetch: func() ([]sf.QueueRow, error) { return sf.ListQueues(alias) },
	}
	d.PublicGroups = Resource[[]sf.PublicGroupRow]{
		Scope: username, Key: "public_groups_v2", TTL: ttl("public_groups", 30*time.Minute),
		Fetch: func() ([]sf.PublicGroupRow, error) { return sf.ListPublicGroups(alias) },
	}
	// Notifications: 5-minute TTL. Chatter mentions / approval pings
	// aren't realtime-critical for a TUI workflow — the user presses
	// r to force-refresh when they care. A 90s TTL was firing 40+
	// notification calls/hour over an idle session (3 calls per
	// refresh: the connect/notifications endpoint + the chatter feed
	// + sometimes a /query for unread counts) which is the single
	// biggest source of background API noise. 5min cuts that to ~12
	// calls/hour and matches the cadence Lightning's bell icon polls.
	d.Notifications = Resource[sf.NotificationsList]{
		Scope: username, Key: "notifications", TTL: ttl("notifications", 5*time.Minute),
		Fetch: func() (sf.NotificationsList, error) {
			return sf.ListNotifications(alias, st.LimitNotifications())
		},
	}
	// RecentlyViewed: 5-minute TTL. Quick to refresh on demand and
	// the server-side list doesn't change second-by-second; longer
	// would feel stale but shorter would re-query every visit.
	d.RecentlyViewed = Resource[[]sf.RecentlyViewedRow]{
		Scope: username, Key: "recently_viewed", TTL: ttl("recently_viewed", 5*time.Minute),
		Fetch: func() ([]sf.RecentlyViewedRow, error) {
			// Source cap matches the local log's max-entries cap so
			// the merged stream's worst case (no overlap, no kind
			// filter) is bounded. Final display cap is
			// settings.RecentLimit, applied after dedupe + chip
			// filter at render time.
			return sf.ListRecentlyViewed(alias, sf.RecentlyViewedOpts{
				Limit:        st.RecentMaxEntries(),
				ExcludeTypes: st.RecentExcludedSFTypes(),
			})
		},
	}
	// Cross-sObject ApexTrigger list — populated when /apex's
	// Triggers chip is active. Same TTL as the apex class list.
	d.ApexTriggersFlat = Resource[[]sf.TriggerRow]{
		Scope: username, Key: "apex_triggers_flat_v2", TTL: ttl("apex_triggers_flat", 30*time.Minute),
		Fetch: func() ([]sf.TriggerRow, error) {
			return sf.ListAllTriggers(alias)
		},
	}
	d.PermSets = Resource[[]sf.PermissionSet]{
		Scope: username, Key: "permsets_full_v2", TTL: ttl("permsets_full", 30*time.Minute),
		Fetch: func() ([]sf.PermissionSet, error) { return sf.ListPermissionSets(alias) },
	}
	d.PSGs = Resource[[]sf.PermissionSetGroup]{
		Scope: username, Key: "psgs_v2", TTL: ttl("psgs", 30*time.Minute),
		Fetch: func() ([]sf.PermissionSetGroup, error) { return sf.ListPermissionSetGroups(alias) },
	}
	d.Profiles = Resource[[]sf.Profile]{
		Scope: username, Key: "profiles_v2", TTL: ttl("profiles", 30*time.Minute),
		Fetch: func() ([]sf.Profile, error) { return sf.ListProfiles(alias) },
	}
}
