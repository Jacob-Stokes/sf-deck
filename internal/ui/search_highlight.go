package ui

// Glue between the active search-state and the per-cell highlight
// helper. Row renderers call m.searchTerms() once per render to get
// the term slice, then wrap each rendered cell via uilayout.Highlight.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func (m Model) searchTerms() []string {
	s := m.currentSearchTabState()
	if s == nil || !s.Applied() {
		return nil
	}
	return uilayout.SearchTerms(s.Buffer())
}

// currentSearchTabState mirrors currentSearch (which is *searchState)
// but as a value-receiver — needed because some callers run on Model
// value receivers and currentSearch's pointer-receiver chain is
// awkward in those contexts. Returns nil when no search is wired for
// the current tab.
func (m Model) currentSearchTabState() *searchState {
	mm := m
	return mm.currentSearch()
}
