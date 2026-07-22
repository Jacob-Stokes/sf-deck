package ui

// Glue between the active search-state and the per-cell highlight
// helper. Row renderers call m.searchTerms() once per render to get
// the term slice, then wrap each rendered cell via uilayout.Highlight.
//
// One intentional simplification: we apply highlights to ALL columns,
// even when the user typed a fielded query (e.g. `name:foo`). The
// matcher already restricts WHICH rows show; the highlighter just
// shows where the literal substring appears. So `name:foo` returns
// only rows where Name contains foo, but if those rows happen to also
// contain "foo" in Label, that match highlights too. Slightly more
// permissive than the matcher, but consistent with the "highlight
// shows you all visible matches" mental model — and doesn't require
// per-column term routing in every row renderer.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// searchTerms returns the current view's search terms, parsed via
// uilayout.SearchTerms (strips field: prefixes, splits on whitespace).
// Empty slice when no search is active or applied.
//
// Used by list-view row renderers: pass the result to
// uilayout.Highlight per cell. Cheap to call once per render — the
// parser is non-allocating for empty queries.
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
