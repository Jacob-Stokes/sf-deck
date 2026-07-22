# sf-deck architecture

A reference for the mental model, written for future-you and new
contributors. The code is the source of truth — if you find drift
between this doc and the code, fix the doc (or flag it).

> **If you're here to add a new tab**, read [`docs/adding-a-tab.md`](adding-a-tab.md) instead. This file explains *why* things are shaped this way; that file is the prescriptive how-to.
>
> **If you want the picture, not the prose**, see [`docs/architecture-diagram.md`](architecture-diagram.md) — four layered diagrams covering package layers, the request lifecycle, registry dispatch, and the list render pipeline.

---

## The big picture

sf-deck is a Bubble Tea v2 TUI. Everything visible lives under
`internal/ui/`. Below it sit thin domain layers:

```
internal/
├── sf/             Salesforce REST + CLI calls + composite batching
├── cache/          SQLite cache for Resource[T] payloads
├── settings/       TOML config (~/.sf-deck/settings.toml)
├── theme/          Palette + named styles
├── usage/          API call counter + recent-call ring buffer
├── devproject/     SQLite-backed dev-project + tag + bundle store
├── query/          AST: predicate eval + SOQL emit
├── applog/         Diagnostic file logger (~/.sf-deck/log/)
├── postprocess/    XLSX transforms (URL hyperlink, detailsify, etc.)
├── exporters/      Generic format writers (CSV / XLSX / JSON / package.xml)
└── ui/             Everything visible
    ├── resource/   Resource[T], ListView[T], SearchState building blocks
    ├── uilayout/   Pure rendering helpers (table, viewport, format)
    ├── keymap/     Declarative keybinding registry
    ├── qchip/      Flat-list filter chips (records / objects / flows)
    ├── treechip/   Hierarchical-navigation chips (reports folders)
    └── orgproject/ Scope abstraction over loaded dev-projects
```

The UI package is intentionally large. Recent thinking (per Codex
review): future splits should extract by **purity**, not by tab. Pure
helpers + domain workflows belong in their own packages; tab-specific
rendering stays in `internal/ui/`.

---

## Vocabulary

| Concept     | Meaning |
|-------------|---------|
| **Tab**     | Top-level destination on the number-key strip (`1 home`, `3 objects`, `4 flows`, …) |
| **Subtab**  | Mode within a Tab; cycled with `[` / `]` or `tab` / `shift+tab` |
| **View**    | A saved query/filter you can switch into with one keystroke (user-facing copy) |
| **Chip**    | The visual pill widget. Two engines render it: qchip (flat) + treechip (tree) |
| **Origin**  | Where a chip came from (BuiltIn / User / Imported) — affects glyph + edit affords |
| **DevProject** | A user-curated bag of items (objects, flows, records, reports, apex, perms) — cross-org |
| **Bundle**  | An on-disk sfdx-project directory linked to a DevProject (one-to-many) |
| **Tag**     | User-defined annotation applied to (item-kind, ref, org) — orthogonal to DevProjects |
| **RowMark** | Code-defined fact about a row (custom-shape, managed-package, invalid, …) — derived, not stored |
| **Mark pill** | Visual badge in the detail pane showing which row marks fire for the cursored item |

User-facing copy says **view**. Source code says **chip**. Old keymap
fields preserve "lens" naming for back-compat — see `keymap/keymap.go`.

---

## The Tab registry

Every tab declares its behavior in one struct value in
`tab_registry.go`. Adding a new tab is one entry in `tabRegistry()`
plus the closures it points at — no central switch needs editing.

### TabSpec hooks

