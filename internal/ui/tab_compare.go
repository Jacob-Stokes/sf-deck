package ui

import (
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m Model) renderCompare(w, innerH int) string {
	return m.dispatchSubtab(w, innerH, m.tabSubtabs(), m.compareSubtabIdx,
		map[Subtab]subtabBranch{
			SubtabCompareResult:  {Render: m.renderCompareResult},
			SubtabCompareSaved:   {Render: m.renderCompareSavedSubtab},
			SubtabCompareHistory: {Render: m.renderCompareHistorySubtab},
		},
		subtabBranch{Render: m.renderCompareNew},
	)
}

func (m Model) renderCompareNew(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	return m.renderCompareSetup(w, innerH, d)
}

func (m Model) renderCompareResult(w, innerH int) string {
	d, ok := m.activeOrgState()
	if !ok {
		return noOrgPlaceholder()
	}
	run := d.Run
	switch {
	case run == nil || run.Phase == comparePhaseSetup:
		var lines []string
		lines = append(lines, sectionTitle("COMPARE · RESULT"))
		lines = append(lines, "")
		lines = append(lines, theme.Subtle.Render("  No active comparison."))
		lines = append(lines, theme.Subtle.Render("  Set one up in the New subtab, or open a saved one from Saved."))
		return strings.Join(lines, "\n")
	case run.Phase == comparePhaseRetrieving:
		return m.renderCompareRetrieving(w, innerH, run)
	default:
		if d.Diff != nil {
			return m.renderCompareDiff(w, innerH, d)
		}
		return m.renderCompareInventory(w, innerH, d)
	}
}

type compareSetupRow int

const (
	setupRowSource compareSetupRow = iota
	setupRowTarget
	setupRowScope
	setupRowMethod
	setupRowCompare
)

func compareSetupRowsFor(d *orgData) []compareSetupRow {
	return []compareSetupRow{setupRowSource, setupRowTarget, setupRowScope, setupRowMethod, setupRowCompare}
}

