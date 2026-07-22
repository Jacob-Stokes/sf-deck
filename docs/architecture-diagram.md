# sf-deck architecture diagrams

Companion to [`architecture.md`](architecture.md) (the prose) and [`adding-a-tab.md`](adding-a-tab.md) (the prescriptive how-to). This file is the visual map.

Four diagrams, ordered by abstraction level:

1. **Package layers** — what depends on what
2. **Request lifecycle** — keypress → state mutation → render frame
3. **Registry dispatch** — how `TabSpec` resolves at runtime
4. **List render pipeline** — `listRenderModel` from data to pixels

---

## 1. Package layers

```
                                 ┌──────────────────────────────────────────┐
                                 │  cmd/sf-deck/main.go                     │
                                 │  (binary entry, settings, store wiring)  │
                                 └──────────────────────────────────────────┘
                                                    │
                                                    ▼
   ╔═══════════════════════════════════════════════════════════════════════════╗
   ║                          internal/ui/  (the TUI)                          ║
   ║                                                                           ║
   ║  ┌─────────────────────────────────────────────────────────────────────┐  ║
   ║  │  Model + orgData                                                    │  ║
   ║  │  (per-org Resources, ListViews, cursors, table states)              │  ║
   ║  └─────────────────────────────────────────────────────────────────────┘  ║
   ║                                                                           ║
   ║  ┌────────────────┐  ┌────────────────┐  ┌────────────────────────────┐   ║
   ║  │  TabSpec       │  │  listSurface   │  │  chipSurface / openSurface │   ║
   ║  │  (dispatch     │  │  (per-list     │  │  (per-list chip + open     │   ║
   ║  │   table)       │  │   render spec) │  │   plumbing)                │   ║
   ║  └────────────────┘  └────────────────┘  └────────────────────────────┘   ║
   ║                                                                           ║
   ║  ┌─────────────────────────────────────────────────────────────────────┐  ║
   ║  │  renderListModel — single render path for every list-table surface  │  ║
   ║  └─────────────────────────────────────────────────────────────────────┘  ║
   ║                                                                           ║
   ║  ┌─────────────────────────┐         ┌────────────────────────────────┐   ║
   ║  │  per-tab orchestrators  │         │  modals / pickers              │   ║
   ║  │  (tab_*.go renderers)   │         │  (chip mgr, edit, choice…)     │   ║
   ║  └─────────────────────────┘         └────────────────────────────────┘   ║
   ╚═══════════════════════════════════════════════════════════════════════════╝
        │              │              │              │              │
        ▼              ▼              ▼              ▼              ▼
 ┌────────────┐ ┌─────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────────┐
 │ uilayout   │ │ qchip       │ │ treechip   │ │ exporters  │ │ resource     │
 │ (lipgloss  │ │ (chip       │ │ (tree-     │ │ (xlsx/csv/ │ │ (typed       │
 │  primitives│ │  registry + │ │  shaped    │ │  json/sfdx │ │  cached      │
 │  + table   │ │  query AST) │ │  chips)    │ │  bundles)  │ │  Resource[T])│
 │  layout)   │ │             │ │            │ │            │ │              │
 └────────────┘ └─────────────┘ └────────────┘ └────────────┘ └──────────────┘
        │              │              │              │              │
        └──────────────┴──────────────┴──────────────┴──────────────┘
                                       │
                                       ▼
                  ┌─────────────────────────────────────────────┐
                  │  Domain packages (no TUI dependency)        │
                  │                                             │
                  │  internal/sf          (Salesforce client)   │
                  │  internal/cache       (SQLite payload cache)│
                  │  internal/devproject  (DevProjects + tags)  │
                  │  internal/query       (chip predicate AST)  │
                  │  internal/postprocess (xlsx transforms)     │
                  │  internal/settings    (TOML config)         │
                  │  internal/applog      (structured logging)  │
                  └─────────────────────────────────────────────┘
```

**Reading the layers (bottom-up):**

- **Domain packages** are TUI-free. They could be linked into a CLI or daemon tomorrow. `internal/sf` wraps the `sf` CLI + REST; `internal/cache` is SQLite; `internal/devproject` is the projects/tags/saved-queries store.
- **`internal/ui/<sub>` packages** are TUI primitives that don't see the parent `Model`. `uilayout` does table layout math; `qchip` is the chip predicate engine; `resource` is the typed cache pattern.
- **`internal/ui` proper** is the Bubble Tea layer. Everything that takes `Model` lives here. The TabSpec / surfaces / `renderListModel` machinery is the spine.

