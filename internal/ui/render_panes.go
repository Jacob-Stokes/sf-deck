package ui

// The main pane sits between the left rail (leftrail.go) and the right
// context sidebar (sidebar.go).

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderMain dispatches to the current view's renderer, then wraps the
// content in the main pane's border. Border color picks up the yellow
// "filter applied" tint when the active view has a committed search.
//
// Renderer dispatch is registry-driven: every tab declares its
// top-level renderer in TabSpec.Renderer (tab_registry.go) and a
// single resolveRenderer call walks subtab → tab to find it.
func (m Model) renderMain(w, h, innerH int) string {
	content := ""
	if msg := m.disconnectedOrgNotice(w - 4); msg != "" {
		content = msg
	} else if fn := m.resolveRenderer(); fn != nil {
		content = fn(m, w, innerH)
	}

	style := theme.Panelled
	if m.focus == focusMain {
		style = theme.PanelledFocus
		// On SidebarFocusable tabs the focused pane is whichever
		// bodyFocus points to — dim the main border when the
		// sidebar has taken over so the user sees the swap.
		if spec := lookupTabSpec(m.tab()); spec != nil && spec.SidebarFocusable && !m.bodyFocus {
			style = theme.Panelled
		}
	}
	if s := m.searchStateForTab(m.tab()); s != nil && s.Applied() {
		style = theme.PanelledFiltered
	}
	// Record-drill tabs get a distinct magenta border so the user
	// always sees at a glance "I'm inside a record." Wins over
	// focus + filtered styling because the drill cue is the more
	// important context — overrides last so it sticks.
	if isRecordDrillTab(m.tab()) {
		style = theme.PanelledDrill
	}
	// Height is a MINIMUM in lipgloss; MaxHeight caps growth when a
	// long line wraps or clipLines slips a line through. Both refer
	// to the final rendered string (border included) — Height(h)
	// internally subtracts verticalBorderSize before padding content,
	// and MaxHeight(h) truncates after the border is applied. So to
	// lock the pane to exactly h rows, use h for both.
	return style.Width(w).Height(h).MaxHeight(h).Render(clipLines(content, innerH))
}

// isRecordDrillTab reports whether the given tab is a "drilled
// into a specific entity" surface. The main panel border switches
// to the thick magenta treatment so the user always knows at a
// glance "I'm inside a single record/field/flow/etc., not
// browsing a list." Misleading name kept for backward grepping —
// the set covers every drill, not just records.
func isRecordDrillTab(t Tab) bool {
	switch t {
	case
		TabRecordDetail,
		TabFieldDetail,
		TabValidationDetail,
		TabRecordTypeDetail,
		TabTriggerDetail,
		TabFlowDetail,
		TabReportDetail,
		TabApexDetail,
		TabLWCDetail,
		TabUserDetail,
		TabPermParentDetail,
		TabQueueDetail,
		TabPublicGroupDetail,
		TabBundleDetail,
		TabDevProjectDetail,
		TabTagDetail:
		return true
	}
	return false
}

// searchStateForTab returns the given tab's active search state, or
// nil if that tab isn't searchable. Used by renderers for visual
// affordances (yellow border on the active tab's pane, filter pill,
// status-bar ⌕ marker, and the magnifier glyph on tab pills in the
// top bar).
//
// Critical: this function MUST honour the `v` argument so the
// magnifier glyph appears on every tab with a committed search, not
// just on whichever tab the user is currently viewing.  Earlier
// versions ignored `v` and routed through `m.tab()`-based resolvers
// — that made the glyph only visible on the active tab.
//
// Resolution order per tab:
//  1. Walk the tab's spec; if it has subtabs, use that tab's own
//     GetSubtabIdx(m) to find the active subtab — NOT
//     m.currentSubtab() (which only knows the active tab).
//  2. Subtab's List surface → SearchPtr(d).
//  3. Subtab's SearchPtr closure.
//  4. Tab's top-level List surface.
//  5. Tab's top-level SearchPtr closure.
//
// All paths read from the active org's orgData since search state
// is per-org-per-list.
func (m Model) searchStateForTab(v Tab) *searchState {
	spec := lookupTabSpec(v)
	if spec == nil {
		return nil
	}
	d, ok := m.activeOrgState()
	if !ok {
		return nil
	}
	// Shadow m.tab() to v for the duration of every SearchPtr /
	// subtab lookup below.  Some SearchPtr closures (e.g.
	// objectDetailSearchPtr, reportsSearchPtr) consult
	// m.currentSubtab() internally — that resolves via m.tab()'s
	// active subtab, NOT the v we're asking about.  Without the
	// override, those closures answer for the user's current tab
	// instead of v, so the magnifier glyph for OTHER tabs comes
	// from the user's active tab's state — magnifier ends up
	// inconsistent across the bar.
	//
	// The override is local to this Model copy (modelRuntime is a
	// value type embedded by value into Model) — bubbletea never
	// sees the modified copy.
	m.tabOverride = v
	m.tabOverrideSet = true
	// Resolve THIS tab's active subtab (not the user's current tab's).
	sub := subtabSpecForTab(spec, m)
	if sub != nil {
		if sub.List != nil && sub.List.SearchPtr != nil {
			if s := sub.List.SearchPtr(d); s != nil {
				return s
			}
		}
		if sub.SearchPtr != nil {
			if s := sub.SearchPtr(m); s != nil {
				return s
			}
		}
	}
	if spec.List != nil && spec.List.SearchPtr != nil {
		if s := spec.List.SearchPtr(d); s != nil {
			return s
		}
	}
	if spec.SearchPtr != nil {
		return spec.SearchPtr(m)
	}
	return nil
}

// subtabSpecForTab returns the active SubtabSpec for the given
// TabSpec, reading the tab's own subtab index via its GetSubtabIdx
// closure.  Unlike TabSpec.activeSubtabSpec (which uses
// m.currentSubtab() — only valid for the user's current tab), this
// works for any tab.
func subtabSpecForTab(s *TabSpec, m Model) *SubtabSpec {
	if s == nil || len(s.Subtabs) == 0 || s.GetSubtabIdx == nil {
		return nil
	}
	idx := s.GetSubtabIdx(m)
	if idx < 0 || idx >= len(s.Subtabs) {
		return nil
	}
	return &s.Subtabs[idx]
}

// disconnectedOrgNotice returns the full-pane notice shown instead
// of tab content while the active org can't authenticate. ONE gate
// for every org-backed surface (drills included) — cached data is
// hidden, not deleted; it returns and refreshes after re-auth.
// Empty string = render normally.
func (m Model) disconnectedOrgNotice(inner int) string {
	if len(m.orgs) == 0 {
		return ""
	}
	o := m.orgs[m.selected]
	if canUseOrg(o) {
		return ""
	}
	if spec := lookupTabSpec(m.tab()); spec != nil && spec.OrgIndependent {
		return ""
	}
	label := o.Display()
	lines := []string{
		"",
		redLine("  ● " + label + " is disconnected (" + o.Status + ")"),
		"",
		dimLine("  Cached data is hidden until the org re-authenticates —", inner),
		dimLine("  nothing is deleted; it returns (and refreshes) on reconnect.", inner),
		"",
		dimLine("  Re-authenticate: "+firstPretty(Keys.FocusOrgs)+" (orgs rail) → "+
			firstPretty(Keys.OrgManageOpen)+" (manager) → "+firstPretty(Keys.OrgReauth)+" (re-login web)", inner),
	}
	return strings.Join(lines, "\n")
}
