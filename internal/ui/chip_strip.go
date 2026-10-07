package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type chipRow struct {
	ID    string // stable identifier (used for keymap / persistence)
	Label string // user-visible, e.g. "Custom"
	Count int    // -1 means "unknown/lazy"
}

// renderChipStrip draws a horizontal strip of view chips with the
// current selection highlighted. Always exactly ONE row tall — chips
// that would overflow the width are dropped and an ellipsis marker
// is appended so callers (including the dashboard-height math) never
// have to plan for a variable-height strip. The selected chip is
// preserved by shifting it into the visible window when needed.
// Inactive chips dim; active chip is blue bold with a background
// tint; counts follow the label in parens.
//
// trailingHint is an optional dim string appended to the right of the
// last chip (after the M sentinel when present). Used to surface
// L = source / V = manage so users discover those keys without
// hunting through the status bar. Pass "" to suppress.
func renderChipStrip(chips []chipRow, selected int, width int, trailingHint string) string {
	if len(chips) == 0 || width <= 0 {
		return ""
	}
	rendered := make([]string, len(chips))
	widths := make([]int, len(chips))
	for i, c := range chips {
		rendered[i] = renderChip(c, i == selected)
		widths[i] = ansi.StringWidth(rendered[i])
	}
	ellipsis := lipgloss.NewStyle().Foreground(theme.FgDim).Render("…")
	ellipsisW := ansi.StringWidth(ellipsis)

	hintStyled := ""
	hintW := 0
	if trailingHint != "" {
		styled := lipgloss.NewStyle().Foreground(theme.FgDim).Render(trailingHint)
		w := ansi.StringWidth(styled)
		if w+3 < width {
			hintStyled = styled
			hintW = w + 2 // +2 for the minimum gap
		}
	}
	// Sticky overflow sentinel: when the LAST chip is the "+ N more"
	// affordance and there's room for it, reserve its width up-front
	// and rendering loop fits the remaining chips into the smaller
	// budget. The whole point of the affordance is that the user can
	// always reach the hidden chips — so it must never itself be
	// hidden by truncation. Detected via chipOverflowID.
	overflowIdx := -1
	overflowW := 0
	if last := len(chips) - 1; last >= 0 && chips[last].ID == chipOverflowID {
		if widths[last]+2 < width-hintW {
			overflowIdx = last
			overflowW = widths[last] + 1 // +1 for the gap before it
		}
	}
	chipBudget := width - hintW - overflowW

	// Iteration end-cap: when the overflow sentinel is sticky, don't
	// let the greedy fitter consume it in the normal pass — it gets
	// stitched on at the end.
	fitEnd := len(rendered)
	if overflowIdx >= 0 {
		fitEnd = overflowIdx
	}

	fit := func(start int) (end int, needsEllipsis bool) {
		used := 0
		for i := start; i < fitEnd; i++ {
			gap := 0
			if i > start {
				gap = 1
			}
			if used+gap+widths[i] > chipBudget {
				return i, true
			}
			used += gap + widths[i]
		}
		return fitEnd, false
	}

	start := 0
	end, needsEllipsis := fit(0)
	if selected >= end && selected < fitEnd {
		start = selected
		end, needsEllipsis = fit(selected)
	}

	if needsEllipsis {
		for end > start+1 {
			used := 0
			for i := start; i < end; i++ {
				gap := 0
				if i > start {
					gap = 1
				}
				used += gap + widths[i]
			}
			if used+1+ellipsisW <= chipBudget {
				break
			}
			end--
		}
	}

	parts := append([]string{}, rendered[start:end]...)
	if needsEllipsis {
		parts = append(parts, ellipsis)
	}
	if overflowIdx >= 0 {
		parts = append(parts, rendered[overflowIdx])
	}
	out := strings.Join(parts, " ")
	if hintStyled != "" {
		chipsW := ansi.StringWidth(out)
		gap := width - chipsW - ansi.StringWidth(hintStyled)
		if gap < 2 {
			gap = 2
		}
		out = out + strings.Repeat(" ", gap) + hintStyled
	}
	return out
}

func renderChip(c chipRow, active bool) string {
	label := c.Label
	if c.Count >= 0 {
		label = fmt.Sprintf("%s %d", c.Label, c.Count)
	}
	transient := c.Count == chipRowKindTransient
	preview := c.Count == chipRowKindPreview
	switch {
	case active && preview:
		return lipgloss.NewStyle().
			Foreground(theme.Bg).
			Background(theme.Cyan).
			Bold(true).
			Padding(0, 1).
			Render(label)
	case active && transient:
		return lipgloss.NewStyle().
			Foreground(theme.Bg).
			Background(theme.Yellow).
			Bold(true).
			Padding(0, 1).
			Render(label)
	case active:
		return lipgloss.NewStyle().
			Foreground(theme.Bg).
			Background(theme.Blue).
			Bold(true).
			Padding(0, 1).
			Render(label)
	case preview:
		return lipgloss.NewStyle().
			Foreground(theme.Cyan).
			Italic(true).
			Padding(0, 1).
			Render(label)
	case transient:
		return lipgloss.NewStyle().
			Foreground(theme.Yellow).
			Padding(0, 1).
			Render(label)
	}
	return lipgloss.NewStyle().
		Foreground(theme.Muted).
		Padding(0, 1).
		Render(label)
}