The arrows point **down** — `internal/ui` imports everything below it; nothing below it imports `internal/ui`. This is the boundary that lets the TUI become a CLI's UI later without rewriting the domain logic.

---

## 2. Request lifecycle (keypress → frame)

```
   USER PRESSES 'j'
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Bubble Tea runtime delivers tea.KeyPressMsg{Code: 'j'}         │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Model.Update(msg) — update.go                                  │
   │  switch msg.(type) {                                            │
   │    case tea.KeyPressMsg: m.handleKey(msg)                       │
   │    case soqlResultMsg:   m.applySOQLResult(msg)                 │
   │    case ... 30+ kinds                                            │
   │  }                                                               │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  m.handleKey(msg) — update_keys.go                              │
   │                                                                 │
   │  matches(key, Keys.MoveDown) → m.moveCursor(+1)                 │
   │  matches(key, Keys.OpenLensManager) → m.openChipManager…        │
   │  matches(key, Keys.Paginate) → m.handlePaginateToggle()         │
   │  ... 80+ key bindings                                            │
   └─────────────────────────────────────────────────────────────────┘
         │ (j → moveCursor(+1))
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  m.moveCursor(delta) — update_nav.go                            │
   │                                                                 │
   │  ┌─────────────────────────────────────────┐                    │
   │  │ Resolve via TabSpec registry:           │                    │
   │  │  if surf := m.resolveListSurface();     │                    │
   │  │     surf != nil && surf.MoveCursor !=…  │                    │
   │  │      surf.MoveCursor(d, delta)          │                    │
   │  │  else if fn := resolveMoveCursor()      │                    │
   │  │      fn(&m, delta)   // bespoke         │                    │
   │  └─────────────────────────────────────────┘                    │
   └─────────────────────────────────────────────────────────────────┘
         │  (mutates d.SObjectList.Cursor or similar via ListView.MoveBy)
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Returns (Model, tea.Cmd)  — usually (m, nil)                   │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Bubble Tea calls Model.View() to redraw                         │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Model.View() — render.go                                       │
   │  defer recover() → renderPanicFrame on panic                    │
   │  return m.viewImpl()                                            │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  m.viewImpl() — render.go + render_panes.go                     │
   │                                                                 │
   │   ┌──────────────┐  ┌────────────────────┐  ┌────────────────┐  │
   │   │  Left rail   │  │  Main pane         │  │  Sidebar       │  │
   │   │  (orgs +     │  │  (active tab's     │  │  (TabSpec      │  │
   │   │   utilities) │  │   Renderer)        │  │   .Sidebar)    │  │
   │   └──────────────┘  └────────────────────┘  └────────────────┘  │
   │                              │                                  │
   │                              ▼                                  │
   │       resolveRenderer() walks TabSpec registry                  │
   │       (subtab.Renderer → tab.Renderer)                          │
   │                              │                                  │
   │                              ▼                                  │
   │       e.g. m.renderObjects(w, h)                                │
   │       → calls foosListSurface.BuildRenderModel(m, d)            │
   │       → renderListModel(model, ...)  [see diagram 4]            │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  Returns tea.View → Bubble Tea draws to terminal                │
   └─────────────────────────────────────────────────────────────────┘
```

**Key invariants:**

- **`Model` is value-typed.** Every `Update` returns a fresh `Model`. Bubble Tea's concurrency model forbids shared mutable state. The exception is `*orgData` (and other pointer-typed fields embedded on Model) — those are intentionally pointer-shared because they hold the per-org Resources + ListViews.
- **No panic propagates to the runtime.** `View()` wraps `viewImpl()` in `defer recover()`; on panic, `renderPanicFrame` returns a fallback view with the recovered message + a hint to check the log file. Tests bypass this wrapper deliberately so render bugs surface loudly during test runs.
- **`tea.Cmd` is the only way to do async work.** `runSOQLCmd`, `Resource.Ensure`, etc. all return a `tea.Cmd` that runs on a goroutine and lands a typed message back in `Update`. No goroutines are spawned outside this pattern.

---

## 3. Registry dispatch (TabSpec resolution)

