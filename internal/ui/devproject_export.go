package ui

// DevProject export — the `e` key on /dev-projects (or Dev Project
// Detail) triggers a two-step modal: pick format → pick save path →
// write file. Mirrors the report-export flow shape.
//
// Format-specific writing happens in internal/exporters; this file
// is just the TUI glue (modals, key wiring, file I/O, flash) plus
// the URL resolver that knows how to build a Lightning URL from an
// Item + the org it came from.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/exporters"
	dpexport "github.com/Jacob-Stokes/sf-deck/internal/exporters/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// triggerExportProject is the `e` key handler on /dev-projects (and
// on the rail's Dev Projects panel). Opens the format-picker modal;
// the save-path picker chains after format is chosen.
//
// On TabDevProjectDetail it exports the drilled-in project's items
// using the active scope (this org / all orgs per
// devProjectShowAllOrgs). On TabDevProjects (master list) it exports
// the cursored project, defaulting to "this org" scope.
func (m *Model) triggerExportProject() tea.Cmd {
	if m.devProjects == nil {
		m.flash("dev-projects unavailable")
		return nil
	}
	dpID, dpName, ok := m.exportProjectSelection()
	if !ok {
		m.flash("no project selected")
		return nil
	}
	// Default scope: when on the detail tab we honour the user's
	// current scope toggle (so what they see is what they export).
	// Elsewhere we default to "this org" — the most common use.
	scopeAllOrgs := false
	if m.tab() == TabDevProjectDetail {
		scopeAllOrgs = m.devProjectShowAllOrgs
	}
	return m.openExportFormatPicker(dpID, dpName, scopeAllOrgs)
}

// exportProjectSelection figures out which DevProject the export is
// scoped to. On the detail tab it's the drilled-in project; on the
// master list it's the cursored row; on the rail's Dev Projects
// panel it's also the cursored row in that list.
func (m Model) exportProjectSelection() (id, name string, ok bool) {
	switch m.tab() {
	case TabDevProjectDetail:
		if m.devProjectCur == "" {
			return "", "", false
		}
		if dp, found := m.devProjectByID(m.devProjectCur); found {
			return dp.ID, dp.Name, true
		}
		return m.devProjectCur, m.devProjectCur, true
	}
	if p, found := m.devProjectList.Selected(); found {
		return p.ID, p.Name, true
	}
	return "", "", false
}

// openExportFormatPicker is step 1: pick CSV / Excel / JSON / Manifest.
func (m *Model) openExportFormatPicker(dpID, dpName string, scopeAllOrgs bool) tea.Cmd {
	formats := exporters.AllFormats()
	opts := make([]choiceOption, 0, len(formats))
	for _, f := range formats {
		hint := ""
		switch f {
		case exporters.FormatPackageXML:
			hint = "package.xml + records.csv + README — drop into an existing project"
		case exporters.FormatSfdxProject:
			hint = "package.xml + sfdx-project.json + force-app/ skeleton (you cd in and retrieve)"
		case exporters.FormatSfdxProjectRetrieve:
			hint = "full sfdx project + runs `sf project retrieve` for you (30-120s)"
		default:
			if ext := f.Extension(); len(ext) > 1 {
				hint = "writes a " + ext[1:] + " file"
			}
		}
		opts = append(opts, choiceOption{
			Label: f.Label(),
			Hint:  hint,
			Value: string(f),
		})
	}
	state := choiceModalState{
		Title:   "Bundle / export · " + dpName,
		Hint:    "Pick a format · Enter to continue · Esc to cancel",
		Options: opts,
		OnSuccessTyped: func(val any) tea.Cmd {
			s, _ := val.(string)
			return func() tea.Msg {
				return exportProjectFormatPickedMsg{
					DevID:        dpID,
					DevName:      dpName,
					Format:       exporters.Format(s),
					ScopeAllOrgs: scopeAllOrgs,
				}
			}
		},
	}
	return m.openChoiceModal(state)
}

// exportProjectFormatPickedMsg flows into Update from the format
// picker; Update opens the path picker as step 2.
type exportProjectFormatPickedMsg struct {
	DevID, DevName string
	Format         exporters.Format
	ScopeAllOrgs   bool
}

