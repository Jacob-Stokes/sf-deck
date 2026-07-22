package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// renderSOQL draws the bespoke SOQL workspace: query editor, run
// button, results table. The editor captures keys while m.soqlEditing
// is true (see handleSOQLKey in update_keys.go).
//
// Above-the-table chrome (input widget, tooling badge, busy/error
// states) stays in this orchestrator since it's SOQL-specific. The
// results-table block flows through the shared renderListModel via
// soqlRenderModel below — same renderer everything else on a
// list-table surface uses.
//
// Branches by subtab: Library uses the listSurface registry path
// (rendered uniformly by renderListSurface); Editor stays in this
// function because the input widget + run/error/results chrome is
// SOQL-specific and doesn't fit the shared list shell.
//
// The subtab strip is rendered once here and shared between the
// two branches so the user sees Editor / Library pills regardless
// of which subtab is active. Each branch then gets the remaining
// vertical budget to fill.
func (m Model) renderSOQL(w, innerH int) string {
	// No org connected? The editor / saved / history surfaces all
	// depend on an active org for either running or scoping rows;
	// route through the shared placeholder so the user is pointed
	// back to /home where the welcome panel lives.
	if len(m.orgs) == 0 {
		return noOrgPlaceholder()
	}
	return m.dispatchSubtab(w, innerH, m.tabSubtabs(), m.soqlSubtabIdx,
		map[Subtab]subtabBranch{
			SubtabSOQLSaved:   {Render: m.renderSOQLSaved},
			SubtabSOQLHistory: {Render: m.renderSOQLHistory},
		},
		subtabBranch{Render: m.renderSOQLEditor},
	)
}

// renderSOQLEditor is the Editor subtab body — query input,
// run/error/results chrome. dispatchSubtab handles the strip; this
// function only owns the Editor-specific layout.
func (m Model) renderSOQLEditor(w, innerH int) string {
	return m.renderSOQLSessionBody(&m.soqlSession, w, innerH, soqlSessionBodyOptions{
		showSavedActions: true,
		title:            "SOQL",
	})
}

type soqlSessionBodyOptions struct {
	title            string
	showSavedActions bool
	modal            bool
}