func (m Model) renderCompareSetup(w, innerH int, d *orgData) string {
	inner := w - 4
	var lines []string
	lines = append(lines, sectionTitle("COMPARE"))
	lines = append(lines, "")
	lines = append(lines, dimLine("  Set up an org-to-org metadata comparison.", inner))
	lines = append(lines, "")

	source, target, scope, method := m.compareSetupValues(d)
	rowVal := map[compareSetupRow]struct{ label, val string }{
		setupRowSource: {"Source", source},
		setupRowTarget: {"Target", target},
		setupRowScope:  {"Scope", scope},
		setupRowMethod: {"Method", method},
	}
	rows := compareSetupRowsFor(d)
	cur := d.SetupCursor
	for i, rk := range rows {
		if rk == setupRowCompare {
			continue // rendered separately below
		}
		r := rowVal[rk]
		prefix := "   "
		labelStyle := theme.Subtle
		if i == cur {
			prefix = lipgloss.NewStyle().Foreground(theme.BorderHi).Render(" ▌ ")
			labelStyle = lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
		}
		lines = append(lines, prefix+labelStyle.Render(padRight(r.label, 9))+"▸  "+r.val)
	}
	lines = append(lines, "")

	lines = append(lines, "")
	lines = append(lines, dimLine("  "+m.compareCostLine(d), inner))

	onCompare := cur == len(rows)-1
	actLabel := theme.Subtle.Render("  Compare")
	if onCompare {
		actLabel = lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("❮ Compare ❯")
	}
	lines = append(lines, "     "+actLabel)
	lines = append(lines, "")
	lines = append(lines, dimLine("  ↑↓ move · enter change/run · esc back", inner))
	if d.Run != nil && d.Run.Err != nil {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render("  "+d.Run.Err.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m Model) compareSetupValues(d *orgData) (source, target, scope, method string) {
	var src, tgt endpoint
	mth := compareMethodAuto
	if d.Run != nil {
		src, tgt = d.Run.Source, d.Run.Target
		scope = scopeLabel(d.Run.Scope)
		mth = d.Run.Method
	}
	if src.IsZero() && len(m.orgs) > 0 {
		src = orgEndpoint(m.orgs[m.selected].Username)
	}
	source = endpointDisplay(m, src)
	if tgt.IsZero() {
		target = theme.Subtle.Render("(choose target)")
	} else {
		target = endpointDisplay(m, tgt)
	}
	if scope == "" {
		scope = theme.Subtle.Render("(none — press enter to pick types)")
	}
	method = mth.String() + "  " + theme.Subtle.Render(methodHint(mth))
	return source, target, scope, method
}

// compareCostLine renders "est. ~N API calls · X of Y daily remaining"
// for the active run's method+scope, so the user sees the tradeoff
// before running. The estimate is deliberately rough — order of
// magnitude is what matters for the route decision.
func (m Model) compareCostLine(d *orgData) string {
	method := compareMethodAuto
	var scope []string
	if d.Run != nil {
		method = d.Run.Method
		scope = d.Run.Scope
	}
	if len(scope) == 0 {
		scope = defaultCompareScope()
	}
	est := estimateCompareCalls(method, scope)
	budget := ""
	if rem, max, ok := m.dailyAPIBudget(); ok {
		budget = fmt.Sprintf(" · %d of %d daily API calls remaining", rem, max)
	}
	// "≥" because the retrieve cost scales with component COUNTS (not just
	// type counts), which we can't know until we've listed — so the real
	// number is this or higher. Per org, ×2 for both source+target.
	return fmt.Sprintf("est. ≥%d API calls per org, ×2 orgs (%s)%s", est, method.String(), budget)
}

// estimateCompareCalls is a rough per-org API-call estimate built from the
// same comparePlan the runner uses. It's a floor: readMetadata cost grows
// with component counts, which we cannot know until after listMetadata.
func estimateCompareCalls(method compareMethod, scope []string) int {
	return buildComparePlan(scope, method).EstimatedCalls
}

func (m Model) dailyAPIBudget() (int, int, bool) {
	if len(m.orgs) == 0 {
		return 0, 0, false
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return 0, 0, false
	}
	for _, l := range d.Home.Value().KeyLimits {
		if l.Name == "DailyApiRequests" && l.Max > 0 {
			return l.Remaining, l.Max, true
		}
	}
	return 0, 0, false
}

func methodHint(cm compareMethod) string {
	switch cm {
	case compareMethodTooling:
		return "fast · Apex only · lazy bodies"
	case compareMethodMetadataAPI:
		return "slower · fewest API calls · all types"
	default:
		return "Tooling where possible, Metadata API for the rest"
	}
}

func defaultCompareScope() []string {
	var out []string
	for _, p := range compareProviders() {
		out = append(out, p.TypeLabel())
	}
	return out
}

func scopeLabel(scope []string) string {
	if len(scope) == 0 {
		return ""
	}
	return strings.Join(scope, ", ")
}

func endpointDisplay(m Model, e endpoint) string {
	if e.IsZero() {
		return theme.Subtle.Render("(none)")
	}
	if e.Kind == endpointProject {
		return theme.Subtle.Render("project:") + e.Ref
	}
	username := e.Ref
	for _, o := range m.orgs {
		if o.Username == username {
			if o.Alias != "" {
				return o.Alias + theme.Subtle.Render("  "+username)
			}
			return username
		}
	}
	return username
}

func endpointLabel(m Model, e endpoint) string {
	if e.IsZero() {
		return "(none)"
	}
	if e.Kind == endpointProject {
		return "project:" + e.Ref
	}
	for _, o := range m.orgs {
		if o.Username == e.Ref && o.Alias != "" {
			return o.Alias
		}
	}
	return e.Ref
}

func (m Model) renderCompareRetrieving(w, innerH int, run *compareRun) string {
	inner := w - 4
	var lines []string
	lines = append(lines, sectionTitle("COMPARE   "+m.compareTitleArrow(run)))

	done, failed, total := 0, 0, run.expected
	for _, p := range run.Progress {
		switch p.State {
		case retrieveDone:
			done++
		case retrieveFailed:
			done++
			failed++
		}
	}
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	spin := compareSpinner(m.compareFrame)
	bar := compareAnimatedBar(done, total, 30, m.compareFrame)
	dots := compareDots(m.compareFrame)
	status := fmt.Sprintf("%s Retrieving metadata%s  %s  %d/%d (%d%%)", spin, dots, bar, done, total, pct)
	if run.diffing || (done >= total && total > 0) {
		status = fmt.Sprintf("%s Retrieved %d/%d — computing diff%s (this can take a moment for large scopes)", spin, done, total, dots)
	}
	lines = append(lines, "  "+lipgloss.NewStyle().Foreground(theme.Yellow).Render(status))

	// Failures summary: list each failed (side, type) WITH its reason up
	// front, so the user never has to scroll the ~600-row list to find or
	// diagnose them. Capped; the rest are reachable by scrolling.
	if failed > 0 {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render(
			fmt.Sprintf("%d type(s) failed:", failed)))
		const maxShown = 6
		shown := 0
		red := lipgloss.NewStyle().Foreground(theme.Red)
		for _, t := range run.Scope {
			for _, side := range []string{"source", "target"} {
				p := run.Progress[progressKey(side, t)]
				if p.State != retrieveFailed {
					continue
				}
				if shown >= maxShown {
					continue
				}
				reason := p.Note
				if reason == "" {
					reason = "failed"
				}
				lines = append(lines, "    "+red.Render("✗ "+t)+theme.Subtle.Render(" ["+side+"] ")+red.Render(reason))
				shown++
			}
		}
		if failed > maxShown {
			lines = append(lines, theme.Subtle.Render(fmt.Sprintf("    …and %d more (scroll the list below)", failed-maxShown)))
		}
	}
	lines = append(lines, "")

	colW := (inner - 3) / 2
	if colW < 16 {
		colW = 16
	}
	bar2 := lipgloss.NewStyle().Foreground(theme.BorderHi).Render("│")
	header := "  " + padRight("SOURCE  "+endpointLabel(m, run.Source), colW) + " " + bar2 + " " +
		"TARGET  " + endpointLabel(m, run.Target)

	// Window the per-type rows against remaining height so a 600-type
	// scope doesn't overflow the viewport (the bug: failures off-screen,
	// unreachable). budget = lines left after the header rows + header
	// row + footer + the "more" affordances.
	budget := innerH - len(lines) - 4
	if budget < 3 {
		budget = 3
	}
	total2 := len(run.Scope)
	start := clampRetrieveScroll(run.RetrieveScroll, total2, budget)
	run.RetrieveScroll = start
	end := start + budget
	if end > total2 {
		end = total2
	}

	lines = append(lines, header)
	if start > 0 {
		lines = append(lines, theme.Subtle.Render(fmt.Sprintf("  ↑ %d more above", start)))
	}
	for i := start; i < end; i++ {
		t := run.Scope[i]
		s := run.Progress[progressKey("source", t)]
		g := run.Progress[progressKey("target", t)]
		left := compareProgressCell(t, s, m.compareFrame)
		right := compareProgressCell(t, g, m.compareFrame)
		lines = append(lines, "  "+padRight(left, colW)+" "+bar2+" "+right)
	}
	if end < total2 {
		lines = append(lines, theme.Subtle.Render(fmt.Sprintf("  ↓ %d more below", total2-end)))
	}
	lines = append(lines, "")
	lines = append(lines, dimLine("  ↑↓/jk scroll · esc to cancel", inner))
	return strings.Join(lines, "\n")
}

func (m Model) compareRetrieveScrollActive() bool {
	if m.tab() != TabCompare || m.currentSubtab() != SubtabCompareResult {
		return false
	}
	d := m.activeOrgData()
	return d != nil && d.Run != nil && d.Run.Phase == comparePhaseRetrieving
}

// clampRetrieveScroll keeps the retrieving-list scroll offset valid for
// the row count and window height.
func clampRetrieveScroll(off, total, window int) int {
	if off < 0 || total <= window {
		return 0
	}
	max := total - window
	if off > max {
		return max
	}
	return off
}

func compareProgressCell(typeLabel string, p retrieveProgress, frame int) string {
	var icon string
	switch p.State {
	case retrieveDone:
		icon = lipgloss.NewStyle().Foreground(theme.Green).Render("✓")
	case retrieveFailed:
		icon = lipgloss.NewStyle().Foreground(theme.Red).Render("✗")
	default:
		icon = lipgloss.NewStyle().Foreground(theme.Yellow).Render(compareSpinner(frame))
	}
	suffix := ""
	switch p.State {
	case retrieveDone:
		suffix = theme.Subtle.Render(fmt.Sprintf(" (%d)", p.Count))
		if p.Note != "" {
			suffix += theme.Subtle.Render(" " + p.Note)
		}
	case retrieveFailed:
		reason := p.Note
		if reason == "" {
			reason = "failed"
		}
		suffix = lipgloss.NewStyle().Foreground(theme.Red).Render(" " + reason)
	}
	return icon + " " + typeLabel + suffix
}

func shortErr(err error) string {
	if err == nil {
		return "failed"
	}
	s := err.Error()
	if i := strings.Index(s, ": "); i >= 0 && i < 40 {
		rest := s[i+2:]
		if rest != "" {
			s = rest
		}
	}
	s = strings.TrimSpace(s)
	const max = 60
	if len(s) > max {
		s = ansi.Truncate(s, max, "…")
	}
	return s
}

var compareSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func compareSpinner(frame int) string {
	if len(compareSpinnerFrames) == 0 {
		return ""
	}
	return compareSpinnerFrames[frame%len(compareSpinnerFrames)]
}

func compareDots(frame int) string {
	return strings.Repeat(".", frame%4)
}

func compareAnimatedBar(done, total, width, frame int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < width; i++ {
		switch {
		case i < filled:
			b.WriteString("█")
		case i-filled == frame%maxInt(1, width-filled):
			b.WriteString("▓") // the travelling shimmer in the remaining track
		default:
			b.WriteString("░")
		}
	}
	b.WriteString("]")
	return b.String()
}

func humanizeCompareAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "moments"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) renderCompareInventory(w, innerH int, d *orgData) string {
	inner := w - 4
	run := d.Run
	var head []string
	same, different, aOnly, bOnly := run.Inv.Summary()
	head = append(head, sectionTitle("COMPARE   "+m.compareTitleArrow(run)))
	head = append(head, dimLine(fmt.Sprintf("  %d components · %d differ · %d source-only · %d target-only · %d same",
		len(run.Inv.Rows), different, aOnly, bOnly, same), inner))
	if !run.OpenedSavedAt.IsZero() {
		age := humanizeCompareAge(time.Since(run.OpenedSavedAt))
		head = append(head, lipgloss.NewStyle().Foreground(theme.Yellow).Render(
			fmt.Sprintf("  ⚠ saved result — ran %s ago; press R to re-run for current state", age)))
	}
	if len(run.Inv.Errors) > 0 {
		for _, e := range run.Inv.Errors {
			head = append(head, lipgloss.NewStyle().Foreground(theme.Red).Render(
				fmt.Sprintf("  %s (%s): %v", e.Type, e.Side, e.Err)))
		}
	}
	head = append(head, "")

	model, ok := compareInventoryListSurface.BuildRenderModel(m, d)
	if !ok {
		return strings.Join(append(head, theme.Subtle.Render("  no comparison")), "\n")
	}
	budget := innerH - len(head)
	if budget < 5 {
		budget = 5
	}
	body := renderListModel(m, model, m.focus, inner, budget)
	return strings.Join(append(head, body...), "\n")
}

