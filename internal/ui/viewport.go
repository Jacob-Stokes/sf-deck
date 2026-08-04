package ui

// viewport.go — thin wrappers forwarding to uilayout.
// All viewport/scroll logic now lives in internal/ui/uilayout/viewport.go.
// These package-level aliases allow existing callers in internal/ui/
// to keep using unqualified names without any changes.

import "github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"

func renderRows(
	n, sel, innerH, reserved, trailing, inner int,
	renderRow func(i int) string,
) []string {
	return uilayout.RenderRows(n, sel, innerH, reserved, trailing, inner, renderRow)
}

func renderRowsN(
	n, sel, innerH, reserved, trailing, inner, rowLines int,
	renderRow func(i int) string,
) []string {
	return uilayout.RenderRowsN(n, sel, innerH, reserved, trailing, inner, rowLines, renderRow)
}
