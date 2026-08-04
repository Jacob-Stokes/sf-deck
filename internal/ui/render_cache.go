package ui

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

type renderCache struct {
	tabRows     map[tabBarCacheKey]cachedLayerBlock
	leftTabRows map[leftTabBarCacheKey]cachedLayerBlock
	statusBars  map[statusBarCacheKey]string
	dashboards  map[dashboardCacheKey]string

	// pagedRows caches the per-row rendered strings used by paginated
	// list-table surfaces. Keyed by everything that affects a row's
	// appearance besides its cursor-selected state (tab + subtab +
	// inner width + focus + search terms + list version + sort
	// version + page). The cursor row is intentionally NEVER stored
	// in this cache — it's the only row that flips per tick (bold
	// first cell), so caching it would either bake in stale highlight
	// or force invalidation every tick. Caller short-circuits the
	// cursor index and re-renders that row each tick; non-cursor rows
	// hit the cache. Cleared on cache invalidation events (page
	// change, list mutation, focus shift) by virtue of the key
	// changing.
	pagedRows    map[pagedRowCacheKey]string
	pagedRowsKey pagedRowGroupKey

	lastFrame string
	skipFrame bool
}

// pagedRowGroupKey is the "everything but the row index" portion of
// the paged-row cache key. When this changes (page advances, search
// committed, list mutation, focus flip, …), the entire pagedRows
// map is dropped — the previous group's row strings are no longer
// valid for the new context. Compared against the active group at
// the start of each paginated render; mismatch wipes the map.
type pagedRowGroupKey struct {
	tab       Tab
	subtab    Subtab
	inner     int
	focus     focus
	pageSize  int
	page      int
	listVer   int
	sortKey   string
	searchKey string
	scopeName string
	// renderCfg folds every render-time setting / state field that
	// affects how a row looks — flag column mode, gutter widths,
	// HScroll, FrozenCols, UserWidths, theme, etc. Single field so
	// future toggles only need to be added to renderConfigVersion()
	// without growing the key shape; the cache invalidates as soon
	// as any such input changes.
	renderCfg int
}

// pagedRowCacheKey is the per-row entry. rowIndex is the **filtered
// list index** (post-Filtered, post-sort) — same `i` the rowFn
// closure receives.
type pagedRowCacheKey struct {
	rowIndex int
}

type tabBarCacheKey struct {
	width         int
	tab           Tab
	stem          Tab
	focus         focus
	scopeLoaded   bool
	scopeName     string
	devProjectCur string
	searchApplied bool
	searchBuf     string
	// allTabSearchFingerprint encodes the search-applied state of
	// EVERY top-level tab — one rune per tab in TabsForNumbers order
	// ("1" if Applied, "0" otherwise).  Folded into the cache key so
	// the magnifier-glyph set in the cached tab strip invalidates
	// when any tab's search state changes, not just the active one.
	allTabSearchFingerprint string
	// leftOpen controls whether the main tab bar prepends an Orgs
	// pill (only when the rail is collapsed). Toggle changes the
	// pill set, so it must invalidate the cached row.
	leftOpen bool
	// overflowSet + overflowTab drive the slot-9 pill. Omitting them
	// served stale pre-overflow rows for tabs visited BEFORE the
	// first overflow-tab visit — the "9 only shows on some tabs" bug
	// (2026-06-12): cache entries keyed per tab kept whichever pill
	// set existed when that tab was first rendered.
	overflowSet bool
	overflowTab Tab
}

type leftTabBarCacheKey struct {
	width          int
	focus          focus
	leftUtilityIdx int
}

// dashboardCacheKey captures every input the chip dashboard render
// observes: title text, chip list shape, selected index, width, and
// the trailing hint (which depends on tab + active sObject context).
//
// chipsHash is a fast stable hash of the chip slice — id, label,
// count for each row. This avoids re-comparing the slice every
// render; the hash itself only changes when the strip's composition
// changes (chip cycle, project chip prepend, transient slot, …).
type dashboardCacheKey struct {
	title     string
	chipsHash uint64
	chipsLen  int
	selected  int
	width     int
	hint      string
	collapsed bool
}

