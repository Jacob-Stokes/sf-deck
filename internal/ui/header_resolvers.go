package ui

// Per-tab BusyLabel + ErrorLabel resolvers. One closure per tab
// reading that tab's primary resource(s) and surfacing a human
// label for the header's activity / error zones. Wired into
// TabSpec.BusyLabel + TabSpec.ErrorLabel.
//
// Adding a new tab with a network resource = two closures here +
// two pointers on its TabSpec entry. The header dispatch
// (currentTabSyncingLabel, currentTabError) is one resolver call.

// busyHome / errHome — TabHome's Home resource.
func busyHome(_ Model, d *orgData) string {
	if d != nil && d.Home.Busy() {
		return "syncing limits…"
	}
	return ""
}
func errHome(_ Model, d *orgData) string {
	if d != nil && d.Home.Err() != nil {
		return d.Home.Err().Error()
	}
	return ""
}

// busyObjects + errObjects cover both /objects and the parent
// SObjects resource on /object-detail. ObjectDetail's describe
// busy state is layered on by busyObjectDetail below.
func busyObjects(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.SObjects.Busy() {
		return "syncing sobjects…"
	}
	return ""
}
func errObjects(_ Model, d *orgData) string {
	if d != nil && d.SObjects.Err() != nil {
		return d.SObjects.Err().Error()
	}
	return ""
}

// busyObjectDetail layers describe + listview-fetch states on top
// of the parent /objects sync.
func busyObjectDetail(m Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.SObjects.Busy() {
		return "syncing sobjects…"
	}
	if d.DescribeCur != "" {
		// Records subtab can still be syncing the chip-resource even
		// when describe is loaded.
		if m.currentSubtab() == SubtabRecords && d.RecordsSObjectCur == "" && d.DescribeCur != "" {
			if activeChipBusy(d, d.DescribeCur) {
				if currentChipMode(d, d.DescribeCur) == ChipModeSalesforce {
					return "fetching list view results…"
				}
				return "fetching records…"
			}
		}
		if r, ok := d.Describes[d.DescribeCur]; ok && r.Busy() {
			return "describing " + d.DescribeCur + "…"
		}
		if lv, ok := d.ListViewsPerSObject[d.DescribeCur]; ok && lv.Busy() {
			return "fetching list views…"
		}
	}
	return ""
}
func errObjectDetail(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.SObjects.Err() != nil {
		return d.SObjects.Err().Error()
	}
	if d.DescribeCur != "" {
		if r, ok := d.Describes[d.DescribeCur]; ok && r.Err() != nil {
			return r.Err().Error()
		}
	}
	return ""
}

// busyPackages / errPackages — TabPackages.
func busyPackages(_ Model, d *orgData) string {
	if d != nil && d.Packages.Busy() {
		return "syncing packages…"
	}
	return ""
}
func errPackages(_ Model, d *orgData) string {
	if d != nil && d.Packages.Err() != nil {
		return d.Packages.Err().Error()
	}
	return ""
}

// busyFlows / errFlows — TabFlows + TabFlowDetail share the parent
// resource; FlowDetail layers FlowVersions on top.
func busyFlows(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.Flows.Busy() {
		return "syncing flows…"
	}
	return ""
}
func busyFlowDetail(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.Flows.Busy() {
		return "syncing flows…"
	}
	if d.FlowCur != "" {
		if r, ok := d.FlowVersions[d.FlowCur]; ok && r.Busy() {
			return "loading versions…"
		}
	}
	return ""
}
func errFlows(_ Model, d *orgData) string {
	if d != nil && d.Flows.Err() != nil {
		return d.Flows.Err().Error()
	}
	return ""
}
func errFlowDetail(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.Flows.Err() != nil {
		return d.Flows.Err().Error()
	}
	if d.FlowCur != "" {
		if r, ok := d.FlowVersions[d.FlowCur]; ok && r.Err() != nil {
			return r.Err().Error()
		}
	}
	return ""
}

// busyReports / errReports — Reports list + ReportDetail's run.
func busyReports(_ Model, d *orgData) string {
	if d != nil && d.Reports.Busy() {
		return "syncing reports…"
	}
	return ""
}
func busyReportDetail(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.Reports.Busy() {
		return "syncing reports…"
	}
	if d.ReportCur != "" {
		if r, ok := d.ReportRuns[d.ReportCur]; ok && r.Busy() {
			return "running report…"
		}
	}
	return ""
}
func errReports(_ Model, d *orgData) string {
	if d != nil && d.Reports.Err() != nil {
		return d.Reports.Err().Error()
	}
	return ""
}
func errReportDetail(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.Reports.Err() != nil {
		return d.Reports.Err().Error()
	}
	if d.ReportCur != "" {
		if r, ok := d.ReportRuns[d.ReportCur]; ok && r.Err() != nil {
			return r.Err().Error()
		}
	}
	return ""
}

// busyRecordDetail / errRecordDetail — single-record drill-in.
func busyRecordDetail(_ Model, d *orgData) string {
	if d == nil || d.RecordDetailCur == "" {
		return ""
	}
	if r, ok := d.RecordDetails[d.RecordDetailCur]; ok && r.Busy() {
		return "loading record…"
	}
	return ""
}
func errRecordDetail(_ Model, d *orgData) string {
	if d == nil || d.RecordDetailCur == "" {
		return ""
	}
	if r, ok := d.RecordDetails[d.RecordDetailCur]; ok && r.Err() != nil {
		return r.Err().Error()
	}
	return ""
}

// busy/errSystemLogs / busy/errSystemDeploys — per-subtab on TabSystem.
func busySystemLogs(_ Model, d *orgData) string {
	if d != nil && d.ApexLogs.Busy() {
		return "syncing logs…"
	}
	return ""
}
func busySystemDeploys(_ Model, d *orgData) string {
	if d != nil && d.Deploys.Busy() {
		return "syncing deploys…"
	}
	return ""
}
func errSystemLogs(_ Model, d *orgData) string {
	if d != nil && d.ApexLogs.Err() != nil {
		return d.ApexLogs.Err().Error()
	}
	return ""
}
func errSystemDeploys(_ Model, d *orgData) string {
	if d != nil && d.Deploys.Err() != nil {
		return d.Deploys.Err().Error()
	}
	return ""
}

// busySOQL / errSOQL — modal-level state, not per-org.
func busySOQL(m Model, _ *orgData) string {
	if m.soqlRunning {
		return "running query…"
	}
	return ""
}
func errSOQL(m Model, _ *orgData) string {
	if m.soqlErr != nil {
		return m.soqlErr.Error()
	}
	return ""
}

// busyRecords — TabRecords' chip resource (records-mode) or
// SObjects sync (picker mode).
func busyRecords(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.RecordsSObjectCur != "" {
		if activeChipBusy(d, d.RecordsSObjectCur) {
			if currentChipMode(d, d.RecordsSObjectCur) == ChipModeSalesforce {
				return "fetching list view results…"
			}
			return "fetching " + d.RecordsSObjectCur + " records…"
		}
		return ""
	}
	if d.SObjects.Busy() {
		return "syncing sobjects…"
	}
	return ""
}
func errRecords(_ Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.SObjects.Err() != nil {
		return d.SObjects.Err().Error()
	}
	if d.RecordsSObjectCur != "" {
		if r, ok := d.Records[d.RecordsSObjectCur]; ok && r.Err() != nil {
			return r.Err().Error()
		}
	}
	return ""
}