| Hook | What it answers |
|------|-----------------|
| `Renderer` | What does the main pane look like? |
| `Sidebar` | What's in the right context panel? |
| `Identity` | What's the stable cursored item? Drives tag picker, openable lookup |
| `Breadcrumb` | What header path segments precede the cursored item? |
| `BusyLabel` | What "syncing X…" text shows in the activity zone? |
| `ErrorLabel` | What error text shows when the resource Errs? |
| `Chips` / `Open` / `List` | Per-domain surface registry pointers |
| `ListTable` | Dynamic-column list-table state + cols (SOQL, report run) |
| `EnsureData` / `RefreshData` | Tab's primary resource lifecycle |
| `Activate` | Custom Enter handler (when `Open.Drill` doesn't fit) |
| `MoveCursor` / `ResetCursor` / `SearchPtr` | Bespoke cursor primitives |
| `CycleChip` | Custom chip-strip ←/→ for multi-axis cursors |
| `EscBack` | Where Esc pops to on drill tabs |
| `Subtabs` | Per-subtab spec list — mirrors TabSpec hooks for overrides |
| `Stem` | Top-level tab this drill-in belongs to (number-key restoration) |

All hooks are pure functions on the live Model. No hidden mutation;
safe to call from anywhere.

### The cached registry

`tabRegistry()` builds the map of all TabSpec entries. **Never call it
directly from a hot path** — it rebuilds the map every time. Use
`lookupTabSpec(t)` (cached, sync.Once) or the `activeSpec()` helper
that walks subtab → tab in one lookup.

```go
func (m Model) activeSpec() (*TabSpec, *SubtabSpec) {
    spec := lookupTabSpec(m.tab())
    if spec == nil {
        return nil, nil
    }
    if len(spec.Subtabs) == 0 || spec.GetSubtabIdx == nil {
        return spec, nil
    }
    idx := spec.GetSubtabIdx(m)
    if idx < 0 || idx >= len(spec.Subtabs) {
        return spec, nil
    }
    return spec, &spec.Subtabs[idx]
}
```

Resolvers (`resolveItemIdentity`, `resolveSidebar`, `resolveBreadcrumb`,
`currentTabError`, `currentTabSyncingLabel`, `activeListTableMeasured`)
all route through this. Cache leak in any new resolver = perf
regression on every keystroke.

### Identity vs openSurface.Openable

Both can answer "what's the openable under the cursor?" but exist for
different reasons:

- **`Identity`** is the canonical stable selected-item tuple. Wire it
  once, get tags + opens for free.
- **`openSurface.Openable`** is for surfaces whose openable doesn't
  fit the Identity tuple — synthetic record refs, branching opens.

`cursorOpenable` consults Identity first; openSurface.Openable is the
fallback. New tabs should set Identity and leave openSurface.Openable
nil unless there's a real reason to override.

### Surface registries

Three per-domain registries hold uniform-list-shape behavior:

- `chipSurface` — chip-strip cursor + chip predicate application (chip_surface.go)
- `openSurface` — Openable + Drill (open_surface.go)
- `listSurface` — list-table state + columns + Cell + Marks + everything else

A tab that fits the uniform pattern just registers pointers from its
TabSpec entry. Bespoke tabs leave the pointers nil and use per-hook
closures.

---

## The list-table render pipeline

Every list-shaped surface in sf-deck flows through one renderer:
`renderListModel` in `listrender_model.go`. This is recent
consolidation work; a year ago there were three parallel renderers.

### `listRenderModel` — the per-frame value

A surface (or orchestrator) hands the renderer a per-frame value:

```go
type listRenderModel struct {
    Title        string                   // "FLOWS · 14 · just now"
    State        *uilayout.ListTableState // sort, scroll, column widths
    Search       *searchState             // filter buffer + applied flag
    Cols         []uilayout.ListColumn
    N            int
    Cursor       int
    Cell         func(row, col int) string
    Marks        []uilayout.RowMark      // managed-package, custom-shape, etc.
    Gutters      []uilayout.GutterSpec   // tag pill, project pill columns
    Recolor      func(row, col int, base lipgloss.Style) lipgloss.Style
    Empty        string                   // shown when N == 0
    FooterExtras string                   // surface-specific footer keys
}
```

This is the **single source of truth** for any list-table render.
Sort, snap-to-content, marks, gutters, recolor, footer hint, legend
— all read from the model. No drift possible because Cell has one
home.

### Three ways to feed the model

1. **`listSurface.BuildRenderModel(m, d)`** — the canonical opt-in.
   Tabs set this on their existing listSurface entry. Tab renderer
   becomes a thin orchestrator: org guards → call BuildRenderModel
   → call `renderListModel`.

2. **`renderListSurface` adapter** — for tabs whose body is "the
   table and only the table" (no chip strip, no dashboard, no busy
   chrome above). Wraps BuildRenderModel + renderListModel in one
   call. Used by /apex-logs, /deploys, /packages, /recent, /perms
   subtabs.

