package ui

import (
	"context"
	"sync/atomic"

	"charm.land/bubbles/v2/textarea"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var nextSOQLSessionID uint64

type soqlSession struct {
	id uint64

	soqlInput   textarea.Model
	soqlHistory []string
	soqlResult  sf.QueryResult
	soqlErr     error
	soqlRunning bool
	soqlEditing bool
	soqlTooling bool
	soqlBulk    bool
	soqlRowCur  RawRow // raw backing-row cursor; display mapping via soqlSessionTableAdapter

	soqlCancel context.CancelFunc
	soqlRunGen uint64
	soqlTable  uilayout.ListTableState

	soqlSearch *searchState

	autocomplete *autocompleteState
}

// modelSOQL owns the tab-specific SOQL state. The editor/result fields
// live in the embedded session; the subtab and saved-query edit marker
// remain tab-only because the modal intentionally has no Saved/History
// UI in this first pass.
type modelSOQL struct {
	soqlSession

	soqlSubtabIdx int

	// soqlEditingSavedID, when non-empty, is the id of the saved
	// query the editor currently holds. Set when the user loads a
	// row via Enter from the Saved subtab; reset when the editor
	// is cleared or the user saves a new one. Drives the "S
	// updates in place vs creates new" decision. Lives on the tab
	// session (not orgData) because the editor itself is shared
	// across orgs — the user types one query at a time, full stop.
	soqlEditingSavedID string

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

func (e *soqlRenderEntry) cell(row, col int) string {
	if e == nil || col < 0 || col >= len(e.cells) {
		return ""
	}
	if row < 0 || row >= len(e.cells[col]) {
		return ""
	}
	return e.cells[col][row]
}
