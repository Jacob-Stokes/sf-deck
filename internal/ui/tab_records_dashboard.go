package ui

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// Chip strip for the /records / Object-drill Records subtab.
//
// Two modes coexist on the same surface (toggled by the chip-mode
// switch keybinding):
//
//   ChipModeLocal      chips come from the unified qchip registry —
//                      built-in chips + user-saved ones in settings.
//   ChipModeSalesforce chips are the org's own ListView records,
//                      fetched via /sobjects/<X>/listviews and
//                      cached in d.ListViewsPerSObject.
//
// Chip IDs:
//   ChipModeLocal       a qchip.Chip.ID, e.g. "recent" / "today"
//   ChipModeSalesforce  a Salesforce ListView Id, e.g. "00BgL00000VTKjd"

// syntheticRecentID is the canonical "always-shipped" chip id. Kept
// as a constant for places that need to special-case the default.
const syntheticRecentID = "recent"

// currentChipMode returns the active mode for sobject, defaulting to
// ChipModeLocal so first-visit users see sf-deck chips.
func currentChipMode(d *orgData, sobject string) ChipMode {
	if d == nil {
		return ChipModeLocal
	}
	return d.ChipMode[sobject]
}

// setChipMode persists the mode for sobject + clears the chip
// selection so a stale id from the other mode doesn't survive the
// switch.
func setChipMode(d *orgData, sobject string, mode ChipMode) {
	if d == nil {
		return
	}
	d.ChipMode[sobject] = mode
	delete(d.ListViewCur, sobject)
}

// activeRecordsSObject returns the sobject the chip strip + record
// list are currently driven by — d.DescribeCur on the Object-drill
// Records subtab, d.RecordsSObjectCur on the legacy /records tab,
// or "" when neither surface is active.
func (m Model) activeRecordsSObject() (*orgData, string) {
	if len(m.orgs) == 0 {
		return nil, ""
	}
	d := m.data[m.orgs[m.selected].Username]
	if d == nil {
		return nil, ""
	}
	switch m.tab() {
	case TabObjectDetail:
		if m.currentSubtab() == SubtabRecords && d.DescribeCur != "" {
			return d, d.DescribeCur
		}
	case TabRecords:
		if d.RecordsSObjectCur != "" {
			return d, d.RecordsSObjectCur
		}
	}
	return nil, ""
}

// toggleChipMode flips the chip-strip mode on the active records
// surface, OR the data-source mode on the active /home → Recent
// surface.  Same key (L by default) drives both because the user-
// facing semantic is identical: "show me Salesforce's view of this,
// not sf-deck's."
//
// No-op on tabs that don't participate.
func (m Model) toggleChipMode() (Model, tea.Cmd) {
	// /home → Recent: flips d.HomeRecentMode between sf-deck local
	// log and Salesforce RecentlyViewed.  Kicks the SF fetch when
	// flipping into Salesforce mode so the list isn't empty on
	// first switch.
	if m.tab() == TabHome && m.currentSubtab() == SubtabHomeRecent {
		d := m.activeOrgData()
		if d == nil {
			return m, nil
		}
		if d.HomeRecentMode == ChipModeLocal {
			d.HomeRecentMode = ChipModeSalesforce
			m.flash("source: Salesforce RecentlyViewed")
			// Re-apply chip predicate to the newly-active list so the
			// "All" chip's listview filter actually fires on the SF
			// list.  Without this, switching modes leaves the SF list
			// with no Extra set and the user sees raw unfiltered rows.
			m.applySelectedChipMatcher(d)
			return m, d.RecentlyViewed.Ensure(m.cache)
		}
		d.HomeRecentMode = ChipModeLocal
		m.flash("source: sf-deck visit log")
		m.applySelectedChipMatcher(d)
		return m, nil
	}
	d, sobj := m.activeRecordsSObject()
	if sobj == "" {
		return m, nil
	}
	if currentChipMode(d, sobj) == ChipModeLocal {
		setChipMode(d, sobj, ChipModeSalesforce)
		m.flash("source: Salesforce list views")
	} else {
		setChipMode(d, sobj, ChipModeLocal)
		m.flash("source: sf-deck views")
	}
	return m, m.onTabChanged()
}

