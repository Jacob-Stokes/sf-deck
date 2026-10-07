package ui

// SOQL result export.

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

type openSOQLExportPathMsg struct {
	Format exporters.Format
}

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

type startSOQLExportMsg struct {
	Format    exporters.Format
	Path      string
	OpenAfter bool
	Overwrite bool
}

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

func (m Model) defaultSOQLExportPath(format exporters.Format) string {
	dir := expandTilde(m.settings.ReportExportDir())
	ts := time.Now().Format("20060102-150405")
	fname := "soql-" + ts + format.Extension()
	return filepath.Join(dir, fname)
}
