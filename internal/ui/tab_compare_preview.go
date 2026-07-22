package ui

// /compare — live diff preview in the side panel.
//
// On the Result subtab's inventory, the side panel shows a compact diff
// of the SELECTED row without drilling into the main pane:
//   - RHS (narrow) mode  → single-column unified diff.
//   - Stacked (wide) mode → side-by-side source | target.
// It auto-focuses the first difference; n/N jump between diff hunks; f
// fetches a body that the memory cap dropped; Enter drills into the full
// view (handled by the existing activate path).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/diff"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// compareInventoryPreviewActive reports whether the side panel should be
// showing the inventory diff preview (Result subtab, inventory phase, no
// drill-in open).
func (m Model) compareInventoryPreviewActive() bool {
	if m.tab() != TabCompare || m.currentSubtab() != SubtabCompareResult {
		return false
	}
	d, ok := m.activeOrgState()
	return ok && d.Run != nil && d.Run.Phase == comparePhaseInventory && d.Diff == nil
}

// ensureComparePreview (re)builds the cached preview for the currently
// selected inventory row if stale. Returns the row + whether a preview is
// available (false when the row's body was dropped and not yet fetched).
func (m Model) ensureComparePreview(d *orgData) (diff.Row, bool) {
	row, ok := d.InventoryList.Selected()
	if !ok {
		d.preview = nil
		d.previewKey = ""
		return diff.Row{}, false
	}
	key := row.Type + "|" + row.Key
	if d.previewKey == key && d.preview != nil {
		return row, true
	}
	// Row changed → rebuild. Reset focus to the first difference.
	d.previewKey = key
	d.previewScroll = 0
	d.preview = nil

	// Both bodies retained? Build the diff straight from the snapshots.
	aHave := snapHasBody(d.Run.snapA, row.Type, row.Key)
	bHave := snapHasBody(d.Run.snapB, row.Type, row.Key)
	aMissing := !aHave && row.AID != ""
	bMissing := !bHave && row.BID != ""
	if aMissing || bMissing {
		return row, false // a dropped body — needs fetch (press f)
	}
	res := diff.BodyDiffFromSnapshots(row, d.Run.snapA, d.Run.snapB)
	d.preview = &res
	d.previewScroll = firstDiffLine(res.Lines)
	return row, true
}

// firstDiffLine returns the index of the first non-equal line (so the
// preview opens focused on the first difference), or 0 if all equal.
func firstDiffLine(lines []diff.Line) int {
	for i, l := range lines {
		if l.Op != diff.OpEqual {
			return i
		}
	}
	return 0
}

// nextDiffHunk returns the start index of the next diff hunk after `from`
// (a run of non-equal lines following equal lines). Stays put if none.
func nextDiffHunk(lines []diff.Line, from int) int {
	i := from
	// Skip the current hunk (consecutive non-equal lines).
	for i < len(lines) && lines[i].Op != diff.OpEqual {
		i++
	}
	// Skip the equal gap.
	for i < len(lines) && lines[i].Op == diff.OpEqual {
		i++
	}
	if i >= len(lines) {
		return from // no further hunk
	}
	return i
}

// prevDiffHunk returns the start index of the diff hunk before `from`.
func prevDiffHunk(lines []diff.Line, from int) int {
	i := from - 1
	// Skip the equal gap above us.
	for i >= 0 && lines[i].Op == diff.OpEqual {
		i--
	}
	if i < 0 {
		return from
	}
	// Walk to the START of that hunk.
	for i > 0 && lines[i-1].Op != diff.OpEqual {
		i--
	}
	return i
}

