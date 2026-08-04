package ui

// List-table key handling — column resize ([ / ] / { / }), horizontal
// scroll (, / .), zen-mode toggle (z). All operate against whichever
// list-table-shaped surface the user is on right now.
//
// Surfaces with a list-table:
//   - Records subtab on TabObjectDetail (per-(sobject, chip) state)
//   - Record-list mode on TabRecords     (per-(sobject, chip) state)
//   - SOQL results on TabSOQL            (model-level state)
//   - Report run on TabReportDetail      (model-level state)
//
// Other tabs return nil from activeListTable; key handlers no-op.

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// activeListTable returns the per-surface ListTableState pointer and
// rebuilds the ListColumn slice the renderer would have used for the
// current data. Returns (nil, nil) when no list-table is active.
//
// Why rebuild the columns: resize/snap key handlers need to know the
// column at colTarget's name (for SetUserWidth), its min/max (for
// clamps). Rather than threading "columns at last render" through
// state, we just rebuild — the cost is one pass over the visible
// rows, same shape as a render pass.
func (m *Model) activeListTable() (*uilayout.ListTableState, []uilayout.ListColumn) {
	ctx := m.activeListTableContext()
	return ctx.State, ctx.Cols
}

// activeListTableState returns only the current list-table state. Use
// this from hot paths that need a boolean/mode flag (wheel routing,
// status hints, zen check) but do not need columns.
//
// Drives off the same TabSpec.ListTable / listSurface.State machinery
// activeListTableMeasured uses — that path already has registry hooks
// for TabRecords, TabObjectDetail, TabSOQL, TabReportDetail plus the
// uniform list surfaces, so the per-tab switch the previous
// implementation carried was duplicated dispatch logic. Discarding the
// column build keeps this cheap.
func (m *Model) activeListTableState() *uilayout.ListTableState {
	return m.activeListTableContext().State
}

func (m *Model) activeListTableContext() listTableContext {
	spec, sub := m.activeSpec()
	if sub != nil && sub.ListTable != nil {
		st, cols := sub.ListTable(m)
		var measure func(int) int
		if sub.MeasureCell != nil {
			measure = func(col int) int { return sub.MeasureCell(m, col) }
		}
		ctx := listTableContext{State: st, Cols: cols, Measure: measure}
		if d, ok := m.activeOrgState(); ok {
			ctx.OrgUsername = d.username
			ctx.Scope = m.tabSpecListTableWidthScope()
		}
		m.applyListTableWidthPrefs(ctx)
		return ctx
	}
	if spec != nil && spec.ListTable != nil {
		st, cols := spec.ListTable(m)
		var measure func(int) int
		if spec.MeasureCell != nil {
			measure = func(col int) int { return spec.MeasureCell(m, col) }
		}
		ctx := listTableContext{State: st, Cols: cols, Measure: measure}
		if d, ok := m.activeOrgState(); ok {
			ctx.OrgUsername = d.username
			ctx.Scope = m.tabSpecListTableWidthScope()
		}
		m.applyListTableWidthPrefs(ctx)
		return ctx
	}
	if surf := m.resolveListSurface(); surf != nil {
		d, ok := m.activeOrgState()
		if !ok {
			return listTableContext{}
		}
		var state *uilayout.ListTableState
		if surf.State != nil {
			state = surf.State(d)
		}
		var cols []uilayout.ListColumn
		if surf.Cols != nil {
			cols = surf.Cols()
		}
		var measure func(col int) int
		var cellFn func(row, col int) string
		var rowCount int
		var renderCols []uilayout.ListColumn
		// Surfaces that opt into the shared renderer expose a
		// per-frame Cell via BuildRenderModel — drive snap from
		// that so list view + snap can't disagree on what's in
		// the cells. Falls back to the legacy MeasureCell path
		// for surfaces that haven't migrated.
		if surf.BuildRenderModel != nil {
			if model, ok := surf.BuildRenderModel(*m, d); ok && model.Cell != nil {
				cellsCols := model.Cols
				if cellsCols == nil {
					cellsCols = cols
				}
				cellFn = model.Cell
				rowCount = model.N
				renderCols = cellsCols
				measure = func(col int) int {
					if col < 0 || col >= len(cellsCols) {
						return 0
					}
					max := 0
					for r := 0; r < model.N; r++ {
						w := lipgloss.Width(model.Cell(r, col))
						if w > max {
							max = w
						}
					}
					return max
				}
			}
		}
		if measure == nil && surf.MeasureCell != nil {
			measure = func(col int) int { return surf.MeasureCell(d, col) }
		}
		ctx := listTableContext{
			State:       state,
			Cols:        cols,
			Measure:     measure,
			OrgUsername: d.username,
			Scope:       m.listSurfaceWidthScope(surf, d),
			Cell:        cellFn,
			RowCount:    rowCount,
			RenderCols:  renderCols,
		}
		m.applyListTableWidthPrefs(ctx)
		return ctx
	}
	return listTableContext{}
}

