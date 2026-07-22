package ui

// Per-frame list-table render model.
//
// Codex's review pulled this out of the failed shared-renderer
// experiment: instead of growing listSurface into a god-spec
// (Title + N + Cell + Marks + Gutters + Recolor + Empty + ...),
// each surface that opts in produces a per-frame value carrying
// everything render + sort + snap-to-content need. One Cell
// closure, three consumers, no drift.
//
// listSurface stays focused on interaction state (cursor, search,
// move/reset, column-mode state pointer). The render model is a
// separate concern, optionally provided via BuildRenderModel.
//
// Surfaces NOT opting in keep their bespoke renderers — the
// listRenderModel path is purely additive. Migrations happen one
// surface at a time; nothing forces a tab into this shape if its
// content doesn't fit (LWC bundle file tabs, /dev-projects items
// tree, reports folder/report mixed list — those stay bespoke).

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// listRenderModel is the per-frame snapshot a surface hands back to
// the shared list-table renderer (renderListModel). All fields are
// values captured at frame-build time — the model is ephemeral, not
// cached on listSurface or orgData. A new one comes back from
// BuildRenderModel each render pass.
type listRenderModel struct {
	// Title is the bold header label rendered with the search pill
	// ("APEX LOGS · 14 · just now"). Free-form string so callers
	// can include whatever counts/timestamps make sense.
	Title string

	// State + Search are the live persistent state for column-mode
	// interactions + the per-list search buffer. Both are pointers
	// so the renderer can mutate them in place (e.g. column resize
	// during column-mode).
	State  *uilayout.ListTableState
	Search *searchState

	// Cols is the column spec. The N + Cell pair drives row count
	// and per-cell content. Cell receives row indices in the order
	// handed to this model. For non-ListView surfaces, renderListModel
	// applies the table sort when one is active. For ListView-backed
	// surfaces, installListViewOrder applies sort before Filtered()
	// returns so selected/open/yank use the same order.
	Cols []uilayout.ListColumn
	N    int
	Cell func(row, col int) string

	// Cursor is the current selected row in the filtered view.
	// Renderer clamps to [0, N) defensively; callers should
	// pre-clamp too so the highlight bar lands sensibly when
	// filters change.
	Cursor int

	// Marks + Gutters are the row-level visual annotations.
	// uilayout.RowMark drives inline tints/badges; Gutters are
	// synthetic frozen leftmost columns (tag pills, project
	// pills). Both nil = no decoration.
	Marks        []uilayout.RowMark
	Gutters      []uilayout.GutterSpec
	RightGutters []uilayout.GutterSpec

	// Recolor (optional) lets a surface tweak per-cell styles
	// after the base column style is applied — used by surfaces
	// where row-state drives column tints (apex Valid=false → red,
	// flow Status palette). Returns the cell's effective style.
	Recolor func(row, col int, base lipgloss.Style) lipgloss.Style

	// Empty is the message shown when N == 0. Empty string falls
	// back to "no matches" — surfaces with bespoke empty copy
	// (loading…, project-mode hints) should set this explicitly.
	Empty string

	// Err, when non-nil, is the resource's last fetch error. When the
	// list is empty AND Err is set, the empty-state renders the error
	// (why the list is empty) instead of the generic "no rows" copy —
	// so an org that can't serve a surface (no API access, missing FLS,
	// a Tooling failure) tells the user what happened rather than
	// looking like an empty-but-fine list. See errEmptyMessage.
	Err error

	// FooterExtras is the surface-specific keys to splice into
	// the standard list-table footer hint ("↵ open · r refresh").
	// "" omits the addition.
	FooterExtras string

	// DataVersion is a monotonic counter that bumps every time the
	// underlying item set changes (rows added/removed/reordered,
	// chip predicate swapped, search committed). Used by the paged-
	// row render cache as the cache-invalidation key — when
	// DataVersion is unchanged, two ticks with the same page see the
	// same rows in the same order. Zero is fine for surfaces that
	// don't drive the cache (it just means the cache treats every
	// frame as a fresh group, which is the correct fallback).
	DataVersion int

	// SortDataKey identifies the rows/cell values behind the current
	// table for the shared sort-permutation cache. It deliberately
	// mirrors DataVersion's invalidation concept but stays a string so
	// bespoke surfaces can include slice pointers, search text, or
	// resource timestamps without lossy integer folding. Empty falls
	// back to DataVersion for ordinary ListView-backed surfaces.
	SortDataKey string
}

