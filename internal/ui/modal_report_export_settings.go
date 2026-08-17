package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m *Model) openReportExportSettingsModal() tea.Cmd {
	dir := "(default ~)"
	if m.settings != nil && m.settings.UI.ReportExportDir != "" {
		dir = m.settings.UI.ReportExportDir
	}
	pattern := "(default {name}-{timestamp})"
	if m.settings != nil && m.settings.UI.ReportExportFilenamePattern != "" {
		pattern = m.settings.UI.ReportExportFilenamePattern
	}
	opts := []choiceOption{
		{Label: "Save directory", Hint: dir, Value: "dir"},
		{Label: "Filename pattern",
			Hint:  pattern + "  ·  tokens: {name} {id} {view} {file} {timestamp} {date} {time}",
			Value: "pattern"},
	}
	state := choiceModalState{
		Title:   "Report export defaults",
		Hint:    "Enter to edit  ·  Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return openReportExportSettingMsg{pick: pick}
			}
		},
	}
	return m.openChoiceModal(state)
}

type openReportExportSettingMsg struct {
	pick string
}

func (m *Model) openReportExportDirEditor() tea.Cmd {
	current := ""
	if m.settings != nil {
		current = m.settings.UI.ReportExportDir
	}
	return m.openEditModal(editModalState{
		Title:       "Report export · save directory",
		Hint:        "Enter to save  ·  Esc to cancel  ·  blank to revert to default",
		InitialBody: current,
		SuccessMsg:  "saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			m.settings.SetReportExportDir(val)
			return m.settings.Save()
		},
	})
}

func (m *Model) openReportExportPatternEditor() tea.Cmd {
	current := ""
	if m.settings != nil {
		current = m.settings.UI.ReportExportFilenamePattern
	}
	return m.openEditModal(editModalState{
		Title:       "Report export · filename pattern",
		Hint:        "tokens: {name} {id} {view} {file} {timestamp} {date} {time}  ·  Enter to save",
		InitialBody: current,
		SuccessMsg:  "saved",
		Save: func(val string, _ any) error {
			if m.settings == nil {
				return nil
			}
			m.settings.SetReportExportFilenamePattern(val)
			return m.settings.Save()
		},
	})
}