type statusBarCacheKey struct {
	width           int
	tab             Tab
	subtab          Subtab
	dashboardClosed bool
	searchCommitted bool
	searchBuf       string
	hasListTable    bool
	subtabCount     int
	bannerActive    bool
	banner          string
	exportActive    bool
	exportFrame     int
	// sidebarOpen affects whether the `\ side` hint appears (only
	// shown when the sidebar is hidden; the in-sidebar button
	// covers it otherwise). Without this field the cache returns a
	// stale row after `\` until something else invalidates it.
	sidebarOpen bool
	// canEscBack drives the "esc back" footer hint. It reads per-org
	// state (DrillReturnTab) + the record drill stack, neither of
	// which is otherwise in the key — so switching orgs on the same
	// tab/width/subtab (identical key) would otherwise show a stale
	// hint. Same bug class as the sidebarOpen miss.
	canEscBack bool
}

type cachedLayerBlock struct {
	text   string
	layers []cachedLayer
}

type cachedLayer struct {
	text    string
	id      string
	x, y, z int
}

func newRenderCache() *renderCache {
	return &renderCache{
		tabRows:     map[tabBarCacheKey]cachedLayerBlock{},
		leftTabRows: map[leftTabBarCacheKey]cachedLayerBlock{},
		statusBars:  map[statusBarCacheKey]string{},
		dashboards:  map[dashboardCacheKey]string{},
		pagedRows:   map[pagedRowCacheKey]string{},
	}
}

func (m Model) clearRenderCache() {
	if m.renderCache == nil {
		return
	}
	*m.renderCache = *newRenderCache()
}

func (m Model) skipNextFrameRender() {
	if m.renderCache == nil {
		return
	}
	m.renderCache.skipFrame = true
}

