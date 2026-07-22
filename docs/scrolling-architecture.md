# Scrolling, Cursor, And Render Performance Architecture

This document describes how sf-deck scrolls list-like views today: cursor movement, row coordinate systems, list-table rendering, wheel handling, pagination, and the in-memory caches that keep scrolling responsive.

This is the current reference for the `RowSpace`, shared list renderer,
pagination, and column-width persistence architecture.

## Short Version

Most scrolling in sf-deck is cursor-driven, not pixel-driven. Keyboard and wheel input produce a row delta. That delta is routed to the active surface's cursor owner. Rendering then shows the page or viewport containing that cursor.

For ordinary list surfaces, the cursor lives inside `resource.ListView[T]`.

For records, SOQL results, reports, and a few bespoke grids, the cursor is stored in raw backing-row coordinates and translated through `RowSpace` into the visible/sorted display row used by the renderer.

Performance depends on a small set of invariants:

- stable scroll over unchanged data must hit the same filtered/projection caches;
- sorted views must reuse one shared sort permutation per row set;
- paginated mode should re-render only the cursor row plus any newly visible rows;
- render-only events that are deliberately dropped should reuse the last frame;
- every cache key must include the data identity that affects row order and row appearance.

## Key Files

- `internal/ui/update_nav.go` - keyboard cursor routing, jump sizes, page jump.
- `internal/ui/update_wheel.go` - mouse wheel throttling and accepted wheel deltas.
- `internal/ui/uilayout/viewport.go` - row windowing and pagination row slicing.
- `internal/ui/uilayout/listtable.go` - list-table state, column widths, horizontal scroll, sort permutations.
- `internal/ui/listrender_model.go` - shared per-frame list-table renderer.
- `internal/ui/render_cache.go` - frame, chrome, dashboard, and paged-row render caches.
- `internal/ui/resource/listview.go` - generic `ListView[T]` cursor, filtering, search, default order, and memoization.
- `internal/ui/sort_cursor.go` - `RowSpace`, display/visible/raw row translation, shared sort cache key.
- `internal/ui/tab_records_dashboard.go` - records visible-row memo, records cursor movement, records sort context.
- `internal/ui/tab_records.go` - records projection cache and records shared-renderer handoff.
- `internal/ui/tab_soql.go` - SOQL projection cache and `RowSpace` use.
- `internal/ui/tab_reports.go` and `internal/ui/tab_reports_hooks.go` - report-run grid rendering and cursor movement.
- `internal/ui/listtable_keys.go` - column resize, horizontal scroll, sort, pagination, and active table discovery.

## Row Coordinate Systems

The most important concept is that a row can have several indexes at once.

`RawRow`

The index in the backing data slice as fetched or stored. Example: `d.Records[sobject].Value().Records[17]`.

`VisibleRow`

The index after local filters/search/default ordering have produced the current visible slice, but before a user column sort is applied.

`DisplayRow`

The row the user sees after column sort. If the user sorts by `Name`, display row `0` may point to visible row `19`.

`Page row`

The row's position inside the currently rendered page. This is not stored as persistent state; it is derived during render.

`RowSpace` in `internal/ui/sort_cursor.go` owns translation between these spaces:

- `RawToVisible`
- `RawToNearestVisible`
- `VisibleToRaw`
- `DisplayToVisible`
- `VisibleToDisplay`
- `DisplayToRaw`
- `RawToDisplay`

`tableRowAdapter` in `internal/ui/table_row_adapter.go` is the shared wrapper around `RowSpace` for table-shaped surfaces whose stored cursor is raw but whose UI moves in display space. Records, SOQL results, report run rows, and Salesforce list-view result rows use this adapter so move/reset/render/open paths share the same translation rules.

The rule of thumb: if a cursor is meant to survive filtering, store it in raw coordinates and translate at render/move/open time. If a view is a simple in-memory list with no separate backing coordinate system, use `ListView[T]`.

## Cursor Ownership

### ListView-backed surfaces

Most standard list surfaces use `resource.ListView[T]`:

