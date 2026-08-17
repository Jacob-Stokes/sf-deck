package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/postprocess"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type reportExportDoneMsg struct {
	JobID     string
	Path      string
	Err       error
	OpenAfter bool
}

type exportActivityTickMsg struct{}

const exportActivityTickInterval = 500 * time.Millisecond

func (m *Model) exportActivityTickCmd() tea.Cmd {
	if m.exportTickRunning {
		return nil
	}
	if m.exports == nil || !m.exports.hasInflight() {
		return nil
	}
	m.exportTickRunning = true
	return tea.Tick(exportActivityTickInterval, func(time.Time) tea.Msg {
		return exportActivityTickMsg{}
	})
}

func (m Model) triggerReportExport() (Model, tea.Cmd) {
	if len(m.orgs) == 0 {
		return m, nil
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return m, nil
	}
	var id, name string
	switch m.tab() {
	case TabReportDetail:
		if d.ReportCur == "" {
			return m, nil
		}
		id = d.ReportCur
		for _, r := range d.Reports.Value() {
			if r.ID == id {
				name = r.Name
				break
			}
		}
	case TabReports:
		subs, reps := m.visibleReportsItems()
		row := m.reportsRowCursor()
		if row < len(subs) {
			return m, nil
		}
		idx := row - len(subs)
		if idx < 0 || idx >= len(reps) {
			return m, nil
		}
		r := reps[idx]
		id = r.ID
		name = r.Name
	}
	if id == "" {
		return m, nil
	}

	opts := []choiceOption{
		{Label: "Formatted Report · xlsx",
			Hint:  "report header, groupings, filter settings (Excel)",
			Value: "formatted/xlsx"},
		{Label: "Details Only · xlsx",
			Hint:  "detail rows only, ready for further calculations (Excel)",
			Value: "details/xlsx"},
		{Label: "Formatted Report · csv",
			Hint:  "report header + groupings as comma-separated text",
			Value: "formatted/csv"},
		{Label: "Details Only · csv",
			Hint:  "detail rows as comma-separated text",
			Value: "details/csv"},
	}
	state := choiceModalState{
		Title:   "Export · " + name,
		Hint:    "Enter to pick format  ·  Esc to cancel",
		Options: opts,
		Cursor:  0,
		OnSuccessTyped: func(val any) tea.Cmd {
			pick, _ := val.(string)
			parts := strings.SplitN(pick, "/", 2)
			if len(parts) != 2 {
				applog.Error("export.format_pick", map[string]any{"pick": pick})
				return nil
			}
			format := sf.ReportExportFormat{View: parts[0], File: parts[1]}
			applog.Info("export.format_picked", map[string]any{
				"id":   id,
				"view": parts[0],
				"file": parts[1],
			})
			return func() tea.Msg {
				return openReportExportPathMsg{
					ID:     id,
					Name:   name,
					Format: format,
				}
			}
		},
	}
	return m, m.openChoiceModal(state)
}

type openReportExportPathMsg struct {
	ID     string
	Name   string
	Format sf.ReportExportFormat
}

