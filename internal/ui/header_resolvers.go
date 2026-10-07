package ui

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

func busyObjectDetail(m Model, d *orgData) string {
	if d == nil {
		return ""
	}
	if d.SObjects.Busy() {
		return "syncing sobjects…"
	}
	if d.DescribeCur != "" {
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
