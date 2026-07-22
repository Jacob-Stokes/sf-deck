package ui

// orgData ↔ ListView synchronization.
//
// ListViews wrap each Resource[T] payload with cursor + search +
// filter state. When a resource lands (cache load, refresh complete,
// list-view fetch), the wrapping ListView needs its items refreshed.
//
// Two flavours of sync helper live here:
//
//   - SyncListViews — the bulk path. Fans out to every per-resource
//     helper. Used at bootstrap; safe but resets every cursor.
//   - SyncXxxList — per-resource targeted helpers. Used by the
//     update-loop's resource dispatcher so an apex_classes refresh
//     doesn't wipe the /objects cursor (or vice versa).
//
// Each helper is a one-line `lv.Set(resource.Value())` — kept
// separate so the dispatcher in update.go can reach for the right
// one without touching unrelated state.

// SyncListViews copies the latest Resource payloads into their wrapping
// ListView items. Keep this for bulk/bootstrap syncs only; resource
// update routing should call the targeted sync helper for the resource
// that actually landed so unrelated cursors/searches survive background
// refreshes.
func (d *orgData) SyncListViews() {
	// Registered list resources sync generically (see
	// list_resource_registrations.go). Only surfaces NOT on the registry
	// — those with a bespoke apply or a non-[]T shape — keep an explicit
	// line below.
	syncRegisteredLists(d)
	d.SyncSObjectsList()
	d.SyncDeploysList()
	d.SyncRecentList()
	d.SyncPermSetsList()
	d.SyncPSGsList()
	d.SyncProfilesList()
	d.SyncHomeLists()
	d.SyncNotificationsList()
}

func (d *orgData) SyncSObjectsList() {
	d.SObjectList.Set(d.SObjects.Value())
}

func (d *orgData) SyncApexLogsList() {
	d.ApexLogList.Set(d.ApexLogs.Value())
}

func (d *orgData) SyncSetupAuditList() {
	d.SetupAuditList.Set(d.SetupAudit.Value())
}

func (d *orgData) SyncFlowInterviewList() {
	d.FlowInterviewList.Set(d.FlowInterviews.Value())
}

func (d *orgData) SyncActiveUserList() {
	d.ActiveUserList.Set(d.ActiveUsers.Value())
}

func (d *orgData) SyncAsyncJobList() {
	d.AsyncJobList.Set(d.AsyncJobs.Value())
}

func (d *orgData) SyncScheduledJobList() {
	d.ScheduledJobList.Set(d.ScheduledJobs.Value())
}

func (d *orgData) SyncDeploysList() {
	d.DeployList.Set(d.Deploys.Value())
}

func (d *orgData) SyncPackagesList() {
	d.PackageList.Set(d.Packages.Value())
}

func (d *orgData) SyncFlowsList() {
	d.FlowList.Set(d.Flows.Value())
}

func (d *orgData) SyncReportsList() {
	d.ReportList.Set(d.Reports.Value())
}

func (d *orgData) SyncRecentList() {
	d.RecentList.Set(d.Recent)
}

func (d *orgData) SyncPermSetsList() {
	d.PermSetList.Set(d.PermSets.Value())
}

func (d *orgData) SyncPSGsList() {
	d.PSGList.Set(d.PSGs.Value())
}

func (d *orgData) SyncProfilesList() {
	d.ProfileList.Set(d.Profiles.Value())
}

func (d *orgData) SyncApexClassesList() {
	d.ApexClassList.Set(d.ApexClasses.Value())
}

func (d *orgData) SyncLWCBundlesList() {
	d.LWCBundleList.Set(d.LWCBundles.Value())
}

func (d *orgData) SyncAuraBundlesList() {
	d.AuraBundleList.Set(d.AuraBundles.Value())
}

func (d *orgData) SyncApexTriggersList() {
	d.ApexTriggerList.Set(d.ApexTriggersFlat.Value())
}

func (d *orgData) SyncQueuesList() {
	d.QueueList.Set(d.Queues.Value())
}

func (d *orgData) SyncPublicGroupsList() {
	d.PublicGroupList.Set(d.PublicGroups.Value())
}

func (d *orgData) SyncNotificationsList() {
	d.HomeNotifList.Set(d.Notifications.Value().Notifications)
}

func (d *orgData) SyncHomeLists() {
	// Home subtab list wrappers — each gets its own search buffer +
	// cursor on top of the existing Home / Packages / Notifications
	// resources. The license list merges UserLicense + PermSetLicense
	// into a unified row type.
	h := d.Home.Value()
	// Sort limits by (group, name) so the default rendering clusters
	// rows under each natural group (API / Storage / Email / …). User
	// can re-sort by any column via the list-table sort gesture; the
	// pre-sort is just the initial order. groupOrder gives a stable
	// non-alpha ordering of groups so "API" lands first regardless of
	// the alphabet.
	limits := append([]KeyLimit(nil), h.KeyLimits...)
	sortLimitsByGroup(limits)
	d.HomeLimitList.Set(limits)
	d.HomeUserList.Set(h.Users.RecentLogins)
	licRows := make([]homeLicenseRow, 0, len(h.UserLicenses)+len(h.PermSetLics))
	for _, ul := range h.UserLicenses {
		name := ul.MasterLabel
		if name == "" {
			name = ul.Name
		}
		licRows = append(licRows, homeLicenseRow{
			Name:   name,
			Kind:   "User",
			Used:   ul.UsedLicenses,
			Total:  ul.TotalLicenses,
			Status: ul.Status,
		})
	}
	for _, ps := range h.PermSetLics {
		name := ps.MasterLabel
		if name == "" {
			name = ps.DeveloperName
		}
		licRows = append(licRows, homeLicenseRow{
			Name:   name,
			Kind:   "PermSet",
			Used:   ps.UsedLicenses,
			Total:  ps.TotalLicenses,
			Status: ps.Status,
		})
	}
	d.HomeLicenseList.Set(licRows)
}

func (d *orgData) SyncDashboardsList() {
	d.DashboardList.Set(d.Dashboards.Value())
}

func (d *orgData) SyncReportTypesList() {
	d.ReportTypeList.Set(d.ReportTypes.Value())
}

func (d *orgData) SyncMetaTypesList() {
	d.MetaTypesList.Set(d.MetaTypes.Value())
}

// SyncMetaTypeItemList re-points the shared component list at the
// drilled type's cached rows (empty when nothing drilled / loaded).
func (d *orgData) SyncMetaTypeItemList() {
	if d.MetaTypeCur == "" {
		d.MetaTypeItemList.Set(nil)
		return
	}
	if r, ok := d.MetaTypeItems[d.MetaTypeCur]; ok && r != nil {
		d.MetaTypeItemList.Set(r.Value())
		return
	}
	d.MetaTypeItemList.Set(nil)
}

func (d *orgData) SyncCustomLabelsList() {
	d.CustomLabelList.Set(d.CustomLabels.Value())
}

func (d *orgData) SyncCMTList() {
	d.CMTList.Set(d.CMTTypes.Value())
}

func (d *orgData) SyncCustomSettingsList() {
	d.CustomSettingList.Set(d.CustomSettings.Value())
}

func (d *orgData) SyncStaticResourcesList() {
	d.StaticResourceList.Set(d.StaticResources.Value())
}

func (d *orgData) SyncNamedCredsList() {
	d.NamedCredList.Set(d.NamedCreds.Value())
}

func (d *orgData) SyncRemoteSitesList() {
	d.RemoteSiteList.Set(d.RemoteSites.Value())
}
