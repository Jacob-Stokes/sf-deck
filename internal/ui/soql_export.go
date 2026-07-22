package ui

// SOQL result export.
//
// Pressing x on the SOQL Editor with results loaded opens a two-step
// flow:
//
//   1. format picker: csv / xlsx / json
//   2. path picker: pre-populated with <export-dir>/soql-<timestamp>.<ext>
//
// The data is already in memory (m.soqlResult.Records) — there's no
// network call. The shape pass turns those records into the standard
// exporters.ExportRow form (see internal/exporters/soql) and the
// regular exporters.Write writes the file.
//
// This deliberately reuses the same export-dir + filename-pattern
// settings as report export so users have one place to configure
// "where exports land," not two.

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	exsoql "github.com/Jacob-Stokes/sf-deck/internal/exporters/soql"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
)

// triggerSOQLExport opens the format picker for the current SOQL
// result set. No-ops when the editor has no results.
func (m Model) triggerSOQLExport() (Model, tea.Cmd) {
	if len(m.soqlResult.Records) == 0 {
		m.flash("nothing to export — run a query first")
		return m, nil
	}
	opts := []choiceOption{
		{Label: "Excel (xlsx)", Hint: "spreadsheet, headers + values", Value: string(exporters.FormatXLSX)},
		{Label: "CSV", Hint: "comma-separated text, no formatting", Value: string(exporters.FormatCSV)},
		{Label: "JSON", Hint: "array of objects, keys preserved", Value: string(exporters.FormatJSON)},
	}
	state := choiceModalState{
		Title:   "Export SOQL results",
		Hint:    fmt.Sprintf("%d rows  ·  Enter to pick format  ·  Esc to cancel", len(m.soqlResult.Records)),
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			return func() tea.Msg {
				return openSOQLExportPathMsg{Format: exporters.Format(pick)}
			}
		},
	}
	return m, m.openChoiceModal(state)
}

// openSOQLExportPathMsg arrives after the user picks a format. The
// reducer in update.go routes it to openSOQLExportPathPicker.
type openSOQLExportPathMsg struct {
	Format exporters.Format
}

// openSOQLExportPathPicker opens the 2-field save modal pre-populated
// with <export-dir>/soql-<timestamp>.<ext> and the auto-open
// checkbox on by default. On confirm, fires startSOQLExport which
// writes the file.
func (m *Model) openSOQLExportPathPicker(msg openSOQLExportPathMsg) tea.Cmd {
	defaultPath := m.defaultSOQLExportPath(msg.Format)
	format := msg.Format
	state := exportSaveState{
		Title:         "Save SOQL results · " + msg.Format.Label(),
		Path:          defaultPath,
		OpenAfter:     true,
		ShowOpenAfter: true,
		Confirm: func(path string, openAfter bool, overwrite bool) tea.Cmd {
			return func() tea.Msg {
				return startSOQLExportMsg{
					Format:    format,
					Path:      path,
					OpenAfter: openAfter,
					Overwrite: overwrite,
				}
			}
		},
	}
	return m.openExportSaveModal(state)
}

// startSOQLExportMsg lands on the main loop after both format and
// path are confirmed. The reducer calls startSOQLExport.
type startSOQLExportMsg struct {
	Format    exporters.Format
	Path      string
	OpenAfter bool
	Overwrite bool
}

// startSOQLExport runs the export pipeline against the current
// in-memory result set. Synchronous-feeling but goroutine-backed via
// tea.Cmd so a giant write doesn't block the TUI; in practice the
// data is already loaded and the write is fast (<100ms for typical
// result sets).
func (m *Model) startSOQLExport(msg startSOQLExportMsg) tea.Cmd {
	records := m.soqlResult.Records
	cols := collectColumns(records, m.soqlInput.Value()) // SELECT-order columns
	format := msg.Format
	savePath := expandTilde(msg.Path)
	openAfter := msg.OpenAfter
	overwrite := msg.Overwrite

	return func() tea.Msg {
		headers, rows := exsoql.Shape(records, cols)
		if err := securefile.Write(savePath, overwrite, func(w io.Writer) error {
			return exporters.Write(w, format, headers, rows, "SOQL Results")
		}); err != nil {
			return soqlExportDoneMsg{OpenAfter: openAfter, Err: fmt.Errorf("write %s: %w", format, err)}
		}
		return soqlExportDoneMsg{Path: savePath, OpenAfter: openAfter}
	}
}

// soqlExportDoneMsg lands on the main loop after the export runs.
// Path is empty + Err non-nil on failure; otherwise Path is the
// absolute saved file.
type soqlExportDoneMsg struct {
	Path      string
	Err       error
	OpenAfter bool
}

// applySOQLExportDone folds the result message into the model — a
// short flash on success, a longer one on failure so the path /
// error stays readable. Fires openPath in the background when the
// user asked for auto-open.
func (m *Model) applySOQLExportDone(msg soqlExportDoneMsg) {
	if msg.Err != nil {
		m.flashFor("export failed: "+msg.Err.Error(), 8*time.Second)
		applog.Error("soql.export.failed", map[string]any{"err": msg.Err.Error()})
		return
	}
	m.flash("saved → " + filepath.Base(msg.Path))
	applog.Info("soql.export.saved", map[string]any{
		"path":       msg.Path,
		"open_after": msg.OpenAfter,
	})
	if msg.OpenAfter && msg.Path != "" {
		go func(p string) {
			if err := openPath(p); err != nil {
				applog.Error("soql.export.auto_open_failed", map[string]any{
					"path": p, "err": err.Error(),
				})
			}
		}(msg.Path)
	}
}

// defaultSOQLExportPath resolves the default save path for a SOQL
// export. Reuses the same export-dir setting as report export so
// users only configure "where exports land" once. Filename is a
// fixed pattern — the renderer doesn't have a name like a report
// does, so we use "soql-<timestamp>".
func (m Model) defaultSOQLExportPath(format exporters.Format) string {
	dir := expandTilde(m.settings.ReportExportDir())
	ts := time.Now().Format("20060102-150405")
	fname := "soql-" + ts + format.Extension()
	return filepath.Join(dir, fname)
}
