package uilayout

// Shared list-table primitive. Used by every "table of N projected
// columns where each row is one record" surface — records subtab,
// SOQL results, reports run, Salesforce list-view results.
//
// Goals:
//   - One implementation: width policy, horizontal scroll, frozen
//     leftmost column, fullscreen ("zen") flag, per-column resize
//     state all live here. Surface code is a thin shim that builds a
//     ListTableSpec and calls RenderListTable.
//   - Auto-fit by default: derive ideal widths from longest visible
//     cell (capped), shrink toward header-only minimums when the
//     pane is tight, fall back to horizontal scroll only when even
//     minimums don't fit. Users can override per column with < / >
//     (step) and { / } (snap to min/max).
//   - Frozen column: when horizontal scroll is engaged, the first
//     column stays put on the left so the user always knows which
//     row they're on.
//
// What this does NOT handle:
//   - Vertical scroll: that's RenderRows / RenderRowsViewport's job.
//     Callers compose: build the column block here, then feed each
//     rendered row into RenderRows for cursor-following row scroll.

import (
	"image/color"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// ListColumn is the column spec a caller hands to RenderListTable.
// Min, Ideal, Max are the auto-fit hints; the layout engine picks an
// actual width within [Min, Max] depending on pane budget and the
// caller's per-column override (UserWidth, 0 = no override).
type ListColumn struct {
	// Name is the stable identifier (e.g. SF API field name). Used as
	// the persistence key for user-set widths.
	Name string
	// Header is the user-visible column label.
	Header string
	// Min is the floor — rendering narrower than this is forbidden.
	// Sensible default: max(8, len(header)+2). 0 means "use default".
	Min int
	// Ideal is the target width when the pane has room. Usually the
	// 90th-percentile cell width capped at AutoMaxIdeal. 0 means
	// "use Min as ideal too" (rare; for short fixed cols).
	Ideal int
	// Max is the ceiling — pressing the snap-to-max key jumps here.
	// Usually the longest visible cell width. 0 means "use Ideal".
	Max int
	// Style applied to body cells. Zero-value defaults to theme.Fg.
	Style lipgloss.Style
	// Unsortable marks a column whose cells are composite/glyph blobs
	// (e.g. the FLAGS strip) where a lex sort on the rendered string is
	// meaningless. The sort key handler refuses to sort on these and
	// flashes a hint instead. Filtering by flag is done via the chip
	// strip, not column sort.
	Unsortable bool
}

// AutoMaxIdeal caps any single column's auto-derived ideal width so
// one absurdly-wide column (a 500-char description) doesn't squash
// every other column into uselessness. The user can still snap to
// the cell's real max via } if they want it fully expanded.
const AutoMaxIdeal = 40

// ListTableState is the per-surface persistent UI state. Owned by the
// caller (e.g. records subtab keeps one per (sobject, chip)) so the
// view remembers scroll position + user resizes across renders.
//
// Zero-value works: HScroll=0, UserWidths empty → pure auto-fit.
type ListTableState struct {
	// HScroll is the leftmost non-frozen column index that's visible.
	// Bounded by the resolver to [0, len(cols)-1].
	HScroll int
	// UserWidths maps column Name → user-pinned width. When set,
	// the layout uses this verbatim (clamped to [Min, Max]) instead
	// of deriving from auto-fit.
	UserWidths map[string]int
	// Zen toggles fullscreen mode for this view. Caller checks this
	// to decide whether to suppress chrome (header / sidebar / left
	// rail / dashboard) — RenderListTable itself doesn't change.
	Zen bool
	// FrozenCols is how many leftmost columns stay anchored when
	// HScroll > 0. Default 1 (the "name" column). 0 disables.
	FrozenCols int
	// ColCursor is the index of the highlighted column. Always live —
	// the cursor is the target for sort, resize, snap-min, snap-max,
	// edit-view. Defaults to the leftmost non-frozen column on first
	// paint; the dispatcher updates it as the user h-scrolls. Bounded
	// to [0, len(cols)-1] by the dispatcher.
	ColCursor int

	// SortColumn is the column Name (matches ListColumn.Name) the user
	// sorted by, applied client-side as a final pass over the visible
	// rows. Empty = no user sort, fall back to the row order the data
	// source returned (chip's ORDER BY, report definition, etc.).
	// Toggled by `s` (sorts the cursored column); cleared by `S`.
	SortColumn string
	// SortDesc reverses the sort. Default false (ascending).
	SortDesc bool
	// RowsOrdered reports that the caller has already applied
	// SortColumn/SortDesc to the row slice before rendering. The
	// header still shows the sort arrow, but RenderListModel skips
	// its own per-frame sort permutation. Set by ListView adapters
	// whose Filtered() cache returns display-ordered rows.
	RowsOrdered  bool
	sortCacheKey string
	sortCache    []int

	// Paginated toggles paged mode for this view. When true, the
	// renderer slices rows into page-sized windows (one page =
	// the number of rows that fit the pane). Standard j/k/arrow
	// navigation rolls over: moving past the bottom of a page
	// advances to the next page (cursor lands at row 0 of it);
	// moving above the top reverses to the previous (cursor
	// lands at the last row of it). Toggled by `P`.
	//
	// Page semantics: pages are 0-indexed, page 0 holds rows
	// [0, pageSize); pageSize is computed at render time from the
	// pane budget so resizing the terminal changes "what page X is."
	Paginated bool
	// Page is the active page index (0-based). Renderer clamps to
	// [0, totalPages-1]; key handlers update via SetPage.
	Page int
}

// SetPage clamps p to [0, totalPages-1] and writes it. totalPages is
// the caller's view of "how many pages exist for the current pane
// budget" — passing 0 forces page 0.
func (s *ListTableState) SetPage(p, totalPages int) {
	if totalPages <= 0 {
		s.Page = 0
		return
	}
	if p < 0 {
		p = 0
	}
	if p >= totalPages {
		p = totalPages - 1
	}
	s.Page = p
}

// PageSizeFor returns the rows-per-page for the given pane budget.
// "Fit-to-pane" semantics: one page = whatever fits the visible
// area. Floor of minViewportRows so tiny terminals still get a
// usable page; cap at the row total so pagination on small lists
// doesn't show "Page 1 of 1 · 5/3 rows".
func PageSizeFor(budget, n int) int {
	if budget < minViewportRows {
		budget = minViewportRows
	}
	if n > 0 && budget > n {
		budget = n
	}
	return budget
}

// TotalPages returns ceil(n / pageSize), clamped to ≥1 when n>0.
// pageSize<=0 returns 1 (defensive — paginated mode without a
// computed size renders the whole list as page 0).
func TotalPages(n, pageSize int) int {
	if n <= 0 {
		return 0
	}
	if pageSize <= 0 {
		return 1
	}
	return (n + pageSize - 1) / pageSize
}

// EnsureUserWidths lazy-allocates the override map.
func (s *ListTableState) EnsureUserWidths() {
	if s.UserWidths == nil {
		s.UserWidths = map[string]int{}
	}
}

// SetUserWidth records a per-column override. width <= 0 clears the
// override (back to auto-fit for that column).
func (s *ListTableState) SetUserWidth(name string, width int) {
	if width <= 0 {
		if s.UserWidths != nil {
			delete(s.UserWidths, name)
		}
		return
	}
	s.EnsureUserWidths()
	s.UserWidths[name] = width
}

// ListTableSpec is the per-render input: columns, the cell-fetching
// closure, and the row count. Callers build this every render — it's
// cheap and keeps the data flow obvious.
type ListTableSpec struct {
	Cols []ListColumn
	N    int
	// SortCacheKey identifies the row data backing this render. When
	// set, SortedIndices memoises the permutation on ListTableState
	// so large records/SOQL/report tables don't re-sort every frame.
	// Leave empty when the caller cannot provide a stable version.
	SortCacheKey string
	// Cell returns the body string for (row, col). Called once per
	// visible cell per render; should be fast (no SF round-trips).
	Cell func(row, col int) string

	// Gutters is the list of synthetic frozen leftmost columns that
	// sit OUTSIDE the regular Cols slice — never sortable, never
	// scrollable, never part of column-mode editing. Rendered in
	// declaration order between the cursor-bar and the regular
	// columns. Empty / nil → no gutters.
	//
	// Typical uses: tag-dot markers, project-membership pills.
	// Each entry self-contained so toggling one gutter on/off is a
	// matter of including or excluding its spec on the per-render
	// caller side.
	Gutters []GutterSpec

	// RightGutters mirrors Gutters but renders AFTER the regular
	// columns. Same semantics — fixed width, never sortable, never
	// scrollable. Used to push system-derived metadata (project
	// membership, etc.) to the right edge so user-curated columns
	// (Tags via Gutters, Name) anchor the left side.
	RightGutters []GutterSpec

	// Marks are per-row visual annotations — name-tint, inline
	// badges, dim treatment — applied at render time. Each mark
	// declares a Matches closure + a Treatment; the renderer walks
	// them per row and composes matching treatments. See
	// rowmarks.go for the primitive + composition rules.
	//
	// Empty / nil → no marks; rows render with their declared
	// column styles unchanged. Specs that opt in declare marks
	// once; per-row dispatch happens at render time.
	Marks []RowMark
}

// GutterSpec is one synthetic column. Width is fixed (not flex);
// callers pre-pick it large enough for their longest expected
// content. Header may be empty for unlabelled gutters.
//
// Cell is called once per visible row, returning the body string
// for this gutter at this row. Should be fast (no SF round-trips
// during render). Empty return means "untagged" / "no membership"
// — the gutter still renders as blank padding so columns to the
// right stay aligned.
type GutterSpec struct {
	Width  int
	Header string
	Cell   func(row int) string
}

// ResolvedWidths is the per-column width chosen for the current render.
// Returned alongside the rendered rows so resize handlers know what
// the auto-fit picked (vs. the user's pin) and can compute the next
// step on [ / ] keypresses.
type ResolvedWidths struct {
	Widths []int
	// FromUser[i] is true when Widths[i] came from state.UserWidths
	// rather than auto-fit. Resize handlers use this to decide
	// whether [ / ] should clear or modify the override.
	FromUser []bool
	// HScroll is the actual scroll offset used (state.HScroll
	// clamped to a valid range).
	HScroll int
	// FrozenCount is the number of frozen leftmost columns actually
	// rendered (state.FrozenCols clamped to len(cols)-1).
	FrozenCount int
	// Overflow is true when the column set didn't all fit and
	// horizontal scroll is engaged.
	Overflow bool
	// FullWidth is the total width all columns would consume at
	// their resolved widths (sum of Widths + separators + gutter).
	// Surface code uses this for the "X of Y cols" indicator.
	FullWidth int
}

// LayoutListTable picks per-column widths for the given pane size.
// Algorithm:
//
//  1. Compute per-column widths, preferring UserWidths > Ideal > Min.
//  2. If sum-of-widths fits in the pane → no overflow, return.
//  3. Else flex non-user columns down toward Min, proportionally to
//     how much slack each has. If everything fits at Min → no overflow.
//  4. Else: overflow=true. Render frozen leftmost cols + as many
//     scrolled cols as fit, starting from HScroll.
//
// inner is the pane's content width (already chrome-discounted by the
// caller). Negative or zero inner returns an empty resolution so the
// caller's render loop can skip cleanly.
func LayoutListTable(spec ListTableSpec, state *ListTableState, inner int) ResolvedWidths {
	if inner <= 0 || len(spec.Cols) == 0 {
		return ResolvedWidths{}
	}
	// Gutters eat fixed leftmost space before regular columns. Discount
	// inner by every gutter cell + the Sep that follows each so the
	// column-width fitter has the right budget. Gutters themselves
	// aren't real columns — they're never sorted, scrolled, or resized.
	for _, g := range spec.Gutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		inner -= g.Width + len(Sep)
	}
	// Right gutters consume the same kind of budget but render AFTER
	// the columns. Same discount applies.
	for _, g := range spec.RightGutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		inner -= g.Width + len(Sep)
	}
	if inner < 4 {
		inner = 4
	}
	n := len(spec.Cols)
	widths := make([]int, n)
	fromUser := make([]bool, n)
	mins := make([]int, n)
	for i, c := range spec.Cols {
		min := c.Min
		if min == 0 {
			min = defaultMinFor(c)
		}
		ideal := c.Ideal
		if ideal == 0 {
			ideal = min
		}
		if ideal < min {
			ideal = min
		}
		mins[i] = min
		w := ideal
		if state != nil && state.UserWidths != nil {
			if uw, ok := state.UserWidths[c.Name]; ok && uw > 0 {
				w = clampInt(uw, min, columnMaxFor(c))
				fromUser[i] = true
			}
		}
		widths[i] = w
	}

	// Step 2: do they all fit at the chosen widths?
	sepW := 3 // " │ "
	gutter := 2
	totalAt := func(ws []int) int {
		t := gutter
		for i, w := range ws {
			if i > 0 {
				t += sepW
			}
			t += w
		}
		return t
	}
	if totalAt(widths) <= inner {
		return ResolvedWidths{
			Widths:      widths,
			FromUser:    fromUser,
			HScroll:     0,
			FrozenCount: 0,
			Overflow:    false,
			FullWidth:   totalAt(widths),
		}
	}

	// Step 3: flex non-user columns down toward min. Iterate proportional
	// shrink until either we fit or every flex column is at its min.
	for {
		over := totalAt(widths) - inner
		if over <= 0 {
			break
		}
		// Slack = how much each flex column can still give up.
		totalSlack := 0
		flexCount := 0
		for i := range widths {
			if fromUser[i] {
				continue
			}
			s := widths[i] - mins[i]
			if s > 0 {
				totalSlack += s
				flexCount++
			}
		}
		if totalSlack == 0 {
			break // can't shrink further
		}
		// Distribute the over-by-amount across flex columns weighted by slack.
		took := 0
		for i := range widths {
			if fromUser[i] {
				continue
			}
			s := widths[i] - mins[i]
			if s <= 0 {
				continue
			}
			share := over * s / totalSlack
			if share == 0 {
				share = 1 // at least 1 px so we make progress
			}
			if share > s {
				share = s
			}
			widths[i] -= share
			took += share
			if took >= over {
				break
			}
		}
		if took == 0 {
			// Couldn't shrink anything by even 1 even though slack
			// said we could — defensive guard against infinite loop.
			break
		}
	}

	// Step 4: did flexing get us under? If yes → no overflow.
	if totalAt(widths) <= inner {
		return ResolvedWidths{
			Widths:      widths,
			FromUser:    fromUser,
			HScroll:     0,
			FrozenCount: 0,
			Overflow:    false,
			FullWidth:   totalAt(widths),
		}
	}

	// Overflow: clamp HScroll, identify frozen cols, render starts
	// from HScroll. The renderer will skip non-frozen cols < HScroll.
	frozen := 1
	if state != nil {
		frozen = state.FrozenCols
	}
	if frozen < 0 {
		frozen = 0
	}
	if frozen >= n {
		frozen = n - 1
	}

	hscroll := 0
	if state != nil {
		hscroll = state.HScroll
	}
	if hscroll < frozen {
		hscroll = frozen
	}
	if hscroll >= n {
		hscroll = n - 1
	}

	return ResolvedWidths{
		Widths:      widths,
		FromUser:    fromUser,
		HScroll:     hscroll,
		FrozenCount: frozen,
		Overflow:    true,
		FullWidth:   totalAt(widths),
	}
}

