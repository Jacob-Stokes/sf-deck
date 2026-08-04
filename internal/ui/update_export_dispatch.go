package ui

// Export-flow message dispatch.
//
// Extracted from the main Update switch in update.go to keep the
// god-switch tractable. dispatchExportMsg handles every message tied
// to the export pipelines (SOQL, records, bulk, report, devproject).
// Returns handled=true when the message was matched so Update can
// short-circuit; handled=false means "not an export message, keep
// dispatching."
//
// Pattern is intended to be replicated for other feature clusters
// (modals, perms, orgs) as the update.go split continues. See
// docs/code-review-2026-05-11.md.

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
)

// dispatchExportMsg routes export-related messages. m is *Model so
// the same mutation semantics as the inline switch apply.
//
// Cases match the "export" feature surface:
//   - SOQL: openSOQLExportPathMsg, startSOQLExportMsg, soqlExportDoneMsg
//   - Records: openRecordsExportPathMsg, openRecordsExportFormatMsg,
//     startRecordsExportMsg, recordsExportDoneMsg
//   - Bulk (full records): openBulkExportPathMsg, startBulkExportMsg,
//     bulkExportDoneMsg
//   - Report: openReportExportPathMsg, openReportExportMsg,
//     reportExportDoneMsg, openReportExportSettingMsg
//   - DevProject: exportProjectFormatPickedMsg, exportProjectPathPickedMsg
func (m *Model) dispatchExportMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	// --- SOQL export -----------------------------------------------------
	case openSOQLExportPathMsg:
		return m.openSOQLExportPathPicker(msg), true
	case startSOQLExportMsg:
		m.flash(fmt.Sprintf("exporting %d rows…", len(m.soqlResult.Records)))
		return m.startSOQLExport(msg), true
	case soqlExportDoneMsg:
		m.applySOQLExportDone(msg)
		return nil, true

	// --- Records export (in-view) ---------------------------------------
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

	// --- Records export (bulk / full dataset) ---------------------------
	case bulk.OpenPathMsg:
		return bulk.OpenPathPicker(m, msg), true
	case bulk.StartMsg:
		return bulk.Start(m, msg), true
	case bulk.ProgressMsg:
		bulk.ApplyProgress(m, msg)
		// Re-arm the channel reader so the next event arrives.
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

	// --- Report export --------------------------------------------------
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

	// --- DevProject export ----------------------------------------------
	case exportProjectFormatPickedMsg:
		return m.applyExportProjectFormatPicked(msg), true
	case exportProjectPathPickedMsg:
		return m.applyExportProjectPathPicked(msg), true
	}
	return nil, false
}
