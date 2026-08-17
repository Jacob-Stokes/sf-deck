package ui

import tea "charm.land/bubbletea/v2"

func (m Model) switchToViewIndex(i int) (tea.Model, tea.Cmd) {
	views := TabsForNumbers()
	if i < 0 || i >= len(views) {
		return m, nil
	}
	m.setTab(m.resolveStem(views[i]))
	m.focus = focusMain
	return m, m.onTabChanged()
}

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
		(&m).activeListTableContext()
		if spec.SubtabReloadOnSwitch != nil && spec.SubtabReloadOnSwitch(m, i) {
			return m, m.onTabChanged()
		}
		return m, nil
	}
	return m, nil
}
