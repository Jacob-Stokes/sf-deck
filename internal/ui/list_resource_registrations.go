package ui

// Registrations for every list-backed resource that uses the generic
// apply/sync/refresh path. Adding a surface = one entry here (plus the
// sf fetcher, column schema, sidebar, registry entry, render func).
//
// Surfaces with BESPOKE apply logic (deploys' completion flash, the
// sObject catalogue's autocomplete bust via AfterSync, recently-viewed's
// per-sObject fan-out) either use AfterSync or keep an explicit case in
// applyResourceMsg — see the switch there.

import "github.com/Jacob-Stokes/sf-deck/internal/sf"

func init() {
	// --- /system ---------------------------------------------------------
	registerListResource(listResourceSpec[sf.ApexLogRow]{
		Key:  "apexlogs",
		Res:  func(d *orgData) *Resource[[]sf.ApexLogRow] { return &d.ApexLogs },
		List: func(d *orgData) *ListView[sf.ApexLogRow] { return &d.ApexLogList },
	})
	registerListResource(listResourceSpec[sf.SetupAuditRow]{
		Key:  "setup_audit_v1",
		Res:  func(d *orgData) *Resource[[]sf.SetupAuditRow] { return &d.SetupAudit },
		List: func(d *orgData) *ListView[sf.SetupAuditRow] { return &d.SetupAuditList },
	})
	registerListResource(listResourceSpec[sf.FlowInterviewRow]{
		Key:  "flow_interviews_v1",
		Res:  func(d *orgData) *Resource[[]sf.FlowInterviewRow] { return &d.FlowInterviews },
		List: func(d *orgData) *ListView[sf.FlowInterviewRow] { return &d.FlowInterviewList },
	})
	registerListResource(listResourceSpec[sf.ActiveUserRow]{
		Key:  "active_users_v1",
		Res:  func(d *orgData) *Resource[[]sf.ActiveUserRow] { return &d.ActiveUsers },
		List: func(d *orgData) *ListView[sf.ActiveUserRow] { return &d.ActiveUserList },
	})
	registerListResource(listResourceSpec[sf.CommunityRow]{
		Key:  "communities_v1",
		Res:  func(d *orgData) *Resource[[]sf.CommunityRow] { return &d.Community },
		List: func(d *orgData) *ListView[sf.CommunityRow] { return &d.CommunityList },
	})
	registerListResource(listResourceSpec[sf.AsyncJobRow]{
		Key:  "async_jobs_v1",
		Res:  func(d *orgData) *Resource[[]sf.AsyncJobRow] { return &d.AsyncJobs },
		List: func(d *orgData) *ListView[sf.AsyncJobRow] { return &d.AsyncJobList },
	})
	registerListResource(listResourceSpec[sf.CronTriggerRow]{
		Key:  "scheduled_jobs_v1",
		Res:  func(d *orgData) *Resource[[]sf.CronTriggerRow] { return &d.ScheduledJobs },
		List: func(d *orgData) *ListView[sf.CronTriggerRow] { return &d.ScheduledJobList },
	})

	// --- top-level tabs --------------------------------------------------
	registerListResource(listResourceSpec[sf.InstalledPackage]{
		Key:  "packages",
		Res:  func(d *orgData) *Resource[[]sf.InstalledPackage] { return &d.Packages },
		List: func(d *orgData) *ListView[sf.InstalledPackage] { return &d.PackageList },
	})
	registerListResource(listResourceSpec[sf.Flow]{
		Key:  "flows_v2",
		Res:  func(d *orgData) *Resource[[]sf.Flow] { return &d.Flows },
		List: func(d *orgData) *ListView[sf.Flow] { return &d.FlowList },
	})
	registerListResource(listResourceSpec[sf.ReportSummary]{
		Key:  "reports",
		Res:  func(d *orgData) *Resource[[]sf.ReportSummary] { return &d.Reports },
		List: func(d *orgData) *ListView[sf.ReportSummary] { return &d.ReportList },
	})
	registerListResource(listResourceSpec[sf.DashboardRow]{
		Key:  "dashboards_v1",
		Res:  func(d *orgData) *Resource[[]sf.DashboardRow] { return &d.Dashboards },
		List: func(d *orgData) *ListView[sf.DashboardRow] { return &d.DashboardList },
	})

	// --- /meta -----------------------------------------------------------
	registerListResource(listResourceSpec[sf.MetadataTypeInfo]{
		Key:  "metatypes_v1",
		Res:  func(d *orgData) *Resource[[]sf.MetadataTypeInfo] { return &d.MetaTypes },
		List: func(d *orgData) *ListView[sf.MetadataTypeInfo] { return &d.MetaTypesList },
	})
	registerListResource(listResourceSpec[sf.CustomLabelRow]{
		Key:  "custom_labels_v1",
		Res:  func(d *orgData) *Resource[[]sf.CustomLabelRow] { return &d.CustomLabels },
		List: func(d *orgData) *ListView[sf.CustomLabelRow] { return &d.CustomLabelList },
	})
	registerListResource(listResourceSpec[sf.MetaEntityRow]{
		Key:  "cmt_types_v1",
		Res:  func(d *orgData) *Resource[[]sf.MetaEntityRow] { return &d.CMTTypes },
		List: func(d *orgData) *ListView[sf.MetaEntityRow] { return &d.CMTList },
	})
	registerListResource(listResourceSpec[sf.MetaEntityRow]{
		Key:  "custom_settings_v1",
		Res:  func(d *orgData) *Resource[[]sf.MetaEntityRow] { return &d.CustomSettings },
		List: func(d *orgData) *ListView[sf.MetaEntityRow] { return &d.CustomSettingList },
	})
	registerListResource(listResourceSpec[sf.StaticResourceRow]{
		Key:  "static_resources_v1",
		Res:  func(d *orgData) *Resource[[]sf.StaticResourceRow] { return &d.StaticResources },
		List: func(d *orgData) *ListView[sf.StaticResourceRow] { return &d.StaticResourceList },
	})
	registerListResource(listResourceSpec[sf.NamedCredentialRow]{
		Key:  "named_creds_v1",
		Res:  func(d *orgData) *Resource[[]sf.NamedCredentialRow] { return &d.NamedCreds },
		List: func(d *orgData) *ListView[sf.NamedCredentialRow] { return &d.NamedCredList },
	})
	registerListResource(listResourceSpec[sf.RemoteSiteRow]{
		Key:  "remote_sites_v1",
		Res:  func(d *orgData) *Resource[[]sf.RemoteSiteRow] { return &d.RemoteSites },
		List: func(d *orgData) *ListView[sf.RemoteSiteRow] { return &d.RemoteSiteList },
	})
	registerListResource(listResourceSpec[sf.ReportTypeRow]{
		Key:  "report_types_v1",
		Res:  func(d *orgData) *Resource[[]sf.ReportTypeRow] { return &d.ReportTypes },
		List: func(d *orgData) *ListView[sf.ReportTypeRow] { return &d.ReportTypeList },
	})

	// --- /apex + /components --------------------------------------------
	registerListResource(listResourceSpec[sf.ApexClassRow]{
		Key:  "apex_classes_v2",
		Res:  func(d *orgData) *Resource[[]sf.ApexClassRow] { return &d.ApexClasses },
		List: func(d *orgData) *ListView[sf.ApexClassRow] { return &d.ApexClassList },
	})
	registerListResource(listResourceSpec[sf.LWCBundle]{
		Key:  "lwc_bundles_v2",
		Res:  func(d *orgData) *Resource[[]sf.LWCBundle] { return &d.LWCBundles },
		List: func(d *orgData) *ListView[sf.LWCBundle] { return &d.LWCBundleList },
	})
	registerListResource(listResourceSpec[sf.AuraBundle]{
		Key:  "aura_bundles_v2",
		Res:  func(d *orgData) *Resource[[]sf.AuraBundle] { return &d.AuraBundles },
		List: func(d *orgData) *ListView[sf.AuraBundle] { return &d.AuraBundleList },
	})
	registerListResource(listResourceSpec[sf.TriggerRow]{
		Key:  "apex_triggers_flat_v2",
		Res:  func(d *orgData) *Resource[[]sf.TriggerRow] { return &d.ApexTriggersFlat },
		List: func(d *orgData) *ListView[sf.TriggerRow] { return &d.ApexTriggerList },
	})

	// --- /perms groups ---------------------------------------------------
	registerListResource(listResourceSpec[sf.QueueRow]{
		Key:  "queues_v2",
		Res:  func(d *orgData) *Resource[[]sf.QueueRow] { return &d.Queues },
		List: func(d *orgData) *ListView[sf.QueueRow] { return &d.QueueList },
	})
	registerListResource(listResourceSpec[sf.PublicGroupRow]{
		Key:  "public_groups_v2",
		Res:  func(d *orgData) *Resource[[]sf.PublicGroupRow] { return &d.PublicGroups },
		List: func(d *orgData) *ListView[sf.PublicGroupRow] { return &d.PublicGroupList },
	})
}
