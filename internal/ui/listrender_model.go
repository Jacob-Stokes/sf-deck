package ui

// Per-frame list-table render model.

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

type listRenderModel struct {
	Title string

	State  *uilayout.ListTableState
	Search *searchState

	Cols []uilayout.ListColumn
	N    int
	Cell func(row, col int) string

	// Cursor is the current selected row in the filtered view.
	// Renderer clamps to [0, N) defensively; callers should
	// pre-clamp too so the highlight bar lands sensibly when
	// filters change.
	Cursor int

	Marks        []uilayout.RowMark
	Gutters      []uilayout.GutterSpec
	RightGutters []uilayout.GutterSpec

	Recolor func(row, col int, base lipgloss.Style) lipgloss.Style

	Empty string

	// Err, when non-nil, is the resource's last fetch error. When the
	// list is empty AND Err is set, the empty-state renders the error
	// (why the list is empty) instead of the generic "no rows" copy —
	// so an org that can't serve a surface (no API access, missing FLS,
	// a Tooling failure) tells the user what happened rather than
	// looking like an empty-but-fine list. See errEmptyMessage.
	Err error

	FooterExtras string

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
	spec.SortCacheKey = cursorSortCacheKey(model.State, model.Cols, model.N, listModelSortDataKey(model))
	res := uilayout.LayoutListTable(spec, model.State, inner)
	terms := m.searchTerms()
	lines = append(lines, uilayout.RenderListTableHeader(spec, res, model.State, inner))
	var sortPerm []int
	if model.State == nil || !model.State.RowsOrdered {
		sortPerm = uilayout.SortedIndices(spec, model.State)
	}

	rowsHeader := len(lines)
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
		rowBudget := budget - rowsHeader - trailing
		pageSize := uilayout.PageSizeFor(rowBudget, model.N)
		if pageSize > 0 {
			model.State.Page = cur / pageSize
		}
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
		cachedFn := m.wrapPagedRowFn(model, focus, inner, pageSize, terms, cur, rowFn)
		rows, _ := uilayout.RenderRowsPaged(model.N, cur, model.State.Page, pageSize, inner, cachedFn)
		decoratePagedRowsWithScrollbar(rows, cur, model.N, pageSize, inner)
		lines = append(lines, rows...)
	} else {
		rowBudget := budget - rowsHeader - trailing
		if rowBudget < 1 {
			rowBudget = 1
		}
		rowBlock := renderRows(model.N, cur, budget, rowsHeader, trailing, inner, rowFn)
		if model.N > rowBudget {
			decorateRowsWithScrollbar(rowBlock, cur, model.N, rowBudget, inner)
		}
		lines = append(lines, rowBlock...)
	}

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
