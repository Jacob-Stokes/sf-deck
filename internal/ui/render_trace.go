package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
)

const (
	renderTraceEnv      = "SF_DECK_RENDER_TRACE"
	renderTracePathEnv  = "SF_DECK_RENDER_TRACE_PATH"
	renderTraceEveryEnv = "SF_DECK_RENDER_TRACE_EVERY"
)

type renderTracer struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	every  uint64
	seq    uint64
	active *renderFrameTrace
}

type renderFrameTrace struct {
	tracer   *renderTracer
	start    time.Time
	memStart runtime.MemStats
	record   renderTraceRecord
}

type renderTraceRecord struct {
	Event       string             `json:"event"`
	Seq         uint64             `json:"seq"`
	TS          string             `json:"ts"`
	Tab         string             `json:"tab,omitempty"`
	Subtab      string             `json:"subtab,omitempty"`
	Focus       string             `json:"focus,omitempty"`
	Width       int                `json:"width,omitempty"`
	Height      int                `json:"height,omitempty"`
	PaneH       int                `json:"pane_h,omitempty"`
	InnerH      int                `json:"inner_h,omitempty"`
	WidgetW     int                `json:"widget_w,omitempty"`
	MainW       int                `json:"main_w,omitempty"`
	SidebarW    int                `json:"sidebar_w,omitempty"`
	LeftOpen    bool               `json:"left_open"`
	SidebarOpen bool               `json:"sidebar_open"`
	Path        string             `json:"path,omitempty"`
	Overlay     bool               `json:"overlay"`
	Picker      bool               `json:"picker"`
	Cached      bool               `json:"cached"`
	Zen         bool               `json:"zen"`
	FrameBytes  int                `json:"frame_bytes,omitempty"`
	FrameLines  int                `json:"frame_lines,omitempty"`
	PhasesMS    map[string]float64 `json:"phases_ms,omitempty"`
	TotalMS     float64            `json:"total_ms,omitempty"`
	AllocBytes  uint64             `json:"alloc_bytes,omitempty"`
	Mallocs     uint64             `json:"mallocs,omitempty"`
	Frees       uint64             `json:"frees,omitempty"`
	HeapAlloc   uint64             `json:"heap_alloc,omitempty"`
	HeapObjects uint64             `json:"heap_objects,omitempty"`
	NumGC       uint32             `json:"num_gc,omitempty"`
	GCDelta     uint32             `json:"gc_delta,omitempty"`
	GCPauseNS   uint64             `json:"gc_pause_ns,omitempty"`
	List        *renderTraceList   `json:"list,omitempty"`
	Cache       *renderTraceCache  `json:"cache,omitempty"`
}

type renderTraceList struct {
	Title           string `json:"title,omitempty"`
	N               int    `json:"n"`
	Cursor          int    `json:"cursor"`
	Cols            int    `json:"cols"`
	LeftGutters     int    `json:"left_gutters,omitempty"`
	RightGutters    int    `json:"right_gutters,omitempty"`
	SearchActive    bool   `json:"search_active"`
	SearchCommitted bool   `json:"search_committed"`
	SearchLen       int    `json:"search_len,omitempty"`
	Zen             bool   `json:"zen"`
	Paginated       bool   `json:"paginated"`
	Page            int    `json:"page,omitempty"`
	HScroll         int    `json:"hscroll,omitempty"`
	SortColumn      string `json:"sort_column,omitempty"`
	SortDesc        bool   `json:"sort_desc"`
}

type renderTraceCache struct {
	TabRows     int  `json:"tab_rows"`
	LeftTabRows int  `json:"left_tab_rows"`
	StatusBars  int  `json:"status_bars"`
	LastFrame   int  `json:"last_frame_bytes,omitempty"`
	SkipFrame   bool `json:"skip_frame"`
}

type renderTraceWheelRecord struct {
	Event           string  `json:"event"`
	TS              string  `json:"ts"`
	Button          string  `json:"button"`
	Dropped         bool    `json:"dropped"`
	Reason          string  `json:"reason"`
	SinceSeenMS     float64 `json:"since_seen_ms,omitempty"`
	SinceAcceptedMS float64 `json:"since_accepted_ms,omitempty"`
	QuietGapMS      float64 `json:"quiet_gap_ms"`
	MinIntervalMS   float64 `json:"min_interval_ms"`
}

