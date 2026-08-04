package ui

// Centralised column-spec helpers for the list-table primitive.
//
// Each helper returns a fresh slice of uilayout.ListColumn — the
// caller (the renderer) clones + tints style fields, but the Min /
// Ideal / Name / Header values are shared with activeListTable's
// resolution path so column-mode / sort / scroll target the same
// column definition the renderer drew.
//
// New tabs / subtabs add a helper here when they want c + s + z to
// work; activeListTable then routes to it. Without a helper, the
// list-table machinery has nothing to operate on.

import (
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// /home subtab helpers — used by activeListTable so c/s/z work.
// Names + Min/Ideal are the canonical column geometry; renderers
// clone these via withColStyles and add per-column foreground colors
// on top. NEVER duplicate the column literals inline in renderers
// (column-mode operates on the geometry returned here, so a
// drifting inline copy would resize the WRONG column).

func homeNotifCols() []uilayout.ListColumn {
	return schemaListColumns(homeNotifColumnSchema())
}

// withColStyles returns a clone of `cols` with each column's Style
// set from the matching entry in `styles` (keyed by Name). Columns
// without a style entry keep the original (or unstyled) Style.
//
// Use this in /home renderers so the canonical geometry from the
// homeXxxCols helpers stays the source of truth for column-mode and
// the rendered table — without it, the renderer's inline literal can
// drift from the column-mode helper and resize the wrong column.
func withColStyles(cols []uilayout.ListColumn, styles map[string]lipgloss.Style) []uilayout.ListColumn {
	out := make([]uilayout.ListColumn, len(cols))
	copy(out, cols)
	for i := range out {
		if s, ok := styles[out[i].Name]; ok {
			out[i].Style = s
		}
	}
	return out
}
