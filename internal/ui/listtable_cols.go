package ui

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