func (m Model) renderSOQLSessionBody(s *soqlSession, w, innerH int, opts soqlSessionBodyOptions) string {
	if s == nil {
		return ""
	}
	inner := w - 4
	title := opts.title
	if title == "" {
		title = "SOQL"
	}
	titleSuffix := ""
	if s.soqlTooling {
		titleSuffix += "  " + lipgloss.NewStyle().Foreground(theme.Magenta).Render("[tooling]")
	}
	if s.soqlBulk {
		titleSuffix += "  " + lipgloss.NewStyle().Foreground(theme.Cyan).Render("[bulk]")
	}

	var lines []string
	lines = append(lines, sectionTitle(title+titleSuffix))

	// Size the textarea to fit ALL its content (logical lines +
	// soft-wrap). With MaxHeight=500 (set in newSOQLInput) and
	// height >= cursor row, repositionView always settles YOffset
	// to 0, so the textarea's internal viewport never scrolls and
	// the cursor renders via the widget's native virtual-cursor
	// path.
	inputW := inner - 2
	if inputW < 20 {
		inputW = 20
	}
	s.soqlInput.SetWidth(inputW)
	visibleRows := computeTextareaVisibleRows(s.soqlInput.Value(), inputW)
	if visibleRows < 1 {
		visibleRows = 1
	}
	s.soqlInput.SetHeight(visibleRows)
	editorLines := strings.Split(s.soqlInput.View(), "\n")
	if !s.soqlEditing && len(editorLines) == 1 && strings.TrimSpace(editorLines[0]) == "" {
		editorLines = []string{lipgloss.NewStyle().Foreground(theme.FgDim).
			Render("(press " + firstPretty(Keys.SOQLEdit) + " to edit)")}
	}
	// "> " prompt on first line, two-space indent on continuations.
	for i, line := range editorLines {
		prompt := "  "
		if i == 0 {
			prompt = "  > "
		}
		lines = append(lines, ansi.Truncate(prompt+line, inner, "…"))
	}

	// Autocomplete popup — always rendered below the input line
	// while the editor is active so the layout never jumps as
	// suggestions appear/disappear. Lazy-init the state on first
	// render so the engine can populate it on the first keystroke.
	if s.soqlEditing {
		if s.autocomplete == nil {
			s.autocomplete = &autocompleteState{Enabled: true}
		}
		if popup := renderAutocompletePopup(s.autocomplete, inner, m.settings.LayoutAutocompleteRows()); len(popup) > 0 {
			lines = append(lines, popup...)
		}
	}

	help := "  "
	if s.soqlEditing {
		// Editing-mode keys are mode-internal; keep them literal.
		// Slim line: only the keys distinct to edit mode. Esc/ctrl+u
		// are universal cancel/clear and don't need their own slot.
		help += "↵ run · shift+↵ newline · ctrl+l format"
	} else {
		// Idle mode: focus on the SOQL-specific gestures. The
		// global status bar already advertises yank, refresh,
		// open, drill — don't double-list them here.
		help += firstPretty(Keys.SOQLEdit) + " edit · " +
			firstPretty(Keys.Drill) + " run"
		if opts.showSavedActions {
			help += " · " + firstPretty(Keys.SOQLSave) + " save · " +
				firstPretty(Keys.SOQLExport) + " export"
		}
		// Tooling / Bulk are mode TOGGLES, not actions — the
		// active state already shows up as a [tooling] / [bulk]
		// badge next to the SOQL title above. Dim suffix so the
		// keys are discoverable but visually subordinate.
		help += "  ·  " +
			firstPretty(Keys.SOQLToggleTooling) + " tooling · " +
			firstPretty(Keys.SOQLToggleBulk) + " bulk"
		if opts.modal {
			help += " · esc close · ctrl+p open in /soql tab"
		}
	}
	lines = append(lines, dimLine(help, inner))

	// Auto-suggest bulk for large LIMITs / unbounded queries. Skips
	// the hint when bulk is already on (the user knows) or while the
	// editor is mid-type (visual noise while composing). REST pages
	// 2000 rows at a time so a LIMIT 200000 query burns ~100 API
	// calls; one ctrl+b keystroke collapses that to 1 bulk job.
	if !s.soqlBulk && !s.soqlEditing && !s.soqlTooling {
		if hint := bulkSuggestHint(s.soqlInput.Value()); hint != "" {
			lines = append(lines, dimLine("  "+hint, inner))
		}
	}
	lines = append(lines, "")

	if s.soqlRunning {
		busy := "  running…  (ctrl+c to cancel)"
		if s.soqlBulk {
			busy = "  bulk job running (submit → poll → download)…  (ctrl+c to cancel)"
		}
		lines = append(lines, theme.Subtle.Render(busy))
		return strings.Join(lines, "\n")
	}
	if s.soqlErr != nil {
		lines = append(lines, redLine("  "+s.soqlErr.Error()))
		return strings.Join(lines, "\n")
	}
	if len(s.soqlResult.Records) == 0 {
		lines = append(lines, theme.Subtle.Render("  no results (type a query, press enter)"))
		return strings.Join(lines, "\n")
	}

	model := m.soqlSessionRenderModel(s, inner)
	tableBudget := innerH - len(lines)
	focus := m.focus
	if opts.modal {
		focus = focusMain
	}
	lines = append(lines, renderListModel(m, model, focus, inner, tableBudget)...)
	return strings.Join(lines, "\n")
}