func (m Model) cachedFrameView() (tea.View, bool) {
	if m.renderCache == nil || !m.renderCache.skipFrame || m.renderCache.lastFrame == "" {
		return tea.View{}, false
	}
	m.renderCache.skipFrame = false
	v := tea.NewView(m.renderCache.lastFrame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v, true
}

func (m Model) rememberFrame(s string) {
	if m.renderCache == nil {
		return
	}
	m.renderCache.lastFrame = s
	m.renderCache.skipFrame = false
}

func (m Model) cachedTabBar(width int) (string, []*lipgloss.Layer) {
	if m.renderCache == nil {
		return m.renderTabBar(width)
	}
	key := m.tabBarCacheKey(width)
	if hit, ok := m.renderCache.tabRows[key]; ok {
		return hit.text, hit.materialize()
	}
	text, layers := m.renderTabBar(width)
	m.renderCache.tabRows[key] = cachedLayerBlock{
		text:   text,
		layers: cacheLayers(layers),
	}
	return text, layers
}

func (m Model) cachedLeftTabBar(width int) (string, []*lipgloss.Layer) {
	if m.renderCache == nil {
		return m.renderLeftTabBar(width)
	}
	key := m.leftTabBarCacheKey(width)
	if hit, ok := m.renderCache.leftTabRows[key]; ok {
		return hit.text, hit.materialize()
	}
	text, layers := m.renderLeftTabBar(width)
	m.renderCache.leftTabRows[key] = cachedLayerBlock{
		text:   text,
		layers: cacheLayers(layers),
	}
	return text, layers
}

func (m Model) cachedStatusBar() string {
	if m.renderCache == nil {
		return m.renderStatusBar()
	}
	key := m.statusBarCacheKey()
	if hit, ok := m.renderCache.statusBars[key]; ok {
		return hit
	}
	text := m.renderStatusBar()
	m.renderCache.statusBars[key] = text
	return text
}

// wrapPagedRowFn wraps a per-row renderer with the paged-row cache.
// Behaviour:
//   - The cursor row (i == cursor) always re-renders fresh, never
//     touches the cache. Its bold-first-cell highlight changes every
//     tick, so caching it would either bake in stale state or force
//     a per-tick invalidation — the wrapper keeps it out entirely.
//   - All other rows hit the cache. On group-key change the cache
//     wipes wholesale; subsequent tick fills it back as RenderRowsPaged
//     walks the page.
//
// When renderCache is nil (tests) or the surface didn't supply a
// DataVersion, falls through to the unwrapped rowFn — no cache, no
// behavioural change.
func (m Model) wrapPagedRowFn(
	model listRenderModel,
	focus focus,
	inner, pageSize int,
	terms []string,
	cursor int,
	rowFn func(i int) string,
) func(i int) string {
	if m.renderCache == nil || model.DataVersion == 0 {
		return rowFn
	}
	group := pagedRowGroupKey{
		tab:       m.tab(),
		subtab:    m.currentSubtab(),
		inner:     inner,
		focus:     focus,
		pageSize:  pageSize,
		page:      model.State.Page,
		listVer:   model.DataVersion,
		sortKey:   sortKey(model.State),
		searchKey: searchTermsKey(terms),
		scopeName: scopeKeyForRowCache(m),
		renderCfg: m.renderConfigVersion(model.State),
	}
	if group != m.renderCache.pagedRowsKey {
		// Group changed — drop every entry. Cheap: maps are small
		// (one page worth of rows, ~50 entries) and re-fill happens
		// inline as the page renders.
		m.renderCache.pagedRows = map[pagedRowCacheKey]string{}
		m.renderCache.pagedRowsKey = group
	}
	return func(i int) string {
		if i == cursor {
			return rowFn(i)
		}
		key := pagedRowCacheKey{rowIndex: i}
		if hit, ok := m.renderCache.pagedRows[key]; ok {
			return hit
		}
		out := rowFn(i)
		m.renderCache.pagedRows[key] = out
		return out
	}
}

// sortKey folds the active sort spec down to a comparable string.
// Different sort columns / directions need different cache groups
// because rowFn's `i → row` mapping (via sortPerm) differs.
func sortKey(state *uilayout.ListTableState) string {
	if state == nil || state.SortColumn == "" {
		return ""
	}
	if state.SortDesc {
		return state.SortColumn + "↓"
	}
	return state.SortColumn + "↑"
}

// searchTermsKey concatenates the search-term slice for cache
// keying. Terms are short (one buffer split on whitespace) so a
// joined string is cheaper than building a more clever fingerprint.
func searchTermsKey(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	// Strings.Join allocates one buffer; the result is one entry
	// in the cache key, replaced (not retained per-row) on group
	// change. Acceptable.
	return joinSearchTerms(terms)
}

// joinSearchTerms is a tiny helper to keep the term-join out of the
// hot path's inline expansion. Pulled out so the wrapping happens
// once per group-key build, not once per row.
func joinSearchTerms(terms []string) string {
	const sep = "\x00"
	total := 0
	for _, t := range terms {
		total += len(t) + 1
	}
	out := make([]byte, 0, total)
	for i, t := range terms {
		if i > 0 {
			out = append(out, sep[0])
		}
		out = append(out, t...)
	}
	return string(out)
}

// renderConfigVersion is the single source of truth for "anything
// that changes how a row looks." Returns an FNV-1a hash over a
// fingerprint string built from every relevant input. The paged-
// row cache puts this in its group key so any toggle that affects
// row appearance — without changing the underlying data — wipes
// the cache and forces a fresh render.
//
// **Contract for new settings/state**: anything that affects what
// the renderer puts on a row MUST be folded into the fingerprint
// here. Categories to think about:
//
//  1. user-settings — flag column mode/visibility, theme, …
//  2. per-state knobs — column widths, frozen cols, hscroll, zen, …
//  3. per-Model UI state — anything Cell / Recolor / Marks reads
//  4. computed widths — gutter widths derive from settings + state
//
// Forgetting to fold something in produces the "toggle X but rows
// don't update" bug. The fix in every case is to add a line here.
//
// FNV-1a chosen because it's tiny, deterministic, and order-
// preserving (the ordering of the writeXxx calls below matters —
// changing it would invalidate every existing cache entry, which
// is fine but would cause a one-frame stall after upgrade).
//
// Pass the active state so per-state fields participate; pass nil
// when called outside a list-table context.
func (m Model) renderConfigVersion(state *uilayout.ListTableState) int {
	const (
		fnvOffset uint64 = 14695981039346656037
		fnvPrime  uint64 = 1099511628211
	)
	h := fnvOffset
	mix := func(b byte) {
		h ^= uint64(b)
		h *= fnvPrime
	}
	mixInt := func(n int) {
		for i := 0; i < 8; i++ {
			mix(byte(n >> (i * 8)))
		}
		mix(0xff) // separator
	}
	mixBool := func(b bool) {
		if b {
			mix(1)
		} else {
			mix(0)
		}
		mix(0xff)
	}
	mixStr := func(s string) {
		for i := 0; i < len(s); i++ {
			mix(s[i])
		}
		mix(0xff)
	}

	// 1. User settings that affect row visuals.
	if m.settings != nil {
		mixStr(m.settings.FlagColumnDisplayMode())
		mixBool(m.settings.FlagColumnVisible())
		mixStr(m.settings.Theme())
	}
	// 2. Computed widths (gutters depend on settings + state).
	mixInt(m.tagGutterWidth())
	mixInt(m.projectGutterWidth())
	// 3. Per-state knobs that change what each row looks like.
	if state != nil {
		mixInt(state.HScroll)
		mixInt(state.FrozenCols)
		mixBool(state.Zen)
		// User-pinned column widths are a map; iterate sorted so
		// the hash is deterministic across runs. Map iteration
		// order is non-deterministic in Go, so unsorted would
		// produce different hashes for the same config.
		mixInt(len(state.UserWidths))
		if len(state.UserWidths) > 0 {
			names := make([]string, 0, len(state.UserWidths))
			for n := range state.UserWidths {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				mixStr(n)
				mixInt(state.UserWidths[n])
			}
		}
	}
	return int(h)
}

// dataVersionFromStore returns the devproject store's mutation
// generation, or 0 when the store isn't open. Folded into per-list
// DataVersion fields so tag / project changes invalidate the
// paged-row cache (gutters change even when the underlying list
// itself didn't).
func dataVersionFromStore(m Model) int {
	if m.devProjects == nil {
		return 0
	}
	return int(m.devProjects.Generation())
}

// listVersionWithStore is the canonical way for a listSurface's
// BuildRenderModel to compute its DataVersion. Combines the
// underlying ListView's version (bumps on Set / SetExtra / SetMatch)
// with the devproject store's mutation generation (bumps on tag /
// project edits). The constants serve as a "rotation" so the two
// counters don't trivially collide; +1 keeps it strictly positive
// so the cache wrapper (which treats 0 as "no cache, fall through")
// engages.
//
// Surfaces whose row content depends on additional state outside
// these two should fold that in themselves — e.g. by adding a
// constant offset per chip selection, or by including a struct-time
// counter. The cache treats DataVersion as opaque.
func listVersionWithStore(listVer int, m Model) int {
	return listVer*1_000_003 + dataVersionFromStore(m) + 1
}

// scopeKeyForRowCache returns a stable identifier for the active
// org scope so the row cache invalidates on org switch. Cheap
// concatenation; retained as one cache-key field across the whole
// page render.
func scopeKeyForRowCache(m Model) string {
	if scope := m.activeScope(); scope != nil {
		return scope.ProjectName
	}
	return ""
}

func cacheLayers(layers []*lipgloss.Layer) []cachedLayer {
	out := make([]cachedLayer, 0, len(layers))
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		out = append(out, cachedLayer{
			text: layer.GetContent(),
			id:   layer.GetID(),
			x:    layer.GetX(),
			y:    layer.GetY(),
			z:    layer.GetZ(),
		})
	}
	return out
}

