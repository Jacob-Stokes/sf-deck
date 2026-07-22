package ui

// Cursor navigation, drill-in (Enter), refresh, and the search-state
// lookup helpers that other handlers reach into.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

// currentSearch returns the searchState for the currently-focused view,
// or nil if that view isn't searchable.
//
// currentSearch returns the search state for the focused view.
// Walks the same resolution order as searchStateForTab: list surface
// first (uniform list-bearing surfaces), then TabSpec.SearchPtr (for
// surfaces with idiosyncratic shapes).
func (m *Model) currentSearch() *searchState {
	if len(m.orgs) > 0 {
		m.ensureOrgData(m.orgs[m.selected].Username)
	}
	if surf := m.resolveListSurface(); surf != nil && surf.SearchPtr != nil {
		if d, ok := m.activeOrgState(); ok {
			return surf.SearchPtr(d)
		}
	}
	// Use the spec resolver (subtab SearchPtr → tab SearchPtr) rather
	// than reading spec.SearchPtr directly — otherwise a bespoke
	// searchable SUBTAB (one without a listSurface) would advertise as
	// searchable in the registry/docs while / and clear-search silently
	// no-op here.
	if fn := m.resolveSearchPtr(); fn != nil {
		return fn(*m)
	}
	return nil
}

// resetCursorForCurrentView snaps the cursor back to 0 after the
// search buffer changes, so the highlighted row stays in-bounds.
// Walks list surface first (uniform reset), then TabSpec.ResetCursor.
func (m *Model) resetCursorForCurrentView() {
	if len(m.orgs) > 0 {
		m.ensureOrgData(m.orgs[m.selected].Username)
	}
	if surf := m.resolveListSurface(); surf != nil && surf.ResetCursor != nil {
		if d, ok := m.activeOrgState(); ok {
			surf.ResetCursor(d)
			return
		}
	}
	if fn := m.resolveResetCursor(); fn != nil {
		fn(m)
	}
}

// cycleChip shifts the current tab's dashboard-selected view by delta
// (usually +1 for Next, -1 for Prev). No-op on tabs that don't have a
// chip strip. No flash banner — the chip strip's own highlight is the
// authoritative indicator.
func (m Model) cycleChip(delta int) (Model, tea.Cmd) {
	// Generic path: any tab/subtab registered in chipSurfaces gets
	// uniform cycle behaviour.
	if surf := m.resolveChipSurface(); surf != nil {
		if len(m.orgs) == 0 {
			return m, nil
		}
		d := m.ensureOrgData(m.orgs[m.selected].Username)
		m.cycleSimpleChipStrip(delta, d, surf.Registry(&m),
			func() int { return surf.ChipIdx(m) },
			func(i int) { surf.SetChipIdx(&m, i) },
			func() { surf.ResetList(d) })
		// Kick the active tab's EnsureData so chips that swap data
		// sources (e.g. /recent's "From Salesforce" chip) start their
		// lazy fetch as soon as the user lands on them. Cheap no-op
		// for tabs whose data is already cached or doesn't need
		// chip-conditional fetching.
		return m, m.ensureDataFor(m.tab())
	}
	// Bespoke escape hatch: tabs whose chip cursor lives outside the
	// surface registry (multi-axis chips, drill modes).
	if fn := m.resolveCycleChip(); fn != nil {
		mm := m
		return mm, fn(&mm, delta)
	}
	return m, nil
}

// cycleSimpleChipStrip is the body shared by chip-strip cycling on the
// universal-scope list surfaces (/objects, /flows, top-level /records
// before drill-in). The records-detail subtab uses a different path —
// it stores selection on orgData per sobject — so it doesn't fit here.
//
// reg is the registry; idx / setIdx are the model-side getter/setter
// for the chip-strip cursor index; reset is the list-view's
// ResetCursor closure (lets us pass a method without paying the cost
// of generics over T).
func (m Model) cycleSimpleChipStrip(
	delta int,
	d *orgData,
	reg *qchip.Registry,
	idx func() int,
	setIdx func(int),
	reset func(),
) {
	if reg == nil {
		return
	}
	// Arrows only cycle through favourites + the synthetic loaded-
	// project chip (when applicable). The "+ N more…" sentinel at the
	// end of the strip is a click target (M / enter on it), not a
	// cycle stop — landing on it would clear the active matcher and
	// blank the list.
	domain := domainFromRegistry(m, reg)
	strip := m.stripRows(domain, "*")
	navStrip := withoutOverflow(strip)
	if len(navStrip) == 0 {
		return
	}
	// Cursor index is measured against the FULL strip (so the
	// rendered highlight stays correct), but cycling only steps
	// across favourites. Find the cursor in the nav slice by id.
	curIdx := idx()
	curID := ""
	if curIdx >= 0 && curIdx < len(strip) {
		curID = strip[curIdx].ID
	}
	cur := findChipIndex(navStrip, curID)
	cur = wrapIdx(cur+delta, len(navStrip))
	// Map back to the strip-cursor — it equals navStrip[cur]'s
	// position in the full strip.
	for i, row := range strip {
		if row.ID == navStrip[cur].ID {
			setIdx(i)
			break
		}
	}
	reset()
	m.applySelectedChipMatcher(d)
}