3. **Inline model construction** — for surfaces that don't fit the
   listSurface registry shape (SOQL: input widget + dynamic columns
   per query; reports run: cached run with metadata block above).
   The orchestrator constructs the model directly each frame and
   passes it to `renderListModel`.

### `renderListModel` is paranoid

The renderer guards every field:

```go
if model.Cell == nil || len(model.Cols) == 0 ||
   model.State == nil || model.Search == nil {
    return nil
}
```

Bubble Tea panics at the View layer take down the TTY with no
diagnostic by default. The shared renderer is the blast radius — it
returns an empty slice rather than nil-panic when fields are missing.

### Surfaces that stay bespoke

`/records` is the only list-shaped tab that doesn't use the model. The
rationale is its per-(sobject, chip) state and unfiltered/visible cursor
mapping, which don't fit the shared model cleanly. It's a documented
deferral, not a forgotten one.

LWC bundle file tabs and reports folder/report tree are NOT list-
tables — they're tree-ish or dynamic-tab views. Out of scope for the
list-table consolidation.

---

## Row marks (badges + tints)

Codified facts about rows that surface as inline badges and detail-
pane pills.

```go
type RowMark struct {
    ID        string                   // "custom-sobject", "managed-package", …
    Label     string                   // shown in the auto-generated legend
    Matches   func(row int) bool       // per-row predicate
    Treatment Treatment                // visual: NameColor + optional Badge
}
```

Per-domain helpers in `row_marks.go` build the `[]RowMark` slice
fresh each render via `marksFor<Domain>(items)`. The same predicates
also drive the per-item pills in detail panes via
`markPillsFor<Domain>(item)` — single source of truth for "is this
row managed-package" etc.

A row can carry multiple marks; treatments compose:
- NameColor: last-applied wins
- Badges: stack as `[mark1] [mark2]`
- Dim: any matching mark with Dim=true wins

### FLAGS column

Rendered as a dedicated column on every list-table surface that opts
in (sObjects, Apex Classes, Apex Triggers, Flows, LWC, Aura, perm
sets, perm-set groups, profiles). The column header reads `FLAGS`;
each cell shows the matching marks for that row.

The column has three display modes, cycled via `ctrl+g`:

- **letter** (default) — first-letter glyph per mark, no separator
  (e.g. `MS` for managed+session). Width clamped to header text
  ("FLAGS" = 5 chars) so the header always renders cleanly even on
  rows with no flags.
- **full** — full mark labels separated by spaces.
- **hidden** — column not rendered.

Persisted in `settings.toml` as `flag_column_mode`. The cell helper
is `Model.renderFlagsCell(marks, row)` which routes through the
user's mode setting; surface authors don't pick the mode themselves.

### Tags vs marks

| | Tags | RowMarks |
|---|------|----------|
| Source | User-defined | Code-defined |
| Mutable | Yes (apply / remove via `t` modal) | No, derived from data |
| Storage | SQLite tag_bindings | None (recomputed each render) |
| Color | User picks per tag | Defined by the mark |

Both render as bracketed pills in the detail pane, but they answer
different questions. Don't fold them.

---

## Resources and caching

