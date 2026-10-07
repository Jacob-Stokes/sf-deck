package ui

// Modal-flow message dispatch.

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) dispatchModalMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case recordsSearchResultMsg:
		mm := m
		(&mm).applyRecordsSearchResult(msg)
		return mm, nil, true
	case recordsDebounceTickMsg:
		mm := m
		cmd := (&mm).applyRecordsDebounceTick(msg)
		return mm, cmd, true

	case editModalResultMsg:
		mm, cmd := m.applyEditModalResult(msg)
		return mm, cmd, true
	case editModalLoadedMsg:
		mm, cmd := m.applyEditModalLoaded(msg)
		return mm, cmd, true
	case editModalPreviewMsg:
		mm, cmd := m.applyEditModalPreview(msg)
		return mm, cmd, true

	case choiceModalResultMsg:
		mm, cmd := m.applyChoiceModalResult(msg)
		return mm, cmd, true
	case choiceModalLoadedMsg:
		mm, cmd := m.applyChoiceModalLoaded(msg)
		return mm, cmd, true

	// --- Add-org flow steps ----------------------------------------------
	// Multi-step add-org flow chains modals via msg round-trips so the
	// next step is opened on the live Model (not a stale closure copy
	// from the prior modal's OnSuccessTyped). See openAddOrgChoice for
	// the why.
	case addOrgFlowStepMsg:
		mm := m
		var cmd tea.Cmd
		switch msg.Step {
		case "method_picked":
			if msg.Method == "web" {
				cmd = (&mm).openAddOrgInstanceChoice()
			} else {
				cmd = (&mm).startLoginFlow(msg.Method, "")
			}
		case "instance_picked":
			if msg.InstanceURL == "__custom__" {
				cmd = (&mm).openAddOrgCustomURLPrompt()
			} else {
				cmd = (&mm).startLoginFlow("web", msg.InstanceURL)
			}
		case "custom_url":
			cmd = (&mm).startLoginFlow("web", msg.InstanceURL)
		}
		return mm, cmd, true

	case chipWizardResultMsg:
		mm, cmd := m.applyChipWizardResult(msg)
		return mm, cmd, true
	case criterionPickedMsg:
		mm, cmd := m.applyCriterionPicked(msg)
		return mm, cmd, true
	case valuePickedMsg:
		mm, cmd := m.applyValuePicked(msg)
		return mm, cmd, true
	case chipImportDoneMsg:
		mm, cmd := m.applyChipImportDone(msg)
		return mm, cmd, true
	case chipImportListViewsReadyMsg:
		return m, m.openChipImportPicker(msg.Domain), true
	case chipImportListViewsFetchedMsg:
		mm, cmd := m.applyChipImportListViews(msg)
		return mm, cmd, true
	case chipOverflowPickedMsg:
		mm, cmd := m.applyChipOverflowPicked(msg)
		return mm, cmd, true
	case chipManagerInvokeMsg:
		return m, m.applyChipManagerInvoke(msg), true
	case chipScopeKindPickedMsg:
		mm := m
		return mm, (&mm).applyChipScopeKindPicked(msg), true
	case chipScopeChosenMsg:
		mm := m
		return mm, (&mm).applyChipScopeChosen(msg), true

	case collectItemPickedMsg:
		return m, m.applyCollectItemPicked(msg), true
	case collectItemRemovedMsg:
		return m, m.applyCollectItemRemoved(msg), true
	case collectFolderPickedMsg:
		return m, m.applyCollectFolderPicked(msg), true
	case deepCollectConfirmedMsg:
		return m, m.applyDeepCollectConfirmed(msg), true
	case deepCollectPickedMsg:
		return m, m.applyDeepCollectPicked(msg), true

	// --- Tab/subtab overflow modal --------------------------------------
	case tabOverflowPickedMsg:
		t, ok := tabByID(msg.ID)
		if !ok {
			return m, nil, true
		}
		m.setTab(m.resolveStem(t))
		m.focus = focusMain
		return m, m.onTabChanged(), true
	case subtabOverflowPickedMsg:
		// Picked overflow subtab → route through the same path
		// shift+N takes. The picker stored the FULL-list index, so
		// switchToSubtabIndex's bounds check passes and SetSubtabIdx
		// fires.
		mm, cmd := m.switchToSubtabIndex(msg.Index)
		return mm, cmd, true
	}
	return m, nil, false
}
