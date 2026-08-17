package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
)

func (m *Model) dispatchExportMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case openSOQLExportPathMsg:
		return m.openSOQLExportPathPicker(msg), true
	case startSOQLExportMsg:
		m.flash(fmt.Sprintf("exporting %d rows…", len(m.soqlResult.Records)))
		return m.startSOQLExport(msg), true
	case soqlExportDoneMsg:
		m.applySOQLExportDone(msg)
		return nil, true

	case openRecordsExportPathMsg:
		return m.openRecordsExportPathPicker(msg), true
	case openRecordsExportFormatMsg:
		return m.openRecordsExportFormatPicker(msg.Label, msg.RowCount), true
	case startRecordsExportMsg:
		rows, _, _ := m.recordsExportSource()
		m.flash(fmt.Sprintf("exporting %d rows…", len(rows)))
		return m.startRecordsExport(msg), true
	case recordsExportDoneMsg:
		m.applyRecordsExportDone(msg)
		return nil, true

	case bulk.OpenPathMsg:
		return bulk.OpenPathPicker(m, msg), true
	case bulk.StartMsg:
		return bulk.Start(m, msg), true
	case bulk.ProgressMsg:
		bulk.ApplyProgress(m, msg)
		if flight := m.Flight(); flight != nil {
			return bulk.ReadCmd(flight.Events()), true
		}
		return nil, true
	case bulk.DoneMsg:
		bulk.ApplyDone(m, msg)
		return nil, true
	case bulk.CancelMsg:
		bulk.ApplyCancel(m)
		return nil, true

	case openReportExportPathMsg:
		return m.openReportExportPathPicker(msg), true
	case openReportExportMsg:
		m.flash("exporting " + msg.Name + " (" + msg.Format.View + "/" + msg.Format.File + ")…")
		return m.startReportExport(msg.ID, msg.Name, msg.Path, msg.Format, msg.OpenAfter, msg.Overwrite), true
	case reportExportDoneMsg:
		m.applyReportExportDone(msg)
		return nil, true
	case openReportExportSettingMsg:
		switch msg.pick {
		case "dir":
			return m.openReportExportDirEditor(), true
		case "pattern":
			return m.openReportExportPatternEditor(), true
		}
		return nil, true

	case exportProjectFormatPickedMsg:
		return m.applyExportProjectFormatPicked(msg), true
	case exportProjectPathPickedMsg:
		return m.applyExportProjectPathPicked(msg), true
	}
	return nil, false
}