// renderComparePreviewSidebar is the Result subtab's Sidebar func: the
// live diff preview for the selected inventory row.
func (m Model) renderComparePreviewSidebar(inner int) string {
	d, ok := m.activeOrgState()
	if !ok || d.Run == nil || d.Run.Phase != comparePhaseInventory {
		return ""
	}
	row, have := m.ensureComparePreview(d)
	if row.Key == "" {
		return theme.Subtle.Render("  (no row selected)")
	}

	var head []string
	head = append(head, lipgloss.NewStyle().Foreground(theme.BorderHi).Bold(true).Render(
		ansiTrunc(row.Key+" · "+row.Type, inner)))

	// A-only / B-only rows have nothing to diff — say so plainly.
	if row.Status == diff.StatusAOnly || row.Status == diff.StatusBOnly {
		head = append(head, "")
		head = append(head, theme.Subtle.Render("  "+row.Status.String()+" — no counterpart to diff"))
		head = append(head, "")
		head = append(head, theme.Subtle.Render("  enter to view in main pane"))
		return strings.Join(head, "\n")
	}

	if d.previewLoading {
		head = append(head, "")
		head = append(head, theme.Subtle.Render("  "+compareSpinner(m.compareFrame)+" loading body…"))
		return strings.Join(head, "\n")
	}

	if !have {
		// Body dropped by the memory cap — offer to fetch it.
		head = append(head, "")
		head = append(head, theme.Subtle.Render("  body not cached (too large to keep)"))
		head = append(head, theme.Subtle.Render("  f load · enter drill in"))
		return strings.Join(head, "\n")
	}

	res := d.preview
	if len(res.Lines) == 0 {
		head = append(head, "")
		head = append(head, theme.Subtle.Render("  (identical — no differences)"))
		return strings.Join(head, "\n")
	}

	head = append(head, theme.Subtle.Render(fmt.Sprintf("  %d added · %d removed", res.Added, res.Removed)))
	head = append(head, "")

	// Budget = panel inner height minus header + footer. The sidebar's
	// innerH is tracked on the model when it renders.
	budget := m.sidebarInnerH - len(head) - 1
	if budget < 2 {
		budget = 2
	}

	// Reuse the drill-in renderers via a transient view. Stacked (wide) →
	// side-by-side; RHS (narrow) → unified.
	dv := &compareDiffView{Row: row, Result: *res, Scroll: d.previewScroll}
	var body []string
	if m.sidebarStacked {
		body = renderSideBySideDiff(dv, inner, budget)
	} else {
		body = renderUnifiedDiff(dv, inner, budget)
	}
	out := append(head, body...)
	out = append(out, "", theme.Subtle.Render("  n/N next/prev diff · enter drill in"))
	return strings.Join(out, "\n")
}

// handleComparePreviewKey handles the preview-specific keys (n/N hunk nav,
// f fetch) when the inventory preview is active. Returns handled=false to
// let the key fall through.
func (m Model) handleComparePreviewKey(key string) (Model, tea.Cmd, bool) {
	if !m.compareInventoryPreviewActive() {
		return m, nil, false
	}
	d, ok := m.activeOrgState()
	if !ok || d.Run == nil {
		return m, nil, false
	}
	switch key {
	case "n":
		if _, have := m.ensureComparePreview(d); have && d.preview != nil {
			d.previewScroll = nextDiffHunk(d.preview.Lines, d.previewScroll)
		}
		return m, nil, true
	case "N":
		if _, have := m.ensureComparePreview(d); have && d.preview != nil {
			d.previewScroll = prevDiffHunk(d.preview.Lines, d.previewScroll)
		}
		return m, nil, true
	case "f":
		// Fetch a dropped body for the preview (and the eventual drill-in).
		if _, have := m.ensureComparePreview(d); have {
			return m, nil, true // already have it; nothing to fetch
		}
		row, _ := d.InventoryList.Selected()
		aMissing := !snapHasBody(d.Run.snapA, row.Type, row.Key) && row.AID != ""
		bMissing := !snapHasBody(d.Run.snapB, row.Type, row.Key) && row.BID != ""
		if !aMissing && !bMissing {
			return m, nil, true
		}
		d.previewLoading = true
		return m, (&m).refetchComparePreview(d, row, aMissing, bMissing), true
	}
	return m, nil, false
}
