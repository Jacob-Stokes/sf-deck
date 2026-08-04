// Package sources holds concrete TreeSource implementations for each
// tree-shaped Salesforce surface. Each source is a small adapter
// from sf package types to treechip.TreeNode.
//
// Sources live here (not in internal/sf) because they're treechip-
// specific shape adapters; the underlying sf functions return raw
// types. Keeping the adapters in this subpackage avoids leaking
// treechip types into the SF data layer.
package sources

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip"
)

// ReportFolderSource is a treechip.TreeSource over Salesforce report
// folders. The first fetch is one big SOQL on Folder — on big orgs
// (thousands of folders) it's slow enough that we MUST load it
// async, off the render goroutine, or the whole TUI stalls.
//
// Lifecycle:
//   - Constructor (NewReportFolderSource) is cheap — no fetch.
//   - Caller fires LoadAsync() once on tab entry; it returns a
//     tea.Cmd whose result message is reportFoldersLoadedMsg.
//   - Until the message arrives, all reads (Roots / Children / Item)
//     return empty slices + Loading()=true so the renderer can show
//     a "loading…" state instead of blocking.
//   - On message arrival, the registry is hydrated and reads start
//     returning real data.
//
// Refresh() works the same way — fires a new LoadAsync.
type ReportFolderSource struct {
	orgAlias string

	mu       sync.Mutex
	loaded   bool
	loading  bool
	loadErr  error
	byID     map[string]treechip.TreeNode
	children map[string][]treechip.TreeNode // parentID → ordered children, "" key = roots
}

// NewReportFolderSource constructs a source bound to one org alias.
// Cheap — no fetch fires until the caller invokes LoadAsync.
func NewReportFolderSource(orgAlias string) *ReportFolderSource {
	return &ReportFolderSource{
		orgAlias: orgAlias,
		byID:     map[string]treechip.TreeNode{},
		children: map[string][]treechip.TreeNode{},
	}
}

// Loading reports whether a fetch is in flight. Renderers consult
// this to choose between a "loading folders…" state and an empty
// "no folders" message.
func (s *ReportFolderSource) Loading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loading
}

// Loaded reports whether the source has data ready.
func (s *ReportFolderSource) Loaded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loaded
}

// LoadErr returns the most-recent fetch error (or nil).
func (s *ReportFolderSource) LoadErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

// LoadAsync returns a tea.Cmd that fetches folders off the render
// goroutine. The Cmd's result is a ReportFoldersLoadedMsg the UI
// layer routes to ApplyLoaded. Idempotent: re-firing while a load
// is in flight is a no-op.
func (s *ReportFolderSource) LoadAsync() func() any {
	s.mu.Lock()
	if s.loading {
		s.mu.Unlock()
		return nil
	}
	s.loading = true
	alias := s.orgAlias
	s.mu.Unlock()
	applog.Info("report_folders.load_start", map[string]any{"alias": alias})
	return func() any {
		folders, err := sf.ListReportFolders(alias)
		applog.Info("report_folders.load_done", map[string]any{
			"alias":   alias,
			"folders": len(folders),
			"err":     errString(err),
		})
		return ReportFoldersLoadedMsg{
			Source:  s,
			Folders: folders,
			Err:     err,
		}
	}
}

// ReportFoldersLoadedMsg is the message LoadAsync returns when the
// fetch finishes. The UI layer's Update routes it to source.Apply.
type ReportFoldersLoadedMsg struct {
	Source  *ReportFolderSource
	Folders []sf.ReportFolder
	Err     error
}

// Apply hydrates the source from the fetched folder list. Called
// from the UI's Update on receipt of the loaded message.
func (s *ReportFolderSource) Apply(msg ReportFoldersLoadedMsg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loading = false
	s.loadErr = msg.Err
	if msg.Err != nil {
		return
	}
	s.byID = map[string]treechip.TreeNode{}
	s.children = map[string][]treechip.TreeNode{}
	for _, f := range msg.Folders {
		n := treechip.TreeNode{
			ID:       f.ID,
			Label:    f.Name,
			ParentID: f.ParentID,
			Data:     f,
		}
		s.byID[f.ID] = n
		s.children[f.ParentID] = append(s.children[f.ParentID], n)
	}
	for parent, kids := range s.children {
		sort.SliceStable(kids, func(i, j int) bool {
			return kids[i].Label < kids[j].Label
		})
		s.children[parent] = kids
	}
	s.loaded = true
}

// Refresh marks the source dirty + returns a new LoadAsync cmd. The
// UI layer fires this on `r`. Folders change rarely but admins do
// reorganise.
func (s *ReportFolderSource) Refresh() func() any {
	s.mu.Lock()
	s.loaded = false
	s.byID = map[string]treechip.TreeNode{}
	s.children = map[string][]treechip.TreeNode{}
	s.mu.Unlock()
	return s.LoadAsync()
}

// Roots returns folders with no parent. Returns an empty slice when
// not loaded yet — the renderer reads Loading() to distinguish
// "loading" from "genuinely empty."
func (s *ReportFolderSource) Roots() ([]treechip.TreeNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		return nil, nil
	}
	out := make([]treechip.TreeNode, len(s.children[""]))
	copy(out, s.children[""])
	return out, nil
}

// Children returns the direct children of a folder. Empty when
// unloaded; the renderer should branch on Loading().
func (s *ReportFolderSource) Children(parentID string) ([]treechip.TreeNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		return nil, nil
	}
	kids := s.children[parentID]
	out := make([]treechip.TreeNode, len(kids))
	copy(out, kids)
	return out, nil
}

// Item returns one folder by ID. Returns an error when not loaded
// — used by HydrateLastPath which is best-effort and tolerates the
// error by truncating the path.
func (s *ReportFolderSource) Item(id string) (treechip.TreeNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		return treechip.TreeNode{}, fmt.Errorf("folders not loaded yet")
	}
	n, ok := s.byID[id]
	if !ok {
		return treechip.TreeNode{}, fmt.Errorf("folder %s not found", id)
	}
	return n, nil
}

// All returns every folder (a flat slice). Used by the overflow
// modal to show the full tree as a searchable list.
func (s *ReportFolderSource) All() ([]treechip.TreeNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		return nil, nil
	}
	out := make([]treechip.TreeNode, 0, len(s.byID))
	for _, n := range s.byID {
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Label < out[j].Label
	})
	return out, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
