package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m Model) bulkTagsForRecords(sobject string, recs []map[string]any) map[string][]devproject.Tag {
	if m.devProjects == nil || len(recs) == 0 {
		return nil
	}
	if !m.settings.TagColumnVisible() {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	domain := gutterDomainRecord + ":" + sobject
	return d.memoTagsFor(m.devProjects, domain, slicePtrAny(recs), func() map[string][]devproject.Tag {
		keys := recordLookupKeys(sobject, recs)
		if len(keys) == 0 {
			return nil
		}
		out, err := m.devProjects.TagsForItems(o.Username, keys)
		if err != nil {
			warnTagLookupOnce(err)
			return nil
		}
		return out
	})
}

func warnTagLookupOnce(err error) {
	if err == nil {
		return
	}
	tagLookupWarn.Do(func() {
		applog.Warn("devproject.tag_lookup_failed", map[string]any{"err": err.Error()})
	})
}

var tagLookupWarn sync.Once

func (m Model) bulkProjectsForRecords(sobject string, recs []map[string]any) map[string][]devproject.DevProject {
	if m.devProjects == nil || len(recs) == 0 {
		return nil
	}
	if m.settings != nil && !m.settings.ProjectColumnVisible() {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	domain := gutterDomainRecord + ":" + sobject
	return d.memoProjectsFor(m.devProjects, domain, slicePtrAny(recs), func() map[string][]devproject.DevProject {
		keys := recordLookupKeys(sobject, recs)
		if len(keys) == 0 {
			return nil
		}
		out, err := m.devProjects.ProjectsForItems(o.Username, keys)
		if err != nil {
			warnTagLookupOnce(err)
			return nil
		}
		return out
	})
}

func recordLookupKeys(sobject string, recs []map[string]any) []devproject.TagLookupKey {
	keys := make([]devproject.TagLookupKey, 0, len(recs))
	for _, r := range recs {
		id, _ := r["Id"].(string)
		if id == "" {
			continue
		}
		keys = append(keys, devproject.TagLookupKey{
			Kind: devproject.KindRecord, Ref: sobject + ":" + id,
		})
	}
	return keys
}

func (m Model) bulkTagsAndProjectsForRecords(sobject string, recs []map[string]any) (
	map[string][]devproject.Tag, map[string][]devproject.DevProject,
) {
	return m.bulkTagsForRecords(sobject, recs), m.bulkProjectsForRecords(sobject, recs)
}

func (m Model) bulkTagsAndProjectsForFields(sobject string, fields []sf.Field) (
	map[string][]devproject.Tag, map[string][]devproject.DevProject,
) {
	if m.devProjects == nil || len(fields) == 0 {
		return nil, nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil, nil
	}
	keys := make([]devproject.TagLookupKey, 0, len(fields))
	for _, f := range fields {
		keys = append(keys, devproject.TagLookupKey{
			Kind: devproject.KindField, Ref: sobject + "." + f.Name,
		})
	}
	var tags map[string][]devproject.Tag
	var projs map[string][]devproject.DevProject
	if m.settings.TagColumnVisible() {
		var err error
		if tags, err = m.devProjects.TagsForItems(o.Username, keys); err != nil {
			warnTagLookupOnce(err)
		}
	}
	if m.settings == nil || m.settings.ProjectColumnVisible() {
		var err error
		if projs, err = m.devProjects.ProjectsForItems(o.Username, keys); err != nil {
			warnTagLookupOnce(err)
		}
	}
	return tags, projs
}

func (m Model) renderRecords(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	d := m.ensureOrgDataRef(o.Username)

	if d.RecordsSObjectCur == "" {
		return m.renderRecordsPicker(d, w, innerH)
	}
	return m.renderRecordsList(d, w, innerH)
}

func (m Model) renderRecordsPicker(d *orgData, w, innerH int) string {
	inner := w - 4
	if d.SObjects.FetchedAt().IsZero() {
		if d.SObjects.Busy() {
			return theme.Subtle.Render("  loading sobjects…")
		}
		if err := d.SObjects.Err(); err != nil {
			return redLine("  error: " + err.Error())
		}
		return theme.Subtle.Render("  no sobjects")
	}

	// Strip = favourites + transient slot + overflow sentinel.
	chips := m.stripRows(domainObjects, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	sel := m.objectsChipIdx()
	if sel < 0 || sel >= len(chips) {
		sel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, sel, inner)

	pickerCols := m.applyFlagsColumnMode(sobjectListCols())
	pickerResolved := mustResolveColumns(sobjectColumnSchema())
	installListViewOrderRows(&d.SObjectList, &d.ObjectsTableState, pickerCols,
		func(items []sf.SObject, row, col int) string {
			if row < 0 || row >= len(items) || col < 0 || col >= len(pickerCols) {
				return ""
			}
			if pickerCols[col].Name == "Marks" {
				return m.renderFlagsCell(marksForSObjectList(items), row)
			}
			return resolvedSortCellByID(pickerResolved, items[row], pickerCols[col].Name)
		})
	filtered := d.SObjectList.Filtered()
	total := d.SObjectList.Len()
	shown := len(filtered)

	title := fmt.Sprintf("PICK AN SOBJECT · %s · %d / %d · %s",
		chips[sel].Label, shown, total, humanAge(d.SObjects.FetchedAt()))
	header := headerWithSearchPill(title, d.SObjectList.Search)
	searchLine := searchBar(d.SObjectList.Search, inner)

	cur := d.SObjectList.Cursor()
	if cur >= shown {
		cur = 0
	}

	var out []string
	if dash != "" {
		out = append(out, dash)
	}
	out = append(out, header, searchLine, "")
	if shown == 0 {
		if m.projectChipActive() && d.SObjectList.ExtraCount() == 0 {
			out = append(out, theme.Subtle.Render(m.projectEmptyHint("sObjects")))
		} else {
			out = append(out, theme.Subtle.Render("  no matches"))
		}
		return strings.Join(out, "\n")
	}
	out = append(out, sobjectListTable(m, filtered, cur, inner, innerH, len(out), 2, &d.ObjectsTableState)...)
	out = append(out, "", dimLine("  ↵ browse records · esc back", inner))
	return strings.Join(out, "\n")
}

func (m Model) renderRecordsList(d *orgData, w, innerH int) string {
	inner := w - 4
	sobj := d.RecordsSObjectCur

	chips := recordsChips(m, d, sobj)
	chipSel := findChipIndex(chips, selectedRecordsChip(d, sobj))
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	withStrip := func(body string) string {
		if dash == "" {
			return body
		}
		return dash + "\n\n" + body
	}

	// Records-capability gate (central — see records_capability.go).
	// Non-queryable entities (Platform Events / Big Objects / External
	// Objects) can't back any chip; explain under the strip rather than
	// firing a query that errors. Details / Schema / FLS still work.
	recCap := m.recordsCapabilityFor(sobj)
	if recCap.DescribeLoaded && !recCap.Queryable {
		kind, why := nonQueryableReason(sobj)
		body := theme.Subtle.Render("  "+sobj+" "+kind) + "\n\n" +
			dimLine("  "+why, inner)
		return withStrip(body)
	}

	// Project chip with no records for this sObject — show the
	// project-empty hint under the strip instead of waiting on a
	// fetch that's never coming.
	if m.projectChipActive() {
		if scope := m.activeScope(); scope.Loaded() && len(scope.RecordIDsFor(sobj)) == 0 {
			body := theme.Subtle.Render(m.projectEmptyHint("records")) + "\n\n" +
				dimLine("  press "+firstPretty(Keys.CollectItem)+" on a record elsewhere to add it", inner)
			return withStrip(body)
		}
	}
	if selectedRecordsChip(d, sobj) == recentlyViewedChipID {
		if _, ok := m.visitedRecordsChip(d, sobj, d.username); !ok {
			body := theme.Subtle.Render("  no recently-viewed "+sobj+" records") + "\n\n" +
				dimLine("  press → for Changed · drill into a record from anywhere to start tracking", inner)
			return withStrip(body)
		}
	}
	// SF-mode synthetic Recently Viewed chip with no SF-side
	// visits — same empty-state pattern.  Sources IDs from the
	// per-sObject RecentlyViewedPerSObject payload (a SOQL filtered
	// by Type) so it can be genuinely empty even when sf-deck mode's
	// RV chip has rows.  Distinguish "still loading" from "truly
	// empty" by inspecting the per-sObject Resource's fetch state —
	// otherwise the user sees a false "no records" flash during the
	// initial query.
	if selectedRecordsChip(d, sobj) == sfRecentlyViewedChipID {
		if recCap.DescribeLoaded && !recCap.MruEnabled {
			body := theme.Subtle.Render("  "+sobj+" isn't recently-viewable") + "\n\n" +
				dimLine("  Salesforce tracks no LastViewedDate for this object · press "+
					firstPretty(Keys.LensModeToggle)+" for sf-deck views or → for a list view", inner)
			return withStrip(body)
		}
		if _, ok := m.salesforceVisitedRecordsChip(d, sobj, d.username); !ok {
			rv := d.RecentlyViewedPerSObject[sobj]
			if rv == nil || rv.Busy() || rv.FetchedAt().IsZero() {
				return withStrip(theme.Subtle.Render("  loading Salesforce recently-viewed " + sobj + "…"))
			}
			if rv != nil && rv.Err() != nil {
				return withStrip(redLine("  error fetching recently-viewed " + sobj + ": " + rv.Err().Error()))
			}
			body := theme.Subtle.Render("  no Salesforce recently-viewed "+sobj+" records") + "\n\n" +
				dimLine("  press → for the first list view · open something in Lightning to populate this", inner)
			return withStrip(body)
		}
	}
	r := currentRecordsResource(d, sobj)
	if r == nil || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return withStrip(theme.Subtle.Render("  loading " + sobj + " records…"))
		}
		if r != nil && r.Err() != nil {
			return withStrip(redLine("  error: " + r.Err().Error()))
		}
		return withStrip(theme.Subtle.Render("  fetching records…"))
	}

	list := r.Value()

	chipID := selectedRecordsChip(d, sobj)
	search := d.RecordsSearchPtr(sobj, chipID)
	visible, visibleIdx := visibleRecordsAndIdx(d, sobj)

	// Hard-capped: the SOQL carried a LIMIT, so list.Records IS at
	// most the cap. We can't know if SF would have matched more —
	// the title shows "X / N records" (visible / fetched).
	fetched := len(list.Records)
	// Preview marker: when the chip's SOQL carries a LIMIT clause,
	// the result is a slice by construction — we don't know whether
	// more rows exist on the server because SF's response doesn't
	// report the unbounded WHERE-clause count, only the rows it
	// returned. So we always surface the preview hint when a LIMIT
	// is in effect; the user can ctrl+x to pull the full set via
	// Bulk API if they want it.
	title := fmt.Sprintf("%s · %s · %d / %d records · %s",
		sobj, chips[chipSel].Label, len(visible), fetched,
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))
	preview := hasLimitClause(list.Query)
	if preview {
		title += " · preview"
	}
	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	if list.Query != "" && !m.queryLineHidden {
		lines = append(lines, dimLine("  "+list.Query, inner))
	}

	if len(list.Records) == 0 {
		if m.projectChipActive() {
			lines = append(lines, theme.Subtle.Render(m.projectEmptyHint("records")))
		} else {
			lines = append(lines, theme.Subtle.Render("  (no records)"))
		}
		lines = append(lines, "", dimLine("  esc back · "+firstPretty(Keys.Refresh)+" refresh", inner))
		return strings.Join(lines, "\n")
	}
	if len(visible) == 0 {
		lines = append(lines, headerWithSearchPill(title, *search))
		lines = append(lines, "")
		if bar := searchBar(*search, inner); bar != "" {
			lines = append(lines, bar)
		}
		lines = append(lines, theme.Subtle.Render("  no matches"))
		return strings.Join(lines, "\n")
	}

	projection := recordsProjectionFor(d, sobj, chipID, list, visible, search)

	// Build the listRenderModel and hand off to the shared renderer.
	// Records is the last list-table that was rendering bespoke; this
	// migration gives it pagination, snap-to-content via the standard
	// path, and behavioural parity with every other list. The per-
	// (sobject, chip) state + search pointers are resolved up-front
	// here in the orchestrator — renderListModel just sees the same
	// (state, search, cursor) shape every other surface produces.
	tableState := d.RecordsTableStatePtr(sobj, chipID)
	sortDataKey := recordsVisibleSortDataKey(d, sobj)
	rowAdapter := tableRowAdapter{
		State:        tableState,
		Cols:         projection.cols,
		N:            len(visible),
		Cell:         projection.cell,
		VisibleToRaw: visibleIdx,
		DataKey:      sortDataKey,
		RawCursor: func() RawRow {
			return RawRow(d.Cursors.Peek(cursorKindRecordsRow, sobj))
		},
		SetRawCursor: func(raw RawRow) {
			d.Cursors.Set(cursorKindRecordsRow, int(raw), 0, sobj)
		},
	}
	sel := int(rowAdapter.DisplayCursor())
	tagMap := m.bulkTagsForRecords(sobj, visible)
	projMap := m.bulkProjectsForRecords(sobj, visible)
	leftGutters, rightGutters := m.listGutters(
		func(row int) string {
			id, _ := visible[row]["Id"].(string)
			if id == "" {
				return ""
			}
			return m.resolveTagGutterCell(devproject.KindRecord, sobj+":"+id, tagMap)
		},
		func(row int) string {
			id, _ := visible[row]["Id"].(string)
			if id == "" {
				return ""
			}
			return rowProjectGutterFromMap(devproject.KindRecord, sobj+":"+id, projMap)
		},
	)
	footerExtras := firstPretty(Keys.RecordsExport) + " export"
	if preview {
		footerExtras += " full"
	}
	rmodel := listRenderModel{
		Title:  title,
		State:  tableState,
		Search: search,
		Cols:   projection.cols,
		N:      len(visible),
		Cursor: sel,
		Cell: func(row, col int) string {
			return projection.cell(row, col)
		},
		Gutters:      leftGutters,
		RightGutters: rightGutters,
		FooterExtras: footerExtras,
		DataVersion:  listVersionWithStore(int(r.FetchedAt().UnixNano()/int64(1000))+len(chipID)*7919+len(search.Buffer())*131, m),
		SortDataKey:  sortDataKey,
	}
	tableBudget := innerH - usedLines(lines)
	lines = append(lines, renderListModel(m, rmodel, m.focus, inner, tableBudget)...)
	return strings.Join(lines, "\n")
}