// recordsChips builds the chip strip for the active mode. Local pulls
// from the unified registry; Salesforce pulls from cached SF list views.
func recordsChips(m Model, d *orgData, sobject string) []chipRow {
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		return salesforceListViewChips(d, sobject)
	}
	return localChips(m, sobject)
}

// localChips builds chips from the unified records registry. Strip
// shape = favourites + transient slot + overflow sentinel. Falls back
// to a synthetic "Recent" if the registry's somehow empty.
func localChips(m Model, sobject string) []chipRow {
	if m.chipRegistry(domainRecords) == nil {
		return []chipRow{{ID: syntheticRecentID, Label: "Recent", Count: -1}}
	}
	rows := m.stripRows(domainRecords, sobject)
	if len(rows) == 0 {
		rows = append(rows, chipRow{ID: syntheticRecentID, Label: "Recent", Count: -1})
	}
	return rows
}

// salesforceListViewChips builds chips from the cached SF list-view
// metadata. Empty slice when nothing's been fetched yet (the caller
// shows a "loading…" hint).
//
// Always prepends a synthetic "Recently Viewed" chip at position 0
// — works for every sObject (standard + custom) because it queries
// the cross-object RecentlyViewed system table directly rather than
// relying on SF's per-sObject "RecentlyViewed<X>" list view
// (auto-created for standard sObjects only).  Pairs with local
// mode's recentlyViewedChipID default.
//
// Returns nil when the underlying list-view catalog hasn't loaded
// yet — caller treats that as "loading…".  The synthetic chip
// itself doesn't need the catalog, but we want users to see the
// full strip on first paint, not just the synthetic chip.
func salesforceListViewChips(d *orgData, sobject string) []chipRow {
	r, ok := d.ListViewsPerSObject[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return nil
	}
	views := r.Value()
	chips := make([]chipRow, 0, len(views)+1)
	chips = append(chips, chipRow{
		ID:    sfRecentlyViewedChipID,
		Label: "Recently Viewed",
		Count: -1,
	})
	for _, lv := range views {
		label := lv.Name
		if !lv.IsSoqlCompatible {
			label = lv.Name + " ·"
		}
		chips = append(chips, chipRow{ID: lv.ID, Label: label, Count: -1})
	}
	return chips
}

// sfRecentlyViewedChipID is the sentinel id for the synthetic
// "Recently Viewed" chip in Salesforce source mode.  Selected
// chips route through the records fetch dispatcher: when the
// active chip id matches this sentinel, the fetcher calls
// sf.ListRecentlyViewed(alias, {SObject, Limit}) instead of
// /sobjects/<X>/listviews/<id>/results.
const sfRecentlyViewedChipID = "__sf_recent__"

// selectedRecordsChip returns the currently-selected chip's id for
// the given sObject + active mode. Falls back to the first chip in
// the active mode's list — the registry default ("recent") for
// local, the first Salesforce list-view id for Salesforce mode.
//
// Returns "" when Salesforce mode is active but the list-view
// catalog hasn't loaded yet — the renderer treats that as "loading…".
func selectedRecordsChip(d *orgData, sobject string) string {
	if id, ok := d.ListViewCur[sobject]; ok && id != "" {
		return id
	}
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		// Default to the synthetic Recently Viewed chip — works for
		// every sObject (standard or custom) by querying the
		// RecentlyViewed system table directly rather than relying on
		// SF auto-creating a per-sObject "RecentlyViewed<X>" list
		// view (which doesn't exist for custom objects).  Matches the
		// local-mode default of recentlyViewedChipID.
		return sfRecentlyViewedChipID
	}
	// Default on first visit: Recently viewed.  Matches the chip
	// strip's index-0 layout (RV is prepended by stripRows) and
	// avoids the 10k-row "Changed" fetch firing eagerly.  When the
	// user has no recent visits for this sObject the visited chip
	// renders an empty list with a hint pointing at "Changed".
	//
	// Caller (the records resource resolver) treats this id as
	// "no fetch needed" when no visited records exist — see
	// activateRecords / tab_object_hooks.go's records subtab branch.
	return recentlyViewedChipID
}