// errEmptyMessage turns a resource fetch error into the empty-state line
// shown when a list is empty because its fetch failed. For the handful
// of errors a user can act on it adds a plain-language hint; otherwise
// it surfaces the raw error (never swallow it — the user needs to see
// WHY the list is empty, per the "degrade gracefully, show the raw API
// error" principle). Two-line output: a headline + the underlying text.
func errEmptyMessage(err error) string {
	hint := ""
	if e := sf.AsSFError(err); e != nil {
		switch e.Code {
		case "API_DISABLED_FOR_ORG", "API_CURRENTLY_DISABLED":
			hint = "This org doesn't have API access enabled — sf-deck needs it to load this."
		case "INVALID_SESSION_ID", "INVALID_LOGIN":
			hint = "Your session expired — press " + firstPretty(Keys.Refresh) + " to refresh, or re-authenticate the org."
		case "INSUFFICIENT_ACCESS_OR_READONLY",
			"INSUFFICIENT_ACCESS_ON_CROSS_REFERENCE_ENTITY":
			hint = "Your user lacks permission to read this (check the profile / permission set)."
		case "REQUEST_LIMIT_EXCEEDED":
			hint = "The org hit its API request limit — try again shortly."
		case "INVALID_TYPE":
			hint = "This object isn't available in this org (edition or feature not enabled)."
		}
	}
	msg := "  couldn't load: " + err.Error()
	if hint != "" {
		msg += "\n  " + hint
	}
	return msg
}

// renderListSurface is the one-call convenience wrapper for surfaces
// whose body is "the table and only the table" — no chip strip, no
// dashboard, no busy chrome to interleave. Calls BuildRenderModel,
// renders the model, joins the lines.
//
// Returns a starting "no org selected"-style fallback when the
// surface declines to build a model. Used by /apex-logs, /deploys,
// /packages, /recent and the perms dashboard subtabs.
//
// Surfaces that DO need orchestration above the table (chip strip,
// dashboard, busy/loading branches) should call BuildRenderModel +
// renderListModel directly so they own the line splice point.
func renderListSurface(m Model, surf *listSurface, w, innerH int, d *orgData) string {
	if surf == nil || surf.BuildRenderModel == nil {
		return ""
	}
	model, ok := surf.BuildRenderModel(m, d)
	if !ok {
		return ""
	}
	inner := w - 4
	lines := renderListModel(m, model, m.focus, inner, innerH)
	return strings.Join(lines, "\n")
}