type recordsProjectionCache map[string]*recordsProjectionEntry

type recordsProjectionEntry struct {
	rowsPtr    uintptr
	colsPtr    uintptr
	visiblePtr uintptr
	rowsLen    int
	colsLen    int
	visibleLen int
	searchBuf  string
	searchOn   bool
	themeID    string
	projection recordsTableProjection
}

type recordsTableProjection struct {
	cols  []uilayout.ListColumn
	cells [][]string // column-major: cells[col][row]
}

func (p recordsTableProjection) cell(row, col int) string {
	if col < 0 || col >= len(p.cells) {
		return ""
	}
	if row < 0 || row >= len(p.cells[col]) {
		return ""
	}
	return p.cells[col][row]
}

func recordsProjectionFor(
	d *orgData,
	sobject, chipID string,
	list sf.RecordsList,
	visible []map[string]any,
	search *searchState,
) recordsTableProjection {
	searchBuf := ""
	searchOn := false
	if search != nil {
		searchBuf = search.Effective()
		searchOn = search.EffectiveApplied()
	}
	if d == nil {
		return buildRecordsProjection(list, visible)
	}
	if d.recordsProjectionCache == nil {
		d.recordsProjectionCache = recordsProjectionCache{}
	}
	key := sobject + ":" + chipID
	rowsPtr := slicePtrAny(list.Records)
	colsPtr := slicePtrStr(list.Columns)
	visiblePtr := slicePtrAny(visible)
	themeID := theme.Current.ID
	if entry, ok := d.recordsProjectionCache[key]; ok {
		if entry.rowsPtr == rowsPtr &&
			entry.colsPtr == colsPtr &&
			entry.visiblePtr == visiblePtr &&
			entry.rowsLen == len(list.Records) &&
			entry.colsLen == len(list.Columns) &&
			entry.visibleLen == len(visible) &&
			entry.searchBuf == searchBuf &&
			entry.searchOn == searchOn &&
			entry.themeID == themeID {
			return entry.projection
		}
	}
	projection := buildRecordsProjection(list, visible)
	d.recordsProjectionCache[key] = &recordsProjectionEntry{
		rowsPtr:    rowsPtr,
		colsPtr:    colsPtr,
		visiblePtr: visiblePtr,
		rowsLen:    len(list.Records),
		colsLen:    len(list.Columns),
		visibleLen: len(visible),
		searchBuf:  searchBuf,
		searchOn:   searchOn,
		themeID:    themeID,
		projection: projection,
	}
	return projection
}