// findChipIndex returns the index of the chip with the given ID, or 0
// if not found (fallback to first chip).
func findChipIndex(chips []chipRow, id string) int {
	for i, c := range chips {
		if c.ID == id {
			return i
		}
	}
	return 0
}

// findListView returns the ListView metadata row for a given ID in the
// cached list, if present.
func findListView(d *orgData, sobject, id string) (sf.ListView, bool) {
	r, ok := d.ListViewsPerSObject[sobject]
	if !ok || r.FetchedAt().IsZero() {
		return sf.ListView{}, false
	}
	for _, lv := range r.Value() {
		if lv.ID == id {
			return lv, true
		}
	}
	return sf.ListView{}, false
}

// currentRecordsResource returns the Resource backing the
// currently-selected chip on (sobject). After the qchip cutover every
// chip — including the default "recent" — runs through EnsureChipRecords
// and lives in d.ChipRecords keyed by "<sobject>:<chipId>". The legacy
// d.Records path is only consulted as a backstop for the rare case
// where ensure went through the old EnsureRecords helper (e.g. a chip
// with no WHERE / ORDER / LIMIT at all).
func currentRecordsResource(d *orgData, sobject string) *Resource[sf.RecordsList] {
	if d == nil {
		return nil
	}
	selected := selectedRecordsChip(d, sobject)
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		// SF mode normally routes through d.ListViewResults (rendered
		// by renderListViewResult).  The synthetic SF Recently Viewed
		// chip is an exception — it produces records via
		// EnsureChipRecords (SOQL `Id IN (visited-ids)`) which lands
		// in d.ChipRecords, same as local-mode chips.  So look it up
		// there before falling back to nil.
		if selected == sfRecentlyViewedChipID {
			if r, ok := d.ChipRecords[sobject+":"+selected]; ok {
				return r
			}
		}
		return nil
	}
	if r, ok := d.ChipRecords[sobject+":"+selected]; ok {
		return r
	}
	if selected == syntheticRecentID {
		if r, ok := d.Records[sobject]; ok {
			return r
		}
	}
	return nil
}

// activeChipBusy reports whether the resource currently driving the
// on-screen records is fetching. Mirrors activeChipRefreshCmd's
// resolution rules so the header label tracks what `r` actually
// triggers.
func activeChipBusy(d *orgData, sobject string) bool {
	if d == nil || sobject == "" {
		return false
	}
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		selected := selectedRecordsChip(d, sobject)
		if selected == sfRecentlyViewedChipID {
			// Synthetic chip routes through d.ChipRecords (see
			// currentRecordsResource for the same branch).
			if rv := d.RecentlyViewedPerSObject[sobject]; rv != nil && rv.Busy() {
				return true
			}
			if r, ok := d.ChipRecords[sobject+":"+selected]; ok {
				return r.Busy()
			}
			return false
		}
		key := sobject + ":" + selected
		if r, ok := d.ListViewResults[key]; ok {
			return r.Busy()
		}
		return false
	}
	if r := currentRecordsResource(d, sobject); r != nil {
		return r.Busy()
	}
	return false
}

// activeChipRefreshCmd returns the refresh command for whichever
// resource is currently driving the on-screen records — the chip's
// records list (local mode) or the SF list-view result (SF mode).
// Returns nil when nothing's been wired up yet (first paint, etc).
//
// Used by `r` so refresh re-pulls *only* what's on screen — no piggy-
// backed sObject-list or describe re-fetches that have nothing to do
// with the data the user is staring at.
func (m Model) activeChipRefreshCmd(d *orgData, sobject string) tea.Cmd {
	if d == nil || sobject == "" {
		return nil
	}
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		selected := selectedRecordsChip(d, sobject)
		if selected == sfRecentlyViewedChipID {
			o, ok := m.currentOrg()
			if !ok {
				return nil
			}
			rv := d.EnsureRecentlyViewedPerSObject(targetArg(o), sobject)
			return rv.Refresh(m.cache)
		}
		key := sobject + ":" + selected
		if r, ok := d.ListViewResults[key]; ok {
			return r.Refresh(m.cache)
		}
		return nil
	}
	if r := currentRecordsResource(d, sobject); r != nil {
		return r.Refresh(m.cache)
	}
	return nil
}