func (m Model) tabSpecListTableWidthScope() string {
	d, ok := m.activeOrgState()
	if !ok {
		return ""
	}
	switch m.tab() {
	case TabRecords:
		if d.RecordsSObjectCur == "" {
			return "objects"
		}
		return recordsWidthScope(d, d.RecordsSObjectCur)
	case TabObjectDetail:
		if m.currentSubtab() == SubtabRecords && d.DescribeCur != "" {
			return recordsWidthScope(d, d.DescribeCur)
		}
	}
	return ""
}

// listTableSOQL is the SOQL result-grid resolver. Columns are
// dynamic (depend on the query), so the Cols() route through
// listSurface doesn't fit — we declare it here and wire it on
// TabSpec.ListTable.
func listTableSOQL(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	if len(m.soqlResult.Records) == 0 {
		return nil, nil
	}
	// Route through the projection cache so wheel-routing,
	// sidebar render, Zen check, status bar, and render-cache key —
	// every per-frame caller of activeListTableState — share one
	// build per data change instead of each re-walking the full
	// result set. Mirrors recordsListTable's recordsProjectionFor
	// dispatch; without this, /soql on 5K+ rows lags during wheel
	// bursts because listTableSOQL is on the hot path.
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, m.soqlResult.Records, m.soqlSearchPtr(), theme.Current.ID, m.soqlInput.Value())
	return &m.soqlTable, entry.listCols
}

// measureCellSOQL widens column `col` to the widest rendered cell in
// the current SOQL result set. Reads the pre-measured Max from the
// cached projection so snap-to-content matches what soqlRenderModel
// just rendered without re-walking the rows.
func measureCellSOQL(m *Model, col int) int {
	if len(m.soqlResult.Records) == 0 {
		return 0
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, m.soqlResult.Records, m.soqlSearchPtr(), theme.Current.ID, m.soqlInput.Value())
	if col < 0 || col >= len(entry.listCols) {
		return 0
	}
	return entry.listCols[col].Max
}