- objects;
- flows;
- Apex classes/triggers and related static list surfaces;
- saved SOQL and history lists;
- exec saved snippets and history;
- home/download/notification-style lists;
- many tag/project/devproject lists.

`ListView[T]` owns:

- raw items;
- filtered cursor;
- search state;
- optional chip filter (`extra`);
- optional explicit order from column sort (`order`);
- optional default order such as Recently Viewed recency (`defaultOrder`);
- a memoized `Filtered()` result.

Cursor movement usually becomes:

```go
lv.MoveBy(delta)
```

`MoveBy` clamps over `len(lv.Filtered())`, so large deltas can implement page down, go top, and go bottom.

### Bespoke cursor surfaces

Some views cannot use a single `ListView[T]` cursor because their display row and backing row differ:

- `/records` record-list mode;
- `/objects/<sobject>/Records`;
- SOQL result grids;
- report run rows;
- Salesforce list-view results;
- permission/detail grids and code bodies.

These use either `CursorStore` or model-level cursor fields and translate through `RowSpace` when sorting/searching is involved.

`CursorStore` lives in `internal/ui/cursors.go`. It replaces many per-feature cursor maps with keyed cursor namespaces such as:

- `cursorKindRecordsRow`;
- `cursorKindReportRow`;
- `cursorKindFlowVersion`;
- `cursorKindAssignedUsers`;
- `cursorKindDevProjectItem`.

`CursorStore.Get` clamps reads. `CursorStore.Move` performs read/add/clamp/write. `CursorStore.Reset` snaps a key back to zero.

### Records cursor model

Records are the canonical raw/visible/display case.

The stored cursor is raw/unfiltered:

```go
d.Cursors.Peek(cursorKindRecordsRow, sobject)
```

`visibleRecordsAndIdx(d, sobject)` returns:

- `visible []map[string]any`
- `visibleIdx []int`, where `visibleIdx[visibleRow] = rawRow`

`recordsMoveCursor` translates the stored raw row to display row, applies the delta in display space, then stores the resulting raw row:

```go
adapter := recordsRowAdapter(d, sobject, visible, visibleIdx)
adapter.MoveDisplay(delta)
```

This is why cursor movement remains correct when a records list is searched, default-recency-ordered, or column-sorted.

## Input Flow

### Keyboard

Keyboard movement is handled in `internal/ui/update_keys.go` and routed through `Model.moveCursor` in `internal/ui/update_nav.go`.

Default movement bindings are defined in `internal/ui/keymap/commands.go`:

- `j` / Down: move down one row.
- `k` / Up: move up one row.
- `ctrl+down`: jump down by `settings.JumpRows()`, default `5`.
- `ctrl+up`: jump up by `settings.JumpRows()`, default `5`.
- `ctrl+d` / PageDown: jump by roughly half the terminal height.
- `ctrl+u` / PageUp: jump up by roughly half the terminal height.
- `G` / End: huge positive delta, clamped to bottom.
- `g` / Home: huge negative delta, clamped to top.

`pageJump(termHeight)` reserves about six rows for chrome and uses half of the remainder. It floors to `5` on small terminals.

`moveCursor` routes in this order:

1. If focus is on the left rail, route to the active rail utility.
2. If the active view resolves to a `listSurface`, call its `MoveCursor`.
3. Otherwise call the active tab/subtab `MoveCursor` hook.
4. Otherwise do nothing.

This registry-first path is important. New list-like surfaces should usually register `MoveCursor` instead of adding another top-level switch arm.

### Mouse wheel

Wheel input is handled in `internal/ui/update_wheel.go`.

The wheel is also cursor-driven. A wheel event becomes a row delta, then calls `moveCursor(delta)`.

There are two modes:

`handleWheelPaginated`

Used when the active list-table state has `Paginated=true`. This uses `wheelStepSimple`: one accepted event is one row, with a minimum interval from settings. It deliberately avoids momentum accumulation because paginated row caching makes each accepted render cheap and because terminal mouse protocols do not expose enough gesture phase information to separate finger movement from inertial tail reliably.