func compareInventoryColumnSchema() tablemodel.Schema[diff.Row] {
	return tablemodel.Schema[diff.Row]{
		DefaultColumns: func(scope string) []string {
			return []string{"Status", "Type", "Name", "Source", "Target"}
		},
		Columns: map[string]tablemodel.ColumnDef[diff.Row]{
			"Status": {
				Header:     "STATUS",
				Width:      tablemodel.Width{Min: 12, Ideal: 12, Max: 12},
				Unsortable: false,
				Render:     func(r diff.Row) string { return r.Status.String() },
			},
			"Type":   textColumnDef[diff.Row]("TYPE", tablemodel.Width{Min: 10, Ideal: 14}, func(r diff.Row) string { return r.Type }),
			"Name":   textColumnDef[diff.Row]("NAME", tablemodel.Width{Min: 16, Ideal: 34}, func(r diff.Row) string { return r.Key }),
			"Source": textColumnDef[diff.Row]("SOURCE", tablemodel.Width{Min: 8, Ideal: 18}, func(r diff.Row) string { return r.ASummary }),
			"Target": textColumnDef[diff.Row]("TARGET", tablemodel.Width{Min: 8, Ideal: 18}, func(r diff.Row) string { return r.BSummary }),
		},
	}
}

var compareInventoryListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.InventoryTable },
	Cols:        func() []uilayout.ListColumn { return schemaListColumns(compareInventoryColumnSchema()) },
	SearchPtr:   func(d *orgData) *searchState { return d.InventoryList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.InventoryList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.InventoryList.ResetCursor() },
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil || d.Run == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(compareInventoryColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(&d.InventoryList, &d.InventoryTable, cols,
			func(items []diff.Row, row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedSortCellByID(resolved, items[row], cols[col].Name)
			})
		items := d.InventoryList.Filtered()
		return listRenderModel{
			Title:  headerWithSearchPill("INVENTORY", *d.InventoryList.SearchPtr()),
			State:  &d.InventoryTable,
			Search: d.InventoryList.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: d.InventoryList.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) || col < 0 || col >= len(cols) {
					return ""
				}
				return resolvedCellByID(resolved, items[row], cols[col].Name)
			},
			Recolor: func(row, col int, base lipgloss.Style) lipgloss.Style {
				if row < 0 || row >= len(items) {
					return base
				}
				if cols[col].Name != "Status" {
					return base
				}
				switch items[row].Status {
				case diff.StatusDifferent:
					return base.Foreground(theme.Red)
				case diff.StatusAOnly:
					return base.Foreground(theme.Cyan)
				case diff.StatusBOnly:
					return base.Foreground(theme.Green)
				case diff.StatusSame:
					return base.Foreground(theme.FgDim)
				}
				return base
			},
			Empty:        "  no components in scope",
			FooterExtras: "↵ view diff · e edit/re-run · ctrl+s save",
		}, true
	},
}