// applyExportProjectFormatPicked is the Update-side handler — opens
// the path-picker edit modal pre-populated with a sensible default
// path. Confirm fires the actual write.
//
// File formats default to a single-file path with the extension
// matching the format. Bundle formats (package.xml) default to a
// directory path; the writer creates the directory and drops
// package.xml + sidecars inside.
func (m *Model) applyExportProjectFormatPicked(msg exportProjectFormatPickedMsg) tea.Cmd {
	// FormatSfdxProjectRetrieve is the only format that can
	// "update an existing thing" — the bundle stays on disk between
	// runs. When the project has existing bundles, ask the user
	// whether to overwrite one or create a new directory. Other
	// formats always create a fresh artifact, so skip the prompt.
	if msg.Format == exporters.FormatSfdxProjectRetrieve && m.devProjects != nil {
		bundles, err := m.devProjects.ListBundlesFor(msg.DevID)
		if err == nil && len(bundles) > 0 {
			return m.openBundleTargetPicker(msg, bundles)
		}
	}
	return m.openExportPathPicker(msg, "")
}

// openBundleTargetPicker asks the user whether to update one of the
// existing linked bundles or create a fresh one. Each bundle row
// shows path + last-retrieved age so the user can pick the right
// target. Last option is always "Create a new bundle".
func (m *Model) openBundleTargetPicker(msg exportProjectFormatPickedMsg, bundles []devproject.Bundle) tea.Cmd {
	opts := make([]choiceOption, 0, len(bundles)+1)
	for _, b := range bundles {
		ageHint := "never used"
		if !b.LastRetrievedAt.IsZero() {
			ageHint = "last retrieved " + humanTimeAgoBundle(b.LastRetrievedAt)
		}
		if b.Stale() {
			ageHint = "[stale] " + ageHint
		}
		opts = append(opts, choiceOption{
			Label: "Update " + b.Path,
			Hint:  ageHint,
			Value: b.ID,
		})
	}
	opts = append(opts, choiceOption{
		Label: "Create a new bundle",
		Hint:  "writes to a fresh timestamped directory",
		Value: "",
	})
	state := choiceModalState{
		Title:   "Bundle target · " + msg.DevName,
		Hint:    "Pick a destination · Enter to continue · Esc to cancel",
		Options: opts,
		OnSuccessTyped: func(val any) tea.Cmd {
			id, _ := val.(string)
			return func() tea.Msg {
				return bundleTargetPickedMsg{
					Format:   msg,
					BundleID: id, // "" = create new
				}
			}
		},
	}
	return m.openChoiceModal(state)
}

// bundleTargetPickedMsg carries the user's bundle-target choice from
// the picker through Update back into the export flow.
type bundleTargetPickedMsg struct {
	Format   exportProjectFormatPickedMsg
	BundleID string // "" = create new
}

// applyBundleTargetPicked dispatches to either "update an existing
// bundle" (skip path picker, reuse the bundle's path) or "create
// new" (open the path picker as usual).
func (m *Model) applyBundleTargetPicked(msg bundleTargetPickedMsg) tea.Cmd {
	if msg.BundleID == "" {
		return m.openExportPathPicker(msg.Format, "")
	}
	if m.devProjects == nil {
		return nil
	}
	b, err := m.devProjects.GetBundle(msg.BundleID)
	if err != nil {
		m.flash("bundle lookup failed: " + err.Error())
		return m.openExportPathPicker(msg.Format, "")
	}
	// Path is fixed by the existing bundle — skip the path picker
	// entirely and go straight to the write+retrieve flow.
	return func() tea.Msg {
		return exportProjectPathPickedMsg{
			DevID:        msg.Format.DevID,
			DevName:      msg.Format.DevName,
			Format:       msg.Format.Format,
			Path:         b.Path,
			ScopeAllOrgs: msg.Format.ScopeAllOrgs,
			BundleID:     b.ID,
		}
	}
}

