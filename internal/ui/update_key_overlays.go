package ui

import tea "charm.land/bubbletea/v2"

func (m Model) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if m.commandPalette != nil {
		mm := m
		handled, cmd := mm.handleCommandPaletteKey(msg)
		if handled {
			return mm, cmd, true
		}
	}
	if m.keybindingsModal != nil {
		mm := m
		handled, cmd := mm.handleKeybindingsModalKey(msg)
		if handled {
			return mm, cmd, true
		}
	}
	if m.picker != nil {
		next, cmd := m.handlePickerKey(msg)
		return next, cmd, true
	}
	if m.themePicker != nil {
		next, cmd := m.handleThemePickerKey(msg)
		return next, cmd, true
	}
	if m.exportSave != nil {
		next, cmd := m.handleExportSaveKey(msg)
		return next, cmd, true
	}
	if m.editModal != nil {
		next, cmd := m.handleEditModalKey(msg)
		return next, cmd, true
	}
	if m.soqlModal != nil {
		next, cmd := m.handleSOQLModalKey(msg)
		return next, cmd, true
	}
	if m.cacheSettings != nil {
		next, cmd := m.handleCacheSettingsKey(msg)
		return next, cmd, true
	}
	if m.chipWizard != nil && m.choiceModal == nil && m.orgPicker == nil {
		next, cmd := m.handleChipWizardKey(msg)
		return next, cmd, true
	}
	if m.openMenu != nil {
		next, cmd := m.handleOpenMenuKey(msg)
		return next, cmd, true
	}
	if m.orgPicker != nil {
		next, cmd := m.handleOrgPickerKey(msg)
		return next, cmd, true
	}
	if m.deepCollect != nil {
		next, cmd := m.handleDeepCollectKey(msg)
		return next, cmd, true
	}
	if m.choiceModal != nil {
		next, cmd := m.handleChoiceModalKey(msg)
		return next, cmd, true
	}
	if m.compareScope != nil {
		next, cmd := m.handleCompareScopeKey(msg)
		return next, cmd, true
	}
	if m.compareEdit != nil {
		next, cmd := m.handleCompareEditKey(msg)
		return next, cmd, true
	}
	if m.orgManageModal != nil {
		mm := m
		handled, cmd := mm.handleOrgManageModalKey(msg)
		if handled {
			return mm, cmd, true
		}
	}
	if m.tagPicker != nil {
		next, cmd := m.updateTagPicker(msg)
		return next, cmd, true
	}
	if m.tagEditor != nil {
		next, cmd := m.updateTagEditor(msg)
		return next, cmd, true
	}
	if m.globalSearch != nil {
		next, cmd := m.handleGlobalSearchKey(msg)
		return next, cmd, true
	}
	if m.downloadsModal != nil {
		next, cmd := m.handleDownloadsModalKey(msg)
		return next, cmd, true
	}
	if m.infoModal != nil {
		next, cmd := m.handleInfoModalKey(msg)
		return next, cmd, true
	}
	return m, nil, false
}