`handleWheelContinuous`

Used when pagination is off. This uses `wheelStep`: wheel events accumulate in `wheelRuntime.pending`, and accepted ticks drain up to `WheelMaxStep` rows. This preserves fast flick speed without forcing a render for every raw wheel packet.

Wheel settings live on `settings.UI.Input` and are exposed through:

- `JumpRows()`, default `5`;
- `WheelQuietGapMs()`, default `80`;
- `WheelMinIntervalMs()`, default `12`;
- `WheelMaxStep()`, default `20`.

When a wheel event is throttled or deferred, the model calls `skipNextFrameRender()`. The next `View` can reuse the previous frame instead of rebuilding the whole screen for an input that did not move the cursor.

## Rendering Pipeline

Most table-like surfaces now produce a `listRenderModel` and call `renderListModel`.

`listRenderModel` is a per-frame snapshot. It contains:

- title;
- `*uilayout.ListTableState`;
- search state;
- column definitions;
- row count;
- cursor display row;
- cell function;
- optional marks;
- optional left and right gutters;
- optional recolor function;
- empty text;
- footer extras;
- `DataVersion`;
- `SortDataKey`.

`renderListModel` then:

1. guards missing model parts;
2. clamps cursor;
3. renders title/search;
4. builds `uilayout.ListTableSpec`;
5. computes list-table widths through `LayoutListTable`;
6. computes the active sort permutation if rows are not already ordered;
7. renders either paginated rows or continuous viewport rows;
8. appends list-table hints.

The important split is:

- surfaces own data fetching, search/chip orchestration, and building cells;
- `renderListModel` owns row rendering, pagination, list-table chrome, sort application, and cache wrapping.

## ListTable State

`uilayout.ListTableState` stores per-surface table UI state:

- `HScroll`;
- `UserWidths`;
- `Zen`;
- `FrozenCols`;
- `ColCursor`;
- `SortColumn`;
- `SortDesc`;
- `RowsOrdered`;
- sort permutation cache;
- `Paginated`;
- `Page`.

The state is owned by the surface, not the renderer. That is why `/records` can keep separate column widths and pagination settings per `(sobject, chip)`, while SOQL and report run grids use model-level state.

Column controls:

- `<`: shrink cursored column.
- `>`: grow cursored column.
- `{`: snap column to minimum/header.
- `}`: snap column to widest measured cell.
- `W`: reset all user widths for the current table scope.
- Left / `h` / `,`: move column cursor left and horizontally scroll if needed.
- Right / `l` / `.`: move column cursor right and horizontally scroll if needed.
- `s`: cycle sort on the cursored column.
- `S`: clear sort.
- `P`: toggle pagination.
- `z`: toggle zen mode.

`activeListTableContext` in `internal/ui/listtable_keys.go` is the main discovery point for "which table is active right now?". It resolves subtab `ListTable`, tab `ListTable`, then generic `listSurface` state. It also applies persisted width preferences before returning the context.

## Pagination And Continuous Mode

Pagination is a per-table-state setting.

In paginated mode:

- page size is computed from the current row budget with `uilayout.PageSizeFor`;
- page index is derived from the current cursor every render;
- `RenderRowsPaged` renders rows `[page*pageSize, (page+1)*pageSize)`;
- the title is patched with `Page X / Y`;
- a page indicator and scrollbar are rendered;
- row rendering is wrapped by the paged-row cache.

In continuous mode:

- `RenderRows` computes a sliding window around the cursor;
- the cursor sits about one third down the pane when possible;
- a continuous scrollbar is rendered when not all rows fit;
- the paged-row cache is not used.

Operationally, paginated mode is the fast default for large tables because stable page boundaries make row caching highly effective. Continuous mode is still supported, but it can be more expensive on wide/high-cardinality grids because the visible window shifts constantly.

## In-Memory Caches

Scrolling performance comes from several narrow caches. They are intentionally scoped and invalidated by data identity rather than by broad global flushes.

### Resource memory