// RenderListTableHeader renders the header row honouring frozen +
// scroll state. inner is the pane width. The column at
// state.ColCursor is always highlighted (yellow, like the
// transient-chip indicator) so the user sees which column the
// sort/resize/snap keys will target.
func RenderListTableHeader(spec ListTableSpec, res ResolvedWidths, state *ListTableState, inner int) string {
	if len(spec.Cols) == 0 || inner <= 0 {
		return ""
	}
	hdr := lipgloss.NewStyle().Foreground(theme.Muted).Bold(true)
	hdrCursor := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Yellow).Bold(true)
	cells, tailCap := visibleColumnIndices(len(spec.Cols), res, inner)
	if len(cells) == 0 {
		return ""
	}
	cursorIdx := -1
	sortColIdx := -1
	sortDesc := false
	if state != nil {
		// Always show the cursor highlight — column ops target it
		// unconditionally now (sort/resize/snap), so the user
		// always knows the target without having to enter a mode.
		cursorIdx = state.ColCursor
		if state.SortColumn != "" {
			for i, c := range spec.Cols {
				if c.Name == state.SortColumn {
					sortColIdx = i
					sortDesc = state.SortDesc
					break
				}
			}
		}
	}
	parts := make([]string, len(cells))
	for k, idx := range cells {
		w := res.Widths[idx]
		if tailCap > 0 && k == len(cells)-1 {
			w = tailCap
		}
		s := hdr
		if idx == cursorIdx {
			s = hdrCursor
		}
		text := spec.Cols[idx].Header
		if idx == sortColIdx {
			arrow := " ↑"
			if sortDesc {
				arrow = " ↓"
			}
			text += arrow
		}
		parts[k] = s.Width(w).Render(ansi.Truncate(text, w, "…"))
	}
	gutter := ""
	for _, g := range spec.Gutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		gutter += hdr.Width(g.Width).Render(
			ansi.Truncate(g.Header, g.Width, "…")) + Sep
	}
	rightGutter := ""
	for _, g := range spec.RightGutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		rightGutter += Sep + hdr.Width(g.Width).Render(
			ansi.Truncate(g.Header, g.Width, "…"))
	}
	return ansi.Truncate("  "+gutter+strings.Join(parts, Sep)+rightGutter, inner, "…")
}

