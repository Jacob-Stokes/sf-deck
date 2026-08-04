package ui

import tea "charm.land/bubbletea/v2"

// switchToViewIndex jumps to the Nth view in TabsForNumbers, focusing
// main. Bounds-checked so view keys bound past the end of the list are
// no-ops. Restores the user's last drill-in under that stem if any
// (pressing the Objects number from Flows returns to TabFieldDetail
// when that's where they last were, not the Objects list).
func (m Model) switchToViewIndex(i int) (tea.Model, tea.Cmd) {
	views := TabsForNumbers()
	if i < 0 || i >= len(views) {
		return m, nil
	}
	m.setTab(m.resolveStem(views[i]))
	m.focus = focusMain
	return m, m.onTabChanged()
}

// switchToSubtabIndex jumps directly to subtab i of the current tab.
// No-op when the current tab has no subtabs (or only one — the strip
// is hidden in that case anyway).
//
// Routes through the TabSpec registry's SetSubtabIdx hook for tabs
// that support shift+N nav natively. For tabs whose subtab state
// lives in bespoke fields (TabObjectDetail, TabHome, TabPermParentDetail)
// we set the index directly via the matching setter.
func (m Model) switchToSubtabIndex(i int) (tea.Model, tea.Cmd) {
	subs := m.tabSubtabs()
	if len(subs) <= 1 || i < 0 || i >= len(subs) {
		return m, nil
	}
	m.focus = focusMain

	if spec := lookupTabSpec(m.tab()); spec != nil && spec.SetSubtabIdx != nil {
		spec.SetSubtabIdx(&m, i)
		if d := m.activeOrgData(); d != nil {
			m.applySelectedChipMatcher(d)
		}
		// Subtab switch changes the active surface; restore its
		// persisted widths (see onTabChanged).
		(&m).activeListTableContext()
		if spec.SubtabReloadOnSwitch != nil && spec.SubtabReloadOnSwitch(m, i) {
			return m, m.onTabChanged()
		}
		return m, nil
	}
	return m, nil
}