`Resource[T]` owns fetched data, busy/error state, timestamps, and optional disk cache behavior. During scrolling, the important property is that loaded resource values are already in memory; row rendering should not cause Salesforce calls.

Record and SOQL payloads are privacy-sensitive and generally use no disk cache. Their render/projection memos are process-local only.

### ListView.Filtered memo

`ListView[T].Filtered()` memoizes the current filtered/default-ordered slice by:

- list version;
- search buffer;
- active order key.

`version` bumps on `Set`, `SetExtra`, `SetScorer`, and `SetMatch`. `SetOrder` and `SetDefaultOrder` invalidate when their keys change.

This gives standard list surfaces the steady-state invariant: cursor movement over unchanged data returns the same filtered slice without rescanning the whole list.

### Records visible memo

`visibleRecordsAndIdx` uses `memoVisibleRecords` in `internal/ui/tab_records_dashboard.go`.

The memo key includes:

- records slice pointer;
- columns slice pointer;
- search buffer;
- search applied flag;
- recency-on flag;
- recent generation when recency ordering is active.

It stores both `visible` and `visibleIdx` so rendering, cursor movement, opening, and sidebar logic agree on the same row mapping.

This is the records equivalent of `ListView.Filtered()`. A stable wheel burst should hit this cache every time after the first frame.

### Records projection cache

`recordsProjectionFor` in `internal/ui/tab_records.go` builds dynamic record columns and a cell projection for the visible records. It avoids rebuilding column definitions and repeatedly formatting cells on every scroll tick.

Its identity includes the underlying rows, columns, visible slice, search state, and theme. It is in-memory only.

### SOQL projection cache

`soqlProjectionFor` in `internal/ui/tab_soql.go` performs the same job for SOQL results:

- collects dynamic columns;
- builds list columns;
- filters searched rows;
- precomputes cell access;
- stores the filtered-index mapping.

The cache lives on `orgData` so value-receiver `Model` copies do not accidentally lose the memo between update/render calls.

### Gutter cache

Tag and project gutters can otherwise cause per-row database lookups or repeated map construction. Bulk tag/project helpers and gutter caches let a render frame resolve gutter text from prebuilt in-memory maps.

The paged-row render cache key also includes gutter-related render configuration, so toggling tag/project/flag columns invalidates row strings correctly.

### Sort permutation cache

`uilayout.SortedIndices` sorts visible rows for a `ListTableSpec` and stores the permutation on `ListTableState`.

The cache key includes:

- caller-provided `SortCacheKey`;
- sort column;
- sort direction;
- row count;
- sorted column index;
- column names.

For `RowSpace` users, `cursorSortCacheKey` builds the data-aware key. This is critical because render and cursor translation must share the same cached permutation. If they use different keys, each wheel tick can thrash the cache and re-sort.

The data key must change when the row set changes, even if row count and columns are the same. Examples:

- a refetch;
- a new SOQL run;
- search changed;
- recency order changed;
- report re-run;
- chip predicate changed.

### Paged-row render cache

`wrapPagedRowFn` in `internal/ui/render_cache.go` caches rendered row strings in paginated mode.

The cursor row is never cached. Every non-cursor row can be cached for the active page group.

The group key includes:

- tab;
- subtab;
- inner width;
- focus;
- page size;
- page index;
- list data version;
- sort key;
- search terms;
- active scope;
- render config hash.

`renderConfigVersion` folds in row-appearance inputs such as:

- flag column mode/visibility;
- theme;
- tag/project gutter widths;
- `HScroll`;
- frozen columns;
- zen flag;
- user column widths.

The intended steady-state behavior in paginated mode is: moving the cursor within a page re-renders the old cursor row and the new cursor row, while all other rows hit the cache.

### Frame and chrome caches

`renderCache` also stores:

- tab bars;
- left tab bars;
- status bars;
- dashboards;
- the last full frame.

The last frame supports skipped renders after wheel events that did not move the cursor.

## Sorting And Cursor Reset

Column sorting is handled by `handleColSort` in `internal/ui/listtable_keys.go`.