// openExportPathPicker is the path-picker step that runs after the
// (optional) bundle-target picker for retrieve formats. Pulled out
// so both paths (no existing bundles, or "create new") share one
// implementation.
//
// presetBundleID, when non-empty, links the new write to that
// bundle row in the registry so the path stays in sync. "" creates
// a fresh row after the write succeeds.
func (m *Model) openExportPathPicker(msg exportProjectFormatPickedMsg, presetBundleID string) tea.Cmd {
	defaultPath := m.defaultDevProjectExportPath(msg.DevName, msg.Format)
	id := msg.DevID
	name := msg.DevName
	format := msg.Format
	scopeAllOrgs := msg.ScopeAllOrgs
	if !format.IsBundle() {
		state := exportSaveState{
			Title: "Save as · " + msg.DevName + " (" + msg.Format.Label() + ")",
			Path:  defaultPath,
			Confirm: func(path string, _ bool, overwrite bool) tea.Cmd {
				return func() tea.Msg {
					return exportProjectPathPickedMsg{
						DevID:        id,
						DevName:      name,
						Format:       format,
						Path:         path,
						ScopeAllOrgs: scopeAllOrgs,
						BundleID:     presetBundleID,
						Overwrite:    overwrite,
					}
				}
			},
		}
		return m.openExportSaveModal(state)
	}
	var savedPath string
	state := editModalState{
		Title:       "Save as · " + msg.DevName + " (" + msg.Format.Label() + ")",
		Hint:        "Edit destination directory · Enter to save · Esc to cancel",
		InitialBody: defaultPath,
		Save: func(val string, _ any) error {
			savedPath = strings.TrimSpace(val)
			if savedPath == "" {
				return fmt.Errorf("path required")
			}
			return nil
		},
		OnSuccess: func() tea.Cmd {
			return func() tea.Msg {
				return exportProjectPathPickedMsg{
					DevID:        id,
					DevName:      name,
					Format:       format,
					Path:         savedPath,
					ScopeAllOrgs: scopeAllOrgs,
					BundleID:     presetBundleID,
				}
			}
		},
	}
	return m.openEditModal(state)
}

// exportProjectPathPickedMsg flows into Update once both format and
// path are confirmed. Update calls writeDevProjectExport.
//
// BundleID, when non-empty, identifies an existing bundle the export
// should overwrite (path is reused, last_retrieved_at is bumped).
// "" creates a fresh bundle row after the write — the typical
// "first export" path.
type exportProjectPathPickedMsg struct {
	DevID, DevName string
	Format         exporters.Format
	Path           string
	ScopeAllOrgs   bool
	BundleID       string
	Overwrite      bool
}

// applyExportProjectPathPicked writes the export file (or bundle).
// Hits the dev-project store for items, then dispatches by format.
// Single-file formats route through exporters.Write; bundle formats
// (package.xml) get their own writer + sidecars. Flash on
// success / error.
//
// Tracked via the export registry the same way report exports are,
// so Ctrl+J / /home Downloads surface project + manifest writes
// alongside report runs. Bundle formats register kind=manifest;
// single-file formats register kind=project.
func (m *Model) applyExportProjectPathPicked(msg exportProjectPathPickedMsg) tea.Cmd {
	if m.devProjects == nil {
		return nil
	}
	msg.Path = expandTilde(msg.Path)
	// Reconcile BEFORE reading items so the exported manifest never
	// carries duplicate or stale rows — export reads the store directly,
	// not the (filtered) UI list, so this is the last line of defence
	// against a bad package.xml.
	m.reconcileDevProject(msg.DevID)
	orgFilter := ""
	if !msg.ScopeAllOrgs && len(m.orgs) > 0 {
		orgFilter = m.orgs[m.selected].Username
	}
	items, err := m.devProjects.ListItems(msg.DevID, orgFilter)
	if err != nil {
		m.flash("export: " + err.Error())
		return nil
	}
	if len(items) == 0 {
		m.flash("nothing to export — project has no items in this scope")
		return nil
	}

	if msg.Format.IsBundle() {
		return m.exportProjectBundle(msg, items, orgFilter)
	}

	// Register the in-flight job up front so the activity indicator
	// shows even though this path runs synchronously — at minimum,
	// the modal + /home Downloads will see "writing → done" once the
	// caller refreshes. Done immediately on success/failure since
	// we're not on a goroutine.
	job := m.exports.startJob(exportKindProject, msg.DevName, orgFilter, msg.Path, string(msg.Format))
	m.exports.setPhase(job.ID, exportPhaseWriting)

	// Single-file formats: csv / xlsx / json. Build rows, open the
	// destination file, dispatch to the format writer.
	resolver := m.itemURLResolver()
	rows := dpexport.Rows(items, resolver)

	if err := securefile.Write(msg.Path, msg.Overwrite, func(w io.Writer) error {
		return exporters.Write(w, msg.Format, dpexport.Headers, rows, msg.DevName)
	}); err != nil {
		m.exports.markFailed(job.ID, err)
		m.flash("export: " + err.Error())
		return nil
	}
	m.exports.markDone(job.ID, msg.Path)
	applog.Info("devproject.export", map[string]any{
		"project":  msg.DevID,
		"format":   string(msg.Format),
		"path":     msg.Path,
		"items":    len(items),
		"all_orgs": msg.ScopeAllOrgs,
	})
	scope := "this org"
	if msg.ScopeAllOrgs {
		scope = "all orgs"
	}
	m.flash(fmt.Sprintf("exported %d items (%s) → %s", len(items), scope, msg.Path))
	return nil
}

