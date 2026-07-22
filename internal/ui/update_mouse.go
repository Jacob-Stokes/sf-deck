package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.lastCompositor == nil {
		return m, nil
	}
	hit := m.lastCompositor.Hit(msg.X, msg.Y)
	if hit.Empty() {
		return m, nil
	}
	id := hit.ID()

	if cursor, ok := parseZoneChipWizardRowID(id); ok {
		return m.clickChipWizardRow(cursor)
	}
	// Modal click hygiene: while any modal is open, only modal-scoped
	// zones are clickable. Swallow everything else so a click can't
	// reach (and activate) the UI underneath the overlay.
	if m.anyModalActive() {
		return m, nil
	}

	if tab, ok := parseZoneTabID(id); ok {
		views := TabsForNumbers()
		for i, view := range views {
			if view == tab {
				return m.switchToViewIndex(i)
			}
		}
		return m, nil
	}
	if i, ok := parseZoneChipID(id); ok {
		return m.selectChipIndex(i)
	}
	if i, ok := parseZoneSubtabID(id); ok {
		return m.switchToSubtabIndex(i)
	}
	if id == zoneSubtabOverflow {
		return m, m.openSubtabOverflowModal()
	}

	switch id {
	case zoneNavOrgs:
		m.focus = focusOrgs
		m.leftUtilityIdx = orgsUtilityIdx()
		m.leftOpen = true
		return m, nil
	case zoneTabOverflow:
		return m, m.openTabOverflowModal()
	case zoneNavTags:
		m.setTab(TabTags)
		m.focus = focusMain
		return m, m.onTabChanged()
	case zoneNavDevProjects:
		m.setTab(TabDevProjects)
		m.focus = focusMain
		return m, m.onTabChanged()
	case zoneNavLoadedProject:
		if scope := m.activeScope(); scope.Loaded() {
			d := m.activeOrgData()
			if d != nil && d.LoadedDevProjectID != "" {
				m.setActiveDevProject(d.LoadedDevProjectID)
				m.devProjectShowAllOrgs = false
				m.reloadDevProjectItems()
				m.setTab(TabDevProjectDetail)
				m.focus = focusMain
				return m, m.onTabChanged()
			}
		}
		return m, nil
	case zoneSidebarHide:
		// Click parity with `\`: just toggle sidebarOpen.
		m.sidebarOpen = !m.sidebarOpen
		return m, nil
	case zoneSidebarStack:
		// Click parity with `ctrl+\`: stack-toggle, auto-open if
		// currently hidden so the gesture is always visible.
		if !m.sidebarOpen {
			m.sidebarOpen = true
		}
		m.sidebarStacked = !m.sidebarStacked
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) selectChipIndex(i int) (tea.Model, tea.Cmd) {
	if i < 0 || len(m.orgs) == 0 {
		return m, nil
	}
	if surf := m.resolveChipSurface(); surf != nil {
		scope := surfaceManagerScope(*surf, m)
		strip := m.stripRows(surf.Domain, scope)
		if i >= len(strip) {
			return m, nil
		}
		if strip[i].ID == chipOverflowID {
			return m, m.openChipOverflowFor(surf.Domain, scope)
		}
		surf.SetChipIdx(&m, i)
		if d := m.activeOrgData(); d != nil {
			surf.ResetList(d)
			m.applySelectedChipMatcher(d)
		}
		return m, nil
	}
	_, sobj := m.activeRecordsSObject()
	if sobj == "" {
		return m, nil
	}
	strip := m.stripRows(domainRecords, sobj)
	if i >= len(strip) {
		return m, nil
	}
	if strip[i].ID == chipOverflowID {
		return m, m.openChipOverflowFor(domainRecords, sobj)
	}
	if d := m.activeOrgData(); d != nil {
		d.ListViewCur[sobj] = strip[i].ID
		recordsMoveCursor(d, sobj, -1<<30)
		m.applySelectedChipMatcher(d)
	}
	return m, m.onTabChanged()
}
