package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/bundles"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// onBundleDetailKey routes the per-tab keys (r/D/v/o/y) for
// /bundle. Returns (consumed, cmd) — cmd is the goroutine to kick
// for retrieve / deploy / validate ops.
func (m *Model) onBundleDetailKey(key string) (bool, tea.Cmd) {
	if m.tab() != TabBundleDetail || m.bundleCur == "" {
		return false, nil
	}
	if m.devProjects == nil {
		return false, nil
	}
	b, err := m.devProjects.GetBundle(m.bundleCur)
	if err != nil {
		return false, nil
	}
	switch {
	case matches(key, Keys.BundleOpen):
		if b.Path == "" {
			return true, nil
		}
		// On /bundle, the target depends on the active view:
		//   - Components: cursored row's relative Path (the file
		//     the manifest item lives in), falling back to the
		//     bundle dir when no row is selected.
		//   - Files: cursored file or directory under cwd. The
		//     parent (..) row is treated like "open the parent
		//     dir" which is a meaningful action (browse the
		//     bundle's structure outside sf-deck).
		//   - /bundles, no row, anything else: bundle root.
		target := b.Path
		if m.tab() == TabBundleDetail {
			switch m.bundleDetailView {
			case bundleViewFiles:
				row, ok := m.bundleFilesList.Selected()
				if ok {
					if row.IsParent {
						target = b.Path + "/" + m.bundleFilesCwd
					} else if m.bundleFilesCwd == "" {
						target = b.Path + "/" + row.Name
					} else {
						target = b.Path + "/" + m.bundleFilesCwd + "/" + row.Name
					}
				}
			default:
				if row, ok := m.bundleDetailList.Selected(); ok && row.Path != "" {
					target = b.Path + "/" + row.Path
				}
			}
		}
		if err := openPath(target); err != nil {
			m.flash("open failed: " + err.Error())
		}
		return true, nil
	case matches(key, Keys.BundleYankPath):
		_ = writeClipboard(b.Path)
		m.flash("yanked path → " + b.Path)
		return true, nil
	case matches(key, Keys.BundleRetrieve):
		return true, startBundleRetrieve(m, b)
	case matches(key, Keys.BundleDeploy):
		return true, startBundleDeploy(m, b)
	case matches(key, Keys.BundleValidate):
		return true, startBundleValidate(m, b)
	case matches(key, Keys.BundleRefreshDiff):
		alias := b.DefaultOrgAlias
		if alias == "" && len(m.orgs) > 0 {
			alias = targetArg(m.orgs[m.selected])
		}
		if alias == "" {
			m.flash("refresh: no target org")
			return true, nil
		}
		delete(m.bundlePreviews, b.ID)
		m.flash("refreshing diff…")
		return true, loadBundlePreviewCmd(b.ID, b.Path, alias, b.LastRetrievedAt)
	}
	return false, nil
}

// startBundleRetrieve kicks the full retrieve goroutine for the
// bundle. Registers a job in the export tracker so Ctrl+J shows it,
// and updates last_retrieved_at on completion.
func startBundleRetrieve(m *Model, b devproject.Bundle) tea.Cmd {
	alias := b.DefaultOrgAlias
	if alias == "" && len(m.orgs) > 0 {
		alias = targetArg(m.orgs[m.selected])
	}
	if alias == "" {
		m.flash("retrieve: no target org — set default org or pick an org first")
		return nil
	}
	job := m.exports.startJob(exportKindProject, "bundle: "+b.Path, alias, b.Path, "retrieve")
	m.exports.setPhase(job.ID, exportPhaseRetrieving)
	m.flash(fmt.Sprintf("retrieving bundle from %s…", alias))
	jobID := job.ID
	bundleID := b.ID
	service := bundleWriteService(m)
	worker := func() tea.Msg {
		result, err := service.Retrieve(context.Background(), bundles.OperationInput{
			BundleID: bundleID, Target: alias,
		})
		return bundleOpDoneMsg{
			JobID:    jobID,
			BundleID: bundleID,
			Op:       "retrieve",
			Output:   result.Output,
			Err:      err,
		}
	}
	return tea.Batch(worker, m.exportActivityTickCmd())
}

