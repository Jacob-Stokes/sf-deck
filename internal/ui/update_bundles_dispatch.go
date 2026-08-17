package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) dispatchBundlesMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case projectRetrieveDoneMsg:
		m.applyProjectRetrieveDone(msg)
		return m, nil, true

	case bundlePreviewLoadedMsg:
		m.applyBundlePreviewLoaded(msg)
		return m, nil, true

	case bundleOpDoneMsg:
		return m, m.applyBundleOpDone(msg), true

	case bundleTargetPickedMsg:
		return m, m.applyBundleTargetPicked(msg), true

	case devProjectsChangedMsg:
		_ = msg
		m.reloadDevProjects()
		if m.tab() == TabDevProjectDetail {
			m.reloadDevProjectItems()
		}
		return m, nil, true
	}
	return m, nil, false
}
