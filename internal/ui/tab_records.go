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

// bulkTagsAndProjectsForRecords pre-fetches tag bindings + project
// memberships for a slice of record rows. Records are keyed as
// "<sObject>:<Id>" matching the tag-picker / sidebar lookup shape.
// Returns nil maps when the store is unavailable; callers feed them
// to rowTag/Project helpers which handle empty gracefully.
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

// warnTagLookupOnce logs the FIRST tag/project store failure of the
// session. These lookups run on the render path and degrade to "no
// tag column" by design; per-frame logging would flood the session
// log with the same sqlite error, but zero logging made the
// degradation invisible (review finding 2026-06-12).
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

// bulkTagsAndProjectsForFields pre-fetches tag + project bindings for a
// slice of fields (Schema subtab). Field refs are "<sObject>.<Name>".
// Honors the tag/project column-visibility settings — skips the store
// call when the gutter is hidden. nil maps when unavailable.
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

// renderRecords is the data-browsing tab: pick an sObject, see its
// recent records, drill into details via the sidebar. Shares the sObject
// picker widget with /objects but forks once an sObject is chosen.
//
// Modes:
//
//	RecordsSObjectCur == "" → picker mode: sObject list + filter + search
//	RecordsSObjectCur != "" → record mode: list of recent records for it
//
// Esc from record mode returns to the picker. Esc from the picker
// itself is a no-op (top level).
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

// --- picker mode --------------------------------------------------------

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

	// Reuse the exact same filtered list as /objects — the picker is the
	// same widget semantically.
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
		// Distinguish "project chip narrowed everything out" (show
		// collect hint) from "search query filtered to zero" inside
		// a non-empty project (show generic "no matches" so the user
		// knows it's their query, not a missing project).
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

// --- record-list mode ---------------------------------------------------

func (m Model) renderRecordsList(d *orgData, w, innerH int) string {
	inner := w - 4
	sobj := d.RecordsSObjectCur

	// Build the chip strip up-front so empty / loading / error
	// states render WITH the strip visible — without it the user
	// loses the cycle-to-broader-chip affordance exactly when they
	// need it most.  Strip construction is cheap (just reads
	// in-memory state); the per-chip data fetch is gated separately
	// downstream.
	chips := recordsChips(m, d, sobj)
	chipSel := findChipIndex(chips, selectedRecordsChip(d, sobj))
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	// withStrip composes "strip + body" so every early-return path
	// includes the chip bar.  body is the multi-line content the
	// caller wants under the strip — empty-state hint, loading
	// indicator, error, or the rendered table.
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
	// Recently viewed chip with no visits for this sObject — the
	// chip is the default on first visit (see selectedRecordsChip)
	// so without this the user lands on /records/X with no recents
	// and the renderer sits on "fetching records…" forever (the
	// dispatcher correctly doesn't fire EnsureChipRecords when
	// visitedRecordsChip returns ok=false).
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
		// Not every sObject is recently-viewable: setup/metadata
		// entities (BatchProcessJobDefinition, …) have no LastViewedDate,
		// so the per-sObject query would throw INVALID_FIELD. Gate on the
		// central capability (mruEnabled) and explain it in the body —
		// under the chip strip, so the user can still switch views (L / →).
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

	// Per-(sobject, chip) sticky search. Filters across visible cells
	// + the special `field:value` shorthand. Filter only kicks in when
	// the searchState is Active (live-typing) or Committed (Enter-ed)
	// — a stale buffer left after cancelling shouldn't narrow the list.
	chipID := selectedRecordsChip(d, sobj)
	search := d.RecordsSearchPtr(sobj, chipID)
	// Reuse the per-render memo so we don't rebuild visibleIdx
	// (10k×8 bytes = 80KB) and re-walk every record's cell on
	// every frame. visibleRecordsAndIdx hits the cache in steady
	// state (no refetch / search-edit / chip-switch since last
	// call) so a scroll burst pays the O(N) cost once and then
	// returns the same pair on every subsequent tick.
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
	// "preview" marker stays in the title — it's status, not a key
	// hint. The key hints (ctrl+x export, ctrl+, raise default) live
	// on the per-surface hint bar so the title stays scannable.
	preview := hasLimitClause(list.Query)
	if preview {
		title += " · preview"
	}
	// Orchestrator owns the chip dashboard + the SOQL query line;
	// renderListModel owns the title, search bar, table, and
	// footer hint. Splitting it this way means pagination's
	// title-suffix and search-pill behaviour come from the
	// shared renderer rather than being duplicated here.
	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	// The SOQL line is the literal query that drove this records
	// fetch — informative for power users but visual noise for
	// most. Toggle with ctrl+- (paired with ctrl+= for the chip
	// strip — both "remove a piece of chrome" toggles).
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
		// Surface the active filter pill AND the live "/" search
		// bar even on the no-matches path — this is exactly when
		// the user most needs to see WHICH query they're typing
		// and WHAT it's filtering against. The early return below
		// skips renderListModel (which normally emits both), so
		// emit them inline here.
		lines = append(lines, headerWithSearchPill(title, *search))
		lines = append(lines, "")
		if bar := searchBar(*search, inner); bar != "" {
			lines = append(lines, bar)
		}
		lines = append(lines, theme.Subtle.Render("  no matches"))
		// Search-state keys (ctrl+u while typing, / and C/esc once
		// committed) are already in the SearchBar above — no need
		// to repeat them on a fifth line below the empty-state
		// message.
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
	// listRenderModel.Cursor is a display-space int; convert at the
	// render boundary.
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
	// ctrl+x export shown on every records frame — works on any
	// list view, not just previews. The "full" qualifier is
	// appended only when in preview mode (chip has LIMIT) since
	// that's the case where "full" means something different from
	// what's visible. Non-preview lists are already the full
	// matched set; ctrl+x just exports it.
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
		// Records doesn't drive a ListView — data lives on the
		// chip's Resource[List]. Fold the resource's FetchedAt
		// nanoseconds + the active chipID + the search buffer
		// length so the cache invalidates on (1) refetch, (2)
		// chip change, (3) search edit. Devproject generation is
		// folded in via the standard helper for tag-edit
		// invalidation.
		DataVersion: listVersionWithStore(int(r.FetchedAt().UnixNano()/int64(1000))+len(chipID)*7919+len(search.Buffer())*131, m),
		SortDataKey: sortDataKey,
	}
	// Count actual rendered lines (not slice length) — dash is a
	// multi-line block stored as one string with embedded \n. Using
	// len(lines) here undercounts and lets renderListModel claim more
	// vertical budget than it actually has, which pushes the trailing
	// hint past the pane's clipLines cap so it disappears.
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
	// Key cache on Effective() so the debounce-aware buffer gates
	// invalidation — fast typing on a slow filter collapses into
	// one projection rebuild instead of one per keystroke.
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
		// Person columns: readable header + muted, moderate width.
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