// renderListModel renders a per-frame list-table from a
// listRenderModel. The caller has already done their tab
// orchestration (chip strip, busy chrome, dashboard); this fills in
// the table block (header + search + table + footer + legend).
// Returns a slice of lines suitable for splicing into the
// surrounding output.
//
// budget is the total vertical space available to this block. The
// caller subtracts whatever chrome they already emitted above this
// call (chip strip, dashboard, etc.) so `budget` is the remaining
// height for header + body rows + footer.
//
// Defensively guards every field. Missing Title / Cols / Cell /
// State / Search: returns an empty slice (caller's outer chrome
// still renders). N <= 0: emits the empty-state line + footer hint
// and returns. This is the contract Codex flagged as critical:
// shared renderers must be paranoid because they're the blast
// radius.
func renderListModel(m Model, model listRenderModel, focus focus, inner, budget int) []string {
	trace := m.beginListTableTrace()
	if trace != nil {
		defer trace.phase("list_table", time.Now())
	}
	if model.Cell == nil || len(model.Cols) == 0 ||
		model.State == nil || model.Search == nil {
		return nil
	}

	cur := model.Cursor
	if cur < 0 {
		cur = 0
	}
	if cur >= model.N {
		cur = 0
	}
	m.traceListRenderModel(model, cur)

	var lines []string
	if model.Title != "" {
		// Title's pagination suffix is patched in further down once
		// the page index is finalised (cursor may shift it). For
		// now reserve the slot with the bare title; the suffix is
		// appended in-place after pageSize is computed.
		lines = append(lines, headerWithSearchPill(model.Title, *model.Search))
	}
	titleIdx := -1
	if model.Title != "" {
		titleIdx = len(lines) - 1
		// Breathing room: title is metadata (what you're looking
		// at), the search hint is a call to action — separating
		// them avoids the two reading as one continuous line.
		lines = append(lines, "")
	}
	lines = append(lines, searchBar(*model.Search, inner))

	if model.N == 0 {
		// An empty list because the FETCH FAILED is a different story
		// from an empty-but-healthy list. When the resource carries an
		// error, show it (with a hint for the common ones) so an
		// incompatible / API-limited org explains itself instead of
		// masquerading as "nothing here".
		if model.Err != nil {
			lines = append(lines, theme.Subtle.Render(errEmptyMessage(model.Err)))
			return lines
		}
		empty := model.Empty
		if empty == "" {
			empty = "  no matches"
		}
		// Rewrite when the active chip is "Recently viewed" and
		// there's nothing visited yet — gives users a recovery hint
		// pointing at the broader chip.  Done here (instead of in
		// each surface's BuildRenderModel) to avoid an init-cycle
		// between list-surface vars + the chip-strip resolution
		// graph.
		if id := m.activeChipIDForRender(); id == recentlyViewedChipID {
			if domain, _ := m.activeChipScope(); domain != "" {
				empty = recentlyViewedEmptyHintFor(domain)
			}
		}
		lines = append(lines, theme.Subtle.Render(empty))
		return lines
	}

	spec := uilayout.ListTableSpec{
		Cols:         model.Cols,
		N:            model.N,
		Gutters:      model.Gutters,
		RightGutters: model.RightGutters,
		Marks:        model.Marks,
		Cell:         model.Cell,
	}
	// Use the same SortCacheKey shape the cursor-translation helpers
	// in sort_cursor.go build.  Critical: ListTableState has ONE
	// cache slot for the sort permutation.  If the renderer uses a
	// different key than recordsMoveCursor's display↔data
	// translation calls, the two paths invalidate each other on
	// every wheel tick — every operation forces a fresh O(N log N)
	// sort.  Sharing the key makes both paths read/write the same
	// cache entry.
	spec.SortCacheKey = cursorSortCacheKey(model.State, model.Cols, model.N, listModelSortDataKey(model))
	res := uilayout.LayoutListTable(spec, model.State, inner)
	terms := m.searchTerms()
	lines = append(lines, uilayout.RenderListTableHeader(spec, res, model.State, inner))
	var sortPerm []int
	if model.State == nil || !model.State.RowsOrdered {
		sortPerm = uilayout.SortedIndices(spec, model.State)
	}

	rowsHeader := len(lines)
	// trailing reserves vertical space below the row block for the
	// hint chrome we splice in after renderRows returns. The block is:
	//   1. scrollIndicator emitted by renderRows itself (+1)
	//   2. blank separator line above the hint (+1)
	//   3. hint line (+1)
	// Plus 1 line of breathing room below the hint so it doesn't
	// touch the pane bottom border. Total = 4.
	const trailing = 4

	// Recolor scratch buffer — allocated ONCE per frame, reused for
	// every row. rowFn runs sequentially on the render goroutine and
	// RenderListTableRow reads Cols synchronously without retaining
	// the slice, so reuse is safe. The previous per-row make+copy
	// was ~rows×cols allocations per frame on recolored surfaces
	// (flows / apex / deploys status tints).
	var recolorCols []uilayout.ListColumn
	if model.Recolor != nil {
		recolorCols = make([]uilayout.ListColumn, len(model.Cols))
	}
	rowFn := func(i int) string {
		if i < 0 || i >= model.N {
			return ""
		}
		row := i
		if sortPerm != nil {
			if i >= len(sortPerm) {
				return ""
			}
			row = sortPerm[i]
		}
		localSpec := spec
		if model.Recolor != nil {
			copy(recolorCols, model.Cols)
			for c := range recolorCols {
				recolorCols[c].Style = model.Recolor(row, c, recolorCols[c].Style)
			}
			localSpec.Cols = recolorCols
		}
		return uilayout.RenderListTableRow(localSpec, res, row, i == cur, focus == focusMain, inner, terms)
	}

	if model.State.Paginated {
		// Fit-to-pane page size: whatever fits the row budget.
		// Page tracks the cursor — moving the cursor across the
		// page boundary advances/reverses the page automatically,
		// so users navigate with the same j/k they use in scroll
		// mode. Auto-follow keeps the highlight on screen without
		// needing dedicated page-nav keys (which most laptops
		// don't have anyway).
		// rowBudget = local budget minus chrome already emitted
		// (rowsHeader) and the trailing hint slot.
		rowBudget := budget - rowsHeader - trailing
		pageSize := uilayout.PageSizeFor(rowBudget, model.N)
		if pageSize > 0 {
			model.State.Page = cur / pageSize
		}
		// Patch the title to include "· Page X / Y" now that the
		// page index has settled. titleIdx is -1 when the title
		// was empty.
		if titleIdx >= 0 {
			total := uilayout.TotalPages(model.N, pageSize)
			page := model.State.Page + 1
			if page > total {
				page = total
			}
			if page < 1 {
				page = 1
			}
			lines[titleIdx] = headerWithSearchPill(
				fmt.Sprintf("%s · Page %d / %d", model.Title, page, total),
				*model.Search,
			)
		}
		// Wrap rowFn with the paged-row cache. In paginated mode
		// the same rows reappear on every wheel tick within a page
		// — only the cursor index changes. Caching the non-cursor
		// rows turns each tick into "render one row" instead of
		// "render the whole page." See render_cache.go for the key
		// shape; cache invalidates wholesale when any group-level
		// input shifts.
		cachedFn := m.wrapPagedRowFn(model, focus, inner, pageSize, terms, cur, rowFn)
		rows, _ := uilayout.RenderRowsPaged(model.N, cur, model.State.Page, pageSize, inner, cachedFn)
		// Paginated scroll bar: same right-edge anchor as continuous
		// mode. Bar reflects cursor position over the FULL list (not
		// position-within-page) so it tracks the user's progress
		// through the entire result set as pages advance.  Always
		// shown — pagination is discrete, so even a single-page list
		// gets a visual anchor consistent with multi-page surfaces.
		decoratePagedRowsWithScrollbar(rows, cur, model.N, pageSize, inner)
		lines = append(lines, rows...)
	} else {
		// renderRows works in our local scope — `budget` is the
		// vertical space we have for everything in this call (chrome
		// + rows + hint). Subtract the chrome already emitted
		// (rowsHeader) plus the trailing hint slot to get the row
		// budget.
		rowBudget := budget - rowsHeader - trailing
		if rowBudget < 1 {
			rowBudget = 1
		}
		rowBlock := renderRows(model.N, cur, budget, rowsHeader, trailing, inner, rowFn)
		// Continuous-mode scroll bar: 1-char track + thumb pinned to
		// the pane's inner right edge. Decorates only the row lines
		// (rowBlock without the trailing scrollIndicator); pads each
		// to inner-1 first so the bar always lands at the same column
		// regardless of how wide the table's last column resolved to.
		if model.N > rowBudget {
			decorateRowsWithScrollbar(rowBlock, cur, model.N, rowBudget, inner)
		}
		lines = append(lines, rowBlock...)
	}

	// Hint sits right under the rows / page indicator with one blank
	// line of breathing room. We do NOT pad to budget — the
	// surrounding pane has a fixed Height(h) that fills any unused
	// vertical space with whitespace below the content for free.
	// Padding here would just duplicate that, and the pad/border math
	// is fragile when callers stack multi-line chrome above this block
	// (e.g. dashboards with embedded newlines counted as one slice
	// element). Leaving the pane's intrinsic padding to do the job
	// means the hint always sits just below the rows with one blank
	// line of gap, regardless of caller stacking.
	hintBlock := []string{"", m.footerHint(m.listTableHint(model.State, res, len(model.Cols), model.Search, model.FooterExtras), inner)}
	lines = append(lines, hintBlock...)
	return lines
}

func listModelSortDataKey(model listRenderModel) string {
	if model.SortDataKey != "" {
		return model.SortDataKey
	}
	if model.DataVersion != 0 {
		return strconv.Itoa(model.DataVersion)
	}
	return ""
}

// usedLines counts the actual terminal-line height of a string slice.
// Each entry contributes 1 + number of embedded newlines, so a multi-
// line dashboard string passed in as one element doesn't undercount.
func usedLines(lines []string) int {
	n := 0
	for _, s := range lines {
		n += strings.Count(s, "\n") + 1
	}
	return n
}

// _ keeps strings imported for any future helpers; safe to remove
// when concrete code uses it directly.
var _ = strings.Join