// currentRecordRowCount returns the number of rows the currently-
// selected chip would show *after the user's search filter* — used by
// moveCursor + cursorOpenable so they bound the cursor to the rows
// actually on screen. Salesforce-mode list views aren't searched
// client-side yet (their filtering lives in the SF list-view def);
// only sf-deck-chip mode honours the search buffer here.
func currentRecordRowCount(d *orgData, sobject string) int {
	visible, _ := visibleRecordsAndIdx(d, sobject)
	return len(visible)
}

// recordsVisibleEntry is a memoized (visible, visibleIdx) pair for
// one (sobject, chipID) combination. The cache lives on orgData and
// is consulted by visibleRecordsAndIdx on every call.
//
// Cache identity needs to change when ANY input that affects the
// pair changes — those are:
//  1. The underlying records slice (re-fetch / refresh / mode flip)
//  2. The columns slice (different chip / different SOQL projection)
//  3. The search buffer (live-edit narrows / widens the filter)
//  4. The "search applied" flag (Esc-to-cancel un-narrows)
//  5. The chipID (chip cycle changes the resource entirely, but
//     caller already keys the map by sobject+":"+chipID so this
//     naturally falls out of map lookup)
//
// We use slice header pointers via reflect.ValueOf().Pointer() for
// the "did the underlying slice change" check — same pattern as the
// gutter cache — because it's O(1) and changes whenever Set is
// called on the resource.
//
// Privacy: this is purely in-memory state on orgData. Records data
// already runs through Resource[RecordsList] with NoCache:true so
// no SF record content ever touches disk. The memo keeps that
// guarantee; nothing here persists past process exit.
type recordsVisibleEntry struct {
	rowsPtr    uintptr // slice header pointer of underlying records
	colsPtr    uintptr // slice header pointer of columns spec
	searchBuf  string  // search buffer at compute time
	searchOn   bool    // search-applied flag at compute time
	recencyOn  bool    // whether the recency reorder was applied
	recencyGen uint64  // d.recentGen at compute time (invalidates on visit)
	visible    []map[string]any
	visibleIdx []int
}

// visibleRecordsCache is the per-(sobject, chipID) memo store.
// Cleared wholesale on chip-mode flip / cache invalidation events
// the renderer can trigger explicitly. No size cap because each org
// only opens a handful of (sobject, chip) combinations per session
// and entries are cheap (two slice headers + two ints per entry).
type visibleRecordsCache map[string]*recordsVisibleEntry

// visibleRecordsAndIdx returns the current visible record set plus the
// parallel slice of unfiltered indices. Single source of truth for
// "what's on screen for this (sobject, chip)" — used by the renderer,
// cursor maths, and openable resolution. Returns (nil, nil) when
// data isn't loaded yet.
func visibleRecordsAndIdx(d *orgData, sobject string) ([]map[string]any, []int) {
	if currentChipMode(d, sobject) == ChipModeSalesforce {
		chipID := selectedRecordsChip(d, sobject)
		key := sobject + ":" + chipID
		if chipID == sfRecentlyViewedChipID {
			r := currentRecordsResource(d, sobject)
			if r == nil {
				return nil, nil
			}
			v := r.Value()
			s := d.RecordsSearchPtr(sobject, chipID)
			return memoVisibleRecords(d, key, v.Records, recordListFields(v), s.Effective(), s.EffectiveApplied(), false)
		}
		if r, ok := d.ListViewResults[key]; ok {
			recs := r.Value().Records
			// Same memo path for SF list-view chips. cols is "" since
			// list-view results don't go through the column-aware
			// search filter; we still use the slice pointer + len
			// pair as the identity key. Recency reorder doesn't
			// apply on SF list-view chips — those rows are whatever
			// the user's saved view returned.
			return memoVisibleRecords(d, key, recs, nil, "", false, false)
		}
		return nil, nil
	}
	r := currentRecordsResource(d, sobject)
	if r == nil {
		return nil, nil
	}
	v := r.Value()
	chipID := selectedRecordsChip(d, sobject)
	s := d.RecordsSearchPtr(sobject, chipID)
	// Recency reorder kicks in when the synthetic Recently Viewed chip
	// is active AND there's no column sort overriding it.  Mirrors the
	// chipSurface-based surfaces' defaultOrder semantics: column sort
	// always wins, recency fills in when none is set.
	recencyOn := chipID == recentlyViewedChipID &&
		d.RecordsTableStatePtr(sobject, chipID).SortColumn == ""
	return memoVisibleRecords(d, sobject+":"+chipID, v.Records, recordListFields(v), s.Effective(), s.EffectiveApplied(), recencyOn)
}