func (m Model) soqlSessionRenderModel(s *soqlSession, inner int) listRenderModel {
	if s == nil {
		return listRenderModel{}
	}
	records := s.soqlResult.Records
	search := s.searchPtr()
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, records, search, theme.Current.ID, s.soqlInput.Value())
	visible := entry.filtered
	tagMap, projMap := m.bulkTagsAndProjectsForSOQLRows(records)
	leftGutters, rightGutters := m.listGutters(
		func(row int) string {
			if row < 0 || row >= len(visible) {
				return ""
			}
			ref := soqlRowRef(visible[row])
			if ref == "" {
				return ""
			}
			return m.resolveTagGutterCell(devproject.KindRecord, ref, tagMap)
		},
		func(row int) string {
			if row < 0 || row >= len(visible) {
				return ""
			}
			ref := soqlRowRef(visible[row])
			if ref == "" {
				return ""
			}
			return rowProjectGutterFromMap(devproject.KindRecord, ref, projMap)
		},
	)
	// Map the stored unfiltered cursor to a position within the
	// currently-visible (filtered) rows — same shape records uses for
	// search-cursor identity.
	sortDataKey := soqlSortDataKey(entry)
	adapter := soqlSessionTableAdapter(s, entry)
	// listRenderModel.Cursor is a display-space int; convert at the
	// render boundary.
	cursor := int(adapter.DisplayCursor())
	return listRenderModel{
		Title:        soqlResultsTitle(s.soqlResult, len(visible)),
		State:        &s.soqlTable,
		Search:       search,
		Cols:         entry.listCols,
		N:            len(visible),
		Cursor:       cursor,
		Cell:         entry.cell,
		Gutters:      leftGutters,
		RightGutters: rightGutters,
		// "e edit query" was here but the editor is visible right
		// above the results — surfacing the key in two places is
		// chrome noise. Drop.
		FooterExtras: "",
		// Fold search state into the data version so the per-row
		// render cache (paginated mode) invalidates correctly when
		// the user types into the search buffer.
		DataVersion: listVersionWithStore(
			len(records)*1009+len(entry.listCols)*7+len(search.Buffer())*131,
			m,
		),
		SortDataKey: sortDataKey,
	}
}

// bulkSuggestHint returns a short hint string when the editor's
// current SOQL looks like a big-result query and Bulk API would save
// meaningful API calls. Returns "" when the query is small enough
// that REST is fine.
//
// Heuristics (deliberately conservative — false-positive hints train
// users to ignore them):
//   - Explicit LIMIT > 5000 → "LIMIT 200000 → ~100 API calls"
//     (REST chunks 2000 rows/call so the call count is N/2000)
//   - No LIMIT clause at all → "no LIMIT — large result sets burn
//     many calls" (we don't know the unbounded count here, so the
//     hint is softer — Bulk still wins if the count is large)
//
// Doesn't fire when the SOQL is empty, on aggregate queries, or on
// queries that already look small. Tooling queries aren't suggested
// for Bulk (incompatible) — caller suppresses on m.soqlTooling.
func bulkSuggestHint(soql string) string {
	trimmed := strings.TrimSpace(soql)
	if trimmed == "" {
		return ""
	}
	low := strings.ToLower(trimmed)
	// Aggregates return one row — Bulk would be wasteful.
	if strings.Contains(low, "count(") || strings.Contains(low, "count_distinct(") {
		return ""
	}
	if i := strings.LastIndex(low, " limit "); i >= 0 {
		tail := strings.TrimSpace(trimmed[i+len(" limit "):])
		n := 0
		for _, r := range tail {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
			if n > 1_000_000_000 {
				break
			}
		}
		if n > 5000 {
			calls := (n + 1999) / 2000
			return fmt.Sprintf("LIMIT %d → ~%d REST calls (ctrl+b for bulk)", n, calls)
		}
		return ""
	}
	// No LIMIT at all. Soft hint — we don't know the row count
	// without running the query, so word it conditionally.
	return "no LIMIT (ctrl+b for bulk on big sets)"
}

func (m Model) soqlSelectedRecord() (map[string]any, bool) {
	return m.soqlSessionSelectedRecord(&m.soqlSession)
}

func (m Model) soqlSessionSelectedRecord(s *soqlSession) (map[string]any, bool) {
	if s == nil || len(s.soqlResult.Records) == 0 {
		return nil, false
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, s.soqlResult.Records, s.searchPtr(), theme.Current.ID, s.soqlInput.Value())
	if entry == nil || len(entry.filtered) == 0 {
		return nil, false
	}
	adapter := soqlSessionTableAdapter(s, entry)
	visibleIdx, ok := adapter.VisibleAtDisplay(adapter.DisplayCursor())
	if !ok || int(visibleIdx) >= len(entry.filtered) {
		return nil, false
	}
	return entry.filtered[visibleIdx], true
}