// syncInventoryList pushes the run's rows into the ListView. Idempotent
// (Set no-ops on same backing slice). Installs the matcher once.
func (d *orgData) syncInventoryList() {
	if d.Run == nil {
		d.InventoryList.Set(nil)
		return
	}
	if !d.InventoryList.HasMatch() {
		d.InventoryList.SetMatch(func(r diff.Row, q string) bool {
			return strings.Contains(strings.ToLower(r.Key+" "+r.Type), q)
		})
	}
	d.InventoryList.Set(d.Run.Inv.Rows)
}

func (m *Model) moveCompareCursor(delta int) {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	switch m.currentSubtab() {
	case SubtabCompareNew:
		d.SetupCursor = clampDelta(d.SetupCursor, delta, len(compareSetupRowsFor(d)))
	case SubtabCompareResult:
		if d.Run == nil || d.Run.Phase != comparePhaseInventory {
			return // retrieving screen scrolls via compareRetrieveScrollActive
		}
		if d.Diff != nil {
			d.Diff.Scroll = clampScroll(d.Diff.Scroll+delta, len(d.Diff.Result.Lines))
			return
		}
		d.InventoryList.MoveBy(delta)
	}
}

func (m *Model) resetCompareCursor() {
	d, ok := m.activeOrgState()
	if !ok {
		return
	}
	if d.Diff != nil {
		d.Diff.Scroll = 0
		return
	}
	d.InventoryList.ResetCursor()
}