func recordsVisibleSortDataKey(d *orgData, sobject string) string {
	if d == nil || d.visibleRecordsCache == nil {
		return sobject
	}
	chipID := selectedRecordsChip(d, sobject)
	entry, ok := d.visibleRecordsCache[sobject+":"+chipID]
	if !ok || entry == nil {
		return sobject + ":" + chipID
	}
	var b strings.Builder
	b.WriteString(sobject)
	b.WriteByte(':')
	b.WriteString(chipID)
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(uint64(entry.rowsPtr), 10))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(len(entry.visibleIdx)))
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(uint64(entry.colsPtr), 10))
	b.WriteByte('|')
	if entry.searchOn {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	b.WriteString(entry.searchBuf)
	b.WriteByte('|')
	if entry.recencyOn {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(entry.recencyGen, 10))
	return b.String()
}

// memoVisibleRecords is the cache shim around the
// "build (visible, visibleIdx) for these inputs" computation. Per-
// (sobject, chipID) entry; cache hit returns the previously-built
// pair untouched. On miss, runs filterRecords (search applied) or
// builds an identity-index pair (no search) and stores both for
// next call.
//
// The CRITICAL property: a steady-state wheel-scroll burst —
// where the user is moving cursor through a stable record set
// without typing or refetching — produces only cache hits. That's
// what closes the perf gap with /objects (whose Filtered() memo
// gives the same property).
func memoVisibleRecords(
	d *orgData,
	cacheKey string,
	rows []map[string]any,
	cols []string,
	searchBuf string,
	searchApplied bool,
	recencyOn bool,
) ([]map[string]any, []int) {
	if d == nil {
		return rows, identityIdx(len(rows))
	}
	if d.visibleRecordsCache == nil {
		d.visibleRecordsCache = visibleRecordsCache{}
	}
	rowsPtr := slicePtrAny(rows)
	colsPtr := slicePtrStr(cols)
	// recencyGen tracks d.recentGen at compute time so a visit (which
	// bumps recentGen via rememberRecent / RecentlyViewed.Apply) cache-
	// misses and triggers a re-sort.  When recencyOn is false the gen
	// field carries 0 — value doesn't matter, the recencyOn flag handles
	// hit/miss alone.
	curGen := uint64(0)
	if recencyOn {
		curGen = d.recentGen
	}
	if entry, ok := d.visibleRecordsCache[cacheKey]; ok {
		if entry.rowsPtr == rowsPtr &&
			entry.colsPtr == colsPtr &&
			entry.searchOn == searchApplied &&
			entry.searchBuf == searchBuf &&
			entry.recencyOn == recencyOn &&
			entry.recencyGen == curGen {
			return entry.visible, entry.visibleIdx
		}
	}
	var visible []map[string]any
	var visibleIdx []int
	if !searchApplied {
		// Copy rows into a fresh slice when no search is active.  Earlier
		// versions aliased `visible = rows` which made an in-place sort
		// (e.g. recency reorder below) mutate the resource's underlying
		// SF result. Always own our own slice from here on.
		visible = append([]map[string]any(nil), rows...)
		visibleIdx = identityIdx(len(rows))
	} else {
		visible, visibleIdx = filterRecords(rows, cols, searchBuf)
	}
	if recencyOn && len(visible) > 1 {
		// Derive the rank map from the union of local + SF recent
		// streams — that's the user's "what have I touched recently
		// for this sObject" intent.  Compute happens on cache miss
		// only; subsequent reads of the cache entry are O(1) and
		// don't touch this code.
		rank := rankRecordsFromStream(recentUnionStream(d), sobjectFromCacheKey(cacheKey))
		if len(rank) > 0 {
			perm := recencyPermutation(visible, rank)
			visible, visibleIdx = applyRecordsPerm(visible, visibleIdx, perm)
		}
	}
	d.visibleRecordsCache[cacheKey] = &recordsVisibleEntry{
		rowsPtr:    rowsPtr,
		colsPtr:    colsPtr,
		searchBuf:  searchBuf,
		searchOn:   searchApplied,
		recencyOn:  recencyOn,
		recencyGen: curGen,
		visible:    visible,
		visibleIdx: visibleIdx,
	}
	return visible, visibleIdx
}