func newRenderTracerFromEnv() *renderTracer {
	if !envTruthy(os.Getenv(renderTraceEnv)) {
		return nil
	}
	path := os.Getenv(renderTracePathEnv)
	if path == "" {
		path = defaultRenderTracePath()
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		applog.Warn("render_trace.open_failed", map[string]any{"path": path, "err": err.Error()})
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		applog.Warn("render_trace.open_failed", map[string]any{"path": path, "err": err.Error()})
		return nil
	}
	every := uint64(1)
	if raw := os.Getenv(renderTraceEveryEnv); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			every = uint64(n)
		}
	}
	t := &renderTracer{file: file, path: path, every: every}
	t.writeLocked(map[string]any{
		"event": "render_trace_start",
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"path":  path,
		"every": every,
	})
	applog.Info("render_trace.started", map[string]any{"path": path, "every": every})
	return t
}

func defaultRenderTracePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	name := "render-" + time.Now().Format("20060102-150405") + "-" + strconv.Itoa(os.Getpid()) + ".jsonl"
	return filepath.Join(home, ".sf-deck", "log", name)
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

func (m Model) beginRenderTrace() *renderFrameTrace {
	if m.renderTrace == nil {
		return nil
	}
	return m.renderTrace.begin(m)
}

func (m Model) beginListTableTrace() *renderFrameTrace {
	if m.renderTrace == nil {
		return nil
	}
	return m.renderTrace.activeFrame()
}

func (t *renderTracer) begin(m Model) *renderFrameTrace {
	if t == nil || t.file == nil {
		return nil
	}
	t.mu.Lock()
	t.seq++
	seq := t.seq
	every := t.every
	if every == 0 {
		every = 1
	}
	if seq%every != 0 {
		t.mu.Unlock()
		return nil
	}
	f := &renderFrameTrace{
		tracer: t,
		start:  time.Now(),
		record: renderTraceRecord{
			Event:    "render_frame",
			Seq:      seq,
			TS:       time.Now().UTC().Format(time.RFC3339Nano),
			Tab:      m.tab().String(),
			Subtab:   string(m.currentSubtab()),
			Focus:    traceFocusName(m.focus),
			Width:    m.width,
			Height:   m.height,
			PhasesMS: map[string]float64{},
		},
	}
	runtime.ReadMemStats(&f.memStart)
	t.active = f
	t.mu.Unlock()
	return f
}

func (f *renderFrameTrace) phase(name string, started time.Time) {
	if f == nil || started.IsZero() {
		return
	}
	f.record.PhasesMS[name] += durationMS(time.Since(started))
}

func (f *renderFrameTrace) setPath(path string) {
	if f != nil {
		f.record.Path = path
	}
}

func (f *renderFrameTrace) setOutput(s string) {
	if f == nil {
		return
	}
	f.record.FrameBytes = len(s)
	if s != "" {
		f.record.FrameLines = strings.Count(s, "\n") + 1
	}
}

func (f *renderFrameTrace) setLayout(widgetW, mainW, sidebarW, paneH, innerH int, leftOpen, sidebarOpen bool) {
	if f == nil {
		return
	}
	f.record.WidgetW = widgetW
	f.record.MainW = mainW
	f.record.SidebarW = sidebarW
	f.record.PaneH = paneH
	f.record.InnerH = innerH
	f.record.LeftOpen = leftOpen
	f.record.SidebarOpen = sidebarOpen
}

func (f *renderFrameTrace) setOverlay(overlay, picker bool) {
	if f == nil {
		return
	}
	f.record.Overlay = overlay
	f.record.Picker = picker
}

func (f *renderFrameTrace) markCached() {
	if f != nil {
		f.record.Cached = true
	}
}

func (f *renderFrameTrace) markZen() {
	if f != nil {
		f.record.Zen = true
	}
}

