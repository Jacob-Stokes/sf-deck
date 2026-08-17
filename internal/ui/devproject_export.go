package ui

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
	scopeAllOrgs := false
	if m.tab() == TabDevProjectDetail {
		scopeAllOrgs = m.devProjectShowAllOrgs
	}
	return m.openExportFormatPicker(dpID, dpName, scopeAllOrgs)
}

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

type exportProjectFormatPickedMsg struct {
	DevID, DevName string
	Format         exporters.Format
	ScopeAllOrgs   bool
}

func (m *Model) applyExportProjectFormatPicked(msg exportProjectFormatPickedMsg) tea.Cmd {
	if msg.Format == exporters.FormatSfdxProjectRetrieve && m.devProjects != nil {
		bundles, err := m.devProjects.ListBundlesFor(msg.DevID)
		if err == nil && len(bundles) > 0 {
			return m.openBundleTargetPicker(msg, bundles)
		}
	}
	return m.openExportPathPicker(msg, "")
}

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

type bundleTargetPickedMsg struct {
	Format   exportProjectFormatPickedMsg
	BundleID string // "" = create new
}

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

type exportProjectPathPickedMsg struct {
	DevID, DevName string
	Format         exporters.Format
	Path           string
	ScopeAllOrgs   bool
	BundleID       string
	Overwrite      bool
}

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

	job := m.exports.startJob(exportKindProject, msg.DevName, orgFilter, msg.Path, string(msg.Format))
	m.exports.setPhase(job.ID, exportPhaseWriting)

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

	fullProject := msg.Format == exporters.FormatSfdxProject ||
		msg.Format == exporters.FormatSfdxProjectRetrieve

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

	if len(result.Records) > 0 {
		recordsPath := filepath.Join(msg.Path, "records.csv")
		resolver := m.itemURLResolver()
		recRows := dpexport.Rows(result.Records, resolver)
		if err := securefile.Write(recordsPath, true, func(w io.Writer) error {
			return exporters.Write(w, exporters.FormatCSV, dpexport.Headers, recRows, "records")
		}); err != nil {
			m.exports.markFailed(job.ID, err)
			m.flash("export: " + err.Error())
			return nil
		}
	}

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
		return tea.Batch(worker, m.exportActivityTickCmd())
	}

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

type projectRetrieveDoneMsg struct {
	JobID      string
	BundleID   string // SQLite bundles row id; empty when no bundle was created/linked
	BundlePath string
	Output     []byte
	Err        error
}

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

func lightningPathForItem(it devproject.Item) string {
	switch it.Kind {
	case devproject.KindSObject:
		if it.Ref == "" {
			return ""
		}
		return "/lightning/setup/ObjectManager/" + it.Ref + "/Details/view"
	case devproject.KindField:
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