// RenderListTableRow renders one body row honouring frozen + scroll +
// selection styling. row is the row index passed to spec.Cell.
//
// terms is an optional slice of search highlight terms (typically
// from uilayout.SearchTerms). When non-empty, each cell goes through
// HighlightInStyle so matches show a yellow background while the
// surrounding column colour survives. Empty / nil → no highlighting.
func RenderListTableRow(spec ListTableSpec, res ResolvedWidths, row int, selected, focused bool, inner int, terms ...[]string) string {
	if len(spec.Cols) == 0 || inner <= 0 {
		return ""
	}
	cells, tailCap := visibleColumnIndices(len(spec.Cols), res, inner)
	if len(cells) == 0 {
		return ""
	}
	var hlTerms []string
	if len(terms) > 0 {
		hlTerms = terms[0]
	}
	// Resolve row marks once before per-cell loop so we can apply
	// the name-column tint + the row-wide dim flag. Mark *labels*
	// render in their own column via spec.Cell when the surface
	// declares a Marks column; the inline-append shape was retired
	// 2026-05-02 in favour of column tabulation.
	markName, markDim := ApplyMarks(spec.Marks, row)
	// Row-wide background tint for the selected cursor row. Reads as a
	// full-row band rather than a single left-edge mark, which makes
	// "which row am I on" scannable at a glance — important on wide
	// tables (SOQL grids with 10+ cols) where the eye can't easily
	// trace the bar to a specific cell row. Unfocused panes get a
	// gentler tint so the highlight reads as "current row, but you're
	// not steering here right now."
	var (
		rowBg    color.Color
		hasRowBg bool
	)
	if selected {
		hasRowBg = true
		// Same tint for focused + unfocused: BgAlt is already a
		// subtle palette colour designed to coexist with all the
		// per-column fg styles. Differentiating the two states adds
		// theme machinery for marginal value — focus is already
		// signalled by the pane border (PanelledFocus vs Panelled).
		rowBg = theme.BgAlt
	}
	parts := make([]string, len(cells))
	for k, idx := range cells {
		w := res.Widths[idx]
		if tailCap > 0 && k == len(cells)-1 {
			w = tailCap
		}
		s := spec.Cols[idx].Style
		if s.GetForeground() == nil {
			s = lipgloss.NewStyle().Foreground(theme.Fg)
		}
		// Apply mark-driven name color to the primary identifier
		// column (the first column index, regardless of visible
		// position). Subsequent columns use their declared style.
		if idx == 0 && markName != nil {
			s = s.Foreground(markName)
		}
		// Dim treatment applies to every column on the row.
		if markDim {
			s = s.Foreground(theme.Muted)
		}
		if selected && k == 0 {
			s = s.Bold(true)
		}
		if hasRowBg {
			s = s.Background(rowBg)
		}
		body := spec.Cell(row, idx)
		truncated := ansi.Truncate(body, w, "…")
		if len(hlTerms) == 0 {
			parts[k] = s.Width(w).Render(truncated)
		} else {
			rendered := HighlightInStyle(truncated, hlTerms, s)
			cellStyle := lipgloss.NewStyle().Width(w)
			if hasRowBg {
				cellStyle = cellStyle.Background(rowBg)
			}
			parts[k] = cellStyle.Render(rendered)
		}
	}
	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !focused {
			barColor = theme.Muted
		}
		barStyle := lipgloss.NewStyle().Foreground(barColor)
		gapStyle := lipgloss.NewStyle()
		if hasRowBg {
			barStyle = barStyle.Background(rowBg)
			gapStyle = gapStyle.Background(rowBg)
		}
		prefix = barStyle.Render("▌") + gapStyle.Render(" ")
	}
	gutter := ""
	for _, g := range spec.Gutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		gv := g.Cell(row)
		// Render at fixed width so columns stay aligned even when the
		// gutter content is shorter (most rows: untagged → empty).
		gs := lipgloss.NewStyle().Width(g.Width)
		sepStyle := lipgloss.NewStyle()
		if hasRowBg {
			gs = gs.Background(rowBg)
			sepStyle = sepStyle.Background(rowBg)
		}
		gutter += gs.Render(ansi.Truncate(gv, g.Width, "…")) + sepStyle.Render(Sep)
	}
	rightGutter := ""
	for _, g := range spec.RightGutters {
		if g.Cell == nil || g.Width <= 0 {
			continue
		}
		gv := g.Cell(row)
		gs := lipgloss.NewStyle().Width(g.Width)
		sepStyle := lipgloss.NewStyle()
		if hasRowBg {
			gs = gs.Background(rowBg)
			sepStyle = sepStyle.Background(rowBg)
		}
		rightGutter += sepStyle.Render(Sep) + gs.Render(ansi.Truncate(gv, g.Width, "…"))
	}
	// Column separator between regular cells also needs the bg so the
	// row reads as one continuous band rather than striped pickets.
	sepBetween := Sep
	if hasRowBg {
		sepBetween = lipgloss.NewStyle().Background(rowBg).Render(Sep)
	}
	return ansi.Truncate(prefix+gutter+strings.Join(parts, sepBetween)+rightGutter, inner, "…")
}

