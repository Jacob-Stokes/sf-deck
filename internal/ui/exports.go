package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/securefile"
)

type exportPhase string

const (
	exportPhaseQueued      exportPhase = "queued"
	exportPhaseDownloading exportPhase = "downloading"
	exportPhasePostProcess exportPhase = "post-processing"
	exportPhaseConverting  exportPhase = "converting"
	exportPhaseWriting     exportPhase = "writing"
	exportPhaseRetrieving  exportPhase = "retrieving"
	exportPhaseDone        exportPhase = "done"
	exportPhaseFailed      exportPhase = "failed"
)

type exportKind string

const (
	exportKindReport   exportKind = "report"
	exportKindProject  exportKind = "project"
	exportKindManifest exportKind = "manifest"
)

type exportJob struct {
	ID         string      `json:"id"`
	Kind       exportKind  `json:"kind"`
	Name       string      `json:"name"`
	OrgAlias   string      `json:"org_alias,omitempty"`
	Path       string      `json:"path"`
	Format     string      `json:"format,omitempty"`
	Phase      exportPhase `json:"phase"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at,omitempty"`
	SizeBytes  int64       `json:"size_bytes,omitempty"`
	ErrMsg     string      `json:"err,omitempty"`
}

type exportRegistry struct {
	mu         sync.Mutex
	inflight   []*exportJob
	history    []*exportJob
	path       string // absolute path to exports.json; "" disables persistence
	historyCap int    // settings.ExportHistoryMax() at construction time; 0 → unlimited
}

// newExportRegistry constructs a registry rooted at
// ~/.sf-deck/exports.json. Load failures (missing file, malformed
// JSON) are silently treated as "fresh start" — losing history is
// preferable to refusing to launch.
//
// historyCap is the size limit for the kept-history list, typically
// settings.ExportHistoryMax() at startup. 0 disables capping.
func newExportRegistry(historyCap int) *exportRegistry {
	r := &exportRegistry{historyCap: historyCap}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return r
	}
	r.path = filepath.Join(home, ".sf-deck", "exports.json")
	r.load()
	return r
}

func (r *exportRegistry) load() {
	if r.path == "" {
		return
	}
	body, err := os.ReadFile(r.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			applog.Warn("exports.load_failed", map[string]any{"err": err.Error()})
		}
		return
	}
	var hist []*exportJob
	if err := json.Unmarshal(body, &hist); err != nil {
		applog.Warn("exports.load_invalid", map[string]any{"err": err.Error()})
		return
	}
	r.mu.Lock()
	r.history = hist
	r.mu.Unlock()
}

// save writes history to exports.json atomically (tmp + rename) so a
// crash mid-write can't corrupt the file. In-flight jobs aren't
// persisted — they only exist while the process is alive.
func (r *exportRegistry) save() {
	if r.path == "" {
		return
	}
	r.mu.Lock()
	hist := make([]*exportJob, len(r.history))
	copy(hist, r.history)
	r.mu.Unlock()
	body, err := json.MarshalIndent(hist, "", "  ")
	if err != nil {
		return
	}
	if err := securefile.WriteFile(r.path, body, true); err != nil {
		applog.Warn("exports.save_failed", map[string]any{"err": err.Error()})
	}
}

func (r *exportRegistry) startJob(kind exportKind, name, orgAlias, path, format string) *exportJob {
	j := &exportJob{
		ID:        time.Now().UTC().Format("20060102-150405.000") + "-" + name,
		Kind:      kind,
		Name:      name,
		OrgAlias:  orgAlias,
		Path:      path,
		Format:    format,
		Phase:     exportPhaseQueued,
		StartedAt: time.Now(),
	}
	r.mu.Lock()
	r.inflight = append(r.inflight, j)
	r.mu.Unlock()
	return j
}

// setPhase updates a job's phase. Identifies jobs by ID (callers hold
// the *exportJob but mutating directly would skip the lock).
func (r *exportRegistry) setPhase(id string, phase exportPhase) {
	r.mu.Lock()
	for _, j := range r.inflight {
		if j.ID == id {
			j.Phase = phase
			break
		}
	}
	r.mu.Unlock()
}

func (r *exportRegistry) markDone(id, finalPath string) {
	r.mu.Lock()
	idx := -1
	for i, j := range r.inflight {
		if j.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		r.mu.Unlock()
		return
	}
	j := r.inflight[idx]
	j.Phase = exportPhaseDone
	j.FinishedAt = time.Now()
	j.Path = finalPath
	if info, err := os.Stat(finalPath); err == nil {
		j.SizeBytes = info.Size()
	}
	r.inflight = append(r.inflight[:idx], r.inflight[idx+1:]...)
	r.history = append([]*exportJob{j}, r.history...)
	if r.historyCap > 0 && len(r.history) > r.historyCap {
		r.history = r.history[:r.historyCap]
	}
	r.mu.Unlock()
	r.save()
}

// markFailed is markDone's sad twin. Records the error and still
// pushes onto history so the user can see "you tried this and it
// failed because X" rather than wondering why nothing appears.
func (r *exportRegistry) markFailed(id string, err error) {
	r.mu.Lock()
	idx := -1
	for i, j := range r.inflight {
		if j.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		r.mu.Unlock()
		return
	}
	j := r.inflight[idx]
	j.Phase = exportPhaseFailed
	j.FinishedAt = time.Now()
	if err != nil {
		j.ErrMsg = err.Error()
	}
	r.inflight = append(r.inflight[:idx], r.inflight[idx+1:]...)
	r.history = append([]*exportJob{j}, r.history...)
	if r.historyCap > 0 && len(r.history) > r.historyCap {
		r.history = r.history[:r.historyCap]
	}
	r.mu.Unlock()
	r.save()
}

func (r *exportRegistry) removeFromHistory(id string) {
	r.mu.Lock()
	idx := -1
	for i, j := range r.history {
		if j.ID == id {
			idx = i
			break
		}
	}
	if idx >= 0 {
		r.history = append(r.history[:idx], r.history[idx+1:]...)
	}
	r.mu.Unlock()
	r.save()
}

// snapshot returns DEEP copies of inflight + history for rendering.
//
// The returned pointers address fresh exportJob values, NOT the
// registry's live structs, so the render path can read Phase / StartedAt /
// SizeBytes lock-free while a worker goroutine advances the real job under
// r.mu. exportJob is flat, so a value copy is a complete independent snapshot.
func (r *exportRegistry) snapshot() (inflight, history []*exportJob) {
	r.mu.Lock()
	inflight = make([]*exportJob, len(r.inflight))
	for i, j := range r.inflight {
		cp := *j
		inflight[i] = &cp
	}
	history = make([]*exportJob, len(r.history))
	for i, j := range r.history {
		cp := *j
		history[i] = &cp
	}
	r.mu.Unlock()
	sort.SliceStable(inflight, func(i, j int) bool {
		return inflight[i].StartedAt.Before(inflight[j].StartedAt)
	})
	return inflight, history
}

func (r *exportRegistry) hasInflight() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.inflight) > 0
}

func (r *exportRegistry) mostRecentDone(k exportKind) *exportJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.history {
		if j.Kind == k && j.Phase == exportPhaseDone && j.Path != "" {
			return j
		}
	}
	return nil
}