Three independent cache layers stack between Salesforce and the
rendered frame. Each answers a different question and they don't
interact:

```
   Salesforce REST                              <- network
           ↓ (Resource.Fetch closure)
   on-disk SQLite cache (~/.sf-deck/cache.db)   <- Layer 1: survives restarts
           ↓ (Resource.Value())
   in-memory []sf.Foo                            <- Layer 2: per-org payload
           ↓ (lv.Set on Sync<X>List)
   ListView.items                                <- mutable view-state wrapper
           ↓ (lv.Filtered() with chip predicate + search)
   ListView.filteredCache                        <- Layer 3: per-render memo
           ↓ (BuildRenderModel reads it)
   visible rows on screen
```

**Layer 1 — on-disk SQLite cache** (`internal/cache`). Persists
serialised payloads across `sf-deck` restarts, keyed by
`(scope, key)` (e.g. `("user@org", "flows_v2")`). Re-launches read
from this immediately so the user lands on cached data while a
background refresh fires. Bump the key suffix (`flows_v2` →
`flows_v3`) instead of running schema migrations when the on-disk
shape changes.

**Layer 2 — in-memory `Resource[T]`** (`internal/ui/resource`).
Holds the deserialised slice on `*orgData`, one copy per org. The
async lifecycle is:

```
1. Ensure(cache) → cheap cache-load command
2. resource.UpdatedMsg lands → Apply populates Resource.data
3. If stale (TTL expired), MaybeRefreshAfterCacheLoad fires network refresh
4. Refresh result lands as another UpdatedMsg → data updates again
```

TTLs are per-Resource and per-org. Configurable via `[ui.cache]` in
`settings.toml`. Per-resource sync helpers (`SyncSObjectsList`,
`SyncFlowsList`, … in `internal/ui/model_sync.go`) copy a freshly
applied payload into its wrapping `ListView` via `Set`. The update
loop calls only the targeted helper when a single resource lands so
unrelated cursors survive background refreshes — the bulk
`SyncListViews()` exists for bootstrap paths.