// startBundleDeploy is startBundleRetrieve's sibling for
// `sf project deploy start`.
func startBundleDeploy(m *Model, b devproject.Bundle) tea.Cmd {
	org, ok := m.bundleTargetOrg(b)
	if !ok {
		m.flash("deploy: no target org — set default org or pick an org first")
		return nil
	}
	alias := targetArg(org)
	if ok, reason := m.canWriteOrg(org, settings.WriteMetadata); !ok {
		m.flash(reason)
		return nil
	}
	job := m.exports.startJob(exportKindProject, "deploy: "+b.Path, alias, b.Path, "deploy")
	m.exports.setPhase(job.ID, exportPhaseRetrieving) // Reuse phase label; "deploying" can come later
	m.flash(fmt.Sprintf("deploying bundle to %s…", alias))
	jobID := job.ID
	bundleID := b.ID
	service := bundleWriteService(m)
	worker := func() tea.Msg {
		result, err := service.Deploy(context.Background(), bundles.OperationInput{
			BundleID: bundleID, Target: alias,
		})
		return bundleOpDoneMsg{
			JobID:    jobID,
			BundleID: bundleID,
			Op:       "deploy",
			Output:   result.Output,
			Err:      err,
		}
	}
	return tea.Batch(worker, m.exportActivityTickCmd())
}

func (m Model) bundleTargetOrg(b devproject.Bundle) (sf.Org, bool) {
	if b.DefaultOrgAlias != "" {
		return m.orgForTarget(b.DefaultOrgAlias)
	}
	return m.currentOrg()
}

// startBundleValidate kicks `sf project deploy validate` — server-side
// check without committing changes. Validate doesn't commit, but it's
// still a privileged op against the org (runs validation rules + Apex
// tests, consumes a deploy slot), so it's gated the same as deploy:
// resolve the bundle's target org and require WriteMetadata.
func startBundleValidate(m *Model, b devproject.Bundle) tea.Cmd {
	org, ok := m.bundleTargetOrg(b)
	if !ok {
		m.flash("validate: no target org — set default org or pick an org first")
		return nil
	}
	alias := targetArg(org)
	if ok, reason := m.canWriteOrg(org, settings.WriteMetadata); !ok {
		m.flash(reason)
		return nil
	}
	m.flash(fmt.Sprintf("validating against %s…", alias))
	bundleID := b.ID
	service := bundleWriteService(m)
	return func() tea.Msg {
		result, err := service.Validate(context.Background(), bundles.OperationInput{
			BundleID: bundleID, Target: alias,
		})
		return bundleOpDoneMsg{
			BundleID: bundleID,
			Op:       "validate",
			Output:   result.Output,
			Err:      err,
		}
	}
}

func bundleWriteService(m *Model) *bundles.Service {
	if m.bundleOps != nil {
		return m.bundleOps
	}
	gate := orgwrite.NewGate(func(target string) (sf.Org, error) {
		if org, ok := m.orgForTarget(target); ok {
			return org, nil
		}
		return sf.Org{}, fmt.Errorf("org not found: %s", target)
	}, func(org sf.Org) settings.SafetyLevel {
		return m.settings.Resolve(org.Username, settings.OrgKind(org.Kind()), org.Alias)
	})
	return bundles.New(m.devProjects, gate)
}

// bundleOpDoneMsg lands on Update when a retrieve/deploy/validate
// goroutine returns. Op tells applyBundleOpDone which timestamp to
// update + which flash to show.
type bundleOpDoneMsg struct {
	JobID    string
	BundleID string
	Op       string // "retrieve" | "deploy" | "validate"
	Output   []byte
	Err      error
}

// applyBundleOpDone updates the registry job + bundle timestamps +
// flashes a result. On success, also kicks a fresh preview load so
// the diff tables refresh against the new state.
func (m *Model) applyBundleOpDone(msg bundleOpDoneMsg) tea.Cmd {
	if msg.Err != nil {
		if msg.JobID != "" {
			m.exports.markFailed(msg.JobID, msg.Err)
		}
		m.flashFor(msg.Op+" failed: "+msg.Err.Error(), 12*time.Second)
		applog.Error("bundle.op_failed", map[string]any{
			"op":     msg.Op,
			"bundle": msg.BundleID,
			"err":    msg.Err.Error(),
		})
		return nil
	}
	b, err := m.devProjects.GetBundle(msg.BundleID)
	if err != nil {
		return nil
	}
	switch msg.Op {
	case "retrieve":
		_ = m.devProjects.MarkRetrieved(msg.BundleID)
		m.flashFor("retrieve complete → "+b.Path, 6*time.Second)
	case "deploy":
		_ = m.devProjects.MarkDeployed(msg.BundleID)
		m.flashFor("deploy complete → "+b.DefaultOrgAlias, 6*time.Second)
	case "validate":
		m.flashFor("validate ok — deploy would succeed", 6*time.Second)
	}
	if msg.JobID != "" {
		m.exports.markDone(msg.JobID, b.Path)
	}
	if msg.Op != "validate" {
		alias := b.DefaultOrgAlias
		if alias == "" && len(m.orgs) > 0 {
			alias = targetArg(m.orgs[m.selected])
		}
		if alias != "" {
			refreshed, _ := m.devProjects.GetBundle(b.ID)
			return loadBundlePreviewCmd(b.ID, b.Path, alias, refreshed.LastRetrievedAt)
		}
	}
	return nil
}