func soqlTableAdapter(m *Model, entry *soqlRenderEntry) tableRowAdapter {
	if m == nil {
		return soqlSessionTableAdapter(nil, entry)
	}
	return soqlSessionTableAdapter(&m.soqlSession, entry)
}

func soqlSessionTableAdapter(s *soqlSession, entry *soqlRenderEntry) tableRowAdapter {
	if entry == nil {
		return tableRowAdapter{}
	}
	var state *uilayout.ListTableState
	if s != nil {
		state = &s.soqlTable
	}
	return tableRowAdapter{
		State:        state,
		Cols:         entry.listCols,
		N:            len(entry.filtered),
		Cell:         entry.cell,
		VisibleToRaw: entry.filteredIdx,
		DataKey:      soqlSortDataKey(entry),
		RawCursor: func() RawRow {
			if s == nil {
				return 0
			}
			return s.soqlRowCur
		},
		SetRawCursor: func(raw RawRow) {
			if s != nil {
				s.soqlRowCur = raw
			}
		},
	}
}

func soqlSortDataKey(entry *soqlRenderEntry) string {
	if entry == nil {
		return "soql:nil"
	}
	var b strings.Builder
	b.WriteString("soql|")
	b.WriteString(strconv.FormatUint(uint64(entry.rowsPtr), 10))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(entry.rowsLen))
	b.WriteByte('|')
	if entry.searchOn {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	b.WriteString(entry.searchBuf)
	b.WriteByte('|')
	b.WriteString(entry.themeID)
	return b.String()
}

// buildSOQLCols mirrors buildRecordListCols for the SOQL workspace.
// SOQL projections are ad-hoc, so column order comes from
// collectColumns (Id/Name first, alphabetical otherwise) and labels
// are just the API name uppercased.
func buildSOQLCols(names []string, rows []map[string]any) []uilayout.ListColumn {
	out := make([]uilayout.ListColumn, 0, len(names))
	for _, n := range names {
		header := strings.ToUpper(n)
		min := lipglossWidth(header) + 2
		if min < 8 {
			min = 8
		}
		max := min
		for _, rec := range rows {
			v, _ := sf.Record(rec).Field(n)
			if w := lipglossWidth(formatCell(v)); w > max {
				max = w
			}
		}
		ideal := max
		if ideal > uilayout.AutoMaxIdeal {
			ideal = uilayout.AutoMaxIdeal
		}
		style := lipgloss.NewStyle().Foreground(theme.Fg)
		if n == "Id" {
			style = lipgloss.NewStyle().Foreground(theme.Muted)
		}
		out = append(out, uilayout.ListColumn{
			Name: n, Header: header,
			Min: min, Ideal: ideal, Max: max,
			Style: style,
		})
	}
	return out
}