func (b cachedLayerBlock) materialize() []*lipgloss.Layer {
	layers := make([]*lipgloss.Layer, 0, len(b.layers))
	for _, layer := range b.layers {
		layers = append(layers, lipgloss.NewLayer(layer.text).
			X(layer.x).
			Y(layer.y).
			Z(layer.z).
			ID(layer.id))
	}
	return layers
}

func (m Model) tabBarCacheKey(width int) tabBarCacheKey {
	scope := m.activeScope()
	scopeLoaded := scope.Loaded()
	scopeName := ""
	if scope != nil {
		scopeName = scope.ProjectName
	}
	searchApplied := false
	searchBuf := ""
	if s := m.searchStateForTab(m.tab()); s != nil {
		searchApplied = s.Applied()
		searchBuf = s.Buffer()
	}
	return tabBarCacheKey{
		width:                   width,
		tab:                     m.tab(),
		stem:                    m.stemForTab(m.tab()),
		focus:                   m.focus,
		scopeLoaded:             scopeLoaded,
		scopeName:               scopeName,
		devProjectCur:           m.devProjectCur,
		searchApplied:           searchApplied,
		leftOpen:                m.leftOpen,
		overflowSet:             m.overflowSet,
		overflowTab:             m.overflowTab,
		searchBuf:               searchBuf,
		allTabSearchFingerprint: m.allTabSearchFingerprint(),
	}
}

