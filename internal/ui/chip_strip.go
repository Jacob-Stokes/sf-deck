package ui

// Two-zone layout primitives.
//
// Several tabs (Objects, Records, the per-sObject detail drills) share
// a shape: a **dashboard** zone at the top (stats + view chips) and a
// **listable** zone below (the actual rows). This file holds the
// shared chip-strip renderer and the helpers tabs call into.
//
// Per-tab: each tab declares its own list of chipRow entries and
// tracks a selected-index in its own Model state. Cycling happens via
// left/right arrow keys when the main pane is focused.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// chipRow is one predefined filter shown in the dashboard zone.
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

	// Reserve trailing-hint width up-front so chip fitting accounts
	// for it. The hint right-aligns to the far edge of the strip
	// (separated by a stretch of whitespace) rather than sitting
	// immediately after the last chip — visually a "left = chips,
	// right = hint" layout. The hint is dropped (returned as "")
	// when its width would push the chips past the pane.
	hintStyled := ""
	hintW := 0
	if trailingHint != "" {
		styled := lipgloss.NewStyle().Foreground(theme.FgDim).Render(trailingHint)
		w := ansi.StringWidth(styled)
		// Reserve room for the hint + at least 2 cells of breathing
		// space between it and the rightmost chip.
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

	// Greedy fit from the left. If the selected chip would be dropped,
	// re-run starting from `selected` so it stays visible.
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

	// Reserve room for the ellipsis on the right if some chips are
	// hidden. Shrink `end` until the fit + ellipsis fit in chipBudget.
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
		// Pad the chip cluster out to (width - hintWidth) cells so
		// the hint ends up flush-right. ansi.StringWidth gives us
		// the cell count excluding ANSI escapes.
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
		// Active + cross-org preview: cyan background marks it as the
		// ephemeral row from another org you're peeking at. The label
		// already carries "(from <org>)" so origin is always visible.
		return lipgloss.NewStyle().
			Foreground(theme.Bg).
			Background(theme.Cyan).
			Bold(true).
			Padding(0, 1).
			Render(label)
	case active && transient:
		// Active + transient: yellow background says "this is the
		// session-only chip you're looking at". Press F to pin it.
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
		// Inactive preview — cyan italic accent so the user can spot
		// the cross-org peek even when their cursor is elsewhere.
		return lipgloss.NewStyle().
			Foreground(theme.Cyan).
			Italic(true).
			Padding(0, 1).
			Render(label)
	case transient:
		// Inactive transient — yellow accent so the user can tell at
		// a glance which chip is the session-only one even when their
		// cursor is on a different favourite.
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

	// Cache the assembled dashboard string. The chip strip + title
	// + rule fire on every render but their inputs change rarely:
	// the chip selection moves on chip-cycle (~once per minute), the
	// chip composition changes on chip create/delete (rare), the
	// hint depends on stable tab/sObject context. During scroll
	// none of these change — same dashboard every frame.
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

// buildDashboard is the actual rendering body; renderDashboard wraps
// it with the cache. Split out so the cache miss path stays a one-
// liner and the no-cache fallback (when renderCache is nil — tests)
// still has the same logic.
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
		// Count is int — fold its bits.
		c := uint64(uint32(r.Count))
		h ^= c
		h *= fnvPrime
		h ^= 0xff
	}
	return h
}

// viewStripHint returns the trailing key-hint shown to the right of
// the chip strip. Records-shaped surfaces get L (source toggle) plus
// V (manage). /objects, /flows, /users · All users just get V —
// they're single-source surfaces with no live SF list-view preview
// mode to toggle into. Empty string for surfaces without a chip-
// driven view system (defensive — callers only invoke renderDashboard
// on chip-driven tabs today).
func (m Model) viewStripHint() string {
	// Prepend the view-cycle key when the tab has any chip surface at
	// all. The previous predicate (tabHasDashboard) was a hardcoded
	// short list — Objects, Records, ObjectDetail, Flows — which
	// excluded chip-bearing tabs like Apex, Users, Perms, Reports. The
	// hint is meaningful wherever `[` / `]` actually cycles
	// something, so drive it from resolveChipSurface() directly.
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
		// Append the currently-active mode in dim brackets so the L
		// keystroke hint conveys "what will I be toggling TO" at a
		// glance.  Color tracks the mode — Cyan for sf-deck, Yellow
		// for Salesforce — both rendered with FgDim weight so the
		// hint sits behind the keycap.
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
	// Any chip-driven tab (Objects/Flows/Users) gets the manage hint —
	// driven by TabSpec.Chips presence rather than a hard-coded switch
	// so adding a new chip-driven tab gets the hint for free.
	spec := lookupTabSpec(m.tab())
	if spec == nil {
		return viewCycle
	}
	if spec.Chips != nil {
		return join(viewCycle, firstPretty(Keys.OpenLensManager)+" manage")
	}
	// Subtab-level Chips (TabUsers' All-users subtab — the other
	// subtabs don't render a chip strip so renderDashboard isn't
	// called there).
	if sub := spec.activeSubtabSpec(m); sub != nil && sub.Chips != nil {
		return join(viewCycle, firstPretty(Keys.OpenLensManager)+" manage")
	}
	return viewCycle
}