func (f *renderFrameTrace) finish(m Model) {
	if f == nil || f.tracer == nil {
		return
	}
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)
	f.record.TotalMS = durationMS(time.Since(f.start))
	f.record.AllocBytes = memEnd.TotalAlloc - f.memStart.TotalAlloc
	f.record.Mallocs = memEnd.Mallocs - f.memStart.Mallocs
	f.record.Frees = memEnd.Frees - f.memStart.Frees
	f.record.HeapAlloc = memEnd.HeapAlloc
	f.record.HeapObjects = memEnd.HeapObjects
	f.record.NumGC = memEnd.NumGC
	f.record.GCDelta = memEnd.NumGC - f.memStart.NumGC
	f.record.GCPauseNS = memEnd.PauseTotalNs - f.memStart.PauseTotalNs
	f.record.Cache = traceRenderCacheStats(m.renderCache)
	if len(f.record.PhasesMS) == 0 {
		f.record.PhasesMS = nil
	}

	t := f.tracer
	t.mu.Lock()
	if t.active == f {
		t.active = nil
	}
	t.writeLocked(f.record)
	t.mu.Unlock()
}

func (t *renderTracer) activeFrame() *renderFrameTrace {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

func (t *renderTracer) writeLocked(v any) {
	if t == nil || t.file == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = t.file.Write(b)
}

func (m Model) traceListRenderModel(model listRenderModel, cursor int) {
	if m.renderTrace == nil {
		return
	}
	f := m.renderTrace.activeFrame()
	if f == nil {
		return
	}
	list := &renderTraceList{
		Title:        model.Title,
		N:            model.N,
		Cursor:       cursor,
		Cols:         len(model.Cols),
		LeftGutters:  len(model.Gutters),
		RightGutters: len(model.RightGutters),
	}
	if model.Search != nil {
		list.SearchActive = model.Search.Active
		list.SearchCommitted = model.Search.Committed
		list.SearchLen = len(model.Search.Buffer())
	}
	if model.State != nil {
		list.Zen = model.State.Zen
		list.Paginated = model.State.Paginated
		list.Page = model.State.Page
		list.HScroll = model.State.HScroll
		list.SortColumn = model.State.SortColumn
		list.SortDesc = model.State.SortDesc
	}
	f.record.List = list
}

func (m Model) traceWheel(button tea.MouseButton, dropped bool, reason string, sinceSeen, sinceAccepted time.Duration, quietGap, minInterval time.Duration) {
	if m.renderTrace == nil || m.renderTrace.file == nil {
		return
	}
	rec := renderTraceWheelRecord{
		Event:         "wheel",
		TS:            time.Now().UTC().Format(time.RFC3339Nano),
		Button:        traceMouseButtonName(button),
		Dropped:       dropped,
		Reason:        reason,
		QuietGapMS:    durationMS(quietGap),
		MinIntervalMS: durationMS(minInterval),
	}
	if sinceSeen >= 0 {
		rec.SinceSeenMS = durationMS(sinceSeen)
	}
	if sinceAccepted >= 0 {
		rec.SinceAcceptedMS = durationMS(sinceAccepted)
	}
	m.renderTrace.mu.Lock()
	m.renderTrace.writeLocked(rec)
	m.renderTrace.mu.Unlock()
}

func traceRenderCacheStats(c *renderCache) *renderTraceCache {
	if c == nil {
		return nil
	}
	return &renderTraceCache{
		TabRows:     len(c.tabRows),
		LeftTabRows: len(c.leftTabRows),
		StatusBars:  len(c.statusBars),
		LastFrame:   len(c.lastFrame),
		SkipFrame:   c.skipFrame,
	}
}

func traceFocusName(f focus) string {
	switch f {
	case focusOrgs:
		return "orgs"
	case focusMain:
		return "main"
	default:
		return "unknown"
	}
}

func traceMouseButtonName(button tea.MouseButton) string {
	switch button {
	case tea.MouseWheelDown:
		return "wheel_down"
	case tea.MouseWheelUp:
		return "wheel_up"
	default:
		return strconv.Itoa(int(button))
	}
}

func durationMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