func (m Model) compareSearchPtr() *searchState {
	d, ok := m.activeOrgState()
	if !ok {
		return nil
	}
	if m.currentSubtab() == SubtabCompareResult && d.Run != nil &&
		d.Run.Phase == comparePhaseInventory && d.Diff == nil {
		return d.InventoryList.SearchPtr()
	}
	return nil
}

func compareListTable(m *Model) (*uilayout.ListTableState, []uilayout.ListColumn) {
	d, ok := m.activeOrgState()
	if !ok || d.Run == nil || d.Run.Phase != comparePhaseInventory || d.Diff != nil {
		return nil, nil
	}
	if m.currentSubtab() != SubtabCompareResult {
		return nil, nil
	}
	return &d.InventoryTable, schemaListColumns(compareInventoryColumnSchema())
}

func (m Model) compareDiffOpen() bool {
	d, ok := m.activeOrgState()
	return ok && d.Diff != nil
}

func clampScroll(v, n int) int {
	if v < 0 {
		return 0
	}
	if n > 0 && v >= n {
		return n - 1
	}
	if n == 0 {
		return 0
	}
	return v
}

type compareInventoryMsg struct {
	OrgKey       string
	Inv          diff.Inventory
	SnapA, SnapB diff.Snapshot
	Err          error
}