// SortedIndices returns a stable permutation of row indices [0, spec.N)
// sorted by state.SortColumn / state.SortDesc. Returns nil when no
// sort is applied — callers should treat nil as identity row order.
//
// Comparator: stringify each cell via spec.Cell, try numeric parse
// first (so "10" sorts after "9"), then date parse for ISO-shaped
// strings, fall back to case-insensitive lex compare. Stable so rows
// with equal keys preserve their natural (chip-defined) order — gives
// users predictable behaviour when toggling sort on a column with
// many duplicate values.
func SortedIndices(spec ListTableSpec, state *ListTableState) []int {
	if state == nil || state.SortColumn == "" || spec.Cell == nil {
		return nil
	}
	col := -1
	for i, c := range spec.Cols {
		if c.Name == state.SortColumn {
			col = i
			break
		}
	}
	if col < 0 {
		// Sort column doesn't exist on this spec (chip changed columns,
		// stale state). Drop the sort silently and use identity order.
		return nil
	}
	cacheKey := sortedIndicesCacheKey(spec, state, col)
	if cacheKey != "" && state.sortCacheKey == cacheKey && len(state.sortCache) == spec.N {
		return state.sortCache
	}
	out := make([]int, spec.N)
	for i := range out {
		out[i] = i
	}
	// Pre-compute keys once; spec.Cell could be expensive (e.g.
	// relationship traversal). N comparisons would call spec.Cell
	// O(N log N) times otherwise.
	keys := make([]string, spec.N)
	for i := 0; i < spec.N; i++ {
		keys[i] = spec.Cell(i, col)
	}
	desc := state.SortDesc
	sort.SliceStable(out, func(i, j int) bool {
		cmp := compareCells(keys[out[i]], keys[out[j]])
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	if cacheKey != "" {
		state.sortCacheKey = cacheKey
		state.sortCache = out
	}
	return out
}

func sortedIndicesCacheKey(spec ListTableSpec, state *ListTableState, col int) string {
	if spec.SortCacheKey == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(spec.SortCacheKey)
	b.WriteByte('|')
	b.WriteString(state.SortColumn)
	if state.SortDesc {
		b.WriteString(":desc")
	} else {
		b.WriteString(":asc")
	}
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(spec.N))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(col))
	for _, c := range spec.Cols {
		b.WriteByte('|')
		b.WriteString(c.Name)
	}
	return b.String()
}

