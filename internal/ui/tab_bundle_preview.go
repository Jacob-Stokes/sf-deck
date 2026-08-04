package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// bundlePreview is the cached preview data for one bundle. Loaded
// lazily on tab entry; refreshed on tab refresh.
//
// Fallback is true when the data came from ManifestPreviewFallback
// rather than `sf project retrieve preview` — used by the renderer
// to add a "diff via timestamp comparison" caption so users know the
// origin of the data + accept its limitations (no conflict
// detection, no deletion detection).
type bundlePreview struct {
	Retrieve         sf.ManifestPreview
	Deploy           sf.ManifestPreview
	NonSourceTracked bool
	Fallback         bool
	Err              error
}

// bundlePreviewLoadedMsg lands on Update after the preview goroutine
// finishes. JobID identifies the bundle the preview belongs to.
type bundlePreviewLoadedMsg struct {
	BundleID string
	Preview  bundlePreview
}

// loadBundlePreviewCmd kicks the goroutine that runs both retrieve
// and deploy preview commands against the bundle. Returns a tea.Cmd
// suitable for return from EnsureData.
//
// Two-stage with fallback: first try the source-tracking-based
// `sf project retrieve preview` / `deploy preview`. If the org
// returns NonSourceTrackedOrgError (production / Partial Copy /
// Full sandbox without tracking — i.e. most enterprise orgs), fall
// through to ManifestPreviewFallback which queries the Tooling API
// for LastModifiedDates and compares against local mtimes. The
// fallback is coarser (no conflict detection, no deletion detection)
// but works on every org regardless of tracking state.
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

// applyBundlePreviewLoaded folds the preview result into the model.
// Cached on m.bundlePreviews keyed by bundle ID; refresh re-runs the
// goroutine and overwrites the cache entry.
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
	// Refresh the list-table view's row set whenever the preview
	// for the currently-drilled bundle lands. Other bundles'
	// previews can sit in m.bundlePreviews without disturbing the
	// active list — the renderer reads m.bundleCur each frame.
	if m.bundleCur == msg.BundleID {
		m.bundleDetailList.Set(bundleDetailRowsFromPreview(msg.Preview))
	}
}

// ensureBundleDetailData is the EnsureData hook for TabBundleDetail.
// Loads the preview tables (retrieve + deploy) for the drilled-in
// bundle if they aren't already cached. The first paint shows
// "loading…" until the goroutine returns.
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