```
   CURRENT TAB = TabObjects, currentSubtab = ""
   USER PRESSES 'r' (refresh)
         │
         ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │  m.refreshCurrent() looks up TabSpec for the active tab          │
   │                                                                  │
   │   tabSpecs() returns map[Tab]*TabSpec                            │
   │       │                                                          │
   │       └─► spec := tabSpecs()[TabObjects]                         │
   └──────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │  TabSpec resolution chain (subtab → tab → fallback):             │
   │                                                                  │
   │     1. spec.activeSubtabSpec(m).RefreshData                      │
   │         (only if subtabs declared + index in range)              │
   │     2. spec.RefreshData                                          │
   │     3. nil → no-op                                                │
   │                                                                  │
   │   For TabObjects: no subtabs, falls through to:                  │
   │     spec.RefreshData = func(m, d) { return d.SObjects.Refresh… } │
   └──────────────────────────────────────────────────────────────────┘
         │
         ▼
   Returns tea.Cmd → fires fetch → resourceUpdatedMsg → re-render


   THE TabSpec STRUCTURE
   ─────────────────────
   type TabSpec struct {
       Tab                Tab
       Stem               Tab          // for number-key nav
       Renderer           func(...)    // top-level render
       Chips              *chipSurface // chip strip plumbing
       Open               *openSurface // o / ctrl+o handler
       List               *listSurface // list-table state + render
       Identity           func(...)    // "what's cursored?" — drives K/#
       Sidebar            func(...)    // right-pane render
       BusyLabel/ErrorLabel           // status bar pills
       EnsureData/RefreshData         // resource lifecycle
       MoveCursor/SearchPtr/etc.      // bespoke escape hatches
       ListTable          func(...)   // dynamic-column tables (SOQL)
       MeasureCell        func(...)   // snap-to-content for those
       Subtabs            []SubtabSpec // when non-empty, per-subtab
       GetSubtabIdx/SetSubtabIdx       // subtab cursor accessors
       Widget             *Widget      // optional pinned input
   }


   THE listSurface STRUCTURE
   ─────────────────────────
   type listSurface struct {
       State            func(d) *ListTableState  // column widths,
                                                   // cursor, sort, page
       Cols             func() []ListColumn       // column spec
       SearchPtr        func(d) *searchState      // / filter buffer
       MoveCursor       func(d, delta)            // ↑↓ / j k / wheel
       ResetCursor      func(d)                   // post-filter reset
       BuildRenderModel func(m, d) (model, ok)    // ★ THE BIG ONE
   }

   BuildRenderModel produces a listRenderModel — the per-frame
   value that feeds renderListModel. See diagram 4.
```

**The benefit of the registry pattern:**

Adding a new chip-bearing tab is **one TabSpec literal**:

```go
TabFoo: {
    Tab: TabFoo, Stem: TabFoo,
    Renderer: Model.renderFoos,
    List:     &foosListSurface,
    Chips:    &foosChipSurface,
    // ... a few more declarative fields
}
```

…and you get for free: number-key nav, chip cycle, V chip manager, N new chip, M overflow, F favourite, K pin to project, # tag, search `/`, column-mode `c`, sort `s`, snap-to-content `}`, zen `z`, pagination `P`, the smoke test passing for your tab.

The "for free" part is what the registry buys you. Without it, each of those would be a per-tab hand-rolled handler.

---

## 4. List render pipeline (data → frame)