// CompareCells exposes the exact cell comparator used by
// SortedIndices so callers that must apply the same order before
// rendering (for cursor/open consistency) can share the semantics.
func CompareCells(a, b string) int {
	return compareCells(a, b)
}

// HScrollIndicator renders a small "X / Y →" hint when overflow is
// active. Empty string when no overflow. Surface code typically
// drops this on the same line as the per-row scroll indicator.
//
// Returns plain (un-styled) text. The previous version wrapped the
// content in theme.Muted, which embedded a reset escape (\x1b[m) at
// the indicator's end — that reset wiped the OUTER DimLine style the
// caller applied to the whole hint, leaving the trailing ")" (and
// anything after it on the same chunk) in the terminal's default
// foreground rather than the dim hint colour. Letting the caller's
// outer style cover the whole line keeps colours consistent.
func HScrollIndicator(res ResolvedWidths, totalCols int) string {
	if !res.Overflow {
		return ""
	}
	left := "←"
	if res.HScroll <= res.FrozenCount {
		left = " "
	}
	right := "→"
	if res.HScroll >= totalCols-1 {
		right = " "
	}
	return left + " col " + itoaSafe(res.HScroll+1) + " / " + itoaSafe(totalCols) + " " + right
}

// visibleColumnIndices is the rendered column order: frozen prefix
// (always shown) + columns from HScroll onward. Greedily packs as
// many post-scroll columns as fit before the inner width is consumed.
//
// The second return is a width cap for the LAST returned column
// (0 = renders at its resolved width). When the next column doesn't
// fully fit but at least 8 cells remain, it's included truncated to
// the remainder instead of dropped — otherwise a user-widened column
// (e.g. LABEL snapped past the pane width) plus everything after it
// silently vanished, leaving a half-blank row.
func visibleColumnIndices(n int, res ResolvedWidths, inner int) ([]int, int) {
	if !res.Overflow {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out, 0
	}
	const gutter = 2
	const sepW = 3
	used := gutter
	cells := make([]int, 0, n)
	// Frozen prefix.
	for i := 0; i < res.FrozenCount && i < n; i++ {
		w := res.Widths[i]
		if len(cells) > 0 {
			used += sepW
		}
		used += w
		cells = append(cells, i)
		if used > inner {
			// Pane too tight even for the frozen set. Stop here so the
			// renderer at least shows what fits.
			return cells, 0
		}
	}
	// Scrolled tail.
	start := res.HScroll
	if start < res.FrozenCount {
		start = res.FrozenCount
	}
	for i := start; i < n; i++ {
		w := res.Widths[i]
		need := w
		if len(cells) > 0 {
			need += sepW
		}
		if used+need > inner {
			// Partial fit: show the column truncated to whatever's
			// left when that's at least a readable sliver.
			remaining := inner - used
			if len(cells) > 0 {
				remaining -= sepW
			}
			if remaining >= 8 {
				cells = append(cells, i)
				return cells, remaining
			}
			break
		}
		used += need
		cells = append(cells, i)
	}
	return cells, 0
}

