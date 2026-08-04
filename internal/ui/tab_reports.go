package ui

// /reports list + drill-in.
//
// Phase 1 surface: read-only browse + cached preview. Each row is a
// saved report; Enter drills into the cached run and shows the detail
// rows + grand total. `r` re-pulls the cached run; `R`/export are
// reserved for later phases.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// renderReports draws the folder-aware reports browser. Top of pane
// is a breadcrumb (the treechip strip) showing the current folder
// path + pinned-favourite folders. Main pane shows two sections:
//
//	SUBFOLDERS — direct children of the current folder, drillable.
//	REPORTS    — reports whose OwnerId == current folder's Id.
//
// Search and cursor scope is the CURRENT folder only — never the
// whole 12k report set. This keeps `/` responsive on big orgs and
// matches what users expect from a filesystem-style browser.
//
// At the synthetic root the SUBFOLDERS section lists root folders;
// REPORTS shows reports with a missing/unknown folder. Press Enter
// on a folder row to drill; Enter on a report row drills into the
// report detail. Esc / Up cursor pops the path.
//
// Subtab dispatch is centralised via dispatchSubtab — the strip is
// drawn for every branch so shift+1..3 always works. Reports is the
// folder browser; Dashboards and Report Types are shared-list-engine
// surfaces (chips, sort, columns, widths all come for free).
func (m Model) renderReports(w, innerH int) string {
	return m.dispatchSubtab(w, innerH, reportsSubtabs(), m.reportsSubtab(),
		map[Subtab]subtabBranch{
			SubtabReportsDashboards:  {Render: m.renderReportsDashboards},
			SubtabReportsReportTypes: {Render: m.renderReportsReportTypes},
		},
		subtabBranch{Render: m.renderReportsDefault},
	)
}

