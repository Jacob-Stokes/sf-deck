package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
)

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
	if surf := m.resolveChipSurface(); surf != nil {
		if len(m.orgs) == 0 {
			return m, nil
		}
		d := m.ensureOrgData(m.orgs[m.selected].Username)
		m.cycleSimpleChipStrip(delta, d, surf.Registry(&m),
			func() int { return surf.ChipIdx(m) },
			func(i int) { surf.SetChipIdx(&m, i) },
			func() { surf.ResetList(d) })
		return m, m.ensureDataFor(m.tab())
	}
	if fn := m.resolveCycleChip(); fn != nil {
		mm := m
		return mm, fn(&mm, delta)
	}
	return m, nil
}

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
	curIdx := idx()
	curID := ""
	if curIdx >= 0 && curIdx < len(strip) {
		curID = strip[curIdx].ID
	}
	cur := findChipIndex(navStrip, curID)
	cur = wrapIdx(cur+delta, len(navStrip))
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

func (m Model) jumpRows() int {
	if m.settings != nil {
		return m.settings.JumpRows()
	}
	return 5
}

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
		switch m.currentUtility().ID {
		case utilityOrgs:
			changed := m.stepOrgRailCursor(delta)
			if changed {
				return m, m.onOrgChanged()
			}
			return m, nil
		case utilityBookmarks:
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
	if surf := m.resolveListSurface(); surf != nil && surf.MoveCursor != nil {
		if len(m.orgs) > 0 {
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			surf.MoveCursor(d, delta)
		}
		return m, nil
	}
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
		switch m.currentUtility().ID {
		case utilityOrgs:
			m.focus = focusMain
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

func (m Model) refreshCurrent() (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, m.orgsRes.Refresh(m.cache)
	}
	o := m.orgs[m.selected]
	if !canUseOrg(o) {
		return m, nil
	}
	d := m.ensureOrgData(o.Username)

	if spec := lookupTabSpec(m.tab()); spec != nil && spec.RefreshData != nil {
		return m, spec.RefreshData(m, d)
	}
	return m, nil
}