// cycleSubtab shifts the current tab's subtab selection by delta. Only
// meaningful on drilled-in tabs that have multiple subtabs; otherwise
// a no-op.
func (m Model) cycleSubtab(delta int) (Model, tea.Cmd) {
	// When the left rail is focused, subtab keys cycle the utility
	// shown in that pane (Orgs ↔ Bookmarks ↔ …) instead of the main
	// tab's subtabs. Feels right: whichever pane has focus is the
	// one whose subtabs are being operated on.
	if m.focus == focusOrgs {
		utils := leftrailUtilities()
		if len(utils) > 1 {
			m.leftUtilityIdx = wrapIdx(m.leftUtilityIdx+delta, len(utils))
		}
		return m, nil
	}
	subs := m.tabSubtabs()
	if len(subs) <= 1 {
		return m, nil
	}
	// Tab/Shift+Tab cycles only through the pinned subset — the
	// strip's visible pills. Overflow subtabs are reachable via
	// shift+0 / the More… modal; cycling through them silently
	// would mean Tab can take you to a subtab the user can't see
	// on the strip, which is confusing.
	pinned, _ := m.subtabPinSplit()
	cycleLen := pinned
	if cycleLen <= 1 {
		cycleLen = len(subs)
	}
	// Registry-first subtab cycle. Tabs that declare GetSubtabIdx +
	// SetSubtabIdx in TabSpec resolve here. Idiosyncratic tabs
	// (TabObjectDetail with its per-subtab reload pattern, TabHome
	// + TabPermParentDetail with their bespoke subtab fields) keep
	// their own arms below.
	if spec := lookupTabSpec(m.tab()); spec != nil && spec.GetSubtabIdx != nil && spec.SetSubtabIdx != nil {
		// If the cursor sits on an overflow subtab, snap to the
		// adjacent end of the pinned set rather than walking
		// further into overflow.
		cur := spec.GetSubtabIdx(m)
		if cur >= pinned && pinned > 0 {
			if delta < 0 {
				cur = pinned // wrap-back lands at first pinned
			} else {
				cur = -1 // +1 from -1 lands at 0
			}
		}
		next := wrapIdx(cur+delta, cycleLen)
		spec.SetSubtabIdx(&m, next)
		// Re-apply the chip predicate for the new subtab — chip
		// strips are per-subtab, so the active filter on
		// d.<List>.Extra needs to follow the user across the strip.
		// Without this, navigating to a fresh subtab leaves its list
		// in "no filter" mode even though the visible cursor is on
		// the project chip / a custom chip.
		if d := m.activeOrgData(); d != nil {
			m.applySelectedChipMatcher(d)
		}
		if spec.SubtabReloadOnSwitch != nil && spec.SubtabReloadOnSwitch(m, next) {
			return m, m.onTabChanged()
		}
		return m, nil
	}
	return m, nil
}

// wrapIdx cycles an index around [0, n).
func wrapIdx(i, n int) int {
	if n <= 0 {
		return 0
	}
	i = i % n
	if i < 0 {
		i += n
	}
	return i
}

// jumpRows is the delta used by ctrl+up / ctrl+down (and J/K) — a
// configurable hop sized between single-step (j/k) and half-page
// (ctrl+u/d). Reads settings.JumpRows() so the user can tune it via
// the settings modal.
func (m Model) jumpRows() int {
	if m.settings != nil {
		return m.settings.JumpRows()
	}
	return 5
}

// pageJump is the delta used by ctrl+u / ctrl+d (half-page) given the
// current terminal height. We reserve ~6 rows for chrome (header +
// dashboard + status bar) and halve what's left.
func pageJump(termHeight int) int {
	usable := termHeight - 6
	if usable < 10 {
		return 5
	}
	return usable / 2
}

// clampDelta returns cur+delta clamped to [0, n-1]. When n == 0 returns
// 0. Used for list navigation where overshooting should land on the
// edge (page-down past the last page, G to the bottom, etc.).
func clampDelta(cur, delta, n int) int {
	if n <= 0 {
		return 0
	}
	next := cur + delta
	if next < 0 {
		return 0
	}
	if next >= n {
		return n - 1
	}
	return next
}