// renderDashboard wraps the chip strip (and later, stat lines) in a
// small header block with a thin rule below. Takes the chips slice
// directly; tabs compose whatever stat strips they want before calling
// this. Returns empty if the user has collapsed the dashboard.
//
// A trailing key-hint (e.g. "L source · V manage") is auto-derived
// from the active surface — records get the source toggle, /objects
// and /flows just get the manage hint. The hint anchors next to the
// "+ N more (M)" sentinel so all three view-system shortcuts cluster
// at the right edge of the strip.
func (m Model) renderDashboard(title string, chips []chipRow, selected int, width int) string {
	if m.dashboardCollapsed {
		return ""
	}
	hint := m.viewStripHint()

	if m.renderCache != nil {
		key := dashboardCacheKey{
			title:     title,
			chipsHash: hashChipRows(chips),
			chipsLen:  len(chips),
			selected:  selected,
			width:     width,
			hint:      hint,
			collapsed: m.dashboardCollapsed,
		}
		if hit, ok := m.renderCache.dashboards[key]; ok {
			return hit
		}
		out := buildDashboard(title, chips, selected, width, hint)
		// Soft cap: dashboards can accumulate over a long session
		// as the user cycles chips, switches tabs, resizes. 256
		// distinct keys is plenty for any real workflow; flush
		// past that to keep the map bounded. Cheap drop-and-reset
		// is fine — next frame rebuilds whatever's currently shown.
		if len(m.renderCache.dashboards) > 256 {
			m.renderCache.dashboards = map[dashboardCacheKey]string{}
		}
		m.renderCache.dashboards[key] = out
		return out
	}
	return buildDashboard(title, chips, selected, width, hint)
}

func buildDashboard(title string, chips []chipRow, selected int, width int, hint string) string {
	var lines []string
	if title != "" {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(theme.Muted).
			Bold(true).
			Render(title))
	}
	if strip := renderChipStrip(chips, selected, width, hint); strip != "" {
		lines = append(lines, strip)
	}
	if len(lines) == 0 {
		return ""
	}
	rule := lipgloss.NewStyle().Foreground(theme.Border).
		Render(strings.Repeat("─", width))
	lines = append(lines, rule)
	return strings.Join(lines, "\n")
}

// hashChipRows produces a fast stable fingerprint of a chip-row
// slice for cache keying. FNV-1a over each row's id + label +
// count covers the inputs the renderer reads. Not crypto-strength
// — collisions would just cause cache reuse of a similar
// composition, which is harmless because the rendered string is
// already correct for that hash class.
func hashChipRows(rows []chipRow) uint64 {
	const (
		fnvOffset = 14695981039346656037
		fnvPrime  = 1099511628211
	)
	h := uint64(fnvOffset)
	for _, r := range rows {
		for _, b := range []byte(r.ID) {
			h ^= uint64(b)
			h *= fnvPrime
		}
		h ^= 0xff
		for _, b := range []byte(r.Label) {
			h ^= uint64(b)
			h *= fnvPrime
		}
		h ^= 0xff
		c := uint64(uint32(r.Count))
		h ^= c
		h *= fnvPrime
		h ^= 0xff
	}
	return h
}

func (m Model) viewStripHint() string {
	viewCycle := ""
	if m.resolveChipSurface() != nil {
		// Separate the two key labels with a spaced "or", not ", " or
		// "/". The default bindings are themselves brackets ([ and ]):
		// "[, ]" reads as a single malformed token (an empty list) and
		// "/" reads as "press the / key". An explicit " or " is
		// unambiguous for any key pair, bracket or not.
		viewCycle = firstPretty(Keys.PrevView) + " or " +
			firstPretty(Keys.NextView) + " view"
	}
	join := func(parts ...string) string {
		out := ""
		for _, p := range parts {
			if p == "" {
				continue
			}
			if out != "" {
				out += " · "
			}
			out += p
		}
		return out
	}

	if d, sobj := m.activeRecordsSObject(); sobj != "" {
		modeLabel := "[sf-deck]"
		modeColor := theme.Cyan
		if currentChipMode(d, sobj) == ChipModeSalesforce {
			modeLabel = "[Salesforce]"
			modeColor = theme.Yellow
		}
		modePill := lipgloss.NewStyle().Foreground(modeColor).Faint(true).Render(modeLabel)
		return join(
			viewCycle,
			firstPretty(Keys.LensModeToggle)+" source "+modePill,
			firstPretty(Keys.OpenLensManager)+" manage",
		)
	}
	spec := lookupTabSpec(m.tab())
	if spec == nil {
		return viewCycle
	}
	if spec.Chips != nil {
		return join(viewCycle, firstPretty(Keys.OpenLensManager)+" manage")
	}
	if sub := spec.activeSubtabSpec(m); sub != nil && sub.Chips != nil {
		return join(viewCycle, firstPretty(Keys.OpenLensManager)+" manage")
	}
	return viewCycle
}