// sobjectFromCacheKey extracts the sObject API name from the
// "<sobject>:<chipID>" cacheKey shape used by memoVisibleRecords.
// Returns the full key when no ':' is present (defensive — the SF
// list-view branch uses the same shape).
func sobjectFromCacheKey(cacheKey string) string {
	for i := 0; i < len(cacheKey); i++ {
		if cacheKey[i] == ':' {
			return cacheKey[:i]
		}
	}
	return cacheKey
}

// recencyPermutation returns the display→source index permutation
// that sorts `rows` by recency rank (rank 0 = most recent at top).
// Rows not present in `rank` sort to the end, preserving their
// natural order among themselves (stable sort).
func recencyPermutation(rows []map[string]any, rank map[string]int) []int {
	const unranked = 1 << 30
	perm := make([]int, len(rows))
	ranks := make([]int, len(rows))
	for i := range rows {
		perm[i] = i
		id, _ := rows[i]["Id"].(string)
		r, ok := rank[id]
		if !ok {
			ranks[i] = unranked
			continue
		}
		ranks[i] = r
	}
	sortStableByRank(perm, ranks)
	return perm
}

// sortStableByRank is an insertion sort over `perm` by ranks[perm[i]].
// Stable and tiny — visible record sets cap at the SOQL preview limit
// (50 by default), so O(N^2) is comfortably under the per-frame budget
// and avoids a sort.SliceStable closure allocation on every miss.
func sortStableByRank(perm, ranks []int) {
	for i := 1; i < len(perm); i++ {
		for j := i; j > 0 && ranks[perm[j]] < ranks[perm[j-1]]; j-- {
			perm[j], perm[j-1] = perm[j-1], perm[j]
		}
	}
}

// applyRecordsPerm returns fresh (rows, idx) slices reordered by perm.
// Never mutates the inputs — both slices are allocation-fresh so
// downstream cursor math gets a stable view even when rows aliased
// the resource's backing array.
func applyRecordsPerm(rows []map[string]any, idx []int, perm []int) ([]map[string]any, []int) {
	if len(perm) != len(rows) || len(perm) != len(idx) {
		return rows, idx
	}
	outRows := make([]map[string]any, len(rows))
	outIdx := make([]int, len(idx))
	for i, src := range perm {
		outRows[i] = rows[src]
		outIdx[i] = idx[src]
	}
	return outRows, outIdx
}

// slicePtrAny / slicePtrStr return the underlying slice-header
// pointer for cache identity. Same trick as gutter_cache.go:
// reflect.ValueOf(slice).Pointer() is the address of slice header
// element 0; changes whenever Set() / append-grow / refetch
// produces a fresh backing array. Cheap O(1) compare in the cache
// hit path.
func slicePtrAny(s []map[string]any) uintptr {
	if len(s) == 0 {
		return 0
	}
	return reflect.ValueOf(s).Pointer()
}

func slicePtrStr(s []string) uintptr {
	if len(s) == 0 {
		return 0
	}
	return reflect.ValueOf(s).Pointer()
}

func recordsCursorDisplay(d *orgData, sobject string) DisplayRow {
	visible, visibleIdx := visibleRecordsAndIdx(d, sobject)
	if len(visibleIdx) == 0 {
		return 0
	}
	return recordsRowAdapter(d, sobject, visible, visibleIdx).DisplayCursor()
}