// moveCursor routes cursor navigation to whichever structure owns the
// current view's cursor. Deltas larger than the list size clamp rather
// than no-op so the same function handles arrow keys, page jumps, and
// go-top / go-bottom (which pass huge signed deltas).
func (m Model) moveCursor(delta int) (Model, tea.Cmd) {
	if m.focus == focusOrgs {
		// The left rail can host multiple utilities. Cursor nav is per
		// utility — Orgs moves the org selection; other utilities with
		// no navigable list are a no-op (stops arrow keys from silently
		// steering the org cursor on e.g. the Bookmarks subtab).
		switch m.currentUtility().ID {
		case utilityOrgs:
			// Rail cursor walks the unified header+org row list (see
			// buildRailRows). m.selected mirrors whichever org the
			// cursor lands on; landing on a header leaves m.selected
			// alone so "current org" consumers keep working.
			changed := m.stepOrgRailCursor(delta)
			if changed {
				return m, m.onOrgChanged()
			}
			return m, nil
		case utilityBookmarks:
			// Dev Projects panel — moves cursor through the visible
			// project list. The rail only shows the first 12; cursor
			// stays in [0, 12) so it matches what the user sees.
			items := m.devProjectList.Items()
			n := len(items)
			if n > 12 {
				n = 12
			}
			m.devProjectList.MoveBy(delta)
			if m.devProjectList.Cursor() >= n && n > 0 {
				m.devProjectList.SetCursor(n - 1)
			}
			return m, nil
		}
		return m, nil
	}
	// Registry-first cursor move. listSurface entries resolve here
	// — uniform list surfaces just delegate to ListView.MoveBy via
	// the surface's MoveCursor hook. TabSpec.MoveCursor remains the
	// fallback for tabs whose cursor isn't a single ListView (e.g.
	// /reports' per-folder cursor or multi-axis drill surfaces).
	if surf := m.resolveListSurface(); surf != nil && surf.MoveCursor != nil {
		if len(m.orgs) > 0 {
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			surf.MoveCursor(d, delta)
		}
		return m, nil
	}
	// Bespoke escape hatch: per-subtab or per-tab MoveCursor closure.
	if fn := m.resolveMoveCursor(); fn != nil {
		if len(m.orgs) > 0 {
			m.ensureOrgData(m.orgs[m.selected].Username)
		}
		fn(&m, delta)
		return m, nil
	}
	return m, nil
}

// activate handles Enter on the current view. By policy Enter is for
// "drill deeper inside the TUI" — it never opens the browser. (Use
// o / ctrl+o for that.) Views that have nothing deeper to drill into
// return m,nil.
func (m Model) activate() (Model, tea.Cmd) {
	if m.focus != focusMain {
		// Rail-side Enter: when the active utility supports drill-in,
		// fire it. The Orgs panel "drills in" by handing focus back to
		// the main pane — the org switch itself happens on cursor
		// movement (moveCursor with focusOrgs re-fires onOrgChanged
		// per row), so by the time the user presses Enter the new
		// org is already loaded; Enter just snaps focus to where the
		// user can keep working.
		switch m.currentUtility().ID {
		case utilityOrgs:
			m.focus = focusMain
			// Auto-collapse the rail if the user didn't pin it open
			// with `|`. The rail was opened transiently to pick an
			// org; once picked, it should get out of the way.
			if !m.leftPinned {
				m.leftOpen = false
			}
			return m, nil
		case utilityBookmarks:
			p, ok := m.devProjectList.Selected()
			if !ok {
				return m, nil
			}
			m.setActiveDevProject(p.ID)
			m.devProjectShowAllOrgs = false
			m.reloadDevProjectItems()
			m.setTab(TabDevProjectDetail)
			m.focus = focusMain
			return m, m.onTabChanged()
		}
		return m, nil
	}
	// Registry-first drill: openSurface entries with a Drill closure
	// short-circuit here, then bespoke per-subtab/per-tab Activate
	// closures get a chance.
	if surf := m.resolveOpenSurface(); surf != nil && surf.Drill != nil {
		mm := m
		if cmd, ok := surf.Drill(&mm); ok {
			return mm, cmd
		}
		return mm, nil
	}
	if fn := m.resolveActivate(); fn != nil {
		mm := m
		return mm, fn(&mm)
	}
	return m, nil
}

// refreshCurrent forces a re-fetch of the Resources the current view
// reads from. Ignores busy state — if a refresh is in flight the
// Resource will no-op.
func (m Model) refreshCurrent() (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, m.orgsRes.Refresh(m.cache)
	}
	o := m.orgs[m.selected]
	if !canUseOrg(o) {
		return m, nil
	}
	d := m.ensureOrgData(o.Username)

	// Every tab's refresh lifecycle lives on TabSpec.RefreshData —
	// see tab_registry.go. Tabs without a RefreshData hook are
	// data-less (`r` is a no-op).
	if spec := lookupTabSpec(m.tab()); spec != nil && spec.RefreshData != nil {
		return m, spec.RefreshData(m, d)
	}
	return m, nil
}
