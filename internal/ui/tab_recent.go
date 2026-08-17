package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// renderRecent draws the recent-visits log: chip strip on top
// (project / All / per-kind / Misc), then a tabulated grid below
// (WHEN · KIND · NAME · DETAIL · ID). Most-recently visited first.
//
// Goes through the shared list-table primitive — same chrome / sort
// / scroll / column-mode story as /objects, /flows, etc. The chip
// strip uses the same primitives every other chipped surface uses
// (stripRows + renderDashboard), so the loaded-project chip and
// overflow modal both work without bespoke wiring.
func (m Model) renderRecent(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	d := m.ensureOrgDataRef(o.Username)
	m.syncRecentSFList(o.Username)
	inner := w - 4

	chips := m.stripRows(domainRecent, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.recentChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}
	model, ok := recentListSurface.BuildRenderModel(m, d)
	if !ok {
		return strings.Join(append(lines, dimLine("  loading…", inner)), "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

func recentCols() []uilayout.ListColumn {
	return schemaListColumns(recentColumnSchema())
}

func humanTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("2006-01-02")
}