// soqlProjectionFor returns the cached column-spec + cell matrix for
// the current SOQL result + search-buffer combination. Rebuilds on:
//   - new query result (raw rows slice pointer change)
//   - theme switch (column widths depend on lipgloss style output)
//   - search-buffer edit (filter narrows the cell matrix)
//
// Cache lives on *orgData (pointer-stable across the value-receiver
// Model copy) so every per-frame caller — soqlRenderModel (body),
// listTableSOQL (wheel routing / sidebar / status / zen), measureCellSOQL
// (snap-to-content) — hits the same memo. Without that, listTableSOQL
// alone would re-walk every row on every wheel tick × ~5 callers per
// frame; the 20K-row threshold where /soql starts lagging is exactly
// that O(N × cols × callers × frames) cost. Mirrors what /records
// gets from recordsProjectionFor.
//
// d == nil falls through to a fresh build with no caching — happens
// during early-init renders before an org is selected.
// computeTextareaVisibleRows estimates how many terminal rows the
// textarea needs to render `value` at the given width without
// internal viewport scrolling. Each logical line (split on `\n`)
// soft-wraps to ceil(visualWidth / width) visible rows.
//
// Visual width uses ansi.StringWidth so multi-byte runes and
// emoji are counted correctly. Width <= 0 returns 1 (degenerate).
func computeTextareaVisibleRows(value string, width int) int {
	if width <= 0 {
		return 1
	}
	rows := 0
	for _, line := range strings.Split(value, "\n") {
		w := ansi.StringWidth(line)
		if w == 0 {
			rows++
			continue
		}
		rows += (w + width - 1) / width
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func soqlProjectionFor(d *orgData, records []map[string]any, search *searchState, themeID, query string) *soqlRenderEntry {
	rowsPtr := slicePtrAny(records)
	// Key the projection cache on the EFFECTIVE search text — when
	// debounce is active mid-typing, Effective() lags Buffer() so
	// consecutive fast keystrokes collapse into one filter pass.
	searchBuf := ""
	searchOn := false
	if search != nil {
		searchBuf = search.Effective()
		searchOn = search.EffectiveApplied()
	}
	if d != nil {
		if d.soqlRenderCache == nil {
			d.soqlRenderCache = soqlRenderCache{}
		}
		if entry, ok := d.soqlRenderCache["soql"]; ok &&
			entry.rowsPtr == rowsPtr &&
			entry.rowsLen == len(records) &&
			entry.searchBuf == searchBuf &&
			entry.searchOn == searchOn &&
			entry.themeID == themeID &&
			entry.query == query {
			return entry
		}
	}
	filterStart := time.Now()
	colNames := collectColumns(records, query)
	listCols := buildSOQLCols(colNames, records)
	var filtered []map[string]any
	var filteredIdx []int
	if searchOn && searchBuf != "" {
		filtered, filteredIdx = filterRecords(records, colNames, searchBuf)
	} else {
		filtered = records
		filteredIdx = identityIdx(len(records))
	}
	if search != nil {
		search.SetLastFilterDurationMs(int(time.Since(filterStart) / time.Millisecond))
	}
	cells := make([][]string, len(listCols))
	for ci, col := range listCols {
		colCells := make([]string, len(filtered))
		for ri, rec := range filtered {
			v, _ := sf.Record(rec).Field(col.Name)
			colCells[ri] = formatCell(v)
		}
		cells[ci] = colCells
	}
	entry := &soqlRenderEntry{
		rowsPtr:     rowsPtr,
		rowsLen:     len(records),
		searchBuf:   searchBuf,
		searchOn:    searchOn,
		themeID:     themeID,
		query:       query,
		colNames:    colNames,
		listCols:    listCols,
		cells:       cells,
		filtered:    filtered,
		filteredIdx: filteredIdx,
	}
	if d != nil {
		d.soqlRenderCache["soql"] = entry
	}
	return entry
}

// soqlSearchPtr returns the SOQL results grid's sticky search-state.
// Pointer (heap-allocated in New) so the same state survives the
// value-Model copy through Update / render. Mirrors the records
// search contract — `/` opens it, the renderer narrows visible rows.
func (m Model) soqlSearchPtr() *searchState {
	return m.soqlSession.searchPtr()
}

// --- result-table helpers (used only by SOQL for now, but generic) ------

// collectColumns gathers the projected field names for the result
// grid. When `query` is non-empty AND parseable, columns appear in
// the order they were SELECTed — matching what the user typed.
// Falls back to the legacy alpha-sort with Id/Name pinned when the
// query isn't available (results from history, headless callers).
//
// Records' map keys are used to BACKFILL fields the SELECT parser
// missed: relationship traversals (`Account.Name`) flatten into the
// record as nested map structures that don't appear as top-level
// keys, and `FIELDS(ALL/STANDARD/CUSTOM)` expands server-side into
// fields the parser can't predict. Backfill keeps those visible.
func collectColumns(records []map[string]any, query string) []string {
	parsed := parseSelectColumns(query)
	seen := map[string]bool{}
	cols := make([]string, 0, len(parsed)+8)

	// 1. Add SELECT-order columns that actually exist in the data.
	// `Account.Name` references nest in the record map, so the
	// top-level key is `Account` — we keep the dotted path because
	// it's what sf.Record.Field() expects.
	//
	// We also mark the ROOT key (everything before the first `.`)
	// as seen so the backfill step below doesn't re-add it as a
	// duplicate column. Without this, `SELECT CreatedBy.Name FROM
	// Account` would render BOTH a "CreatedBy.Name" column (from
	// the parser) AND a "CreatedBy" column (from the record map's
	// nested-relationship top-level key).
	for _, c := range parsed {
		if seen[c] || c == "" {
			continue
		}
		seen[c] = true
		cols = append(cols, c)
		if i := strings.IndexByte(c, '.'); i > 0 {
			seen[c[:i]] = true
		}
	}

	// 2. Backfill: any record key that wasn't in the SELECT
	// projection (FIELDS() expansions, server-added fields) goes
	// on the end, alpha-sorted.
	var extras []string
	for _, r := range records {
		for k := range r {
			if k == "attributes" || seen[k] {
				continue
			}
			seen[k] = true
			extras = append(extras, k)
		}
	}

	// When no query was passed we have NOTHING in cols yet — every
	// field is an extra. Restore the legacy alpha-sort with Id/Name
	// pinned so headless paths stay deterministic.
	if len(parsed) == 0 {
		sort.Slice(extras, func(i, j int) bool {
			rank := func(s string) int {
				switch s {
				case "Id":
					return 0
				case "Name":
					return 1
				}
				return 2
			}
			ri, rj := rank(extras[i]), rank(extras[j])
			if ri != rj {
				return ri < rj
			}
			return extras[i] < extras[j]
		})
	} else {
		// SELECT-driven mode: just alpha-sort the leftovers so
		// FIELDS() expansions land in a stable order.
		sort.Strings(extras)
	}
	cols = append(cols, extras...)
	return cols
}

// parseSelectColumns extracts the comma-separated projection list
// from the SELECT clause of a SOQL query. Returns the items in
// source order with whitespace trimmed.
//
// Skips items that don't survive into the result rows:
//   - Aggregates (`COUNT(Id)`, `SUM(Amount)`) — SF aliases them
//     as `expr0`, `expr1`, etc. and we can't predict the alias.
//   - Subqueries (`(SELECT Id FROM Contacts)`) — nested arrays
//     rendered separately, not flat columns.
//   - FIELDS() macros — server expands them at runtime.
//
// Aliases (`SELECT Name n FROM Account`) keep the original
// column name, since that's the record-map key we'll look up.
//
// The parser is deliberately simple: regex on the SELECT clause,
// paren-balance for subqueries. Mirrors Inspector Reloaded's
// approach.
func parseSelectColumns(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	selIdx := strings.Index(lower, "select ")
	if selIdx < 0 {
		return nil
	}
	// Find the matching FROM keyword at paren-depth 0. Skipping
	// over (SELECT ...) subqueries that have their own FROM.
	body := q[selIdx+len("select "):]
	depth := 0
	end := -1
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		// Look for " from " (with word boundaries) at depth 0.
		if i+6 <= len(body) && strings.EqualFold(body[i:i+5], " from") {
			next := byte(' ')
			if i+5 < len(body) {
				next = body[i+5]
			}
			if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
				end = i
				break
			}
		}
	}
	if end < 0 {
		end = len(body)
	}
	projection := body[:end]

	// Split on commas at paren-depth 0.
	var raw []string
	start := 0
	depth = 0
	for i := 0; i < len(projection); i++ {
		switch projection[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				raw = append(raw, projection[start:i])
				start = i + 1
			}
		}
	}
	raw = append(raw, projection[start:])

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// Drop subqueries, aggregates, function calls — anything
		// with parens isn't a flat column key in the result map.
		if strings.ContainsAny(item, "()") {
			continue
		}
		// Drop alias: "Name n" → "Name". The first whitespace-
		// separated token is the actual field reference.
		if i := strings.IndexAny(item, " \t"); i > 0 {
			item = item[:i]
		}
		out = append(out, item)
	}
	return out
}