func (m *Model) openReportExportPathPicker(msg openReportExportPathMsg) tea.Cmd {
	defaultPath := m.defaultExportPath(msg.ID, msg.Name, msg.Format)
	applog.Info("export.path_picker.open", map[string]any{
		"id":      msg.ID,
		"view":    msg.Format.View,
		"file":    msg.Format.File,
		"prefill": defaultPath,
	})
	id := msg.ID
	name := msg.Name
	format := msg.Format
	state := exportSaveState{
		Title:         "Save export · " + msg.Name + " (" + msg.Format.View + "/" + msg.Format.File + ")",
		Path:          defaultPath,
		OpenAfter:     true,
		ShowOpenAfter: true,
		Confirm: func(path string, openAfter bool, overwrite bool) tea.Cmd {
			applog.Info("export.path_picker.confirmed", map[string]any{
				"path":       path,
				"open_after": openAfter,
			})
			return func() tea.Msg {
				return openReportExportMsg{
					ID:        id,
					Name:      name,
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

func (m Model) defaultExportPath(reportID, reportName string, format sf.ReportExportFormat) string {
	dir := expandTilde(m.settings.ReportExportDir())
	pattern := m.settings.ReportExportFilenamePattern()
	fname := renderFilename(pattern, reportName, reportID, format) + "." + format.Ext()
	return filepath.Join(dir, fname)
}

type openReportExportMsg struct {
	ID        string
	Name      string
	Format    sf.ReportExportFormat
	Path      string
	OpenAfter bool
	Overwrite bool
}

// startReportExport kicks off the export pipeline for the given
// report with the chosen format and save path.
//
// SF's REST API only returns xlsx in Formatted layout, so we always
// download that. From there:
//   - Formatted · xlsx → URL post-processor only
//   - Details Only · xlsx → Detailsify (strip preamble + groupings) + URL pp
//   - Formatted · csv → URL pp (xlsx-internal) + xlsx→csv conversion
//   - Details Only · csv → Detailsify + xlsx→csv conversion
//
// All four work via Bearer auth at full row count (up to 100k
// rows). Detailsify is heuristic — see internal/postprocess/detailsify.go
// for what it strips and forward-fills.
//
// Returns two cmds via tea.Batch: the worker goroutine that runs the
// pipeline, and a kick to start the activity tick if nothing was
// running before (so the status bar's animated ellipsis updates while
// this export is in flight).
func (m *Model) startReportExport(reportID, reportName, savePath string, format sf.ReportExportFormat, openAfter, overwrite bool) tea.Cmd {
	if reportID == "" || len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	alias := targetArg(o)
	savePath = expandTilde(savePath)

	var transforms []postprocess.Transform
	if format.View == "details" {
		transforms = append(transforms,
			&postprocess.DetailsifyTransform{},
			&postprocess.StripSummaryTransform{},
			&postprocess.StripFormattingTransform{},
		)
	}
	if format.File == "xlsx" {
		transformIDs := m.settings.ReportExportTransforms(reportID)
		if transformIDs == nil {
			transformIDs = []string{"url"}
		}
		transforms = append(transforms, postprocess.ByIDs(transformIDs)...)
	}

	d := m.data[o.Username]
	prefixMap := buildPrefixMap(d)
	instance := orgInstanceURL(o)

	reg := m.exports
	job := reg.startJob(exportKindReport, reportName, alias, savePath, format.View+"/"+format.File)
	jobID := job.ID

	worker := func() tea.Msg {
		reg.setPhase(jobID, exportPhaseDownloading)
		raw, err := sf.ExportReport(alias, reportID, format)
		if err != nil {
			return reportExportDoneMsg{JobID: jobID, OpenAfter: openAfter, Err: fmt.Errorf("download: %w", err)}
		}
		out := raw
		if len(transforms) > 0 {
			reg.setPhase(jobID, exportPhasePostProcess)
			ctx := postprocess.Context{
				InstanceURL:     instance,
				PrefixToSObject: prefixMap,
				ReportName:      reportName,
			}
			out, err = postprocess.Run(raw, transforms, ctx)
			if err != nil {
				return reportExportDoneMsg{JobID: jobID, OpenAfter: openAfter, Err: fmt.Errorf("post-process: %w", err)}
			}
		}
		if format.File == "csv" {
			reg.setPhase(jobID, exportPhaseConverting)
			out, err = postprocess.ToCSV(out)
			if err != nil {
				return reportExportDoneMsg{JobID: jobID, OpenAfter: openAfter, Err: fmt.Errorf("xlsx→csv: %w", err)}
			}
		}
		reg.setPhase(jobID, exportPhaseWriting)
		if err := securefile.WriteFile(savePath, out, overwrite); err != nil {
			return reportExportDoneMsg{JobID: jobID, OpenAfter: openAfter, Err: fmt.Errorf("write %s: %w", savePath, err)}
		}
		return reportExportDoneMsg{JobID: jobID, Path: savePath, OpenAfter: openAfter}
	}
	return tea.Batch(worker, m.exportActivityTickCmd())
}

// applyReportExportDone folds the export-result message into the
// model. On success we flash the saved path AND remember it so the
// user can hit `o` to open the file in their default app. The path
// stays on the model until the next export overwrites it.
//
// Also moves the in-flight job to history (success or failure). Even
// failures get persisted so the downloads modal can show "you tried
// X and it failed because Y" rather than silently swallowing them.
func (m *Model) applyReportExportDone(msg reportExportDoneMsg) {
	if msg.Err != nil {
		errStr := msg.Err.Error()
		if msg.JobID != "" && m.exports != nil {
			m.exports.markFailed(msg.JobID, msg.Err)
		}
		if strings.Contains(errStr, "identity verification") {
			m.flashFor("export blocked: "+errStr, 15*time.Second)
		} else {
			m.flashFor("export failed: "+errStr, 8*time.Second)
		}
		applog.Error("export.failed", map[string]any{"err": errStr})
		return
	}
	if msg.JobID != "" && m.exports != nil {
		m.exports.markDone(msg.JobID, msg.Path)
	}
	m.flash("saved → " + filepath.Base(msg.Path))
	applog.Info("export.saved", map[string]any{
		"path":       msg.Path,
		"open_after": msg.OpenAfter,
	})
	if msg.OpenAfter && msg.Path != "" {
		go func(p string) {
			if err := openPath(p); err != nil {
				applog.Error("export.auto_open_failed", map[string]any{
					"path": p, "err": err.Error(),
				})
			}
		}(msg.Path)
	}
}

func orgInstanceURL(o sf.Org) string {
	u := strings.TrimRight(o.InstanceURL, "/")
	return u
}

func buildPrefixMap(d *orgData) map[string]string {
	if d == nil {
		return nil
	}
	objs := d.SObjects.Value()
	if len(objs) == 0 {
		return nil
	}
	out := make(map[string]string, len(objs))
	for _, s := range objs {
		if len(s.KeyPrefix) == 3 {
			if _, exists := out[s.KeyPrefix]; !exists {
				out[s.KeyPrefix] = s.Name
			}
		}
	}
	return out
}

var safeFilenameRE = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func renderFilename(pattern, name, id string, format sf.ReportExportFormat) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = "{name}-{timestamp}"
	}
	now := time.Now()
	slug := safeFilenameRE.ReplaceAllString(strings.TrimSpace(name), "_")
	slug = strings.Trim(slug, "_")
	// id is the report Id from the org's API response. Sanitise it the
	// same way as the name — a malicious org could return an Id like
	// "../../x" which would otherwise survive into filepath.Join and
	// escape the export dir, both via the empty-slug fallback below and
	// via the {id} token.
	safeID := strings.Trim(safeFilenameRE.ReplaceAllString(id, "_"), "_")
	if slug == "" {
		slug = safeID
	}
	r := strings.NewReplacer(
		"{name}", slug,
		"{id}", safeID,
		"{view}", format.View,
		"{file}", format.File,
		"{timestamp}", now.Format("20060102-150405"),
		"{date}", now.Format("2006-01-02"),
		"{time}", now.Format("150405"),
	)
	return r.Replace(pattern)
}

func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