// renderReportsDashboards is the Dashboards subtab body — chip strip
// plus the shared list-table renderer.
func (m Model) renderReportsDashboards(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainDashboards, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.dashboardsChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.Dashboards.FetchedAt().IsZero() {
		if d.Dashboards.Busy() {
			lines = append(lines, dimLine("  loading dashboards…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load dashboards", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := dashboardsListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// renderReportsReportTypes is the Report Types subtab body — same
// shape as Dashboards over the analytics reportTypes catalogue.
func (m Model) renderReportsReportTypes(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return theme.Subtle.Render("  org data not loaded")
	}
	chips := m.stripRows(domainReportTypes, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.reportTypesChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.ReportTypes.FetchedAt().IsZero() {
		if d.ReportTypes.Busy() {
			lines = append(lines, dimLine("  loading report types…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load report types", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok := reportTypesListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// renderReportsDefault is the Reports-subtab body — the folder-tree
// + report-list view. Receives the budget AFTER the subtab strip is
// drawn (dispatchSubtab handles the strip).
func (m Model) renderReportsDefault(w, innerH int) string {
	inner := w - 4

	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	d := m.ensureOrgDataRef(o.Username)

	// Lazy: registry might not exist on first paint if EnsureData
	// hasn't fired yet. Show a loading state cleanly.
	if d.ReportFolders == nil {
		return theme.Subtle.Render("  initialising folders…")
	}
	reg := d.ReportFolders
	if d.ReportFoldersSrc != nil && d.ReportFoldersSrc.Loading() {
		return theme.Subtle.Render("  loading folders (this can take a few seconds on big orgs)…")
	}
	if d.ReportFoldersSrc != nil && d.ReportFoldersSrc.LoadErr() != nil {
		return redLine("  folders failed to load: " + d.ReportFoldersSrc.LoadErr().Error())
	}

	// Resolve current path's items.
	main, mainErr := reg.MainModel()
	if mainErr != nil {
		return redLine("  " + mainErr.Error())
	}
	curID := reg.Path().CurrentID()
	folderReports := filterReportsByFolder(d.ReportList.Items(), curID)

	// Apply the local search query — scoped to subnodes + folder
	// reports only. Search is case-insensitive substring on the
	// label / report name. Empty query = no filter.
	q := strings.ToLower(strings.TrimSpace(d.ReportList.Search.Buffer()))
	matchSubnodes := main.Subnodes
	matchReports := folderReports
	if q != "" {
		matchSubnodes = matchSubnodes[:0:0]
		for _, n := range main.Subnodes {
			if strings.Contains(strings.ToLower(n.Label), q) {
				matchSubnodes = append(matchSubnodes, n)
			}
		}
		matchReports = matchReports[:0:0]
		for _, r := range folderReports {
			if strings.Contains(strings.ToLower(r.Name), q) {
				matchReports = append(matchReports, r)
			}
		}
	}

	// Header bar. Subtab strip is drawn upstream by dispatchSubtab;
	// here we only own the folder strip + the per-folder header
	// row + search bar.
	var lines []string
	stripLine := m.renderReportFolderStrip(reg, inner)
	if stripLine != "" {
		lines = append(lines, stripLine)
	}
	curFolderLabel := "All folders"
	if reg.Path().Depth() > 0 {
		curFolderLabel = reg.Path().Nodes[len(reg.Path().Nodes)-1].Label
	}
	headerLabel := fmt.Sprintf("REPORTS · %s · %d folders · %d reports · %s",
		curFolderLabel,
		len(matchSubnodes), len(matchReports),
		humanAge(d.Reports.FetchedAt())+stateSuffix(d.Reports.Busy(), d.Reports.Err()))
	lines = append(lines,
		headerWithSearchPill(headerLabel, d.ReportList.Search))
	lines = append(lines, searchBar(d.ReportList.Search, inner))

	// Special "still loading reports" state — folder is non-empty by
	// folder count, but the report SOQL hasn't returned yet so the
	// reports section would otherwise be silently empty.
	reportsLoading := d.Reports.Busy() ||
		(d.ReportList.Len() == 0 && d.Reports.FetchedAt().IsZero())

	// Build a flat row list for the viewport renderer. Section
	// headers become "ghost" rows the cursor skips. Folders come
	// first, then reports.
	type mainRow struct {
		header string // when set, render as section header (non-selectable)
		folder *treechip.TreeNode
		report *sf.ReportSummary
	}
	var rows []mainRow
	// Mappings between visible-row index and selectable-item index.
	selectableToRow := []int{}
	if len(matchSubnodes) > 0 {
		rows = append(rows, mainRow{header: fmt.Sprintf("SUBFOLDERS · %d", len(matchSubnodes))})
		for i := range matchSubnodes {
			selectableToRow = append(selectableToRow, len(rows))
			n := matchSubnodes[i]
			rows = append(rows, mainRow{folder: &n})
		}
	}
	if len(matchReports) > 0 {
		if len(rows) > 0 {
			rows = append(rows, mainRow{header: ""}) // blank spacer
		}
		rows = append(rows, mainRow{header: fmt.Sprintf("REPORTS · %d", len(matchReports))})
		for i := range matchReports {
			selectableToRow = append(selectableToRow, len(rows))
			r := matchReports[i]
			rows = append(rows, mainRow{report: &r})
		}
	}

	// Empty-state messaging when no rows survived the filter.
	if len(rows) == 0 {
		switch {
		case reportsLoading:
			lines = append(lines, theme.Subtle.Render(
				"  loading reports… (full org list, takes a few seconds)"))
		case d.ReportsProjectMode:
			lines = append(lines, theme.Subtle.Render(m.projectEmptyHint("reports")))
		case q != "":
			lines = append(lines, theme.Subtle.Render("  no matches in this folder"))
		default:
			lines = append(lines, theme.Subtle.Render("  empty folder"))
		}
		return strings.Join(lines, "\n")
	}

	// Selectable cursor (over subfolders+reports, not headers).
	totalSelectable := len(matchSubnodes) + len(matchReports)
	sel := m.reportsRowCursor()
	if sel >= totalSelectable {
		sel = 0
	}
	// Convert selectable index → flat-row index for highlight.
	selRowIdx := -1
	if sel >= 0 && sel < len(selectableToRow) {
		selRowIdx = selectableToRow[sel]
	}

	// Pre-style table headers / rows so renderRows can shuffle them
	// without re-doing the work.
	folderStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	reportCols := []tableColumn{
		{Header: "NAME", Width: -1, Style: folderStyle},
		{Header: "FORMAT", Width: 10, Style: lipgloss.NewStyle().Foreground(theme.Cyan)},
		{Header: "LAST RUN", Width: 16, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
	}
	reportTerms := m.searchTerms()

	renderRow := func(i int) string {
		row := rows[i]
		switch {
		case row.header != "" || (row.header == "" && row.folder == nil && row.report == nil):
			if row.header == "" {
				return ""
			}
			return sectionTitle(row.header)
		case row.folder != nil:
			selected := i == selRowIdx
			prefix := "  "
			if selected {
				prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render("▌") + " "
			}
			label := "📁 " + row.folder.Label
			return prefix + uilayout.HighlightInStyle(label, reportTerms, folderStyle)
		case row.report != nil:
			selected := i == selRowIdx
			rowCols := make([]tableColumn, len(reportCols))
			copy(rowCols, reportCols)
			return uilayout.RenderInteractiveTableRowHighlight(rowCols, []string{
				row.report.Name,
				strings.ToLower(dashIfEmpty(row.report.Format)),
				prettyDate(row.report.LastRunDate),
			}, selected, m.focus == focusMain, inner, reportTerms)
		}
		return ""
	}

	// Reserve = chrome before the row block; trailing = bottom hint.
	const trailing = 2 // blank line + hint line
	lines = append(lines, renderRows(
		len(rows), selRowIdx, innerH, len(lines), trailing, inner,
		renderRow)...)
	hint := "  ↵ open · F pin folder · esc up one level · / search"
	if scope := m.activeScope(); scope.Loaded() {
		hint += " · " + firstPretty(Keys.ToggleProjectMode) + " project mode"
	}
	lines = append(lines, "", dimLine(hint, inner))
	return strings.Join(lines, "\n")
}

// renderReportFolderStrip composes the breadcrumb chip strip from
// the registry's StripModel. Reuses the existing chip-pill widget
// shape so it's visually consistent with the qchip strips on
// /records / /objects / /flows.
//
// When the active org has a project loaded, a synthetic 📁 pin is
// prepended (active when ReportsProjectMode is on) — same idea as
// the auto-pinned project chip on flat surfaces, just shaped for
// the breadcrumb model.
func (m Model) renderReportFolderStrip(reg *treechip.Registry, width int) string {
	sm := reg.StripModel()
	var parts []string
	scope := m.activeScope()
	d := m.activeOrgData()
	projectActive := scope.Loaded() && d != nil && d.ReportsProjectMode
	if scope.Loaded() {
		parts = append(parts, renderTreeBreadcrumbChip("📁 "+scope.ProjectName, projectActive))
		sep := lipgloss.NewStyle().Foreground(theme.Border).Render("  │  ")
		parts = append(parts, sep)
	}
	// Always-on "All" chip = path = []. Active when at root and not
	// in project mode.
	allActive := !projectActive && len(sm.Breadcrumb) == 0
	parts = append(parts, renderTreeBreadcrumbChip("All", allActive))
	for i, n := range sm.Breadcrumb {
		active := !projectActive && i == len(sm.Breadcrumb)-1
		parts = append(parts, renderTreeBreadcrumbChip("▸ "+n.Label, active))
	}
	// Pinned favourites (after a separator).
	if len(sm.Pins) > 0 {
		sep := lipgloss.NewStyle().Foreground(theme.Border).Render("  │  ")
		var pinChips []string
		for _, p := range sm.Pins {
			active := !projectActive && p.ID == sm.CurrentID
			pinChips = append(pinChips, renderTreeBreadcrumbChip("★ "+p.Label, active))
		}
		parts = append(parts, sep+strings.Join(pinChips, " "))
	}
	bar := strings.Join(parts, " ")
	if lipglossWidth(bar) > width {
		bar = ansiTruncate(bar, width, "…")
	}
	return bar
}

func renderTreeBreadcrumbChip(label string, active bool) string {
	if active {
		return lipgloss.NewStyle().
			Foreground(theme.Fg).
			Background(theme.Blue).
			Bold(true).
			Padding(0, 1).
			Render(label)
	}
	return lipgloss.NewStyle().
		Foreground(theme.Muted).
		Padding(0, 1).
		Render(label)
}

// filterReportsByFolder returns the subset of reports whose OwnerId
// (== Folder.Id) matches the current folder. At the root (folderID
// = ""), returns reports with no recognised folder — orphans.
func filterReportsByFolder(reports []sf.ReportSummary, folderID string) []sf.ReportSummary {
	out := reports[:0:0]
	for _, r := range reports {
		if r.FolderID == folderID {
			out = append(out, r)
		}
	}
	return out
}

// visibleReportsItems returns the (subfolders, reports) lists at the
// current folder AFTER the active search filter is applied. Single
// source of truth for cursor logic + Enter dispatch + the renderer
// — keeps the three paths in lockstep so cursor never points off
// the visible rows.
func (m Model) visibleReportsItems() ([]treechip.TreeNode, []sf.ReportSummary) {
	d := m.activeOrgData()
	if d == nil || d.ReportFolders == nil {
		return nil, nil
	}
	q := strings.ToLower(strings.TrimSpace(d.ReportList.Search.Buffer()))
	// Project-mode short-circuits folder semantics: report list is the
	// loaded project's collected reports for this org, no subfolders.
	scope := m.activeScope()
	if d.ReportsProjectMode && scope.Loaded() {
		all := d.ReportList.Items()
		reps := make([]sf.ReportSummary, 0, len(scope.ReportIDs))
		for _, r := range all {
			if !scope.ReportIDs[r.ID] {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(r.Name), q) {
				continue
			}
			reps = append(reps, r)
		}
		return nil, reps
	}
	main, _ := d.ReportFolders.MainModel()
	folderID := d.ReportFolders.Path().CurrentID()
	folderReports := filterReportsByFolder(d.ReportList.Items(), folderID)
	if q == "" {
		return main.Subnodes, folderReports
	}
	subs := main.Subnodes[:0:0]
	for _, n := range main.Subnodes {
		if strings.Contains(strings.ToLower(n.Label), q) {
			subs = append(subs, n)
		}
	}
	reps := folderReports[:0:0]
	for _, r := range folderReports {
		if strings.Contains(strings.ToLower(r.Name), q) {
			reps = append(reps, r)
		}
	}
	return subs, reps
}

// reportsRowCursor returns the unified row cursor over the combined
// (visible subfolders + visible reports) list at the current folder.
// Stored on the orgData cursor store keyed by folder id so
// re-drilling restores position. Search-filtered cursor is NOT
// keyed by query — clearing the search puts you back where you
// were before typing.
func (m Model) reportsRowCursor() int {
	d := m.activeOrgData()
	if d == nil || d.ReportFolders == nil {
		return 0
	}
	subs, reps := m.visibleReportsItems()
	total := len(subs) + len(reps)
	key := d.ReportFolders.Path().CurrentID()
	if d.ReportsProjectMode {
		key = "__project__"
	}
	return d.Cursors.Get(cursorKindReportRow, total, key)
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func ansiTruncate(s string, width int, suffix string) string {
	return ansi.Truncate(s, width, suffix)
}

// renderReportDetail draws the cached preview for the drilled-in report
// — the grand-total bucket's rows + any aggregate the SF run returned.
// Joined ("MultiBlock") reports surface as "open in SF" only since the
// row shape differs from tabular/summary/matrix.
func (m Model) renderReportDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.ReportCur == "" {
		return theme.Subtle.Render("  press enter on a report in /reports first")
	}

	// Header from the catalogue row.
	var summary sf.ReportSummary
	for _, r := range d.Reports.Value() {
		if r.ID == d.ReportCur {
			summary = r
			break
		}
	}

	var lines []string
	if summary.Name != "" {
		lines = append(lines, sectionTitle(summary.Name))
		meta := []string{}
		if summary.FolderName != "" {
			meta = append(meta, "folder "+summary.FolderName)
		}
		if summary.Format != "" {
			meta = append(meta, "format "+strings.ToLower(summary.Format))
		}
		if summary.Owner != "" {
			meta = append(meta, "owner "+summary.Owner)
		}
		if summary.LastRunDate != "" {
			meta = append(meta, "last run "+prettyDate(summary.LastRunDate))
		}
		if len(meta) > 0 {
			lines = append(lines, dimLine("  "+strings.Join(meta, " · "), inner))
		}
		if summary.Description != "" {
			lines = append(lines, dimLine("  "+summary.Description, inner))
		}
		lines = append(lines, "")
	}

	// MultiBlock = SF's term for joined reports. Phase 1 doesn't try to
	// render those — the row shape diverges per block; surface as
	// "open in Lightning" only.
	if strings.EqualFold(summary.Format, "MultiBlock") {
		lines = append(lines, theme.Subtle.Render("  joined reports aren't previewed here yet"))
		lines = append(lines, m.footerHint("  "+firstPretty(Keys.OpenDefault)+" → open in Lightning", inner))
		return strings.Join(lines, "\n")
	}

	r, ok := d.ReportRuns[d.ReportCur]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			lines = append(lines, theme.Subtle.Render("  running…"))
		} else if r != nil && r.Err() != nil {
			lines = append(lines, redLine("  "+r.Err().Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching cached run…"))
		}
		return strings.Join(lines, "\n")
	}
	run := r.Value()

	header := fmt.Sprintf("ROWS · %d", len(run.Rows))
	if !run.AllData {
		// SF caps the sync endpoint at 2000 detail rows. Surface that
		// inline so users know to fall back to xlsx export when more
		// matters (export reuses the SF "give me everything" path).
		header += " · capped at 2000"
	}
	lines = append(lines, sectionTitle(header)+stateSuffix(r.Busy(), r.Err()))

	if len(run.Rows) == 0 {
		// Aggregate-only result (e.g. summary report grouped by Stage)
		// or genuinely empty. Either way, render whatever totals came
		// back so the user isn't staring at "no data" when there's a
		// grand total available.
		if len(run.Aggregates) > 0 {
			lines = append(lines, "", sectionTitle("AGGREGATES"))
			for label, val := range run.Aggregates {
				lines = append(lines, dimLine(fmt.Sprintf("  %s · %v", label, val), inner))
			}
		} else {
			lines = append(lines, theme.Subtle.Render("  no rows"))
		}
		lines = append(lines, "", dimLine(
			"  "+firstPretty(Keys.OpenDefault)+" → open in Lightning · "+
				firstPretty(Keys.Refresh)+" refresh", inner))
		return strings.Join(lines, "\n")
	}

	// Hand off the table block to the shared renderer. The
	// orchestrator above keeps the report-specific chrome (header
	// title block + aggregate fallback for empty runs); the
	// per-frame model carries everything the renderer needs.
	listCols := buildReportRunCols(run.Columns, run.Rows)
	rows := run.Rows
	cell := reportRunCell(rows, listCols)
	emptySearch := &searchState{}
	// If the report's projection includes "Id", users can drill into
	// the underlying record from any row. Surface this in the footer
	// hint; suppress when no Id column exists (drill would no-op
	// anyway, no point advertising it).
	footer := firstPretty(Keys.ReportExport) + " export"
	if reportHasIDColumn(run.Columns) {
		footer = firstPretty(Keys.OpenDefault) + " open record · " + footer
	}
	sortDataKey := reportRunSortDataKey(d.ReportCur, r.FetchedAt().UnixNano(), len(rows), len(listCols))
	adapter := reportRunTableAdapter(&m, d, d.ReportCur, r.FetchedAt().UnixNano(), rows, listCols, cell)
	// listRenderModel.Cursor is a display-space int; convert at the
	// render boundary.
	cursor := int(adapter.DisplayCursor())
	model := listRenderModel{
		Title:        "ROWS · " + fmt.Sprintf("%d", len(rows)) + reportCappedSuffix(run.AllData),
		State:        &m.reportRunTable,
		Search:       emptySearch,
		Cols:         listCols,
		N:            len(rows),
		Cursor:       cursor,
		Cell:         cell,
		FooterExtras: footer,
		// Same shape as SOQL: report runs swap wholesale on
		// re-execute. Row + column count as a coarse signature.
		DataVersion: listVersionWithStore(len(rows)*1009+len(listCols)*7, m),
		SortDataKey: sortDataKey,
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

func reportRunSortDataKey(reportID string, fetchedAt int64, rows, cols int) string {
	return fmt.Sprintf("report:%s|%d|%d|%d", reportID, fetchedAt, rows, cols)
}

func reportRunCell(rows []map[string]any, cols []uilayout.ListColumn) func(row, col int) string {
	return func(row, col int) string {
		if row < 0 || row >= len(rows) || col < 0 || col >= len(cols) {
			return ""
		}
		return stringifyReportCell(rows[row][cols[col].Name])
	}
}

func reportRunTableAdapter(
	m *Model,
	d *orgData,
	reportID string,
	fetchedAt int64,
	rows []map[string]any,
	cols []uilayout.ListColumn,
	cell func(row, col int) string,
) tableRowAdapter {
	var state *uilayout.ListTableState
	if m != nil {
		state = &m.reportRunTable
	}
	return tableRowAdapter{
		State:   state,
		Cols:    cols,
		N:       len(rows),
		Cell:    cell,
		DataKey: reportRunSortDataKey(reportID, fetchedAt, len(rows), len(cols)),
		RawCursor: func() RawRow {
			if d == nil {
				return 0
			}
			return RawRow(d.Cursors.Get(cursorKindReportRow, len(rows), reportID))
		},
		SetRawCursor: func(raw RawRow) {
			if d != nil {
				d.Cursors.Set(cursorKindReportRow, int(raw), len(rows), reportID)
			}
		},
	}
}

// reportCappedSuffix appends the "capped at 2000" warning for report
// runs where SF's sync endpoint truncated the result. The Title
// field on the render model carries it so the user sees the cap
// inline with the row count.
func reportCappedSuffix(allData bool) string {
	if allData {
		return ""
	}
	return " · capped at 2000"
}

// buildReportRunCols turns the report run's column metadata + rows
// into the ListColumn spec the shared list-table primitive consumes.
// Min derived from header label; ideal capped at AutoMaxIdeal; max
// is the longest cell width across all rows.
func buildReportRunCols(cs []sf.ReportColumn, rows []map[string]any) []uilayout.ListColumn {
	out := make([]uilayout.ListColumn, 0, len(cs))
	for _, c := range cs {
		label := c.Label
		if label == "" {
			label = c.APIName
		}
		header := strings.ToUpper(label)
		min := lipglossWidth(header) + 2
		if min < 8 {
			min = 8
		}
		max := min
		for _, row := range rows {
			if w := lipglossWidth(stringifyReportCell(row[c.APIName])); w > max {
				max = w
			}
		}
		ideal := max
		if ideal > uilayout.AutoMaxIdeal {
			ideal = uilayout.AutoMaxIdeal
		}
		out = append(out, uilayout.ListColumn{
			Name: c.APIName, Header: header,
			Min: min, Ideal: ideal, Max: max,
			Style: lipgloss.NewStyle().Foreground(theme.Fg),
		})
	}
	return out
}

// reportHasIDColumn reports whether the run's projection includes
// the literal "Id" column — drives the "↵ open record" footer
// hint and gates Activate. Tabular reports configured to show Id
// can drill; aggregate / summary reports without Id can't.
func reportHasIDColumn(cs []sf.ReportColumn) bool {
	for _, c := range cs {
		if c.APIName == "Id" {
			return true
		}
	}
	return false
}

// stringifyReportCell coerces a cell value to a display string. SF
// returns numbers as float64, dates as ISO strings, picklists as
// already-resolved labels (we already prefer label over value at parse
// time). Just renders whatever's there.
func stringifyReportCell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// Strip trailing zeros for cleaner integer display while
		// preserving precision for actual decimals.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	}
	return fmt.Sprintf("%v", v)
}