// exportProjectBundle writes the package.xml manifest bundle: a
// directory containing package.xml, optionally records.csv (when
// the project has KindRecord items), and a README.md describing
// what's there + how to use it.
//
// When the chosen format is FormatSfdxProject we additionally write
// sfdx-project.json + an empty force-app/main/default/ directory so
// the bundle is a self-contained sfdx project the user can `cd` into
// and run sf commands against immediately.
//
// Records sidecar uses the same csv writer the standard ExportRow
// path uses — keeps the records data shape consistent with
// "export DevProject as CSV" (so users with their own spreadsheet
// tooling can ingest it the same way).
func (m *Model) exportProjectBundle(msg exportProjectPathPickedMsg, items []devproject.Item, orgFilter string) tea.Cmd {
	// A fresh export must never silently truncate an existing project.
	// BundleID is populated only after the user explicitly chose "Update"
	// for a registered bundle, which is the TUI equivalent of --force.
	if msg.BundleID == "" {
		if err := bundles.ValidateCreateDestination(msg.Path, false); err != nil {
			m.flashFor("export refused: "+err.Error(), 8*time.Second)
			return nil
		}
	}
	job := m.exports.startJob(exportKindManifest, msg.DevName, orgFilter, msg.Path, string(msg.Format))
	m.exports.setPhase(job.ID, exportPhaseWriting)

	if err := os.MkdirAll(msg.Path, 0o755); err != nil {
		m.exports.markFailed(job.ID, err)
		m.flash("export: " + err.Error())
		return nil
	}

	// Both FormatSfdxProject and FormatSfdxProjectRetrieve need the
	// full project scaffold (sfdx-project.json + force-app/). Retrieve
	// goes one step further by populating force-app/ from the org;
	// see the goroutine kicked off after the synchronous write below.
	fullProject := msg.Format == exporters.FormatSfdxProject ||
		msg.Format == exporters.FormatSfdxProjectRetrieve

	// Write package.xml.
	manifestPath := filepath.Join(msg.Path, "package.xml")
	mf, err := os.Create(manifestPath)
	if err != nil {
		m.exports.markFailed(job.ID, err)
		m.flash("export: " + err.Error())
		return nil
	}
	result, err := dpexport.WritePackageXML(mf, items, dpexport.PackageXMLOptions{
		APIVersion: sf.APIVersionForAlias(orgFilter),
	})
	closeErr := mf.Close()
	if err != nil {
		m.exports.markFailed(job.ID, err)
		m.flash("export: " + err.Error())
		return nil
	}
	if closeErr != nil {
		m.exports.markFailed(job.ID, closeErr)
		m.flash("export: " + closeErr.Error())
		return nil
	}
	if result.IncludedCount == 0 {
		m.exports.markFailed(job.ID, fmt.Errorf("no items mapped to MetadataAPI types"))
		m.flash("export: no items mapped to MetadataAPI types (records / unsupported only)")
		return nil
	}

	// Full-project mode: scaffold sfdx-project.json + the force-app
	// directory tree the retrieve command writes into. Empty force-app
	// on its own works (sfdx creates files inside on retrieve), but
	// `sf project retrieve` errors if the path doesn't exist at all,
	// so we pre-create main/default/ to keep the first-run smooth.
	if fullProject {
		projectJSON := dpexport.SfdxProjectJSON(msg.DevName, sf.APIVersionForAlias(orgFilter))
		jsonPath := filepath.Join(msg.Path, "sfdx-project.json")
		if err := os.WriteFile(jsonPath, []byte(projectJSON), 0o644); err != nil {
			m.exports.markFailed(job.ID, err)
			m.flash("export: " + err.Error())
			return nil
		}
		forceAppDir := filepath.Join(msg.Path, "force-app", "main", "default")
		if err := os.MkdirAll(forceAppDir, 0o755); err != nil {
			m.exports.markFailed(job.ID, err)
			m.flash("export: " + err.Error())
			return nil
		}
	}

	// Records sidecar: emit only when the project has KindRecord
	// items. Uses the standard exporters.Write CSV path on a Rows()
	// slice limited to the records — same column shape as a normal
	// CSV export, so users can ingest with whatever spreadsheet
	// tooling they already have.
	if len(result.Records) > 0 {
		recordsPath := filepath.Join(msg.Path, "records.csv")
		rf, err := os.Create(recordsPath)
		if err != nil {
			m.exports.markFailed(job.ID, err)
			m.flash("export: " + err.Error())
			return nil
		}
		resolver := m.itemURLResolver()
		recRows := dpexport.Rows(result.Records, resolver)
		if err := exporters.Write(rf, exporters.FormatCSV, dpexport.Headers, recRows, "records"); err != nil {
			rf.Close()
			m.exports.markFailed(job.ID, err)
			m.flash("export: " + err.Error())
			return nil
		}
		_ = rf.Close()
	}

	// README.md describing the bundle + how to use it. The fullProject
	// flag switches the README between the two workflow descriptions.
	readmePath := filepath.Join(msg.Path, "README.md")
	if err := os.WriteFile(readmePath, []byte(dpexport.SuggestedReadme(msg.DevName, orgFilter, result, fullProject)), 0o644); err != nil {
		m.exports.markFailed(job.ID, err)
		m.flash("export: " + err.Error())
		return nil
	}

	applog.Info("devproject.export.manifest", map[string]any{
		"project":      msg.DevID,
		"path":         msg.Path,
		"format":       string(msg.Format),
		"full_project": fullProject,
		"included":     result.IncludedCount,
		"records":      len(result.Records),
		"unsupported":  len(result.Unsupported),
		"managed":      len(result.Managed),
		"all_orgs":     msg.ScopeAllOrgs,
		"retrieve":     msg.Format.RunsRetrieve(),
	})

	// When this format includes a follow-up `sf project retrieve`,
	// keep the job in-flight + flip phase to retrieving. Bundle stays
	// on disk if retrieve fails so the user has something to inspect.
	if msg.Format.RunsRetrieve() {
		alias := ""
		if len(m.orgs) > 0 {
			alias = targetArg(m.orgs[m.selected])
		}
		if alias == "" {
			m.exports.markFailed(job.ID,
				fmt.Errorf("no target org for retrieve (bundle written to %s)", msg.Path))
			m.flash("retrieve: no target org — bundle written but not populated")
			return nil
		}
		// Create or look up the persistent bundle row so this retrieve
		// (and future ones via /bundles) updates the same registry
		// entry. msg.BundleID is set when the user picked "update
		// existing bundle" from openBundleTargetPicker; otherwise we
		// create a fresh row pointing at the new directory.
		bundleID := msg.BundleID
		if bundleID == "" {
			if b, err := m.devProjects.CreateBundle(msg.DevID, msg.Path, alias); err == nil {
				bundleID = b.ID
			} else {
				applog.Warn("bundle.create_failed", map[string]any{"err": err.Error()})
			}
		} else {
			// Updating an existing bundle — refresh its default org so
			// later /bundles operations target the right org.
			_ = m.devProjects.SetDefaultOrgAlias(bundleID, alias)
		}
		if bundleID == "" {
			err := fmt.Errorf("bundle registry unavailable for retrieve")
			m.exports.markFailed(job.ID, err)
			m.flash("retrieve: bundle written but registry link failed")
			return nil
		}
		m.exports.setPhase(job.ID, exportPhaseRetrieving)
		m.flash(fmt.Sprintf("retrieving from %s…", alias))
		jobID := job.ID
		linkedBundleID := bundleID
		service := bundleWriteService(m)
		worker := func() tea.Msg {
			result, err := service.Retrieve(context.Background(), bundles.OperationInput{
				BundleID: linkedBundleID, Target: alias,
			})
			return projectRetrieveDoneMsg{
				JobID:      jobID,
				BundleID:   linkedBundleID,
				BundlePath: msg.Path,
				Output:     result.Output,
				Err:        err,
			}
		}
		// Kick the activity tick so the status-bar indicator animates
		// during the retrieve window. exportActivityTickCmd is single-
		// flight so this is a no-op when an unrelated export is also
		// running.
		return tea.Batch(worker, m.exportActivityTickCmd())
	}

	// Synchronous bundle (no retrieve): mark done + flash here.
	m.exports.markDone(job.ID, msg.Path)
	parts := []string{fmt.Sprintf("manifest: %d components", result.IncludedCount)}
	if fullProject {
		parts[0] = "sfdx project: " + parts[0]
	}
	if len(result.Records) > 0 {
		parts = append(parts, fmt.Sprintf("records.csv: %d", len(result.Records)))
	}
	if len(result.Unsupported) > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (unsupported kinds)", len(result.Unsupported)))
	}
	if len(result.Managed) > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped (managed packages)", len(result.Managed)))
	}
	m.flash(fmt.Sprintf("%s → %s", strings.Join(parts, " · "), msg.Path))
	return nil
}