// recordColumnHeader renders a clean column header for a projected
// field. Dotted audit relationships get friendly labels ("CREATED BY"
// not "CREATEDBY.NAME"); everything else upper-cases the field name.
func recordColumnHeader(field string) string {
	switch field {
	case "CreatedBy.Name":
		return "CREATED BY"
	case "LastModifiedBy.Name":
		return "MODIFIED BY"
	}
	return strings.ToUpper(field)
}

// isDateField reports whether a SOQL field name is date-shaped, so
// the column gets fixed-narrow rendering and relativeTime cells.
func isDateField(field string) bool {
	switch field {
	case "LastModifiedDate", "CreatedDate", "SystemModstamp":
		return true
	}
	return strings.HasSuffix(field, "Date") || strings.HasSuffix(field, "DateTime__c")
}

// renderRecordCell stringifies one field value for the records table.
// Special-cases LastModifiedDate (relative time) and nil (em-dash).
//
// Honours dotted relationship traversals: SF returns `Account.Name`
// projections as nested JSON (`rec["Account"]["Name"]`), not as a
// flat `rec["Account.Name"]` key. We walk via sf.Record.Field which
// handles both shapes transparently.
//
// Nested objects (when a chip projects the whole relationship row,
// e.g. `Owner` rather than `Owner.Alias`) flatten to a sensible
// string — typically the .Name leaf, falling back to .Id.
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
		// Salesforce JSON returns numbers as float64. Print integers
		// without the trailing .0.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case map[string]any:
		// One registry handles every map shape SOQL returns —
		// relationship lookups, compound address / person Name /
		// geolocation. See compound_render.go.
		if s, ok := renderCompound(t); ok {
			return s
		}
		return "{…}"
	}
	return fmt.Sprintf("%v", v)
}

// filterRecords applies the search query to the records list and
// returns the matching rows along with the original (unfiltered)
// index of each. The parallel index slice lets RowSpace preserve cursor
// identity across filter changes.
//
// Default mode: case-insensitive substring across every projected
// column's stringified value. A row matches if ANY cell contains the
// query.
//
// Fielded mode: if the query contains a colon (e.g. "email:foo"),
// the prefix names a column (case-insensitive on the API name) and
// only that column is matched. Multiple terms can appear separated
// by whitespace and are AND-ed: `industry:tech name:acme` finds
// rows where Industry contains "tech" AND Name contains "acme".
//
// Empty / whitespace query returns the input slice with identity
// indices.
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

// matchFieldName resolves the user's typed prefix (e.g. "email") to
// the closest projected column name (e.g. "Email" or "Email__c"),
// case-insensitive. Falls back to the raw input if no column matches —
// the matcher will then look for a literal field with that name and
// find nothing, yielding zero rows. That's better than treating
// "typo:value" as a free-text search and surprising the user.
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

// rowMatchesTerm tests one term against one row. field == "" means
// "match any cell". Both the projected value and the query are
// lowercased for case-insensitive substring.
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
		// Salesforce sometimes uses .000+0000 instead of Z — try again.
		t, err = time.Parse("2006-01-02T15:04:05.000-0700", iso)
		if err != nil {
			return iso
		}
	}
	return humanAge(t)
}