func buildRecordsProjection(list sf.RecordsList, visible []map[string]any) recordsTableProjection {
	resolved := resolveRecordColumns(list, visible)
	cols := resolved.ListColumns()
	cell := resolved.Cell(visible)
	cells := make([][]string, len(cols))
	for col := range cols {
		colCells := make([]string, len(visible))
		for row := range visible {
			colCells[row] = cell(row, col)
		}
		cells[col] = colCells
	}
	return recordsTableProjection{cols: cols, cells: cells}
}

var recordIDFields = []string{"Id"}

func recordListFields(list sf.RecordsList) []string {
	if len(list.Columns) == 0 {
		return recordIDFields
	}
	return list.Columns
}

// buildRecordListCols builds the ListColumn spec for the records
// table from the SOQL projection (list.Columns) and the actual
// rendered cell values (visible). Min/Ideal widths are derived from
// the data, but Max is intentionally left open so explicit user
// resizing can grow columns beyond the current preview's content.
// Special-cases:
//   - "Id" — defaults to 20, muted
//   - date-shaped columns ("…Date", "SystemModstamp") — defaults to 14, dim
//   - everything else — measured from cells
func buildRecordListCols(list sf.RecordsList, visible []map[string]any) []uilayout.ListColumn {
	return resolveRecordColumns(list, visible).ListColumns()
}