// projectRetrieveDoneMsg lands on Update when the retrieve goroutine
// returns. Output is the (json-shaped) sf stdout on success, nil on
// failure — Err carries the typed sf error.
type projectRetrieveDoneMsg struct {
	JobID      string
	BundleID   string // SQLite bundles row id; empty when no bundle was created/linked
	BundlePath string
	Output     []byte
	Err        error
}

// applyProjectRetrieveDone folds the retrieve result into the model.
// On failure the job is still marked failed but the bundle path is
// preserved so the user has something to navigate to + inspect.
//
// On success, also bumps the bundle's last_retrieved_at so the
// /bundles list shows the right age.
func (m *Model) applyProjectRetrieveDone(msg projectRetrieveDoneMsg) {
	if msg.Err != nil {
		m.exports.markFailed(msg.JobID,
			fmt.Errorf("%w (bundle written to %s)", msg.Err, msg.BundlePath))
		m.flashFor("retrieve failed: "+msg.Err.Error()+
			" — bundle still at "+msg.BundlePath, 12*time.Second)
		applog.Error("devproject.retrieve_failed", map[string]any{
			"err":    msg.Err.Error(),
			"bundle": msg.BundlePath,
		})
		return
	}
	m.exports.markDone(msg.JobID, msg.BundlePath)
	if msg.BundleID != "" && m.devProjects != nil {
		_ = m.devProjects.MarkRetrieved(msg.BundleID)
	}
	m.flashFor("retrieve complete → "+msg.BundlePath, 6*time.Second)
	applog.Info("devproject.retrieve_done", map[string]any{
		"bundle": msg.BundlePath,
		"bytes":  len(msg.Output),
	})
}