func recordsRowAdapter(d *orgData, sobject string, visible []map[string]any, visibleIdx []int) tableRowAdapter {
	n := len(visibleIdx)
	if len(visible) > 0 || n == 0 {
		n = len(visible)
	}
	var cols []uilayout.ListColumn
	var cell func(row, col int) string
	var state *uilayout.ListTableState
	if c, fn, st, ok := recordsSortContext(d, sobject, visible); ok {
		cols = c
		cell = fn
		state = st
		n = len(visible)
	}
	return tableRowAdapter{
		State:        state,
		Cols:         cols,
		N:            n,
		Cell:         cell,
		VisibleToRaw: visibleIdx,
		DataKey:      recordsVisibleSortDataKey(d, sobject),
		// The cursor store persists plain ints; these closures are the
		// sanctioned raw<->stored conversion seam.
		RawCursor: func() RawRow {
			if d == nil {
				return 0
			}
			return RawRow(d.Cursors.Peek(cursorKindRecordsRow, sobject))
		},
		SetRawCursor: func(raw RawRow) {
			if d != nil {
				d.Cursors.Set(cursorKindRecordsRow, int(raw), 0, sobject)
			}
		},
	}
}

// recordsMoveCursor advances the cursor by delta in *display* space,
// then translates the new visible row back to its unfiltered index
// and saves that. So filter changes preserve cursor identity instead
// of resetting to top.
func recordsMoveCursor(d *orgData, sobject string, delta int) {
	visible, visibleIdx := visibleRecordsAndIdx(d, sobject)
	if len(visibleIdx) == 0 {
		return
	}
	recordsRowAdapter(d, sobject, visible, visibleIdx).MoveDisplay(delta)
}

// currentRecordAt returns the display row at idx as a map, for
// cursorOpenable / sidebar / breadcrumb. idx is in the same
// display-coordinate space renderListModel uses after column sorting.
// Returns (nil, false) if data isn't loaded or idx is out of range.
// currentRecordAt resolves the record at a DISPLAY row (what the user
// sees, post-sort). Callers holding a raw cursor must convert through
// the row space first.
func currentRecordAt(d *orgData, sobject string, idx DisplayRow) (map[string]any, bool) {
	visible, visibleIdx := visibleRecordsAndIdx(d, sobject)
	v, ok := recordsRowAdapter(d, sobject, visible, visibleIdx).VisibleAtDisplay(idx)
	if !ok || int(v) >= len(visible) {
		return nil, false
	}
	return visible[v], true
}

func recordsSortContext(d *orgData, sobject string, visible []map[string]any) ([]uilayout.ListColumn, func(row, col int) string, *uilayout.ListTableState, bool) {
	chipID := selectedRecordsChip(d, sobject)
	if chipID == "" {
		return nil, nil, nil, false
	}
	state := d.RecordsTableStatePtr(sobject, chipID)
	if currentChipMode(d, sobject) == ChipModeSalesforce && chipID != sfRecentlyViewedChipID {
		key := sobject + ":" + chipID
		r, ok := d.ListViewResults[key]
		if !ok {
			return nil, nil, nil, false
		}
		result := r.Value()
		cols := visibleColumns(result.Columns)
		if len(cols) == 0 {
			cols = []sf.ListViewColumn{{Name: "Id", Label: "Id"}}
		}
		resolved := resolveListViewColumns(cols, visible)
		return resolved.ListColumns(), resolved.Cell(visible), state, true
	}
	r := currentRecordsResource(d, sobject)
	if r == nil {
		return nil, nil, nil, false
	}
	list := r.Value()
	search := d.RecordsSearchPtr(sobject, chipID)
	projection := recordsProjectionFor(d, sobject, chipID, list, visible, search)
	return projection.cols, projection.cell, state, true
}

// recordsFetchedAt / recordDetailFetchedAt resolve the /records and
// /record drills' primary freshness stamps (extracted in the
// registry-purity pass — see tab_registry_purity_test.go).
func recordsFetchedAt(m Model, d *orgData) time.Time {
	if d.RecordsSObjectCur != "" {
		if r, ok := d.Records[d.RecordsSObjectCur]; ok {
			return r.FetchedAt()
		}
	}
	return d.SObjects.FetchedAt()
}

func recordDetailFetchedAt(m Model, d *orgData) time.Time {
	if d.RecordDetailCur != "" {
		if r, ok := d.RecordDetails[d.RecordDetailCur]; ok {
			return r.FetchedAt()
		}
	}
	return time.Time{}
}