```
   d.SObjectList holds [N items]      ── (ListView[T])
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  d.SObjectList.Filtered() — memoised                            │
   │     Cache key = (version, Search.Buffer()).                     │
   │     Hit  → return cached []T in O(1).                           │
   │     Miss → apply:                                               │
   │         1. extra predicate (chipSurface.ApplyChip → SetExtra)   │
   │         2. match predicate (search; installed via SetMatch)     │
   │       Store the result + key, return narrowed []T.              │
   │     Version bumps on Set / SetExtra / SetMatch — never on read. │
   │  Saves 2-3 redundant scans per wheel tick (MoveBy + render +    │
   │  Selected each call Filtered()).                                │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  foosListSurface.BuildRenderModel(m, d) — list_surface_*.go     │
   │                                                                 │
   │  Produces listRenderModel{                                      │
   │      Title:    "SOBJECTS · 47 · just now"                       │
   │      State:    &d.ObjectsTableState                             │
   │      Search:   d.SObjectList.SearchPtr()                        │
   │      Cols:     [Name, Label, ...]                               │
   │      N:        len(filtered)                                    │
   │      Cursor:   d.SObjectList.Cursor()                           │
   │      Cell:     func(row, col) string { ... }                    │
   │      Marks:    [...row-level tints/badges]                      │
   │      Gutters:  [tag-pill gutter, project-pill gutter]           │
   │      Recolor:  func(row, col, base) Style { ... }  // optional  │
   │      Empty:    "  no foos"                                      │
   │      FooterExtras: "↵ open · r refresh"                         │
   │  }                                                              │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  renderListModel(m, model, ...) — listrender_model.go           │
   │                                                                 │
   │   1. Title with search-pill + page indicator (if Paginated)     │
   │   2. Search bar (yellow border when filter applied)             │
   │   3. Empty-state branch if N == 0                                │
   │   4. uilayout.LayoutListTable → ResolvedWidths                  │
   │   5. Render header row (column titles + sort indicators)        │
   │   6. For each visible row:                                       │
   │        sortPerm[i] = SortedIndices[i]   // user sort permutation│
   │        recolorred  = Recolor(row, col, baseStyle)               │
   │        RenderListTableRow → row string                          │
   │   7. Slice rows for the viewport:                                │
   │      ┌───────────────────────────────────────────────────────┐   │
   │      │ Paginated?                                            │   │
   │      │   yes → RenderRowsPaged (page = cursor/pageSize)      │   │
   │      │   no  → RenderRows (cursor-following window)          │   │
   │      └───────────────────────────────────────────────────────┘   │
   │   8. Footer hint (column-mode keys / search-clear / extras)     │
   │   9. Legend (row-mark explainer when Marks defined)             │
   └─────────────────────────────────────────────────────────────────┘
         │
         ▼
   []string of pre-styled lines → joined into the main pane


   THE SAME PIPELINE IS USED BY:
   ─────────────────────────────
   /objects, /flows, /apex (Classes + Triggers), /components (LWC + Aura),
   /perms (PermSets + PSGs + Profiles + Queues + Public Groups),
   /apex-logs, /deploys, /packages, /recent,
   /soql Editor results, /soql Saved, /soql History,
   /reports run rows, /home (Recent + Notifications + ...),
   /records list-mode, ObjectDetail Records subtab.

   That's ~17 surfaces. Behavior changes (e.g. adding pagination,
   row-level export, snap-to-content) happen ONCE here and apply
   everywhere automatically.
```

**The single render path is the architectural keystone.** Without it, "every list now supports pagination" would mean editing 17 renderers individually. With it, it means editing `renderListModel` once. Same for any future cross-cutting list feature.

---

## How these fit together

```
   USER ACTION                          STATE                    OUTPUT
   ───────────────────────────────────────────────────────────────────────

   keypress  ─────► Update ─────► Model+orgData ─────► View ─────► frame
                       │              ▲                  │
                       │              │                  │
                       │              │ (mutated by      │
                       │              │  surface hooks   │
                       │              │  resolved via    │
                       │              │  TabSpec)        │
                       ▼              │                  ▼
                  TabSpec lookup ─────┘            renderListModel
                  (subtab → tab)                   (single path for
                  (chip / open /                    every list)
                   list / identity)


   And the supporting machinery (in declaration order):

   tab.go                Tab IDs (string-typed enum)
   subtab.go             Subtab IDs
   model.go              Model + orgData (the state)
   model_sync.go         per-resource Sync<X>List helpers + bulk SyncListViews
   tab_registry.go       TabSpec entries (the dispatch table)
   list_surface_*.go     listSurface declarations (one per tab)
   chip_surface.go       chipSurface declarations (one per tab)
   open_surface.go       openSurface declarations (one per tab)
   list_table_hint.go    standard footer hint composer
   listrender_model.go   the render pipeline
   row_marks.go          per-domain RowMark + markPills helpers
   tab_*.go              per-tab orchestrators (chip strip + render call)
   update.go             Update dispatch (per-message-kind)
   update_keys.go        keymap dispatch (per-key matcher)
   update_nav.go         moveCursor / activate / refreshCurrent
   update_wheel.go       wheel-scroll throttle (drop bursts that pile up)
   tabspec_resolve.go    surface resolution (subtab → tab walks)
   resource/listview.go  ListView[T] + SearchState (memoised Filtered cache)
```

---

## What's NOT shown here

Three things deliberately excluded because they'd dilute the diagrams:

1. **Modals** (chip manager, edit modals, choice pickers) — they're a sibling of the main pane at render time; they intercept `Update` when active. See `modal_*.go`.
2. **The exports tracker** — long-running export jobs run on background goroutines and report progress via `tea.Cmd` ticks. See `report_export.go` and the `exports` registry on Model.
3. **Async Resource fetches** — when `EnsureData` fires, a goroutine runs the SF call, lands a `resourceUpdatedMsg`, and the next `Update` cycle slots the result in. Same pattern as everything async.

For those, see `architecture.md`'s sections on async discipline + render entry point + safety.
