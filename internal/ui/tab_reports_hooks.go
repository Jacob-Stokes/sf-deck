package ui

import (
	tea "charm.land/bubbletea/v2"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// onReportsBrowser reports whether the active surface is the /reports
// folder browser (the Reports subtab) — NOT the Dashboards / Report
// Types list subtabs. Every bespoke /reports behaviour (folder Esc
// pop, F pin, x export, P project mode, Enter drill, the o
// last-export shortcut) must gate on this, not on the tab alone,
// or it hijacks keys on the sibling subtabs.
func (m Model) onReportsBrowser() bool {
	return m.tab() == TabReports && m.currentSubtab() == SubtabReportsReports
}

func (m *Model) activateReports() tea.Cmd {
	if !m.onReportsBrowser() {
		return nil
	}
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.ReportFolders == nil {
		return nil
	}
	subs, reps := m.visibleReportsItems()
	row := m.reportsRowCursor()
	if row < len(subs) {
		if s := d.ReportList.SearchPtr(); s.Active || s.Committed {
			s.Active = false
			s.Committed = false
			s.SetBuffer("")
		}
		d.ReportFolders.Drill(subs[row])
		return nil
	}
	reportIdx := row - len(subs)
	if reportIdx < 0 || reportIdx >= len(reps) {
		return nil
	}
	d.ReportCur = reps[reportIdx].ID
	if s := d.ReportList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	m.setTab(TabReportDetail)
	return m.onTabChanged()
}

func (m *Model) ensureReportsData(d *orgData, o sf.Org) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabReportsDashboards:
		return d.Dashboards.Ensure(m.cache)
	case SubtabReportsReportTypes:
		return d.ReportTypes.Ensure(m.cache)
	}
	cmds := []tea.Cmd{d.Reports.Ensure(m.cache)}
	_, loadCmd := d.EnsureReportFolders(targetArg(o), m.settings)
	if loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return tea.Batch(cmds...)
}