**Layer 3 — per-render `ListView.Filtered()` memo**. ListView wraps
the Resource slice with cursor + search + chip-predicate state. The
predicate-applied result is memoised on a `(version, search buffer)`
key; `version` bumps on `Set` / `SetExtra` / `SetMatch`. Within a
frame, `MoveBy` (clamps cursor), `BuildRenderModel` (slices visible
rows), and `Selected()` (sidebar's current row) all hit the cache
after the first call rebuilds it. Without this memo, a wheel tick on
/apex Classes with the 6-clause "Tests" predicate re-ran the scan
2-3 times per tick — visible scroll lag.

**They don't interact.** Layer 1 saves network calls. Layer 2 saves
re-fetching the same payload during a session. Layer 3 saves
re-running the chip predicate within a single render. A change at
any layer invalidates only its own — bumping a Resource (new payload
lands) calls `Set` which bumps the ListView version, which clears
the Filtered memo. None of the three layers caches a value derived
from another's contents in a way that could go stale.

`ListView[T]` is the standard pattern for "browseable list with `/`
filter". Almost every list-shaped surface in the app uses this. See
"Filtered memo + invalidation" in `docs/adding-a-tab.md` for the
caller contract.

---

## Async discipline

> Command goroutines may call Salesforce / filesystem and return
> messages. They MUST NOT mutate Model, orgData, Resource, registries,
> or settings directly. All state mutation happens in `Update`.

Commands return typed messages. `Update` switches on type and applies
the payload. Reading captured `*orgData` inside a `tea.Cmd` to reach
`Resource.Set` or `settings.Save` directly creates a race with
concurrent renders / updates.

The exception: the export registry (`exports.go`) uses its own mutex
for the in-flight job map because the worker goroutines need to
update phase incrementally. That's the only place in the codebase
where a goroutine writes to shared state under its own lock — and
that mutation is invisible to the model except via re-render ticks.

---

## DevProjects + Bundles + Scope

`/dev-projects` is the umbrella concept for "a curated bag of
Salesforce stuff". Items hang directly off a DevProject; each item
carries the org it was collected from.

### Tab structure

```
/dev-projects (TabDevProjects)
    ├── Projects subtab — the master list
    └── Bundles subtab — every bundle across every project (top-level cross-cut)

/dev-project (TabDevProjectDetail) — drill-in for one project
    ├── Items subtab — flat tree of collected items
    └── Bundles subtab — bundles linked to this project

/bundle (TabBundleDetail) — drill-in for one bundle
    ├── metadata block (path, default org, last retrieved/deployed)
    └── retrieve + deploy preview tables (or fallback diff for non-tracked orgs)
```

### Bundles

A Bundle is an on-disk sfdx-project directory linked to a DevProject.
Created via `e` (export) → "Full sfdx project + retrieve from org",
which:
1. Writes package.xml + sfdx-project.json + force-app/ scaffolding
2. Runs `sf project retrieve start` to populate force-app/
3. Persists a Bundle row in SQLite

After creation, the bundle supports:
- **Retrieve** (r) — re-runs `sf project retrieve start`, refreshes force-app/
- **Deploy** (D) — runs `sf project deploy start`
- **Validate** (v) — runs `sf project deploy validate`
- **Diff preview** — `sf project retrieve preview` + `sf project deploy preview`
  - Falls back to `ManifestPreviewFallback` (Tooling API + local mtime
    comparison) when the org lacks source tracking
- **Reveal in Finder** (o) / yank path (y)

All operations route through the export tracker (Ctrl+J) so the user
can see them in flight from anywhere.

### Managed-package detection

Items collected from managed packages (NamespacePrefix non-empty) are
captured at collect time on `Item.Namespace`. Visible everywhere:
- /dev-project-detail rows: yellow `[ns]` badge inline
- /bundle preview: managed components in their own "Managed (not
  retrievable)" section
- /apex Classes/Triggers + /objects: yellow `[managed]` badge via the
  shared RowMark system

Salesforce refuses to retrieve managed-package source via
MetadataAPI, so the `package.xml` writer skips them and surfaces the
count separately.

### Scope (the bridge)

`internal/ui/orgproject/Scope` is denormalised sets keyed by API
name / definition id / record id, hydrated from SQLite at load + on
every K-collect. Surfaces consult `scope.HasObject(name)` /
`HasFlow(id)` etc. rather than reading from the store directly. Lets
the synthetic project chip render in O(1) per row.

When a project is loaded for an org:
- A `📁 ProjectName` pill appears in the header
- A synthetic project chip auto-pins to chip-shaped surfaces
- `ctrl+k` skips the chooser and toggles the item in the loaded project
  (a second press uncollects); `K` always opens the picker to choose a
  project

---

## Exports tracker

`internal/ui/exports.go` is a process-wide registry tracking
in-flight + recent exports. Persisted to `~/.sf-deck/exports.json`
(last 200 entries, survives restarts).

```go
type exportJob struct {
    ID         string
    Kind       exportKind  // report | project | manifest
    Name       string
    OrgAlias   string
    Path       string
    Format     string
    Phase      exportPhase  // queued | downloading | post-processing |
                            // converting | writing | retrieving | done | failed
    StartedAt  time.Time
    FinishedAt time.Time
    SizeBytes  int64
    ErrMsg     string
}
```

Surfaced in three places:
- **Status bar** (left side): animated activity indicator with phase + elapsed
- **Ctrl+J modal**: in-flight at top + history below; Enter to open, r to reveal in Finder, y to yank, d to remove
- **/home Downloads subtab**: full main-pane view of the same data

The tracker doesn't render anything itself; it just exposes
`hasInflight()`, `snapshot()`, `mostRecentDone(kind)`. Everything
visible is composed by the surfaces that read from it.

---

## Render entry point + safety