func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case map[string]any:
		// One registry handles every map shape SOQL returns:
		// relationship lookups (Account.Name → "Acme"), compound address
		// fields, compound person Name, geolocation. See compound_render.go.
		if s, ok := renderCompound(x); ok {
			return s
		}
		return "{…}"
	}
	return fmt.Sprint(v)
}

// soqlRowRef returns the "<sObject>:<Id>" key used for tag /
// project lookups on a SOQL result row. Empty when the row is
// missing the standard `attributes.type` (sObject name) or `Id`
// fields — usually only happens for aggregate queries that don't
// project from a real sObject.
func soqlRowRef(rec map[string]any) string {
	id, _ := rec["Id"].(string)
	if id == "" {
		return ""
	}
	if attrs, ok := rec["attributes"].(map[string]any); ok {
		if t, _ := attrs["type"].(string); t != "" {
			return t + ":" + id
		}
	}
	return ""
}

// bulkTagsAndProjectsForSOQLRows pre-fetches tag / project bindings
// for SOQL result rows. Rows are keyed by their attributes.type +
// Id so cross-sObject queries land each row under the right key.
// bulkTagsAndProjectsForSOQLRows resolves the tag + dev-project
// membership maps for the rows currently rendered on the SOQL grid.
//
// Routed through orgData.memoTagsFor / memoProjectsFor so the
// SQLite-backed lookups are cached per (rows-slice-pointer,
// devproject-generation). Same gutterCache pattern records-list uses;
// without it, every frame on a 20K-row SOQL result would re-query
// SQLite twice, allocating 40K keys to build the wanted-set — visible
// as scroll lag even when the column-width cache is hot.
func (m Model) bulkTagsAndProjectsForSOQLRows(rows []map[string]any) (
	map[string][]devproject.Tag, map[string][]devproject.DevProject,
) {
	if m.devProjects == nil || len(rows) == 0 {
		return nil, nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil, nil
	}
	d := m.data[o.Username]
	if d == nil {
		return nil, nil
	}
	rowsPtr := slicePtrAny(rows)
	tags := d.memoTagsFor(m.devProjects, "soql", rowsPtr, func() map[string][]devproject.Tag {
		keys := soqlTagKeys(rows)
		if len(keys) == 0 {
			return nil
		}
		out, _ := m.devProjects.TagsForItems(o.Username, keys)
		return out
	})
	projects := d.memoProjectsFor(m.devProjects, "soql", rowsPtr, func() map[string][]devproject.DevProject {
		keys := soqlTagKeys(rows)
		if len(keys) == 0 {
			return nil
		}
		out, _ := m.devProjects.ProjectsForItems(o.Username, keys)
		return out
	})
	return tags, projects
}