func resolveRecordColumns(list sf.RecordsList, visible []map[string]any) tablemodel.Resolved[map[string]any] {
	resolved, err := tablemodel.Resolve(recordColumnSchema(visible), recordListFields(list), list.SObject)
	if err != nil {
		return tablemodel.Resolved[map[string]any]{
			Defs: []tablemodel.ColumnDef[map[string]any]{recordColumnDef("Id", visible)},
		}
	}
	return resolved
}

func recordColumnSchema(visible []map[string]any) tablemodel.Schema[map[string]any] {
	return tablemodel.Schema[map[string]any]{
		DefaultColumns: func(scope string) []string { return []string{"Id"} },
		RequiredFields: func(scope string) []string {
			return []string{"Id"}
		},
		DynamicColumn: func(id string) (tablemodel.ColumnDef[map[string]any], bool) {
			if id == "" {
				return tablemodel.ColumnDef[map[string]any]{}, false
			}
			return recordColumnDef(id, visible), true
		},
	}
}

func recordColumnDef(field string, visible []map[string]any) tablemodel.ColumnDef[map[string]any] {
	header := recordColumnHeader(field)
	width := tablemodel.Width{Min: lipglossWidth(header) + 2}
	if width.Min < 8 {
		width.Min = 8
	}
	style := lipgloss.NewStyle().Foreground(theme.Fg)
	switch {
	case field == "Id":
		header = "ID"
		width = tablemodel.Width{Min: 8, Ideal: 20}
		style = lipgloss.NewStyle().Foreground(theme.Muted)
	case field == "CreatedBy.Name" || field == "LastModifiedBy.Name":
		style = lipgloss.NewStyle().Foreground(theme.Muted)
		width = tablemodel.Width{Min: lipglossWidth(header) + 2, Ideal: 18}
	case isDateField(field):
		width = tablemodel.Width{Min: 8, Ideal: 14}
		style = lipgloss.NewStyle().Foreground(theme.FgDim)
	default:
		max := width.Min
		for _, rec := range visible {
			if w := lipglossWidth(renderRecordCell(rec, field)); w > max {
				max = w
			}
		}
		width.Ideal = max
		if width.Ideal > uilayout.AutoMaxIdeal {
			width.Ideal = uilayout.AutoMaxIdeal
		}
	}
	return tablemodel.ColumnDef[map[string]any]{
		ID:          field,
		Header:      header,
		Width:       width,
		Style:       style,
		FetchFields: []string{field},
		Searchable:  true,
		Exportable:  true,
		Render: func(rec map[string]any) string {
			return renderRecordCell(rec, field)
		},
	}
}