func (m *Model) startCompare(d *orgData) tea.Cmd {
	if d.Run == nil {
		return nil
	}
	run := d.Run
	plan := buildComparePlan(run.Scope, run.Method)
	if err := plan.validate(); err != nil {
		run.Phase = comparePhaseSetup
		run.Err = err
		return nil
	}
	run.Phase = comparePhaseRetrieving
	run.Err = nil
	m.compareSubtabIdx = compareSubtabResultIdx
	orgKey := ""
	if len(m.orgs) > 0 {
		orgKey = m.orgs[m.selected].Username
	}
	source, target := run.Source.OrgRef(), run.Target.OrgRef()

	if run.Method == compareMethodTooling {
		providers := plan.Providers
		work := func() tea.Msg {
			inv := diff.CompareInventory(source, target, providers)
			return compareInventoryMsg{OrgKey: orgKey, Inv: inv}
		}
		return tea.Batch(work, m.compareTickCmd())
	}

	// Auto / Metadata API → ONE retrieve PER (side, type), all fired
	// concurrently (Gearset-style chunking: many bounded retrieves, not
	// one giant one that times out). Each emits a compareTypeDoneMsg the
	// progress screen consumes; the Update handler assembles the snapshot
	// + diffs once all units land.
	// Release the PRIOR run's bodies before allocating new ones, so a
	// fresh comparison reclaims the old (potentially ~hundreds of MB)
	// immediately rather than waiting on the next GC cycle.
	run.snapA = nil
	run.snapB = nil
	run.hashA = nil
	run.hashB = nil
	run.snapA = diff.Snapshot{}
	run.snapB = diff.Snapshot{}
	run.hashA = diff.Snapshot{}
	run.hashB = diff.Snapshot{}
	run.retainedBytes = 0
	run.Progress = map[string]retrieveProgress{}
	run.diffing = false
	run.RetrieveScroll = 0
	// Snapshot the memory budget for this run (so a mid-run settings edit
	// can't skew accounting). See compareRun.recordComponents / retainBody.
	run.bodyCap = m.settings.CompareBodyCapBytes()
	run.retainCeiling = m.settings.CompareRetainCeilingBytes()
	run.sem = make(chan struct{}, m.settings.CompareConcurrency())

	pend := func(side, t string) {
		run.Progress[progressKey(side, t)] = retrieveProgress{Side: side, Type: t, State: retrievePending}
	}
	var cmds []tea.Cmd
	expected := 0
	for _, t := range plan.PerTypes {
		t := t
		pend("source", t)
		pend("target", t)
		cmds = append(cmds,
			retrieveTypeCmd(run, orgKey, "source", source, t),
			retrieveTypeCmd(run, orgKey, "target", target, t),
		)
		expected += 2
	}
	if len(plan.ObjectTypes) > 0 {
		for _, t := range plan.ObjectTypes {
			pend("source", t)
			pend("target", t)
			expected += 2
		}
		cmds = append(cmds,
			retrieveObjectsCmd(run, orgKey, "source", source, plan.ObjectTypes),
			retrieveObjectsCmd(run, orgKey, "target", target, plan.ObjectTypes),
		)
	}
	run.expected = expected
	cmds = append(cmds, m.compareTickCmd()) // drive the retrieving-screen animation
	return tea.Batch(cmds...)
}

