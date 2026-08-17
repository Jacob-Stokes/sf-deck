package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type bundlePreview struct {
	Retrieve         sf.ManifestPreview
	Deploy           sf.ManifestPreview
	NonSourceTracked bool
	Fallback         bool
	Err              error
}

type bundlePreviewLoadedMsg struct {
	BundleID string
	Preview  bundlePreview
}

func loadBundlePreviewCmd(bundleID, bundleDir, alias string, lastRetrievedAt time.Time) tea.Cmd {
	return func() tea.Msg {
		retrieve, retErr := sf.RetrievePreview(bundleDir, alias)
		deploy, depErr := sf.DeployPreview(bundleDir, alias)
		nonTracked := retrieve.NonSourceTracked || deploy.NonSourceTracked
		if nonTracked {
			fb, fbErr := sf.ManifestPreviewFallback(bundleDir, alias, lastRetrievedAt)
			return bundlePreviewLoadedMsg{
				BundleID: bundleID,
				Preview: bundlePreview{
					Retrieve:         fb,
					Deploy:           sf.ManifestPreview{},
					NonSourceTracked: true,
					Fallback:         true,
					Err:              fbErr,
				},
			}
		}
		var firstErr error
		if retErr != nil {
			firstErr = retErr
		} else if depErr != nil {
			firstErr = depErr
		}
		return bundlePreviewLoadedMsg{
			BundleID: bundleID,
			Preview: bundlePreview{
				Retrieve:         retrieve,
				Deploy:           deploy,
				NonSourceTracked: false,
				Fallback:         false,
				Err:              firstErr,
			},
		}
	}
}

func (m *Model) applyBundlePreviewLoaded(msg bundlePreviewLoadedMsg) {
	if m.bundlePreviews == nil {
		m.bundlePreviews = map[string]bundlePreview{}
	}
	m.bundlePreviews[msg.BundleID] = msg.Preview
	if msg.Preview.Err != nil {
		applog.Warn("bundle.preview_failed", map[string]any{
			"bundle": msg.BundleID,
			"err":    msg.Preview.Err.Error(),
		})
	}
	if m.bundleCur == msg.BundleID {
		m.bundleDetailList.Set(bundleDetailRowsFromPreview(msg.Preview))
	}
}

func ensureBundleDetailData(m *Model, d *orgData, o sf.Org) tea.Cmd {
	if m.bundleCur == "" || m.devProjects == nil {
		return nil
	}
	if _, ok := m.bundlePreviews[m.bundleCur]; ok {
		return nil // cached; user can press r to refresh
	}
	b, err := m.devProjects.GetBundle(m.bundleCur)
	if err != nil || b.Stale() {
		return nil
	}
	alias := b.DefaultOrgAlias
	if alias == "" {
		alias = targetArg(o)
	}
	if alias == "" {
		return nil
	}
	return loadBundlePreviewCmd(b.ID, b.Path, alias, b.LastRetrievedAt)
}
