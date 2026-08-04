package resource

import (
	"sort"
	"strings"
	"time"
	"unsafe"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// nowMonotonic + elapsedMs are tiny wrappers around time.Now so the
// filter-timing call sites read cleanly. Returns ms (rounded down)
// so they fit cleanly into SearchState.lastDurationMs (int).
func nowMonotonic() time.Time {
	return time.Now()
}

func elapsedMs(t time.Time) int {
	return int(time.Since(t) / time.Millisecond)
}

// SearchState is the per-view search/filter state. The live buffer
// is owned by a bubbles/textinput so the user gets cursor nav, word
// jumps, and home/end mid-buffer. The active/committed flags are
// our own state machine: "active" means the `/` prompt is visible
// and keys type into the buffer; "committed" means the user pressed
// enter, the prompt closed, and the filter stays applied.
//
// Three modes:
//
//	active=false committed=false → no filter
//	active=true                  → live-typing; filter applied as you type
//	active=false committed=true  → committed filter; list stays narrowed,
//	                               but shortcut keys work again
type SearchState struct {
	Active    bool
	Committed bool
	Input     textinput.Model
	Inited    bool // true once `Input` has been constructed

	// effective is the search text the filter cache currently keys
	// on. Decoupled from Input.Value() so big-list filters can be
	// debounced: keystrokes update Input immediately (textinput
	// stays snappy) but `effective` lags behind until the user
	// pauses, batching multiple keystrokes into one filter pass.
	//
	// Adaptive: when the last filter ran faster than the small-list
	// threshold (settings.SearchFastFilterThresholdMs, default
	// 50ms), `effective` is bumped synchronously on every keystroke
	// — feels identical to the un-debounced behaviour. Only when
	// the filter starts costing more does the debounce window kick
	// in.
	effective string

	// lastDurationMs is the wall-time the most recent filter pass
	// took. Read by callers to decide whether to debounce the next
	// update. Written by the projection / Filtered() path after the
	// filter completes.
	lastDurationMs int

	// debouncePending, when true, means the buffer has changed since
	// `effective` was last synced and the model's debounce-tick
	// dispatcher should promote Buffer→effective on the next tick.
	debouncePending bool
}

// Buffer returns the current search text. Safe on the zero value
// (returns "") so call sites don't need to check for init.
func (s SearchState) Buffer() string {
	if !s.Inited {
		return ""
	}
	return s.Input.Value()
}

// SetBuffer replaces the current search text. Bumps the effective
// text immediately because this path is used by programmatic
// resets (ctrl+u clear, chip switch, etc.) which the user expects
// to apply right away — not via the debounce timer.
func (s *SearchState) SetBuffer(v string) {
	s.EnsureInit()
	s.Input.SetValue(v)
	s.Input.CursorEnd()
	s.effective = v
	s.debouncePending = false
}

// EnsureInit lazily constructs the textinput widget the first time
// the state is touched. Zero-value SearchStates survive reloads and
// the input gets built on demand.
func (s *SearchState) EnsureInit() {
	if s.Inited {
		return
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	StyleInput(&ti)
	s.Input = ti
	s.Inited = true
}

// StyleInput applies our Tokyo-night palette to a textinput widget.
// v2 moved per-state styles into Styles.Focused / Styles.Blurred and
// the cursor's color into a separate CursorStyle; we build a Styles
// value and hand it to SetStyles in one shot so the widget looks
// consistent across focus transitions.
func StyleInput(ti *textinput.Model) {
	s := ti.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Blurred.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	s.Blurred.Placeholder = lipgloss.NewStyle().Foreground(theme.FgDim)
	s.Cursor.Color = theme.BorderHi
	ti.SetStyles(s)
}

// Applied reports whether a non-empty filter is currently narrowing
// results. Works on a by-value copy because it only reads the buffer.
func (s SearchState) Applied() bool {
	return (s.Active || s.Committed) && s.Buffer() != ""
}

// Effective returns the search text the filter cache should
// currently key on. Identical to Buffer() unless an adaptive
// debounce is in flight (big-list path), in which case it lags
// one tick behind so consecutive fast keystrokes collapse into
// a single filter pass.
//
// Safe on the zero value (returns "" — same as Buffer()).
func (s SearchState) Effective() string {
	if !s.Inited {
		return ""
	}
	if s.debouncePending {
		return s.effective
	}
	// Fast path: no debounce pending, effective tracks Buffer().
	// We still return effective rather than Buffer() so the cache
	// key is stable across renders within the same logical state.
	if s.effective == "" {
		return s.Input.Value()
	}
	return s.effective
}

// EffectiveApplied is the Applied()-against-Effective() variant.
// Filter cache keys should compose (Effective, EffectiveApplied)
// instead of (Buffer, Applied) so the debounced effective text
// gates whether the filter runs at all.
func (s SearchState) EffectiveApplied() bool {
	return (s.Active || s.Committed) && s.Effective() != ""
}

// LastFilterDurationMs reads the most recent filter wall-time.
// Callers (debounce dispatcher) use this to decide whether to
// schedule a tick or sync immediately.
func (s SearchState) LastFilterDurationMs() int {
	return s.lastDurationMs
}

// SetLastFilterDurationMs records the wall-time of the filter
// that just completed. Called from the projection cache path
// after running the filter sweep.
func (s *SearchState) SetLastFilterDurationMs(ms int) {
	s.lastDurationMs = ms
}

// DebouncePending reports whether a buffer change is waiting on
// the next tick to promote to effective. The Update-loop sweep
// scans search states for this flag.
func (s SearchState) DebouncePending() bool {
	return s.debouncePending
}

// NoteBufferChanged is called by key handlers / SetBuffer after
// the textinput buffer is mutated. Decides between the fast path
// (sync effective immediately) and the debounced path (mark
// pending, let the tick dispatcher promote it).
//
// fastThresholdMs is the cutoff: if the previous filter completed
// in fewer ms than this, the next update bypasses debouncing and
// feels instant.
func (s *SearchState) NoteBufferChanged(fastThresholdMs int) {
	s.EnsureInit()
	current := s.Input.Value()
	if fastThresholdMs <= 0 || s.lastDurationMs <= fastThresholdMs {
		// Last filter was fast (or no measurement yet) — sync
		// effective synchronously so the next render shows
		// updated results with no perceptible delay.
		s.effective = current
		s.debouncePending = false
		return
	}
	// Slow last filter → defer. Tick dispatcher will sync
	// effective once the debounce window elapses.
	s.debouncePending = true
}

// SyncEffective copies Buffer→effective unconditionally and
// clears the pending flag. Called by the debounce-tick dispatcher
// when the debounce window has elapsed since the last NoteBufferChanged.
func (s *SearchState) SyncEffective() {
	if !s.Inited {
		return
	}
	s.effective = s.Input.Value()
	s.debouncePending = false
}

// ListView wraps a slice of T with cursor + search-filter state. Views
// that show a browsable list use one of these so that cursor bounds,
// search rendering, and filter predicates all live in one place rather
// than being re-implemented per view.
//
// Match is the substring-match predicate, installed via SetMatch.
// It's lowercased once per Filtered() call and passed in —
// implementations compare case-insensitively against whatever fields
// of T make sense.
//
// Usage:
//
//	var lv ListView[sf.SObject]
//	lv.SetMatch(func(s sf.SObject, q string) bool {
//		return strings.Contains(strings.ToLower(s.Name), q) ||
//			strings.Contains(strings.ToLower(s.Label), q)
//	})
//	lv.Set(items)
//	visible := lv.Filtered()
//	hovered := lv.Selected()
//
// Use ListView.Move(delta) for cursor nav; it clamps to filtered bounds.
// Use ListView.Search for the search prompt state (same SearchState type
// used previously).
type ListView[T any] struct {
	items  []T
	cursor int
	Search SearchState

	// match is the user-supplied substring predicate. q is already
	// lowercased. nil → search is a no-op (list unfiltered). Private
	// so callers go through SetMatch which bumps the cache version.
	match func(item T, q string) bool

	// score is the optional relevance ranker used to sort filtered
	// results when search is active. Higher = better match. nil →
	// no ranking (filtered results stay in source order, which is
	// the original behaviour). Set via SetScorer.
	score func(item T, q string) int

	// extra is the optional pre-filter applied before Search (e.g. a
	// chip predicate). Private so callers go through SetExtra, which
	// bumps the cache version. Direct assignment would silently leave
	// stale rows in the Filtered() cache.
	extra func(item T) bool

	// order is an optional display-order permutation applied after
	// Extra/Search/relevance. It lets selected/open/yank paths consume
	// the same order the list-table renderer shows under column sort,
	// without mutating the source items slice. orderKey is part of the
	// cache key because the function can close over UI state.
	order    func(items []T) []int
	orderKey string

	// defaultOrder is a fallback permutation used only when `order` is
	// nil. Lets chip-driven views impose a domain-specific order (e.g.
	// the Recently Viewed chip wants most-recent-first) without
	// fighting the column-sort path, which still owns `order`.
	defaultOrder    func(items []T) []int
	defaultOrderKey string

	// version increments any time `items` or `extra` changes. Combined
	// with the search buffer it forms the cache key for filteredCache.
	// Bumped by Set / SetExtra; never decremented. Callers can rely on
	// "version unchanged → filter inputs unchanged."
	version int

	// filteredCache memoises the most recent Filtered() result so
	// per-frame call sites (MoveBy, BuildRenderModel, Selected, …) all
	// share one O(N) scan rather than re-running the chip predicate
	// 2-3+ times. Invalidated implicitly when version or search buffer
	// disagrees with the cached key on the next Filtered() call.
	filteredCache  []T
	filteredKeyVer int
	filteredKeyBuf string
	filteredKeyOrd string
	filteredCached bool
}

// Set replaces the underlying items and resets the cursor. Bumps the
// cache version so the next Filtered() rebuilds.
//
// No-op fast path: when the new slice is the SAME backing array and
// length as the current one, Set returns without touching the cursor,
// version, or cache. This is what makes per-render callers safe —
// surfaces like /objects/schema call syncFieldList (→ Set) on every
// frame AND on every cursor move, re-passing the stable describe field
// slice. Without this guard the unconditional `cursor = 0` below would
// wipe the cursor on the very next frame, so up/down never visibly
// moved. Identity (backing pointer + len) is exact here: the slice
// comes from Resource.Value().Fields, whose backing array is stable
// until the describe is re-Set with genuinely new data — at which
// point the pointer changes and the reset path correctly runs.
func (lv *ListView[T]) Set(items []T) {
	if len(items) == len(lv.items) &&
		unsafe.SliceData(items) == unsafe.SliceData(lv.items) {
		return
	}
	lv.items = items
	lv.cursor = 0
	lv.version++
	lv.filteredCached = false
}

// SetExtra installs a new pre-filter predicate (or nil to clear) and
// bumps the cache version. Callers should always go through this
// rather than assigning a private field — direct assignment would
// silently leave the Filtered() cache pointing at stale rows.
func (lv *ListView[T]) SetExtra(fn func(item T) bool) {
	lv.extra = fn
	lv.version++
	lv.filteredCached = false
}

// SetScorer installs the relevance ranker used to sort search hits.
// Higher score = better match → lands earlier in Filtered() output.
// Bumps the cache version so the next Filtered() re-runs the sort.
// nil clears the scorer (results return in source order).
func (lv *ListView[T]) SetScorer(fn func(item T, q string) int) {
	lv.score = fn
	lv.version++
	lv.filteredCached = false
}

// SetOrder installs a display-order permutation. The callback receives
// the already-filtered slice and returns display index → filtered index.
// Invalid permutations fall back to identity. key must change whenever
// the ordering semantics change (sort column/direction, visible column
// set, compact/full flag mode); it is folded into the Filtered cache.
func (lv *ListView[T]) SetOrder(fn func(items []T) []int, key string) {
	if fn == nil {
		key = ""
	}
	if lv.orderKey == key && (fn == nil) == (lv.order == nil) {
		lv.order = fn
		return
	}
	lv.order = fn
	lv.orderKey = key
	lv.filteredCached = false
}

func (lv *ListView[T]) OrderKey() string { return lv.orderKey }

// SetDefaultOrder installs a fallback display-order permutation that's
// only consulted when there's no `order` set by the column-sort path.
// Same key contract as SetOrder.
func (lv *ListView[T]) SetDefaultOrder(fn func(items []T) []int, key string) {
	if fn == nil {
		key = ""
	}
	if lv.defaultOrderKey == key && (fn == nil) == (lv.defaultOrder == nil) {
		lv.defaultOrder = fn
		return
	}
	lv.defaultOrder = fn
	lv.defaultOrderKey = key
	lv.filteredCached = false
}

func (lv *ListView[T]) DefaultOrderKey() string { return lv.defaultOrderKey }

// activeOrder returns the order fn + key that should drive Filtered(),
// preferring an explicit `order` set by column sort, falling back to
// `defaultOrder` (e.g. Recently Viewed recency).
func (lv *ListView[T]) activeOrder() (func(items []T) []int, string) {
	if lv.order != nil {
		return lv.order, lv.orderKey
	}
	return lv.defaultOrder, lv.defaultOrderKey
}

// SetMatch installs the substring search predicate. Same shape as
// SetExtra: bumps the cache version so Filtered() rebuilds. Today
// match is set once at orgData init and never swapped, but going
// through a setter keeps the invalidation contract consistent —
// future code that rebuilds the matcher on theme/locale change won't
// accidentally serve stale results.
func (lv *ListView[T]) SetMatch(fn func(item T, q string) bool) {
	lv.match = fn
	lv.version++
	lv.filteredCached = false
}

// HasMatch reports whether a substring matcher has been installed.
// Used by lazy-init call sites that want to install a matcher only
// once: `if !lv.HasMatch() { lv.SetMatch(...) }`. Avoids exposing the
// private field while still letting callers gate setup.
func (lv *ListView[T]) HasMatch() bool { return lv.match != nil }

// Version returns the monotonically increasing data-version counter.
// Bumps every time items or the extra predicate change. Used by
// callers (per-row render caches) that need a stable key for "the
// underlying data has not changed since last tick."
func (lv *ListView[T]) Version() int { return lv.version }

// Items returns the raw unfiltered slice.
func (lv *ListView[T]) Items() []T { return lv.items }

// Len returns the raw count.
func (lv *ListView[T]) Len() int { return len(lv.items) }

// ExtraCount returns the number of items matching the Extra
// predicate alone, ignoring Search. Used by surfaces that need to
// distinguish "project / chip filter has nothing" (show empty-state
// hint) from "search has nothing" (show 'no matches' + keep search
// box visible).
func (lv *ListView[T]) ExtraCount() int {
	if lv.extra == nil {
		return len(lv.items)
	}
	n := 0
	for _, it := range lv.items {
		if lv.extra(it) {
			n++
		}
	}
	return n
}

// Filtered returns the items matching Extra + Search. Memoised: a
// cache hit returns the previously-computed slice in O(1). Invalidated
// when version (Set / SetExtra) or the search buffer changes.
//
// Important: the returned slice is the cached pointer — callers must
// treat it as read-only. Mutating it would corrupt the next render.
func (lv *ListView[T]) Filtered() []T {
	// Cache key on the EFFECTIVE buffer (debounce-aware), not the
	// raw input buffer. While the user is typing past the fast-
	// filter threshold, Effective() lags one tick behind so we
	// don't run the expensive sweep on every keystroke.
	buf := ""
	if lv.Search.EffectiveApplied() {
		buf = lv.Search.Effective()
	}
	orderFn, orderKey := lv.activeOrder()
	if lv.filteredCached &&
		lv.filteredKeyVer == lv.version &&
		lv.filteredKeyBuf == buf &&
		lv.filteredKeyOrd == orderKey {
		return lv.filteredCache
	}
	start := nowMonotonic()
	if lv.extra == nil && !lv.Search.EffectiveApplied() {
		out := lv.items
		if orderFn != nil {
			out = applyOrder(out, orderFn(out))
		}
		lv.filteredCache = out
		lv.filteredKeyVer = lv.version
		lv.filteredKeyBuf = buf
		lv.filteredKeyOrd = orderKey
		lv.filteredCached = true
		lv.Search.SetLastFilterDurationMs(elapsedMs(start))
		return out
	}
	q := strings.ToLower(buf)
	out := make([]T, 0, len(lv.items))
	for _, it := range lv.items {
		if lv.extra != nil && !lv.extra(it) {
			continue
		}
		if q != "" && lv.match != nil && !lv.match(it, q) {
			continue
		}
		out = append(out, it)
	}
	// Relevance ranking: when search is active and a scorer is
	// installed, sort hits high-to-low. Stable sort preserves the
	// source order for ties (so a tie-band still respects the
	// underlying alphabetical / fetch order).
	if q != "" && lv.score != nil && len(out) > 1 {
		scores := make([]int, len(out))
		for i, it := range out {
			scores[i] = lv.score(it, q)
		}
		sortByScoreStable(out, scores)
	}
	if orderFn != nil {
		out = applyOrder(out, orderFn(out))
	}
	lv.filteredCache = out
	lv.filteredKeyVer = lv.version
	lv.filteredKeyBuf = buf
	lv.filteredKeyOrd = orderKey
	lv.filteredCached = true
	lv.Search.SetLastFilterDurationMs(elapsedMs(start))
	return out
}

func applyOrder[T any](items []T, perm []int) []T {
	if len(perm) != len(items) {
		return items
	}
	seen := make([]bool, len(items))
	out := make([]T, len(items))
	for i, src := range perm {
		if src < 0 || src >= len(items) || seen[src] {
			return items
		}
		seen[src] = true
		out[i] = items[src]
	}
	return out
}

// sortByScoreStable sorts the items slice in-place, descending by
// the parallel scores slice. Stable: ties preserve the original
// order. Hand-written vs sort.SliceStable to avoid the indirect
// closure call per comparison and keep allocations to two slices
// (the scratch + the index permutation).
func sortByScoreStable[T any](items []T, scores []int) {
	if len(items) != len(scores) {
		return
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return scores[idx[a]] > scores[idx[b]]
	})
	out := make([]T, len(items))
	for i, j := range idx {
		out[i] = items[j]
	}
	copy(items, out)
}

// Cursor returns the current cursor index (may be >= FilteredLen if the
// filter narrowed recently; Selected handles clamping safely).
func (lv *ListView[T]) Cursor() int { return lv.cursor }

// SetCursor clamps to [0, Filtered-1].
func (lv *ListView[T]) SetCursor(i int) {
	n := len(lv.Filtered())
	if n <= 0 {
		lv.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	lv.cursor = i
}

// Move shifts the cursor by delta, refusing if the result would go out
// of range. Kept for the small handful of call sites that want the
// original "no-op past the edge" semantics.
func (lv *ListView[T]) Move(delta int) {
	filtered := lv.Filtered()
	next := lv.cursor + delta
	if next >= 0 && next < len(filtered) {
		lv.cursor = next
	}
}

// MoveBy shifts the cursor by delta, clamping to [0, Filtered-1] so
// overshooting (page-down on the last page, G to bottom) lands on the
// edge instead of being a no-op.
func (lv *ListView[T]) MoveBy(delta int) {
	filtered := lv.Filtered()
	lv.cursor = clampDelta(lv.cursor, delta, len(filtered))
}

// Selected returns the filtered item under the cursor, and whether one
// exists.
func (lv *ListView[T]) Selected() (T, bool) {
	filtered := lv.Filtered()
	var zero T
	if len(filtered) == 0 {
		return zero, false
	}
	idx := lv.cursor
	if idx >= len(filtered) {
		idx = len(filtered) - 1
	}
	return filtered[idx], true
}

// ResetCursor is the "filter changed, snap cursor back" helper.
func (lv *ListView[T]) ResetCursor() { lv.cursor = 0 }

// SearchPtr returns a pointer to the SearchState so key-handlers can
// mutate it directly (active/committed/buffer).
func (lv *ListView[T]) SearchPtr() *SearchState { return &lv.Search }

// clampDelta returns cur+delta clamped to [0, n-1]. When n == 0 returns
// 0. Used for list navigation where overshooting should land on the
// edge (page-down past the last page, G to the bottom, etc.).
func clampDelta(cur, delta, n int) int {
	if n <= 0 {
		return 0
	}
	next := cur + delta
	if next < 0 {
		return 0
	}
	if next >= n {
		return n - 1
	}
	return next
}