`View()` (in `render.go`) is Bubble Tea's per-frame entry point.
Wrapped in a `defer recover` that writes panic + stack to applog and
returns a fallback frame. A panic anywhere in the render tree no
longer takes down the TTY without diagnostic — the user sees:

```
render panicked — see ~/.sf-deck/log for stack trace.
recovered: <message>

q to quit
```

### Smoke tests

`render_smoke_test.go` walks every TabSpec/subtab in two scenarios
(no org, one org with empty data) and asserts View() doesn't panic.
60+ subtests run in ~1.5s.

This is the safety net that should have existed before any of the
listSurface refactor work. Future UI changes get caught at unit-test
time instead of mid-flight when the user switches tabs.

### Subtab dispatcher

`tab_subtab_dispatch.go` owns the "render subtab strip → branch by
subtab ID → fall through to default" pattern that Reports / SOQL /
Apex / Components / Meta share. Each branch declares a
`subtabBranch` (a `Render` closure or a `Placeholder` struct); the
dispatcher draws the strip exactly once and routes to the branch.

This kills a real bug class: previously each tab hand-rolled the
strip prepend, and the Reports default branch shipped without it
(the strip only appeared after the user cycled away with Tab and
back). Centralising the strip wiring makes that mistake
structurally impossible.

Tabs with dynamic subtab lists (LWCDetail = one subtab per bundle
file, PermParentDetail = subtabs vary by parent kind) don't fit the
helper and stay bespoke.

---

## Chip system: qchip vs treechip

Two chip engines under shared visual primitives:

```
                        ┌─────────── widget ───────────┐
                        │  rounded pill (chip_strip.go) │
                        │  consumed by both engines     │
                        └────────────┬─────────────────┘
                                     │
              ┌──────────────────────┴──────────────────────┐
              │                                             │
       qchip.Chip (flat)                            treechip.Registry
              │                                             │
       label + Query AST                            tree position
              │                                             │
       /records, /objects, /flows                   /reports (folders)
```

**qchip** is for "saved filter on a flat list". A `qchip.Chip` is
`(label, query.Query)`. The Query AST evaluates two ways:
- **server-side** for /records (emits SOQL via `qchip.ApplyToSOQL`,
  fetches the slice from Salesforce)
- **client-side** for /objects + /flows (`query.Eval(node, row)`
  filters a cached slice in memory)

**treechip** is for "I'm at this position in a tree". Each chip is a
node id; selection means *navigation*, not *filtering*. Used today
only by /reports (folder hierarchy).

The split is deliberate: trying to bolt hierarchy onto qchip degrades
it; trying to coerce qchip's flat predicates into treechip is awkward.

---

## Things to know that aren't obvious from the code

- **Cursor state lives in `orgData.Cursors`** (a `CursorStore`), not
  spread across many `map[string]int` fields. See `cursors.go`.
- **`m.recordChips` / `m.objectChips` / `m.flowChips` etc.** are
  qchip registries on Model. Each loads built-ins from code, then
  hydrates user-defined entries from `settings.toml` at startup.
- **The chip manager (`V` key)** dispatches per-tab to the right
  handler via a synthetic `chipManagerInvokeMsg` so callbacks resolve
  against the live Model in Update rather than a captured pointer.
  Same pattern shows up in the settings meta-menu and the wizard
  save path — beware the captured-pointer trap when adding new
  multi-step modal flows.
- **Concurrent-access on `m.data`** is safe because tea is
  single-goroutine for Update, and Resource.Fetch closures only
  mutate Resource fields (not the map). No locks; adding shared
  state requires care.
- **The list-table primitive** (`uilayout/listtable.go`) backs every
  N-column projection surface. Auto-fit widths; horizontal scroll +
  frozen leftmost column when overflow; `c` enters column-edit mode
  for resize / snap; `z` for zen-mode fullscreen; `}` snaps a
  column to the widest visible cell.