// allTabSearchFingerprint returns a string with one rune per top-level
// tab — "1" if that tab has a search currently Applied, "0" otherwise.
// Order matches TabsForNumbers so positions stay stable.  Folded into
// the tab-bar cache key so the magnifier-glyph set invalidates when
// ANY tab's search state changes, not just the active one.
//
// Cheap: walks ~9 tabs and reads in-memory state.  No allocations
// per pill — one byte slice per cache-key build.
func (m Model) allTabSearchFingerprint() string {
	tabs := TabsForNumbers()
	buf := make([]byte, 0, len(tabs))
	for _, v := range tabs {
		if s := m.searchStateForTab(v); s != nil && s.Applied() {
			buf = append(buf, '1')
		} else {
			buf = append(buf, '0')
		}
	}
	return string(buf)
}

func (m Model) leftTabBarCacheKey(width int) leftTabBarCacheKey {
	return leftTabBarCacheKey{
		width:          width,
		focus:          m.focus,
		leftUtilityIdx: m.leftUtilityIdx,
	}
}

func (m Model) statusBarCacheKey() statusBarCacheKey {
	bannerActive := time.Now().Before(m.bannerUntil) && m.banner != ""
	searchCommitted := false
	searchBuf := ""
	if s := m.searchStateForTab(m.tab()); s != nil {
		searchCommitted = s.Committed
		searchBuf = s.Buffer()
	}
	hasListTable := false
	if state := (&m).activeListTableState(); state != nil {
		hasListTable = true
	}
	exportActive := false
	if m.exports != nil {
		exportActive = m.exports.hasInflight()
	}
	return statusBarCacheKey{
		width:           m.width,
		tab:             m.tab(),
		subtab:          m.currentSubtab(),
		dashboardClosed: m.dashboardCollapsed,
		searchCommitted: searchCommitted,
		searchBuf:       searchBuf,
		hasListTable:    hasListTable,
		subtabCount:     len(m.tabSubtabs()),
		bannerActive:    bannerActive,
		banner:          m.banner,
		exportActive:    exportActive,
		exportFrame:     m.exportActivityFrame,
		sidebarOpen:     m.sidebarOpen,
		canEscBack:      m.canEscBack(),
	}
}