// listTableReportDetail is the report-run grid resolver — columns
// come from the run's Columns slice + per-row payload.
func listTableReportDetail(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	if len(m.orgs) == 0 {
		return nil, nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.ReportCur == "" {
		return nil, nil
	}
	runRes := d.ReportRuns[d.ReportCur]
	if runRes == nil || runRes.FetchedAt().IsZero() {
		return nil, nil
	}
	run := runRes.Value()
	if len(run.Rows) == 0 {
		return nil, nil
	}
	return &m.reportRunTable, buildReportRunCols(run.Columns, run.Rows)
}

// measureCellReportDetail widens column `col` to the widest rendered
// cell in the current report run. Mirrors the cell-extraction
// reportDetailBody uses (stringifyReportCell on the per-API-name
// payload).
func measureCellReportDetail(m *Model, col int) int {
	if len(m.orgs) == 0 {
		return 0
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil || d.ReportCur == "" {
		return 0
	}
	runRes := d.ReportRuns[d.ReportCur]
	if runRes == nil || runRes.FetchedAt().IsZero() {
		return 0
	}
	run := runRes.Value()
	cols := buildReportRunCols(run.Columns, run.Rows)
	if col < 0 || col >= len(cols) {
		return 0
	}
	name := cols[col].Name
	max := 0
	for _, row := range run.Rows {
		if w := lipglossWidth(stringifyReportCell(row[name])); w > max {
			max = w
		}
	}
	return max
}

// listTableObjectDetailDispatch routes to the right per-subtab
// list-table for ObjectDetail. Only the Records subtab has a
// list-table-shaped view today; everything else returns nil so
// column-mode / sort / scroll gestures cleanly no-op.
func listTableObjectDetailDispatch(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	switch m.currentSubtab() {
	case SubtabRecords:
		return m.recordsListTable()
	case SubtabSchema:
		return m.schemaListTable()
	}
	return nil, nil
}

// schemaListTable resolves the Schema subtab's field list-table state +
// columns, so cursor / sort / column-nav / resize keys reach it (the
// same machinery /records and /flows use). Returns nil when the
// describe isn't loaded yet.
func (m *Model) schemaListTable() (*uilayout.ListTableState, []uilayout.ListColumn) {
	d, ok := m.activeOrgState()
	if !ok || d.DescribeCur == "" {
		return nil, nil
	}
	r, ok := d.Describes[d.DescribeCur]
	if !ok || r.FetchedAt().IsZero() {
		return nil, nil
	}
	fs := d.syncFieldList(d.DescribeCur, r.Value().Fields)
	return &fs.Table, mustResolveColumns(fieldColumnSchema()).ListColumns()
}

// listTableRecords resolves /records — picker mode (sObject list)
// vs record-list mode (currently selected sObject's recent records).
// The dispatcher branches on RecordsSObjectCur.
func listTableRecords(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	if len(m.orgs) == 0 {
		return nil, nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d != nil && d.RecordsSObjectCur == "" {
		return &d.ObjectsTableState, sobjectListCols()
	}
	return m.recordsListTable()
}

// activeOrgState is a tiny helper for the boilerplate "guard m.orgs
// non-empty, fetch the orgData pointer" check that every TabXxx
// branch above repeats.
func (m Model) activeOrgState() (*orgData, bool) {
	if len(m.orgs) == 0 {
		return nil, false
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil, false
	}
	return d, true
}

// recordsListTable resolves the records-shaped list-table state (used
// by both the /records record-list mode and TabObjectDetail's records
// subtab — same data shape, same state map).
func (m Model) recordsListTable() (*uilayout.ListTableState, []uilayout.ListColumn) {
	d, sobj := m.activeRecordsSObject()
	if sobj == "" {
		return nil, nil
	}
	chipID := selectedRecordsChip(d, sobj)
	state := d.RecordsTableStatePtr(sobj, chipID)
	if currentChipMode(d, sobj) == ChipModeSalesforce && chipID != sfRecentlyViewedChipID {
		key := sobj + ":" + chipID
		r, ok := d.ListViewResults[key]
		if !ok || r.FetchedAt().IsZero() {
			return state, nil
		}
		result := r.Value()
		vcols := visibleColumns(result.Columns)
		if len(vcols) == 0 {
			return state, nil
		}
		return state, buildListViewCols(vcols, result.Records)
	}
	r := currentRecordsResource(d, sobj)
	if r == nil || r.FetchedAt().IsZero() {
		return state, nil
	}
	list := r.Value()
	visible, _ := visibleRecordsAndIdx(d, sobj)
	if len(visible) == 0 {
		return state, nil
	}
	search := d.RecordsSearchPtr(sobj, chipID)
	projection := recordsProjectionFor(d, sobj, chipID, list, visible, search)
	return state, projection.cols
}

// sobjectListCols is the /objects + /records-picker shared spec.
// Mirrors what sobjectListTable builds at render time so column-mode
// resize / sort target the same column slots the user is looking at.
func sobjectListCols() []uilayout.ListColumn {
	return mustResolveColumns(sobjectColumnSchema()).ListColumns()
}

// flowListCols is the /flows shared spec. Built once here so
// activeListTable can advertise the columns to column-mode without
// the renderer having to expose them. Stays in sync with the
// renderer's column set in tab_flows.go's flowListTable helper.
func flowListCols() []uilayout.ListColumn {
	return mustResolveColumns(flowColumnSchema()).ListColumns()
}

// resizeTargetCol picks the column the resize keys (`<`, `>`,
// `Ctrl+[`, `Ctrl+]`) operate on. With the column cursor always
// present, the answer is straightforward: whatever column the
// cursor highlights. The cursor defaults to the leftmost non-
// frozen column on first paint, so resize keys always have a
// predictable target without the user having to opt into a
// "column mode" first.
func resizeTargetCol(state *uilayout.ListTableState, totalCols int) int {
	if state == nil || totalCols == 0 {
		return 0
	}
	// Build a tiny cols slice just to satisfy effectiveColCursor's
	// signature — it only reads len(cols) and FrozenCols.
	dummy := make([]uilayout.ListColumn, totalCols)
	return effectiveColCursor(state, dummy)
}

// ensureColCursorVisible adjusts HScroll so state.ColCursor lands in
// the visible window. When the cursor moves into a frozen column,
// HScroll resets to FrozenCols (no point scrolling; frozen cols are
// always shown). When it moves past the rightmost visible non-frozen
// column, scroll right by one. When it moves before HScroll, scroll
// left to put the cursor at HScroll.
func ensureColCursorVisible(state *uilayout.ListTableState, cols []uilayout.ListColumn, inner int) {
	if state == nil || len(cols) == 0 {
		return
	}
	c := state.ColCursor
	frozen := state.FrozenCols
	if c < frozen {
		// Cursor in a frozen column: no scrolling needed (frozens are
		// always rendered first).
		state.HScroll = frozen
		return
	}
	// Compute the layout to see what's currently visible.
	spec := uilayout.ListTableSpec{Cols: cols, N: 0, Cell: func(int, int) string { return "" }}
	res := uilayout.LayoutListTable(spec, state, inner)
	if !res.Overflow {
		// Everything fits: nothing to scroll.
		return
	}
	if c < res.HScroll {
		state.HScroll = c
		return
	}
	// Walk visible columns forward; if the cursor is past the last
	// visible one, advance HScroll until it falls inside.
	const gutter = 2
	const sepW = 3
	for state.HScroll < c {
		used := gutter
		// Frozen cols always rendered.
		for i := 0; i < frozen && i < len(cols); i++ {
			if used > gutter {
				used += sepW
			}
			used += res.Widths[i]
		}
		// Then from HScroll onward.
		visible := frozen - 1
		for i := state.HScroll; i < len(cols); i++ {
			need := res.Widths[i]
			if used > gutter {
				need += sepW
			}
			if used+need > inner {
				break
			}
			used += need
			visible = i
		}
		if c <= visible {
			return
		}
		state.HScroll++
		if state.HScroll >= len(cols) {
			state.HScroll = len(cols) - 1
			return
		}
	}
}

// handleColShrink / handleColGrow apply a resize step. Use of a fresh
// LayoutListTable call means resize math always runs against the same
// widths the next render will produce.
func (m Model) handleColResize(delta int) (Model, tea.Cmd, bool) {
	ctx := (&m).activeListTableContext()
	if ctx.State == nil || len(ctx.Cols) == 0 {
		return m, nil, false
	}
	target := resizeTargetCol(ctx.State, len(ctx.Cols))
	// Build a minimal spec for layout — the resolver only needs widths.
	spec := uilayout.ListTableSpec{Cols: ctx.Cols, N: 0, Cell: func(int, int) string { return "" }}
	res := uilayout.LayoutListTable(spec, ctx.State, m.contentWidth())
	uilayout.StepResize(spec, ctx.State, res, target, delta, m.settings.LayoutColumnResizeStep())
	return m, m.saveListTableWidthsCmd(ctx), true
}

// handleColSnap snaps the target column to its min (delta < 0) or
// fits-to-content (delta > 0). Routes through the measurer the
// active surface declares so snap-to-content reflects the actual
// rows on screen rather than the column's static Ideal/Max.
func (m Model) handleColSnap(delta int) (Model, tea.Cmd, bool) {
	ctx := (&m).activeListTableContext()
	if ctx.State == nil || len(ctx.Cols) == 0 {
		return m, nil, false
	}
	target := resizeTargetCol(ctx.State, len(ctx.Cols))
	if delta > 0 && ctx.Measure != nil {
		uilayout.SnapResizeTo(ctx.State, ctx.Cols[target], ctx.Measure(target))
	} else {
		uilayout.SnapResize(uilayout.ListTableSpec{Cols: ctx.Cols},
			ctx.State, target, delta)
	}
	return m, m.saveListTableWidthsCmd(ctx), true
}

func (m Model) handleColResetWidths() (Model, tea.Cmd, bool) {
	ctx := (&m).activeListTableContext()
	if ctx.State == nil || len(ctx.Cols) == 0 {
		return m, nil, false
	}
	if len(ctx.State.UserWidths) == 0 {
		m.flash("column widths already auto")
		return m, nil, true
	}
	ctx.State.UserWidths = nil
	m.flash("column widths reset")
	return m, m.saveListTableWidthsCmd(ctx), true
}

// handleColScroll adjusts the table's HScroll AND advances the
// column-cursor highlight. delta > 0 = right, delta < 0 = left.
//
// The cursor advance is the user-facing change: the highlighted
// column is the one `s` (sort), `<` / `>` (resize), and other
// column ops will operate on. Combining "scroll" and "move
// cursor" into one keystroke means the user always knows the
// target — no separate "I'm in column mode" state needed.
//
// Cursor clamps to [0, len(cols)-1] — including frozen columns.
// FrozenCols only constrains HScroll (the frozen ones are always
// visible regardless of scroll), but the cursor can land on them
// so users can still sort / resize the Id, Name, etc. columns.
// HScroll follows via ensureColCursorVisible so the cursored
// column is always on screen.
//
// Side gutters (TAGS, FLAGS, PROJECTS) live OUTSIDE spec.Cols
// (they're synthetic right/left gutters in the renderer). The
// cursor doesn't move into them by design — they're metadata
// pills, not user-data columns.
func (m Model) handleColScroll(delta int) (Model, bool) {
	state, cols := m.activeListTable()
	if state == nil || len(cols) == 0 {
		return m, false
	}
	cur := effectiveColCursor(state, cols)
	next := cur + delta
	if next < 0 {
		next = 0
	}
	if next >= len(cols) {
		next = len(cols) - 1
	}
	state.ColCursor = next
	ensureColCursorVisible(state, cols, m.contentWidth())
	return m, true
}

// handleColSort cycles the sort state on the cursored column:
// unsorted → asc → desc → unsorted. The column cursor is always
// live and defaults to the leftmost non-frozen column, so `s`
// always operates on a predictable target — the column whose
// header is highlighted. Use ←/→ (or h/l, ,/.) to advance the
// cursor before sorting.
//
// After the sort state flips, the row cursor is snapped back to the
// top of the surface so the user sees a clear "the list reordered"
// signal — the rows visibly slide while the cursor jumps to row 0.
// Without this the row cursor stays glued to the resource it was on,
// which slides to its new sorted position; the user's *eye* doesn't
// catch the reordering because the highlight stays roughly where it
// was. Matches the Finder / spreadsheet pattern: clicking a column
// header always reveals the new top-of-list.
func (m Model) handleColSort() (Model, bool) {
	state, cols := m.activeListTable()
	if state == nil || len(cols) == 0 {
		return m, false
	}
	target := effectiveColCursor(state, cols)
	if target < 0 || target >= len(cols) {
		return m, false
	}
	if cols[target].Unsortable {
		// Composite glyph columns (the FLAGS strip) have no meaningful
		// lex order. Refuse and point the user at the chip strip, which
		// is the proper per-flag focus mechanism.
		mm := &m
		mm.flash("can't sort on flags column")
		return *mm, true
	}
	mm, _ := m.sortByColumnName(cols[target].Name)
	return mm, true
}

// sortByColumnName cycles the active list's sort on the named column
// using the user's preferred first-press direction ([ui.startup]
// default_sort; built-in ascending). Used by the s-key.
func (m Model) sortByColumnName(name string) (Model, tea.Cmd) {
	return m.sortByColumnNameDir(name, m.settings.StartupDefaultSortDesc())
}

// sortByColumnNameDir cycles the active list's sort on the named column:
// first press → startDesc direction, then flips to the opposite, then
// clears. startDesc lets callers override the first-press direction — the
// q-s chord passes true so "sort by Last Modified" starts newest-first,
// which is what you almost always want, regardless of the global
// ascending default. Returns model + nil cmd; a no-op (no list / column
// absent) still returns cleanly.
func (m Model) sortByColumnNameDir(name string, startDesc bool) (Model, tea.Cmd) {
	state, _ := m.activeListTable()
	if state == nil || name == "" {
		return m, nil
	}
	var msg string
	switch {
	case state.SortColumn != name:
		state.SortColumn = name
		state.SortDesc = startDesc
		msg = "sorted by " + name + " " + sortArrow(startDesc)
	case state.SortDesc == startDesc:
		state.SortDesc = !startDesc
		msg = "sorted by " + name + " " + sortArrow(!startDesc)
	default:
		state.SortColumn = ""
		state.SortDesc = false
		msg = "sort cleared"
	}
	mm := &m
	mm.resetCursorForCurrentView()
	mm.flash(msg)
	return *mm, nil
}

// sortArrow returns the up/down glyph for a sort direction.
func sortArrow(desc bool) string {
	if desc {
		return "↓"
	}
	return "↑"
}

// effectiveColCursor returns the column index that "the cursor"
// points to. When state.ColCursor has been set (user has cycled
// it), use that. Otherwise fall back to the leftmost non-frozen
// column — that's the natural default for "the column under
// attention" on a fresh surface.
func effectiveColCursor(state *uilayout.ListTableState, cols []uilayout.ListColumn) int {
	if state == nil || len(cols) == 0 {
		return 0
	}
	c := state.ColCursor
	if c >= 0 && c < len(cols) {
		return c
	}
	target := state.FrozenCols
	if target >= len(cols) {
		target = len(cols) - 1
	}
	if target < 0 {
		target = 0
	}
	return target
}

// handleColSortClear unconditionally clears any active sort. Bound to
// `S` (capital). Works regardless of column-mode — if you've sorted
// somewhere and forgot, S always gets you back to the chip's natural
// order. Same cursor-snap + flash treatment as handleColSort so the
// reorder is visually obvious.
func (m Model) handleColSortClear() (Model, bool) {
	state := (&m).activeListTableState()
	if state == nil {
		return m, false
	}
	if state.SortColumn == "" {
		return m, false
	}
	state.SortColumn = ""
	state.SortDesc = false
	mm := &m
	mm.resetCursorForCurrentView()
	mm.flash("sort cleared")
	return *mm, true
}

// handlePaginateToggle flips the Paginated flag on the active
// list-table state. Pagination is per-state, so each list remembers
// whether it's in scroll-mode or paged-mode independently across
// tab switches. Standard j/k/arrow nav drives both modes; in paged
// mode the renderer recomputes which page contains the cursor and
// shows that page (so moving the cursor across the page boundary
// advances/reverses the page automatically).
//
// No-op on surfaces without a list-table state (sidebars, code
// bodies, dashboards).
func (m Model) handlePaginateToggle() (Model, bool) {
	state := (&m).activeListTableState()
	if state == nil {
		return m, false
	}
	state.Paginated = !state.Paginated
	// Page is recomputed from cursor on next render, so don't
	// touch it here. If the cursor is off-screen on the toggle's
	// first frame, the renderer will jump to whatever page the
	// cursor is on.
	return m, true
}

// handleZenToggle flips the global zen mode flag. Was previously
// per-list-table (state.Zen), but that broke "I'm reading
// minimally, drill in to see more" — drilling from /flows into
// flow-version detail switched to a surface with its own
// state.Zen=false and silently dropped the user out of zen. Now
// zen is session-global so the user's "minimal view" preference
// follows them as they navigate.
//
// Per-state Zen is still honoured by the renderer (the OR check in
// viewImpl) — set programmatically by tests / settings / future
// per-surface zen-by-default features. The toggle key only moves
// the master flag.
func (m Model) handleZenToggle() (Model, bool) {
	m.zenMode = !m.zenMode
	// Defensively clear any stale per-state Zen flags so a state
	// flagged true by an earlier (per-state) toggle doesn't keep
	// the renderer in zen after the user toggles off.
	if !m.zenMode {
		(&m).clearAllZenFlags()
	}
	return m, true
}

// clearAllZenFlags walks every list-table state we know about and
// sets Zen=false. Called on Esc-out-of-zen so a user who navigated
// across surfaces while zen was on can't get stuck with a stale
// flag on a tab they're not currently looking at.
//
// Cheap: each org has ~25 states, each state is a small struct, the
// loop runs once per Esc press in zen mode.
func (m *Model) clearAllZenFlags() {
	for _, d := range m.data {
		if d == nil {
			continue
		}
		clearZen(&d.ObjectsTableState)
		clearZen(&d.FlowsTableState)
		clearZen(&d.ApexLogsTableState)
		clearZen(&d.DeploysTableState)
		clearZen(&d.PackagesTableState)
		clearZen(&d.RecentTableState)
		clearZen(&d.PermSetsTableState)
		clearZen(&d.PSGsTableState)
		clearZen(&d.ProfilesTableState)
		clearZen(&d.QueuesTableState)
		clearZen(&d.PublicGroupsTableState)
		clearZen(&d.HomeNotifTableState)
		clearZen(&d.HomeLimitTableState)
		clearZen(&d.HomeUserTableState)
		clearZen(&d.HomeLicenseTableState)
		clearZen(&d.ApexClassesTableState)
		clearZen(&d.ApexTriggersTableState)
		clearZen(&d.LWCBundlesTableState)
		clearZen(&d.AuraBundlesTableState)
		for _, st := range d.RecordsTableState {
			clearZen(st)
		}
	}
	clearZen(&m.soqlTable)
	clearZen(&m.reportRunTable)
}

// clearZen is a nil-guarded one-liner — handles the maybe-nil
// pointer case (records table-state map values can be nil if the
// per-(sobject,chip) entry was never visited).
func clearZen(s *uilayout.ListTableState) {
	if s == nil {
		return
	}
	s.Zen = false
}

// contentWidth approximates the inner pane width the list-table will
// actually receive at render time. Used by resize/scroll handlers to
// run their layout against the same budget the renderer uses, so
// SetUserWidth clamps land in a useful range. Mirrors the math in
// render.go's body-pane calculation.
func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	const borderCols = 2
	widgetTotal := 0
	if m.leftOpen {
		widgetTotal = clamp(m.width/5, 24, 34)
	}
	sideTotal := 0
	if m.sidebarOpen {
		sideTotal = clamp(m.width/4, 28, 48)
	}
	mainTotal := m.width - widgetTotal - sideTotal
	if mainTotal < 20 {
		mainTotal = 20
	}
	return mainTotal - borderCols - 4
}
