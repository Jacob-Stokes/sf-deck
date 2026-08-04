package ui

// Session-scoped SOQL editor + result state.
//
// The top-level /soql tab and the SOQL modal each own a separate
// soqlSession. modelSOQL embeds the tab's session so existing field
// access (m.soqlInput, m.soqlResult, …) keeps working while the state
// itself is no longer welded directly onto Model.

import (
	"context"
	"sync/atomic"

	"charm.land/bubbles/v2/textarea"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var nextSOQLSessionID uint64

// soqlSession owns one SOQL editor/result workspace.
type soqlSession struct {
	id uint64

	// SOQL view state — not cached; results are ephemeral. The input
	// itself is owned by a bubbles/textarea widget so the user gets
	// real cursor nav / word jumps / multi-line composition while
	// building queries. Enter is intercepted by our edit handler
	// (runs the query); shift+enter inserts a newline.
	soqlInput   textarea.Model
	soqlHistory []string
	soqlResult  sf.QueryResult
	soqlErr     error
	soqlRunning bool
	soqlEditing bool
	soqlTooling bool
	// soqlBulk, when true, routes the next run through Bulk API 2.0
	// instead of the synchronous REST /query + nextRecordsUrl path.
	// Tradeoff: Bulk consumes ~1 API call per job regardless of row
	// count (vs REST's one call per 2000 rows), but is async — the
	// user waits for job submission, polling, and CSV download
	// (~10-60s for moderate sizes). Mutually exclusive with
	// soqlTooling: Bulk doesn't support Tooling queries.
	soqlBulk   bool
	soqlRowCur RawRow // raw backing-row cursor; display mapping via soqlSessionTableAdapter

	// soqlCancel cancels the in-flight SOQL query.  Set when
	// runSOQLCmd starts; cleared (after invocation) when the result
	// lands OR when ctrl+c on /soql is pressed.  nil = nothing
	// running.
	//
	// soqlRunGen increments on each new run so late-arriving results
	// from a cancelled query don't overwrite UI state set by the
	// cancel handler.  Sent with each soqlResultMsg; on receipt we
	// drop the message if the gen doesn't match the current one.
	soqlCancel context.CancelFunc
	soqlRunGen uint64
	// soqlTable carries the SOQL result table's per-render state —
	// horizontal scroll, user-pinned column widths, zen flag. Persists
	// across re-renders within a session.
	soqlTable uilayout.ListTableState

	// soqlSearch is the per-session sticky search-buffer for the SOQL
	// results grid. `/` opens it; the renderer narrows the visible
	// rows to substring matches across every projected column.
	// Mirrors the records / objects search contract — search state
	// survives navigation away and back.
	//
	// Pointer (not value) so the same state survives the value-Model
	// copy through Update / render. Lazy-allocated on first access
	// via soqlSearchPtr().
	soqlSearch *searchState

	// autocomplete is the live-suggestion popup state. Lazy-
	// allocated on first edit keystroke. Pointer so the same
	// state survives the value-Model copy through Update.
	autocomplete *autocompleteState
}

// modelSOQL owns the tab-specific SOQL state. The editor/result fields
// live in the embedded session; the subtab and saved-query edit marker
// remain tab-only because the modal intentionally has no Saved/History
// UI in this first pass.
type modelSOQL struct {
	soqlSession

	// soqlSubtabIdx selects between Editor (default), Saved, and
	// History on /soql. The lists themselves live per-org on
	// orgData (SOQLSavedList, SOQLHistoryList) so the standard
	// chip + listSurface plumbing works without a Model-only
	// special case.
	soqlSubtabIdx int

	// soqlEditingSavedID, when non-empty, is the id of the saved
	// query the editor currently holds. Set when the user loads a
	// row via Enter from the Saved subtab; reset when the editor
	// is cleared or the user saves a new one. Drives the "S
	// updates in place vs creates new" decision. Lives on the tab
	// session (not orgData) because the editor itself is shared
	// across orgs — the user types one query at a time, full stop.
	soqlEditingSavedID string

	// reportRunTable mirrors soqlTable for the reports detail surface.
	// One state shared across all reports — switching between reports
	// re-uses the same scroll/zen state, which matches the user's
	// likely intent ("I'm exploring report data, keep my settings").
	reportRunTable uilayout.ListTableState
}

func newSOQLSession(initial string) soqlSession {
	return soqlSession{
		id:         atomic.AddUint64(&nextSOQLSessionID, 1),
		soqlInput:  newSOQLInput(initial),
		soqlSearch: &searchState{},
	}
}

func (s *soqlSession) searchPtr() *searchState {
	if s == nil {
		return nil
	}
	if s.soqlSearch == nil {
		s.soqlSearch = &searchState{}
	}
	return s.soqlSearch
}

func (m *Model) soqlSessionForTarget(target soqlSessionTarget) *soqlSession {
	if m == nil {
		return nil
	}
	switch target {
	case soqlSessionModal:
		if m.soqlModal == nil {
			return nil
		}
		return &m.soqlModal.session
	case soqlSessionTab, "":
		return &m.soqlSession
	default:
		return nil
	}
}

// soqlRenderEntry holds the cached column-discovery + width
// measurement + cell matrix for one SOQL result-set view. Invalidated
// by raw-rows-slice pointer change (new result lands), filtered-rows
// pointer/length change (search-buffer edit), or theme switch (column
// widths may need re-measuring if styling changed cell shape).
//
// cells is column-major: cells[col][row] — so the Cell callback in
// soqlRenderModel becomes a bounds-checked 2D slice lookup. Mirrors
// recordsTableProjection's shape; chosen so the renderer's hot path
// is zero-allocation per visible cell per frame (was: a map lookup
// + dotted-path walk + formatCell type-switch per cell per frame).
type soqlRenderEntry struct {
	rowsPtr     uintptr // raw m.soqlResult.Records header — gates column cache
	rowsLen     int
	searchBuf   string // search query string at build time
	searchOn    bool   // search applied flag at build time
	themeID     string
	query       string // SOQL source — drives column order
	colNames    []string
	listCols    []uilayout.ListColumn
	cells       [][]string       // column-major; cells[col][row] over the FILTERED rows
	filtered    []map[string]any // the post-filter rows slice (may equal raw slice when no filter)
	filteredIdx []int            // filtered-row → raw-row index, for cursor mapping
}

// cell looks up the pre-rendered cell string. Returns "" on out-of-
// bounds — the renderer's nil-guards rely on the same behavior.
func (e *soqlRenderEntry) cell(row, col int) string {
	if e == nil || col < 0 || col >= len(e.cells) {
		return ""
	}
	if row < 0 || row >= len(e.cells[col]) {
		return ""
	}
	return e.cells[col][row]
}
