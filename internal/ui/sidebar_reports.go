package ui

import "fmt"

// Per-surface sidebars for /reports: report, dashboard, report
// type, and cached-report-run detail. Split out of sidebar.go.

func (m Model) sidebarReport(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil {
		return sideEmpty("—")
	}
	r, ok := d.ReportList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"id", r.ID},
		{"folder", dashIfEmpty(r.FolderName)},
		{"format", dashIfEmpty(r.Format)},
		{"owner", dashIfEmpty(r.Owner)},
	}
	if r.LastRunDate != "" {
		rows = append(rows, kv{"last run", prettyDate(r.LastRunDate)})
	}
	extra := []string{}
	if r.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(r.Description, inner-2), inner))
	}
	extra = append(extra, "", sideDim("  ↵ preview cached run", inner))
	title := r.Name
	if title == "" {
		title = "Report"
	}
	return renderKVPanel(inner, title, rows, extra...)
}

func (m Model) sidebarDashboard(inner int) string {
	d := m.activeOrgData()
	if d == nil {
		return sideEmpty("—")
	}
	row, ok := d.DashboardList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	runAs := "viewer"
	if row.Type == "SpecifiedUser" {
		runAs = "fixed user"
	}
	rows := []kv{
		{"id", row.ID},
		{"api name", dashIfEmpty(row.DeveloperName)},
		{"folder", dashIfEmpty(row.FolderName)},
		{"run as", runAs},
	}
	if row.NamespacePrefix != "" {
		rows = append(rows, kv{"namespace", row.NamespacePrefix})
	}
	if row.LastModifiedDate != "" {
		rows = append(rows, kv{"modified", prettyDate(row.LastModifiedDate)})
	}
	if row.LastModifiedByName != "" {
		rows = append(rows, kv{"by", row.LastModifiedByName})
	}
	extra := []string{}
	if row.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(row.Description, inner-2), inner))
	}
	extra = append(extra, "", sideDim("  "+firstPretty(Keys.OpenDefault)+" open in Lightning", inner))
	title := row.Title
	if title == "" {
		title = "Dashboard"
	}
	return renderKVPanel(inner, title, rows, extra...)
}

func (m Model) sidebarReportType(inner int) string {
	d := m.activeOrgData()
	if d == nil {
		return sideEmpty("—")
	}
	row, ok := d.ReportTypeList.Selected()
	if !ok {
		return sideEmpty("no matches")
	}
	rows := []kv{
		{"api name", row.Type},
		{"category", dashIfEmpty(row.Category)},
		{"custom", yesNo(row.Custom)},
		{"joined", yesNo(row.SupportsJoined)},
	}
	extra := []string{}
	if row.Description != "" {
		extra = append(extra, "", sideSection("description"),
			sideDim("  "+wrap(row.Description, inner-2), inner))
	}
	extra = append(extra, "", sideDim("  "+firstPretty(Keys.OpenDefault)+" open Setup · Report Types", inner))
	title := row.Label
	if title == "" {
		title = "Report Type"
	}
	return renderKVPanel(inner, title, rows, extra...)
}

func (m Model) sidebarReportRun(inner int) string {
	o, ok := m.currentOrg()
	if !ok {
		return sideEmpty("no org")
	}
	d := m.data[o.Username]
	if d == nil || d.ReportCur == "" {
		return sideEmpty("—")
	}
	r, ok := d.ReportRuns[d.ReportCur]
	if !ok || r.FetchedAt().IsZero() {
		return sideEmpty("loading…")
	}
	run := r.Value()
	rows := []kv{
		{"rows", fmt.Sprintf("%d", len(run.Rows))},
		{"format", dashIfEmpty(run.Format)},
	}
	if !run.AllData {
		rows = append(rows, kv{"capped", "yes (2000)"})
	}
	if !run.RanAt.IsZero() {
		rows = append(rows, kv{"ran at", run.RanAt.Format("15:04:05")})
	}
	var extra []string
	if len(run.Aggregates) > 0 {
		extra = append(extra, "", sideSection("aggregates"))
		for label, val := range run.Aggregates {
			extra = append(extra, sideKV(label, fmt.Sprintf("%v", val), inner))
		}
	}
	extra = append(extra, "", sideDim(
		"  "+firstPretty(Keys.OpenDefault)+" → open in Lightning · "+
			firstPretty(Keys.Refresh)+" refresh", inner))
	title := run.Name
	if title == "" {
		title = "Report"
	}
	return renderKVPanel(inner, title, rows, extra...)
}
