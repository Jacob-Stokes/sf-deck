package ui

// format.go — thin wrappers forwarding to uilayout.
// All formatting logic now lives in internal/ui/uilayout/format.go.
// These package-level aliases allow existing callers in internal/ui/
// to keep using unqualified names without any changes.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

func sectionTitle(s string) string              { return uilayout.SectionTitle(s) }
func kvLine(k, v string, width int) string      { return uilayout.KvLine(k, v, width) }
func dimLine(s string, width int) string        { return uilayout.DimLine(s, width) }
func redLine(s string) string                   { return uilayout.RedLine(s) }
func stateSuffix(busy bool, err error) string   { return uilayout.StateSuffix(busy, err) }
func searchBar(s searchState, width int) string { return uilayout.SearchBar(s, width) }
func headerWithSearchPill(title string, s searchState) string {
	return uilayout.HeaderWithSearchPill(title, s)
}
func dashIfEmpty(s string) string           { return uilayout.DashIfEmpty(s) }
func capsFlags(d sf.SObjectDescribe) string { return uilayout.CapsFlags(d) }
func prettyDate(iso string) string          { return uilayout.PrettyDate(iso) }

// Table primitives — see uilayout/table.go.
type tableColumn = uilayout.Column

func renderTableHeader(cols []tableColumn, inner int) string {
	return uilayout.RenderTableHeader(cols, inner)
}
func renderInteractiveTableRow(cols []tableColumn, cells []string, selected, focused bool, inner int) string {
	return uilayout.RenderInteractiveTableRow(cols, cells, selected, focused, inner)
}
