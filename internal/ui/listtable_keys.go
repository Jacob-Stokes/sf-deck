package ui

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

func listTableObjectDetailDispatch(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	switch m.currentSubtab() {
	case SubtabRecords:
		return m.recordsListTable()
	case SubtabSchema:
		return m.schemaListTable()
	}
	return nil, nil
}

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

func sobjectListCols() []uilayout.ListColumn {
	return mustResolveColumns(sobjectColumnSchema()).ListColumns()
}

func flowListCols() []uilayout.ListColumn {
	return mustResolveColumns(flowColumnSchema()).ListColumns()
}

func resizeTargetCol(state *uilayout.ListTableState, totalCols int) int {
	if state == nil || totalCols == 0 {
		return 0
	}
	dummy := make([]uilayout.ListColumn, totalCols)
	return effectiveColCursor(state, dummy)
}

func ensureColCursorVisible(state *uilayout.ListTableState, cols []uilayout.ListColumn, inner int) {
	if state == nil || len(cols) == 0 {
		return
	}
	c := state.ColCursor
	frozen := state.FrozenCols
	if c < frozen {
		state.HScroll = frozen
		return
	}
	spec := uilayout.ListTableSpec{Cols: cols, N: 0, Cell: func(int, int) string { return "" }}
	res := uilayout.LayoutListTable(spec, state, inner)
	if !res.Overflow {
		return
	}
	if c < res.HScroll {
		state.HScroll = c
		return
	}
	const gutter = 2
	const sepW = 3
	for state.HScroll < c {
		used := gutter
		for i := 0; i < frozen && i < len(cols); i++ {
			if used > gutter {
				used += sepW
			}
			used += res.Widths[i]
		}
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

func (m Model) handleColResize(delta int) (Model, tea.Cmd, bool) {
	ctx := (&m).activeListTableContext()
	if ctx.State == nil || len(ctx.Cols) == 0 {
		return m, nil, false
	}
	target := resizeTargetCol(ctx.State, len(ctx.Cols))
	spec := uilayout.ListTableSpec{Cols: ctx.Cols, N: 0, Cell: func(int, int) string { return "" }}
	res := uilayout.LayoutListTable(spec, ctx.State, m.contentWidth())
	uilayout.StepResize(spec, ctx.State, res, target, delta, m.settings.LayoutColumnResizeStep())
	return m, m.saveListTableWidthsCmd(ctx), true
}

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

func (m Model) sortByColumnName(name string) (Model, tea.Cmd) {
	return m.sortByColumnNameDir(name, m.settings.StartupDefaultSortDesc())
}

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

func (m Model) handlePaginateToggle() (Model, bool) {
	state := (&m).activeListTableState()
	if state == nil {
		return m, false
	}
	state.Paginated = !state.Paginated
	return m, true
}

func (m Model) handleZenToggle() (Model, bool) {
	m.zenMode = !m.zenMode
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