// defaultMinFor is the floor when a Column doesn't supply Min: at
// least 8 chars and at least header+2.
func defaultMinFor(c ListColumn) int {
	min := 8
	if h := ansi.StringWidth(c.Header) + 2; h > min {
		min = h
	}
	return min
}

// columnMaxFor is the ceiling for a user-overridden column width.
// Only respects an explicit Max declaration; otherwise falls back
// to a generous cap (200 cols) so users can always expand to fit
// content longer than the column's auto-fit Ideal.
//
// Earlier this function fell back to c.Ideal when Max was unset,
// which meant > /  ] could never grow a column past its auto-fit
// target (typically 28-36) — content longer than that ("Mass Email
// Configurations Container") would always truncate even after the
// user explicitly asked for more room. Ideal is a soft auto-layout
// target, not a hard user ceiling.
func columnMaxFor(c ListColumn) int {
	if c.Max > 0 {
		return c.Max
	}
	return userResizeMax
}

// userResizeMax is the upper bound for a user-overridden column
// width when the column doesn't declare an explicit Max. 200 is
// well above any sensible single-column content while still
// catching runaway resize states that would otherwise grow without
// bound. Per-render clamping to actual screen width happens later
// in the layout pass.
const userResizeMax = 200

// StepResize advances widths[col] by delta toward the next snap point.
// Used by the [ / ] resize keys in surface code. Updates state in
// place. delta > 0 grows; delta < 0 shrinks.
func StepResize(spec ListTableSpec, state *ListTableState, res ResolvedWidths, col, delta, step int) {
	if state == nil || col < 0 || col >= len(spec.Cols) {
		return
	}
	if step < 1 {
		step = 4
	}
	c := spec.Cols[col]
	min := c.Min
	if min == 0 {
		min = defaultMinFor(c)
	}
	max := columnMaxFor(c)
	cur := res.Widths[col]
	next := cur + delta*step
	state.SetUserWidth(c.Name, clampInt(next, min, max))
}