The sort cycle is:

1. unsorted to ascending;
2. ascending to descending;
3. descending to unsorted.

After the sort state changes, `resetCursorForCurrentView` is called. The intended user experience is that sorting snaps the highlighted row to display row `0`, making the reorder visible.

For `ListView` surfaces, this is simple: `ResetCursor` sets the filtered cursor to zero.

For raw-coordinate surfaces, reset must translate display row `0` back to raw coordinates. For records, that means:

```go
raw := space.DisplayToRaw(DisplayRow(0))
d.Cursors.Set(cursorKindRecordsRow, int(raw), 0, sobject)
```

If a sorted surface keeps the same record highlighted after pressing `s`, the likely bug is a missing `ResetCursor` hook or a reset that writes visible/raw coordinate `0` instead of display row `0`.

## Records-Specific Flow

`/records` and `/objects/<sobject>/Records` share the records machinery.

High-level flow:

1. Resolve active sObject.
2. Resolve selected chip ID and chip mode.
3. Resolve current records resource:
   - local sf-deck chip records;
   - Salesforce list-view result;
   - Salesforce synthetic Recently Viewed chip backed by chip records.
4. Build or hit `visibleRecordsAndIdx`.
5. Build or hit `recordsProjectionFor`.
6. Build `recordsRowAdapter` from `visibleIdx`, table state, columns, cell function, and sort data key.
7. Ask the adapter to translate the stored raw cursor to display cursor.
8. Call `renderListModel`.

The parallel `visibleIdx` slice is what keeps operations consistent:

- render uses display rows;
- cursor movement stores raw rows;
- open/yank/sidebar can resolve the same displayed row;
- search can remove rows without permanently losing the user's backing-row position.

Recency ordering for the local Recently Viewed chip is applied inside `visibleRecordsAndIdx`, not only in render. That is deliberate: cursor movement and rendering must consume the same `visibleIdx` mapping.

## SOQL-Specific Flow

SOQL results use dynamic columns, search filtering, gutters, and raw cursor storage.

`soqlProjectionFor` returns a `soqlRenderEntry` containing:

- filtered records;
- filtered-to-raw indexes;
- list columns;
- cell function;
- data identity.

`soqlTableAdapter` wraps that entry in `tableRowAdapter`. Cursor movement and selected-record resolution then translate raw/display coordinates the same way records do.

The SOQL result grid uses `m.soqlTable` as its `ListTableState`, so sort, pagination, horizontal scroll, and column widths belong to the SOQL grid as a whole.

## Reports-Specific Flow

Report detail rows are dynamic server-returned grids.

Reports are intentionally less normalized than records/list surfaces because report runs have their own columns, row payloads, aggregate fallback, export behavior, and mixed report chrome.

The row grid still uses the shared renderer and `tableRowAdapter`:

- `buildReportRunCols` creates list columns;
- `reportRunCell` reads row payloads;
- `reportRunTableAdapter` translates display sort order over raw report rows;
- `cursorKindReportRow` stores the selected raw row per report ID.

Because reports are dynamic and export-oriented, treat them as their own integration point rather than forcing them through every generic chip/list abstraction.

## Horizontal Scrolling And Column Widths

Vertical scrolling and horizontal scrolling are separate.

Vertical scrolling is cursor driven.

Horizontal scrolling is table-state driven:

- `HScroll` stores the leftmost non-frozen visible column;
- `FrozenCols` keeps leading columns anchored;
- `ColCursor` identifies which column `s`, `<`, `>`, `{`, and `}` target;
- `UserWidths` stores explicit width overrides.

`LayoutListTable` chooses widths in this order:

1. user width if set;
2. ideal width;
3. shrink non-user columns toward min if the pane is tight;
4. if minimum widths still do not fit, enable horizontal overflow.

Gutters are not regular columns. Tags, flags, and projects can render as left/right gutters outside `spec.Cols`; the column cursor never lands on them.