// defaultDevProjectExportPath builds the pre-filled save path for
// the path-picker. Single-file formats produce
// "<exports-dir>/<slug>-<timestamp>.<ext>"; bundle formats produce
// "<exports-dir>/<slug>-<timestamp>/" (no extension, trailing slash
// implied). Reuses the user's configured export directory so
// project exports land alongside report exports.
func (m Model) defaultDevProjectExportPath(projectName string, format exporters.Format) string {
	dir := expandTilde(m.settings.ReportExportDir())
	slug := dpexport.SuggestedFilename(projectName)
	stamp := time.Now().Format("20060102-150405")
	if format.IsBundle() {
		return filepath.Join(dir, slug+"-"+stamp)
	}
	return filepath.Join(dir, slug+"-"+stamp+format.Extension())
}

// itemURLResolver returns a closure that resolves an Item to its
// Lightning URL. Each item's origin org dictates which instance URL
// is used; orgs not currently authenticated yield "" (the export
// row's URL column will be empty for items from logged-out orgs).
//
// Composed from a per-org instance-URL lookup + per-kind path
// templates. Keeping this here (vs. injecting per-call) keeps the
// exporters/devproject package free of ui/sf imports.
func (m Model) itemURLResolver() dpexport.URLResolver {
	// Cache the org → instance URL map once so we're not iterating
	// m.orgs per item.
	instances := map[string]string{}
	for _, o := range m.orgs {
		if o.InstanceURL != "" {
			instances[o.Username] = strings.TrimRight(o.InstanceURL, "/")
		}
	}
	return func(it devproject.Item) string {
		base := instances[it.OrgUser]
		if base == "" {
			return ""
		}
		path := lightningPathForItem(it)
		if path == "" {
			return ""
		}
		return base + path
	}
}