// SnapResize jumps widths[col] to its min (delta < 0) or to the
// header-width-or-Ideal (delta > 0). delta>0 here is just a fallback
// for surfaces that can't measure cell content; the preferred
// snap-to-content path is SnapResizeTo, which the surface code
// uses when it has visibility into actual rendered cell widths.
//
// Used by { for snap-to-min. } routes through SnapResizeTo
// when possible, falling back here when the surface can't measure.
func SnapResize(spec ListTableSpec, state *ListTableState, col, delta int) {
	if state == nil || col < 0 || col >= len(spec.Cols) {
		return
	}
	c := spec.Cols[col]
	min := c.Min
	if min == 0 {
		min = defaultMinFor(c)
	}
	if delta < 0 {
		state.SetUserWidth(c.Name, min)
		return
	}
	// Fallback fit: header width clamped to the column's bounds.
	want := lipgloss.Width(c.Header)
	if c.Ideal > 0 && c.Ideal > want {
		want = c.Ideal
	}
	if want < min {
		want = min
	}
	max := columnMaxFor(c)
	if want > max {
		want = max
	}
	state.SetUserWidth(c.Name, want)
}

// SnapResizeTo sets the column's width to exactly contentW columns,
// clamped against the column's min + max + the user-resize ceiling.
// Used by snap-to-content (}) once the caller has measured the
// widest cell currently visible.
//
// Header width is included as a floor so the column never snaps
// narrower than its label.
func SnapResizeTo(state *ListTableState, c ListColumn, contentW int) {
	if state == nil {
		return
	}
	min := c.Min
	if min == 0 {
		min = defaultMinFor(c)
	}
	hdrW := lipgloss.Width(c.Header)
	want := contentW
	if want < hdrW {
		want = hdrW
	}
	if want < min {
		want = min
	}
	max := columnMaxFor(c)
	if want > max {
		want = max
	}
	state.SetUserWidth(c.Name, want)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// compareCells is the stable comparator used by SortedIndices.
// Strategy:
//  1. If both cells parse as numeric (after trimming commas + a
//     trailing %), compare numerically. Mixed numeric/non-numeric
//     falls through to lex.
//  2. Empty / em-dash sentinels sort to the end so meaningful
//     values cluster first regardless of direction.
//  3. Case-insensitive lex compare. ISO-shaped dates sort
//     temporally because ISO ordering matches lex ordering.
//
// Returns -1 / 0 / 1 in strings.Compare convention.
func compareCells(a, b string) int {
	if a == b {
		return 0
	}
	an, anOK := parseNumeric(a)
	bn, bnOK := parseNumeric(b)
	if anOK && bnOK {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	}
	aEmpty := isEmptySentinel(a)
	bEmpty := isEmptySentinel(b)
	if aEmpty != bEmpty {
		if aEmpty {
			return 1
		}
		return -1
	}
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

func isEmptySentinel(s string) bool {
	return s == "" || s == "—" || s == "-"
}

func parseNumeric(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	// Trim commas and a trailing percent sign so "1,234" / "97%" parse.
	cleaned := strings.ReplaceAll(s, ",", "")
	cleaned = strings.TrimSuffix(cleaned, "%")
	if cleaned == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(cleaned, 64)
	return f, err == nil
}

// itoaSafe is the small-int formatter used by HScrollIndicator. Avoids
// pulling in fmt for a hot path.
func itoaSafe(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