func recordColumnHeader(field string) string {
	switch field {
	case "CreatedBy.Name":
		return "CREATED BY"
	case "LastModifiedBy.Name":
		return "MODIFIED BY"
	}
	return strings.ToUpper(field)
}

func isDateField(field string) bool {
	switch field {
	case "LastModifiedDate", "CreatedDate", "SystemModstamp":
		return true
	}
	return strings.HasSuffix(field, "Date") || strings.HasSuffix(field, "DateTime__c")
}

func renderRecordCell(rec map[string]any, field string) string {
	v, ok := sf.Record(rec).Field(field)
	if !ok || v == nil {
		return "—"
	}
	if isDateField(field) {
		if s, ok := v.(string); ok {
			return relativeTime(s)
		}
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return "—"
		}
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case map[string]any:
		if s, ok := renderCompound(t); ok {
			return s
		}
		return "{…}"
	}
	return fmt.Sprintf("%v", v)
}

func filterRecords(rows []map[string]any, cols []string, q string) ([]map[string]any, []int) {
	q = strings.TrimSpace(q)
	if q == "" {
		idx := make([]int, len(rows))
		for i := range rows {
			idx[i] = i
		}
		return rows, idx
	}
	type term struct {
		field string // "" = match-any-column
		value string // already lowercased
	}
	var terms []term
	for _, raw := range strings.Fields(q) {
		if i := strings.IndexByte(raw, ':'); i > 0 && i < len(raw)-1 {
			terms = append(terms, term{
				field: matchFieldName(cols, raw[:i]),
				value: strings.ToLower(raw[i+1:]),
			})
			continue
		}
		terms = append(terms, term{value: strings.ToLower(raw)})
	}
	outRows := rows[:0:0]
	outIdx := make([]int, 0, len(rows))
	for i, rec := range rows {
		match := true
		for _, t := range terms {
			if !rowMatchesTerm(rec, cols, t.field, t.value) {
				match = false
				break
			}
		}
		if match {
			outRows = append(outRows, rec)
			outIdx = append(outIdx, i)
		}
	}
	return outRows, outIdx
}

func matchFieldName(cols []string, prefix string) string {
	lp := strings.ToLower(prefix)
	for _, c := range cols {
		if strings.EqualFold(c, prefix) {
			return c
		}
	}
	for _, c := range cols {
		if strings.HasPrefix(strings.ToLower(c), lp) {
			return c
		}
	}
	return prefix
}

func rowMatchesTerm(rec map[string]any, cols []string, field, value string) bool {
	if field != "" {
		return strings.Contains(strings.ToLower(renderRecordCell(rec, field)), value)
	}
	for _, c := range cols {
		if strings.Contains(strings.ToLower(renderRecordCell(rec, c)), value) {
			return true
		}
	}
	return false
}

// relativeTime turns a Salesforce ISO timestamp into "just now",
// "3m ago", "2h ago", "5d ago", etc. Falls back to the raw string if
// we can't parse it.
func relativeTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000-0700", iso)
		if err != nil {
			return iso
		}
	}
	return humanAge(t)
}