Persisted column widths are applied by `activeListTableContext` before column operations read the state. Width scope matters: record list widths should generally scope by sObject and chip, while shared static surfaces can scope by their surface name.

## Cache Invalidation Rules

When adding or changing a scrollable surface, ask these questions:

1. What owns the cursor?
2. Is the stored cursor in filtered/display coordinates or raw backing coordinates?
3. If search filters rows, do we preserve row identity or reset to top?
4. If column sort is active, do render/open/move all use the same permutation?
5. Does the sort cache key include the active row data identity?
6. Does `DataVersion` change when the rows or row order change?
7. Does `renderConfigVersion` include every row-appearance setting this surface reads?
8. Does the surface provide a measured cell path for `}` snap-to-content?
9. Does pagination mode have a stable page group key?
10. Does a chip/sObject/org change produce a different table state or width scope when it should?

Common fixes:

- wrong record opens after sort: check `tableRowAdapter`, `RowSpace`, and shared sort cache keys;
- row stays highlighted after sorting instead of snapping top: add/fix `ResetCursor`;
- sorted scrolling gets slower over time: look for per-frame sort cache misses or closure allocation before a cache short-circuit;
- records scroll lag: check `visibleRecordsAndIdx` cache hits and `recordsProjectionFor`;
- SOQL scroll lag: check `soqlProjectionFor` cache hits;
- stale row render after a visual setting changes: add that input to `renderConfigVersion`;
- column width changes do not stick: check `activeListTableContext` width scope and `saveListTableWidthsCmd`;
- column cannot grow: check `ListColumn.Max`; `0` means "use Ideal", so dynamic columns that should grow to content need a real max or a `MeasureCell` hook for snap.

## Adding A New Scrollable Surface

Prefer this order:

1. If the surface is a simple typed list, use `ListViewTableSpec[T]` and `listSurfaceFromSpec`.
2. If it is chip-driven but still list-like, provide a `listSurface` with `State`, `Cols`, `SearchPtr`, `MoveCursor`, `ResetCursor`, and `BuildRenderModel`.
3. If it has dynamic row/column data, build a projection cache and return `listRenderModel` directly.
4. If the cursor must survive filtering or sort, store raw coordinates and use `tableRowAdapter`.
5. If the surface is not actually a list table, keep it bespoke and only implement the hooks it needs.

Minimum checklist for a `listRenderModel` surface:

- stable `ListTableState`;
- stable search pointer, even if empty;
- `Cols`;
- `N`;
- fast `Cell(row, col)`;
- display-space `Cursor`;
- meaningful `DataVersion`;
- meaningful `SortDataKey` if sorted rows can change without state changing;
- `ResetCursor` if the surface supports sort/search;
- `MeasureCell` if `}` should snap to real content width.

## Performance Expectations

A healthy scroll path should have these properties:

- no Salesforce calls during render or cursor movement;
- no disk reads/writes during render or cursor movement;
- no full filtered-list rebuild on every wheel tick;
- no full dynamic-column projection rebuild on every wheel tick;
- no O(N log N) sort on every wheel tick for unchanged sorted data;
- paginated mode reuses non-cursor row strings inside the current page;
- throttled wheel events skip rendering rather than rebuilding identical frames.

If a list is fast after process start but gets slower during sorted scrolling, suspect heap pressure or cache-key thrash in the sorted path first.

If an unsorted list is fast and sorted list is slow, compare:

- `SortedIndices` cache key stability;
- `cursorSortCacheKey` use in both renderer and cursor translation;
- whether the sort order callback is allocated before a cache short-circuit;
- whether the projection cache returns the same cell function/key for stable rows.

## Related references

- [`architecture.md`](architecture.md) explains the overall package and
  rendering boundaries.
- [`architecture-diagram.md`](architecture-diagram.md) visualises the request
  and render paths.
- [`cache-architecture.md`](cache-architecture.md) covers persistent,
  resource, render, and REST-client cache lifecycles.
- [`adding-a-tab.md`](adding-a-tab.md) is the prescriptive guide for adding a
  new surface without bypassing these shared paths.