// soqlTagKeys projects SOQL rows down to their TagLookupKey shape,
// filtering out rows with no resolvable Ref. Used by both the tag
// and project bulk-fetch paths above.
func soqlTagKeys(rows []map[string]any) []devproject.TagLookupKey {
	keys := make([]devproject.TagLookupKey, 0, len(rows))
	for _, r := range rows {
		ref := soqlRowRef(r)
		if ref == "" {
			continue
		}
		keys = append(keys, devproject.TagLookupKey{
			Kind: devproject.KindRecord, Ref: ref,
		})
	}
	return keys
}

// soqlResultsTitle formats the title bar above a SOQL result grid.
// Surfaces the cap state when the user only got a partial slice
// (Done=false from queryMore, or visible rows < TotalSize) so the
// "showing 50 of 12,000" reality is visible rather than silently
// implied.
func soqlResultsTitle(res sf.QueryResult, visible int) string {
	if !res.Done && res.TotalSize > visible {
		return fmt.Sprintf("RESULTS · %d of %d rows · capped (add LIMIT or refine WHERE)",
			visible, res.TotalSize)
	}
	if res.TotalSize > visible {
		return fmt.Sprintf("RESULTS · %d of %d rows", visible, res.TotalSize)
	}
	return fmt.Sprintf("RESULTS · %d rows", res.TotalSize)
}