func (m Model) refreshReportsData(d *orgData) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabReportsDashboards:
		return d.Dashboards.Refresh(m.cache)
	case SubtabReportsReportTypes:
		return d.ReportTypes.Refresh(m.cache)
	}
	cmds := []tea.Cmd{d.Reports.Refresh(m.cache)}
	if d.ReportFoldersSrc != nil {
		if loadFn := d.ReportFoldersSrc.Refresh(); loadFn != nil {
			cmds = append(cmds, func() tea.Msg { return loadFn() })
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) reportsSearchPtr() *searchState {
	if d := m.activeOrgData(); d != nil {
		return d.ReportList.SearchPtr()
	}
	return nil
}

func (m *Model) moveReportsCursor(delta int) {
	d := m.activeOrgData()
	if d == nil || d.ReportFolders == nil {
		return
	}
	subs, reps := m.visibleReportsItems()
	total := len(subs) + len(reps)
	key := d.ReportFolders.Path().CurrentID()
	if d.ReportsProjectMode {
		key = "__project__"
	}
	d.Cursors.Move(cursorKindReportRow, delta, total, key)
}

func (m *Model) resetReportsCursor() {
	d := m.activeOrgData()
	if d == nil || d.ReportFolders == nil {
		return
	}
	key := d.ReportFolders.Path().CurrentID()
	if d.ReportsProjectMode {
		key = "__project__"
	}
	d.Cursors.Reset(cursorKindReportRow, key)
}

func (m *Model) resetReportDetailCursor() {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.ReportCur == "" {
		return
	}
	r, ok := d.ReportRuns[d.ReportCur]
	if !ok {
		return
	}
	rows := r.Value().Rows
	if len(rows) == 0 {
		d.Cursors.Reset(cursorKindReportRow, d.ReportCur)
		return
	}
	cols := buildReportRunCols(r.Value().Columns, rows)
	cell := reportRunCell(rows, cols)
	reportRunTableAdapter(m, d, d.ReportCur, r.FetchedAt().UnixNano(), rows, cols, cell).ResetDisplayTop()
}

func (m *Model) moveReportDetailCursor(delta int) {
	if len(m.orgs) == 0 {
		return
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.ReportCur == "" {
		return
	}
	r, ok := d.ReportRuns[d.ReportCur]
	if !ok {
		return
	}
	run := r.Value()
	rows := run.Rows
	if len(rows) == 0 {
		return
	}
	cols := buildReportRunCols(run.Columns, rows)
	cell := reportRunCell(rows, cols)
	reportRunTableAdapter(m, d, d.ReportCur, r.FetchedAt().UnixNano(), rows, cols, cell).MoveDisplay(delta)
}

func (m *Model) ensureReportDetailData(d *orgData, o sf.Org) tea.Cmd {
	cmds := []tea.Cmd{d.Reports.Ensure(m.cache)}
	if d.ReportCur != "" {
		r := d.EnsureReportRun(targetArg(o), d.ReportCur)
		cmds = append(cmds, r.Ensure(m.cache))
	}
	return tea.Batch(cmds...)
}

func (m Model) refreshReportDetailData(d *orgData) tea.Cmd {
	if d.ReportCur != "" {
		if r, ok := d.ReportRuns[d.ReportCur]; ok {
			return r.Refresh(m.cache)
		}
	}
	return nil
}

// activateReportDetail handles Enter on a report-run row. When the
// row carries a Salesforce Id we can resolve to an sObject (via
// the cached SObjects keyPrefix table), drill into it through the
// canonical record-detail surface so the user gets all the same
// chrome (KV grid, sidebar actions, recent-tracking) they get from
// /records or /recent. Returns to TabReportDetail on Esc.
//
// No-op (with a flash) when the row has no Id column, when the
// prefix doesn't resolve to a known sObject, or when SObjects
// haven't loaded yet — drilling without a target sObject would be
// a worse UX than not drilling at all.
func (m *Model) activateReportDetail() tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	d := m.ensureOrgData(m.orgs[m.selected].Username)
	if d.ReportCur == "" {
		return nil
	}
	r, ok := d.ReportRuns[d.ReportCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	run := r.Value()
	if len(run.Rows) == 0 {
		return nil
	}
	cols := buildReportRunCols(run.Columns, run.Rows)
	cell := reportRunCell(run.Rows, cols)
	adapter := reportRunTableAdapter(m, d, d.ReportCur, r.FetchedAt().UnixNano(), run.Rows, cols, cell)
	idx, ok := adapter.RawAtDisplay(adapter.DisplayCursor())
	if !ok || idx < 0 || int(idx) >= len(run.Rows) {
		return nil
	}
	row := run.Rows[idx]
	id := reportRowID(row)
	if id == "" {
		m.flash("no Id column in this report — can't drill")
		return nil
	}
	sobject := resolveSObjectFromID(d, id)
	if sobject == "" {
		m.flash("can't resolve sObject for Id " + id + " (sObjects not loaded yet?)")
		return nil
	}
	name := reportRowDisplayName(row, run.Columns, id)
	return m.triggerRecordDrill(sobject, id, name, TabReportDetail)
}

// reportRowID extracts the Salesforce 15/18-char record ID from a
// report-run row, if present. Reports include the Id column when
// configured to (most tabular reports do); summary / aggregate
// reports usually don't. Returns "" when no usable Id is found.
func reportRowID(row map[string]any) string {
	if row == nil {
		return ""
	}
	// Prefer the literal "Id" key — that's what the parser writes
	// when the report's detailColumns contains "Id".
	if v, ok := row["Id"]; ok {
		if s, _ := v.(string); s != "" && (len(s) == 15 || len(s) == 18) {
			return s
		}
	}
	return ""
}

// reportRowDisplayName picks a user-facing label for a row. Tries
// "Name" first (most reports include it on tabular projections),
// then walks the column list for anything that looks named-shaped
// (Subject, Title, CaseNumber). Falls back to the Id when nothing
// readable is found — better than empty.
func reportRowDisplayName(row map[string]any, cols []sf.ReportColumn, id string) string {
	if row == nil {
		return id
	}
	for _, candidate := range []string{"Name", "Subject", "CaseNumber", "Title"} {
		if v, ok := row[candidate]; ok {
			if s, _ := v.(string); s != "" {
				return s
			}
		}
	}
	// Walk the projection in declared order — first non-Id column
	// with a string value wins. Catches custom-projection reports.
	for _, c := range cols {
		if c.APIName == "Id" {
			continue
		}
		if v, ok := row[c.APIName]; ok {
			if s, _ := v.(string); s != "" {
				return s
			}
		}
	}
	return id
}

// resolveSObjectFromID returns the sObject API name a Salesforce Id
// belongs to, by looking up its 3-char key prefix in the cached
// SObjects describe list. Returns "" when SObjects haven't been
// fetched or when the prefix doesn't match anything (custom orgs
// can have prefixes for sObjects we haven't crawled).
func resolveSObjectFromID(d *orgData, id string) string {
	if d == nil || len(id) < 3 {
		return ""
	}
	prefix := id[:3]
	prefixMap := buildPrefixMap(d)
	if prefixMap == nil {
		return ""
	}
	return prefixMap[prefix]
}

// reportDetailFetchedAt resolves the /report drill's primary
// freshness stamp: the cached run when one exists, else the report
// list (extracted in the registry-purity pass).
func reportDetailFetchedAt(m Model, d *orgData) time.Time {
	if d.ReportCur != "" {
		if r, ok := d.ReportRuns[d.ReportCur]; ok {
			return r.FetchedAt()
		}
	}
	return d.Reports.FetchedAt()
}
