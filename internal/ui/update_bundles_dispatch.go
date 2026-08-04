package ui

// Bundle + dev-project message dispatch.
//
// Pairs with the other update_*_dispatch.go files. Handles retrieve /
// deploy / preview callbacks for the in-TUI bundle workflow, plus the
// dev-project list change notification that fires when items are
// added/removed via K-collect.

import (
	tea "charm.land/bubbletea/v2"
)

// dispatchBundlesMsg routes bundle + dev-project callbacks.
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