- **Two hint surfaces, no duplication**:
  - The persistent **status bar** at the bottom of the screen owns
    every globally-bound key (search, drill, open, yank, refresh,
    side, palette, help, quit) plus list-table modes (`c cols`,
    `z zen`, `P page`) when the active surface is a list. Always
    visible.
  - The in-pane **hint line** (composed by `listTableHint`) carries
    only list-specific affordances: `ctrl+t tag col`, `ctrl+p proj
    col`, `ctrl+g flag col`, plus per-surface bespoke extras
    declared in `listRenderModel.FooterExtras` (e.g. /reports
    `export`, /soql Editor `edit query`, /soql Saved
    `load · rename · duplicate · delete`, /recent origin glyph
    legend). Surface authors should NOT re-declare global keys here
    — they're already in the status bar; doubling up creates noise.

  Mode-aware: the hint switches to column-mode keys when `c` is
  active, to `esc to clear search` + extras when search is applied,
  to scroll-cols indicator when columns overflow.

---

## Default keybindings

See `keymap/keymap.go` for source of truth. Highlights:

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor down / up |
| `enter` | Drill in |
| `esc` | Back out / close modal / clear committed search |
| `/` | Search the current view |
| `C` | Clear search |
| `o` | Open in Lightning |
| `ctrl+space` | Global search (sObjects, fields, flows, records) |
| `ctrl+j` | Downloads modal |
| `ctrl+k` | Collect cursored item to the loaded DevProject (toggle) |
| `K` | Collect cursored item — pick a DevProject |
| `_` | Toggle load/unload of cursored DevProject |
| `t` | Apply / remove tags on cursored item |
| `#` | Open tag manager |
| `c` | Column-edit mode (on list-table surfaces) |
| `z` | Zen / fullscreen (on list-table surfaces) |
| `}` | Snap column to widest visible cell |
| `[` / `]` | Cycle subtabs |
| `tab` / `shift+tab` | Cycle subtabs (alternate) |
| `←` / `→` | Cycle views (chip strip) |
| `e` | Edit (FLS, ObjPerm, SOQL, project rename, etc.) |
| `x` | Export (reports + dev-projects, unified) |
| `b` | Open Bundles subtab on /dev-project-detail or /dev-projects |
| `=` | Open settings |
| `?` | Help (per-view) |

---

## When to refactor

The current shape is the result of recent consolidation work. Reflexes
that hold up:

- **Flat list filtering** → qchip. Add a domain const, a registry on
  Model, hydrate from settings, register a TabSpec hook.
- **Tree navigation** → treechip. Implement TreeSource + ItemSource,
  build a Registry, persist via settings.
- **New list-shaped tab** → register in `listSurface`, set
  `BuildRenderModel`, wire into TabSpec. Smoke test will validate.
- **New tag-style annotation** → `tag_bindings` table, follows the
  existing tag picker / pill code paths.
- **New code-derived row annotation** → RowMark entry in `row_marks.go`,
  a `markPillsFor*` helper for detail panes, register in the
  appropriate `marksFor<Domain>List` builder.

If none of those fit, that's a real abstraction need — open a
discussion before adding a new core engine.

---

## Future directions worth knowing about

These are tracked in various TODOs in the codebase. Listed here so
they're discoverable:

- **`/records` migration to listRenderModel** — last bespoke list-
  table renderer. Needs careful per-(sobject, chip) state handling.
- **`internal/workflows` extraction** — pull export/retrieve/deploy
  business logic out of UI handlers so headless mode can reuse it.
- **Headless / CLI mode** — `sf-deck export <ref>`, `sf-deck open
  <ref>`, `sf-deck doctor`. Prerequisites: workflow extraction.
- **Tier 4.6 features** — URL-paste in global search (drill via
  Identity), Salesforce news feed homepage subtab.

The big architectural question that shapes the next year of work:
how to extract the UI from the workflows so headless mode is a
natural second consumer rather than a reimplementation.