// lightningPathForItem returns the Lightning instance-relative path
// for an item, or "" when the kind has no canonical URL (records
// without a known sObject context, abstract aggregates, etc.).
//
// Keeps the path templates here rather than constructing synthetic
// sf.Openable values: the templates are short, the inputs are
// already in Item form, and faking up Openables would mean
// duplicating each kind's struct fields.
func lightningPathForItem(it devproject.Item) string {
	switch it.Kind {
	case devproject.KindSObject:
		if it.Ref == "" {
			return ""
		}
		return "/lightning/setup/ObjectManager/" + it.Ref + "/Details/view"
	case devproject.KindField:
		// Ref is "<sObject>.<FieldApiName>"; Type is the sObject.
		sobj := it.Type
		field := it.Ref
		if sobj == "" {
			if i := strings.IndexByte(field, '.'); i > 0 {
				sobj = field[:i]
				field = field[i+1:]
			}
		} else {
			field = strings.TrimPrefix(it.Ref, sobj+".")
		}
		if sobj == "" || field == "" {
			return ""
		}
		return "/lightning/setup/ObjectManager/" + sobj + "/FieldsAndRelationships/" + field + "/view"
	case devproject.KindFlow, devproject.KindFlowVersion:
		// Flows: open the Flow Builder by definition id.
		defID := it.Ref
		if it.Kind == devproject.KindFlowVersion && it.Type != "" {
			defID = it.Type
		}
		if defID == "" {
			return ""
		}
		return "/builder_platform_interaction/flowBuilder.app?flowId=" + defID
	case devproject.KindRecord:
		if it.Ref == "" {
			return ""
		}
		// Canonical ref "<sObject>:<Id>"; legacy bare-Id refs carry the
		// sObject in Type.
		sobj, id := splitRecordKey(it.Ref)
		if sobj == "" {
			sobj, id = it.Type, it.Ref
		}
		if sobj == "" || id == "" {
			return ""
		}
		return "/lightning/r/" + sobj + "/" + id + "/view"
	case devproject.KindApexClass:
		if it.Ref == "" {
			return ""
		}
		return "/lightning/setup/ApexClasses/page?address=%2F" + it.Ref
	case devproject.KindApexTrigger:
		if it.Ref == "" {
			return ""
		}
		return "/lightning/setup/ApexTriggers/page?address=%2F" + it.Ref
	case devproject.KindReport:
		if it.Ref == "" {
			return ""
		}
		return "/lightning/r/Report/" + it.Ref + "/view"
	case devproject.KindPermissionSet:
		return "/lightning/setup/PermSets/page?address=%2F" + it.Ref
	case devproject.KindPermissionSetGroup:
		return "/lightning/setup/PermSetGroups/page?address=%2F" + it.Ref
	case devproject.KindProfile:
		return "/lightning/setup/EnhancedProfiles/page?address=%2F" + it.Ref
	case devproject.KindValidationRule:
		// Validation rules don't have a stable Lightning URL — the
		// rule lives under its sObject's setup page. Best we can do
		// is link to the sObject's Validation Rules tab.
		if it.Type == "" {
			return ""
		}
		return "/lightning/setup/ObjectManager/" + it.Type + "/ValidationRules/view"
	case devproject.KindRecordType:
		if it.Type == "" {
			return ""
		}
		return "/lightning/setup/ObjectManager/" + it.Type + "/RecordTypes/view"
	case devproject.KindLWC:
		// LWC bundles don't have a Lightning Setup URL — show the
		// general LWC setup page.
		return "/lightning/setup/LightningComponentBundles/home"
	case devproject.KindAura:
		return "/lightning/setup/AuraComponents/home"
	case devproject.KindQueue:
		return "/lightning/setup/Queues/page?address=%2F" + it.Ref
	case devproject.KindPublicGroup:
		return "/lightning/setup/PublicGroups/page?address=%2F" + it.Ref
	}
	return ""
}