type compareTickMsg struct{}

const compareTickInterval = 200 * time.Millisecond

func (m *Model) compareTickCmd() tea.Cmd {
	if m.compareTickRunning {
		return nil
	}
	m.compareTickRunning = true
	return tea.Tick(compareTickInterval, func(time.Time) tea.Msg { return compareTickMsg{} })
}

func (m Model) compareRetrieveInFlight() bool {
	for _, d := range m.data {
		if d != nil && d.Run != nil && d.Run.Phase == comparePhaseRetrieving {
			return true
		}
	}
	return false
}

func splitObjectChildScope(scope []string) (objChildren, perType []string) {
	includeAllObjectBuckets := hasType(scope, "CustomObject")
	addedObject := map[string]bool{}
	addObject := func(t string) {
		if !addedObject[t] {
			addedObject[t] = true
			objChildren = append(objChildren, t)
		}
	}
	if includeAllObjectBuckets {
		for _, t := range compareObjectRootedTypeOrder {
			addObject(t)
		}
	}
	for _, t := range scope {
		if compareObjectRootedTypes[t] {
			if !includeAllObjectBuckets {
				addObject(t)
			}
		} else {
			perType = append(perType, t)
		}
	}
	return
}

type compareTypeDoneMsg struct {
	OrgKey     string
	Side       string // "source" / "target"
	Type       string
	Components map[string]string // name → xml
	Chunked    bool
	Err        error
}

var apexCompareTypes = map[string]bool{"ApexClass": true, "ApexTrigger": true}

func retrieveTypeCmd(run *compareRun, orgKey, side, alias, metadataType string) tea.Cmd {
	return func() tea.Msg {
		var comps map[string]string
		var err error
		if apexCompareTypes[metadataType] {
			run.acquire()
			comps, err = sf.BulkApexBodies(alias, metadataType)
			run.release()
		} else {
			var names []string
			run.acquire()
			names, err = listMembers(alias, metadataType)
			run.release()
			if err == nil {
				var snap sf.SOAPSnapshot
				if snap, err = sf.RetrieveViaSOAPGated(alias, metadataType, names, false, run.acquire, run.release); err == nil {
					comps = snap[metadataType]
				}
			}
		}
		return compareTypeDoneMsg{
			OrgKey: orgKey, Side: side, Type: metadataType, Components: comps, Err: err,
		}
	}
}

type compareObjectsDoneMsg struct {
	OrgKey  string
	Side    string
	InScope []string                     // object-child types the user selected
	Buckets map[string]map[string]string // type → (key → xml)
	Err     error
}

func retrieveObjectsCmd(run *compareRun, orgKey, side, alias string, inScope []string) tea.Cmd {
	return func() tea.Msg {
		run.acquire()
		names, err := listMembers(alias, "CustomObject")
		run.release()
		var buckets map[string]map[string]string
		if err == nil {
			var snap sf.SOAPSnapshot
			if snap, err = sf.RetrieveViaSOAPGated(alias, "CustomObject", names, true, run.acquire, run.release); err == nil {
				buckets = snap
			}
		}
		return compareObjectsDoneMsg{
			OrgKey: orgKey, Side: side, InScope: inScope, Buckets: buckets, Err: err,
		}
	}
}

func listMembers(alias, metadataType string) ([]string, error) {
	c, err := sf.RESTClient(alias)
	if err != nil {
		return nil, err
	}
	byType, err := c.ListMetadata([]string{metadataType})
	if err != nil {
		return nil, err
	}
	items := byType[metadataType]
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.NamespacePrefix == "" {
			out = append(out, it.FullName)
		}
	}
	return out, nil
}

func (m *Model) applyCompareTypeDone(d *orgData, msg compareTypeDoneMsg) tea.Cmd {
	run := d.Run
	if run == nil || run.Phase != comparePhaseRetrieving {
		return nil // stale (user navigated away / re-ran)
	}
	st := retrieveDone
	note := ""
	if msg.Err != nil {
		st = retrieveFailed
		note = shortErr(msg.Err)
	} else if msg.Chunked {
		note = "chunked"
	}
	run.Progress[progressKey(msg.Side, msg.Type)] = retrieveProgress{
		Side: msg.Side, Type: msg.Type, State: st,
		Count: len(msg.Components), Note: note,
	}
	if msg.Err == nil && len(msg.Components) > 0 {
		run.recordComponents(msg.Side, msg.Type, msg.Components)
	}
	return m.maybeFinishCompare(d)
}

func (m *Model) applyCompareObjectsDone(d *orgData, msg compareObjectsDoneMsg) tea.Cmd {
	run := d.Run
	if run == nil || run.Phase != comparePhaseRetrieving {
		return nil
	}
	for _, t := range msg.InScope {
		st := retrieveDone
		note := "via object"
		count := 0
		if msg.Err != nil {
			st = retrieveFailed
			note = shortErr(msg.Err)
		} else {
			comps := msg.Buckets[t]
			count = len(comps)
			run.recordComponents(msg.Side, t, comps)
		}
		run.Progress[progressKey(msg.Side, t)] = retrieveProgress{
			Side: msg.Side, Type: t, State: st, Count: count, Note: note,
		}
	}
	return m.maybeFinishCompare(d)
}

func (run *compareRun) snapshotFor(side string) diff.Snapshot {
	if side == "target" {
		return run.snapB
	}
	return run.snapA
}

func (m *Model) maybeFinishCompare(d *orgData) tea.Cmd {
	run := d.Run
	done := 0
	for _, p := range run.Progress {
		if p.State == retrieveDone || p.State == retrieveFailed {
			done++
		}
	}
	if done < run.expected || run.diffing {
		return nil
	}
	run.diffing = true
	orgKey := ""
	if len(m.orgs) > 0 {
		orgKey = m.orgs[m.selected].Username
	}
	// Diff over the HASH sidecars (complete for every component, even those
	// whose body was dropped). Carry the retained BODIES through as
	// SnapA/SnapB for drill-in. The cmd must not touch Model/run state.
	ha, hb := run.hashA, run.hashB
	ba, bb := run.snapA, run.snapB
	var typeErrs []diff.TypeError
	for _, p := range run.Progress {
		if p.State == retrieveFailed {
			note := p.Note
			if note == "" {
				note = "retrieve failed"
			}
			typeErrs = append(typeErrs, diff.TypeError{
				Type: p.Type, Side: p.Side, Err: errors.New(note),
			})
		}
	}
	return func() tea.Msg {
		inv := diff.CompareSnapshots(ha, hb, typeErrs...)
		return compareInventoryMsg{OrgKey: orgKey, Inv: inv, SnapA: ba, SnapB: bb}
	}
}

func (m Model) compareTitleArrow(run *compareRun) string {
	return endpointLabel(m, run.Source) + " → " + endpointLabel(m, run.Target)
}

func providersForScopeMethod(scope []string, method compareMethod) []diff.Provider {
	all := providersForMethod(method)
	if len(scope) == 0 {
		return all
	}
	want := map[string]bool{}
	for _, s := range scope {
		want[s] = true
	}
	var out []diff.Provider
	for _, p := range all {
		if want[p.TypeLabel()] {
			out = append(out, p)
		}
	}
	return out
}
