# Adding a tab to sf-deck

This is the prescriptive guide. **Do exactly what's here** unless you have a documented reason not to. If you do diverge, leave a comment in the spec saying why — future-you will thank you.

The architecture overview (`docs/architecture.md`) explains *why* things are shaped this way. This file says *what to do*. For visual map see [`docs/architecture-diagram.md`](architecture-diagram.md).

> **TL;DR**: every tab is a `TabSpec` literal in `tab_registry.go`. Lists hang off `orgData`. Chips, opens, identity, and renders are declarative. Subtabs route through `dispatchSubtab`. New tabs are 80% data plumbing, 20% spec wiring. If a feature you need isn't on `TabSpec`, you're probably reaching for an escape hatch when a registry slot would do.

> **Scaffold it first.** For a list-backed surface, run
> `go run ./scripts/newsurface -name X -row XRow -key x_v1 -fetch ListX`
> to generate the sf fetcher + every boilerplate snippet. The plumbing
> that used to be ~10 hand edits across 6 files is now **one
> `registerListResource(...)` entry** (see
> `internal/ui/list_resource_registrations.go`). That registration
> derives the ListView sync, the `applyResourceMsg` routing, and the
> global-refresh enumeration — you no longer hand-write a `case "x_v1":`
> in update.go or a `SyncXList()` line. The drift tests
> (`TestEveryOrgDataResourceKeyIsRoutable`, `TestLoadedResourcesCoversTopLevel`)
> now validate the registration, not hand-written cases.

---

## The decision tree

Before you start, answer these. They determine which slots on `TabSpec` you fill.

1. **Does the tab show a list of things?** → you need a `listSurface`. The list lives on `orgData` as a `ListView[T]`.
2. **Does the user filter the list with chips?** → you need a `chipSurface` + a `qchip.Builtins` slice + a registry pointer on `Model`.
3. **Does Enter "open" something in Salesforce?** → the cursored row needs to expose an `sf.Openable` (via `Identity` or `openSurface`).
4. **Does the user pin/tag the cursored row?** → wire `Identity`, that's all. K (collect) and `#` (tag) are free.
5. **Does the tab have multiple modes (subtabs)?** → declare `Subtabs []SubtabSpec`. Each subtab can shadow every behaviour field on `TabSpec`.
6. **Does the tab need a top widget (input box, status card)?** → `Widget *Widget`. Otherwise leave nil.

Anything else is bespoke. **Bespoke is a smell** unless you have a real reason — see "When to escape the registry" near the end.

---

## The nine steps

In order. Don't skip steps; don't combine them in your head before writing them. The point of this list is that it's mechanical.

(Step 8 — `dispatchSubtab` — is skippable for single-mode tabs. The other eight always apply.)

### 1. Declare the `Tab` and any `Subtab` constants

`internal/ui/tab.go` and `internal/ui/subtab.go`.

```go
// tab.go
const TabFoo Tab = "foo"

// subtab.go (only if you have subtabs)
const (
    SubtabFooList   Subtab = "foo-list"
    SubtabFooDetail Subtab = "foo-detail"
)
```

IDs are stable on disk (settings.toml + cache keys reference them). **Renaming an ID later is a breaking change for users** — pick names you'll keep.

### 2. Add the data layer to `orgData`

`internal/ui/model.go`. **Every list lives on `orgData`, full stop.** Do not park lists on `Model` because "the data is org-agnostic" — you'll spend an afternoon untangling that decision later (ask me how I know).

```go
type orgData struct {
    // ... existing fields ...

    // Foo tab.
    Foos          Resource[[]sf.Foo]      // typed cached payload
    FooList       ListView[sf.Foo]        // filterable + cursored slice
    FooTableState uilayout.ListTableState // column widths, sort, scroll
    FoosChipIdx   int                     // active chip cursor
}
```

Then in `newOrgData()` (same file), wire the `Resource` with its fetcher + cache key + TTL:

```go
d.Foos = Resource[[]sf.Foo]{
    Scope: username, Key: "foos", TTL: ttl("foos", 5*time.Minute),
    Fetch: func() ([]sf.Foo, error) { return sf.ListFoos(username) },
}
```

And the `ListView`'s `match` predicate (later in the same constructor). Always go through `SetMatch` — direct field assignment is impossible (`match` is private) and using the setter bumps the cache version so `Filtered()` rebuilds correctly. See "Filtered memo + invalidation" below.

```go
d.FooList.SetMatch(uilayout.MakeMatcher(uilayout.MatchSpec[sf.Foo]{
    Any: func(f sf.Foo) string { return strings.ToLower(f.Name + " " + f.Label) },
    Field: func(f sf.Foo, field string) string {
        switch field {
        case "Name":  return strings.ToLower(f.Name)
        case "Label": return strings.ToLower(f.Label)
        }
        return ""
    },
    Fields: []string{"Name", "Label"},
}))
```

The sync helpers in `internal/ui/model_sync.go` copy a `Resource[T]`
payload into its wrapping `ListView[T]`. Two pieces:

1. **Per-resource helper** — one method per resource, called by the
   update-loop dispatcher when only that resource lands so unrelated
   ListView cursors survive background refreshes:

   ```go
   func (d *orgData) SyncFoosList() {
       d.FooList.Set(d.Foos.Value())
   }
   ```

2. **Bulk fan-out** — `SyncListViews()` calls every per-resource
   helper. Used by bootstrap paths.

For a new tab, **add one line each** in two places:

```go
// model_sync.go: add the per-resource helper
func (d *orgData) SyncFoosList() {
    d.FooList.Set(d.Foos.Value())
}

// model_sync.go: register it in SyncListViews
func (d *orgData) SyncListViews() {
    // ... existing calls ...
    d.SyncFoosList()
}
```

Then in `internal/ui/update.go`'s resource dispatcher, route the
"foos" resource key to your per-resource helper:

```go
case "foos":
    if d.Foos.Apply(msg) {
        d.SyncFoosList()
    }
    if msg.FromCache {
        refresh = d.Foos.MaybeRefreshAfterCacheLoad(m.cache)
    }
```

### 3. Implement `query.Row` on the row type

`internal/sf/foo.go` (or wherever your row type lives).

This is what makes chip predicates work. `query.Row` is a one-method interface:

```go
func (f Foo) Field(name string) (any, bool) {
    switch name {
    case "Id":              return f.ID, true
    case "Name":            return f.Name, true
    case "NamespacePrefix": return f.NamespacePrefix, true
    case "IsActive":        return f.IsActive, true
    }
    return nil, false
}
```

**Surface every field a chip might want to predicate on.** It's cheap; missing fields silently fail to match.

### 4. Declare the `listSurface`

New file: `internal/ui/list_surface_foo.go`. Mirror an existing one — `list_surface_apex.go` is a good reference for "list of typed records with chip filter."

```go
package ui

import (
    "fmt"
    "charm.land/lipgloss/v2"
    "github.com/Jacob-Stokes/sf-deck/internal/devproject"
    "github.com/Jacob-Stokes/sf-deck/internal/sf"
    "github.com/Jacob-Stokes/sf-deck/internal/theme"
    "github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

var foosListSurface = listSurface{
    State:       func(d *orgData) *uilayout.ListTableState { return &d.FooTableState },
    Cols:        fooListCols,
    SearchPtr:   func(d *orgData) *searchState { return d.FooList.SearchPtr() },
    MoveCursor:  func(d *orgData, n int) { d.FooList.MoveBy(n) },
    ResetCursor: func(d *orgData) { d.FooList.ResetCursor() },
    BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
        if d == nil { return listRenderModel{}, false }
        items := d.FooList.Filtered()
        // bulk-load any per-row metadata (tags, projects) here, NOT inside Cell
        tagMap := m.bulkTagsForFoos(items)
        return listRenderModel{
            Title:  fmt.Sprintf("FOOS · %d", len(items)),
            State:  &d.FooTableState,
            Search: d.FooList.SearchPtr(),
            Cols:   fooListCols(),
            N:      len(items),
            Cursor: d.FooList.Cursor(),
            Cell: func(row, col int) string {
                if row < 0 || row >= len(items) { return "" }
                return fooCellAt(items[row], col)
            },
            Recolor:      func(row, col int, base lipgloss.Style) lipgloss.Style { return base },
            Empty:        "  no foos",
            FooterExtras: firstPretty(Keys.OpenDefault) + " open · " +
                          firstPretty(Keys.Refresh) + " refresh",
        }, true
    },
}

func fooListCols() []uilayout.ListColumn {
    return []uilayout.ListColumn{
        {Name: "Name",  Header: "NAME",  Min: 16, Ideal: 28},
        {Name: "Label", Header: "LABEL", Min: 16, Ideal: 32},
    }
}

func fooCellAt(f sf.Foo, col int) string {
    switch col {
    case 0: return f.Name
    case 1: return f.Label
    }
    return ""
}
```

**Rules:**
- `Cell` must be **pure** and **fast** — it's called once per visible cell per frame. Do all expensive work (tag joins, project lookups) **outside** the closure, capture the result, read inside.
- `BuildRenderModel` returns `ok=false` when the surface has no data ready. The orchestrating renderer handles "loading…" / "press r" states above the table.
- Don't re-implement search or filter machinery. The `match` predicate on `ListView` (installed via `SetMatch`) handles search; chip surfaces handle filter via `SetExtra`.

### 5. Declare the `chipSurface` (only if the tab has chips)

`internal/ui/chip_surface.go`. Same file every other surface lives in.

```go
var foosChipSurface = chipSurface{
    Domain:     domainFoos,
    ChipIdx:    func(m Model) int { return m.foosChipIdx() },
    SetChipIdx: func(m *Model, i int) { m.setFoosChipIdx(i) },
    ResetList:  func(d *orgData) { d.FooList.ResetCursor() },
    Registry:   func(m *Model) *qchip.Registry { return m.fooChips },
    ApplyChip: func(d *orgData, c qchip.Chip) {
        d.FooList.SetExtra(chipMatcherFor[sf.Foo](c))
    },
    // Optional — only if items can be project-scoped:
    ApplyProjectChip: func(d *orgData, scope *orgproject.Scope) {
        d.FooList.SetExtra(func(f sf.Foo) bool { return scope.HasFoo(f.ID) })
    },
    ScopeCount: func(s *orgproject.Scope) int { return len(s.FooIDs) },

    // ManagerTitle is the title shown atop the V (chip manager)
    // modal. Required for V to feel polished — without it the
    // modal renders the fallback "Views" placeholder. Receives
    // Model so per-row scopes (Records → "Views · Account") can
    // compose the right label.
    ManagerTitle: func(Model) string { return "Views · Foos" },

    // ImportFromSF flags whether V offers "Import from
    // Salesforce…" — the Lightning list-view import flow. Only
    // meaningful for ListView entities (Records, Flows). Default
    // false; opt in only when the import flow makes sense.
    // ImportFromSF: true,
}
```

That's it — no separate registry map to add to. The `TabSpec.Chips` (or `SubtabSpec.Chips`) pointer in step 8 is the single declaration site. `allChipSurfaces()` walks the registry to build the iterable that `domainFromRegistry`, `chipSurfaceForDomain`, and the project-chip cursor compensator need.

### Filtered memo + invalidation

`ListView[T].Filtered()` is called 2-3 times per render (once in `MoveBy` to clamp the cursor, once in `BuildRenderModel`, once in `Selected()` for the sidebar) and on every wheel tick / keystroke. To keep that cheap, the result is memoised on the `ListView` and only rebuilt when one of the filter inputs changes.

**Cache key**: `(version int, search-buffer string)`.

**Bumps version → invalidates cache**:
- `lv.Set(items)` — the underlying slice changed
- `lv.SetExtra(fn)` — the chip predicate changed
- `lv.SetMatch(fn)` — the search matcher changed

**Cache rebuilds also when `Search.Buffer()` changes** (typing in `/`).

**Why `extra` and `match` are private fields with setters**: assigning them directly via a public field would silently leave `Filtered()` returning a stale slice — the cache version wouldn't bump, the next call would return last frame's filtered rows even though the predicate had changed. The setters make invalidation impossible to forget.

**The contract for callers**: never reach inside the ListView. Use `SetExtra` / `SetMatch` / `Set` for writes, `Filtered` / `Items` / `Cursor` / `Selected` for reads. The returned slice from `Filtered()` is the cached pointer — treat as read-only. Mutating it would corrupt the next render.

`HasMatch()` is a tiny exception: lazy-init code paths (e.g. `reloadSOQLSaved`) want to install a matcher only on first run. `if !lv.HasMatch() { lv.SetMatch(...) }` does that without exposing the private field directly.

### 6. Define the chip domain + builtins + registry

Three small additions:

**`internal/ui/chip_manager.go`** — add a `chipDomain` constant:

```go
domainFoos chipDomain = "foos"
```

**`internal/ui/qchip/builtins.go`** — define the built-in chip set:

```go
var FooBuiltins = []Chip{
    {
        ID: "all", Label: "All", Scope: "*", Origin: OriginBuiltIn,
        Favourite: true, LockedFavourite: true,
        Query: query.Query{},
    },
    {
        ID: "active", Label: "Active", Scope: "*", Origin: OriginBuiltIn,
        Favourite: true,
        Query: query.Query{Where: query.Cmp("IsActive", query.OpEq, true)},
    },
}
```

**`internal/ui/model.go`** — add the per-`Model` registry pointer:

```go
fooChips *qchip.Registry // /foo chips
```

In `New(...)`:

```go
fooChips: qchip.NewRegistry("foos", qchip.FooBuiltins),
```

After the `m.flowChips.LoadFromSettings(st)` line:

```go
m.fooChips.LoadFromSettings(st)
```

And the `chipIdx` accessors in `internal/ui/active_state.go`:

```go
func (m Model) foosChipIdx() int {
    if d := m.activeOrgData(); d != nil { return d.FoosChipIdx }
    return 0
}

func (m *Model) setFoosChipIdx(i int) {
    if len(m.orgs) == 0 { return }
    m.ensureOrgData(m.orgs[m.selected].Username).FoosChipIdx = i
}
```

V (chip manager), N (new chip), M (overflow), F (favourite), ←/→ (cycle) all work for free once these are in place. **You don't add per-tab key handlers for any of them** — the dispatcher routes via the registry domain.

V and M call `m.resolveChipSurface()`, which walks the registry to find the active surface (subtab → tab → fallback for /records). The surface's `Domain` + `ManagerTitle` + `ImportFromSF` feed the modal directly. As long as you've declared `TabSpec.Chips` (or `SubtabSpec.Chips`) in step 8, V/M will Just Work.

### 7. Declare the `openSurface` (if Enter opens a Lightning URL)

If your row type has a stable Lightning URL, declare an `openSurface` in `internal/ui/open_surface.go`. Most simple cases follow this shape:

```go
var foosOpenSurface = openSurface{
    Openable: func(m Model) sf.Openable {
        d, ok := m.activeOrgState()
        if !ok { return nil }
        f, ok := d.FooList.Selected()
        if !ok { return nil }
        return f // sf.Foo implements sf.Openable
    },
}
```

**Don't bother if your `Identity` already returns an `Openable`.** Identity-first wins; `openSurface` is the fallback for surfaces that don't have Identity yet.

### 8. Use `dispatchSubtab` if your tab has subtabs

Single-mode tabs skip this step. Tabs with multiple subtabs (Reports, Apex, Components, SOQL, Meta) **must** route through `dispatchSubtab` — it draws the subtab strip exactly once and routes to the active branch, killing the "default branch forgot the strip" bug class.

```go
func (m Model) renderFoos(w, innerH int) string {
    return m.dispatchSubtab(w, innerH, fooSubtabs(), m.fooSubtab(),
        map[Subtab]subtabBranch{
            SubtabFooDetails: {Render: m.renderFooDetails},
            SubtabFooHistory: {Placeholder: &subtabPlaceholder{
                Header:      "FOO HISTORY",
                Description: "Coming soon — execution log per foo.",
                SetupURL:    "/lightning/setup/FooHistory/home",
            }},
        },
        subtabBranch{Render: m.renderFooList},
    )
}
```

Rules:

- The branch's `Render` closure receives `(w, innerH int)` where `innerH` is **already reduced** by the strip's height. Don't draw the strip yourself.
- `Placeholder` takes precedence over `Render` when both are set — useful for half-built subtabs that should fall back to "coming soon" until the renderer is ready.
- A nil `defaultBranch` is fine for tabs that are entirely placeholders (e.g. `/meta`).

If the subtab list is dynamic (one subtab per LWC bundle file, subtabs vary by parent kind), `dispatchSubtab` doesn't fit — fall through to the bespoke pattern. See "When to escape the registry."

#### Lazy-loading on subtab entry

If a subtab needs to hydrate cached data the first time the user enters it (e.g. SOQL Library reading from the local store), declare an `OnEnter` closure on the SubtabSpec instead of branching inside `SetSubtabIdx`:

```go
{
    ID:    SubtabFooHistory,
    Label: "History",
    List:  &fooHistoryListSurface,
    OnEnter: func(m *Model) {
        if d, ok := m.activeOrgState(); ok && !d.FooHistoryLoaded {
            m.reloadFooHistory(d)
        }
    },
},
```

Wire `SetSubtabIdx` through the standard helper so it walks the active subtab and calls OnEnter automatically:

```go
SetSubtabIdx: setSubtabWithOnEnter(TabFoo, func(m *Model, i int) { m.fooSubtabIdx = i }),
```

`OnEnter` is for synchronous local work (cache reads, cursor resets). For network fetches, use `EnsureData` + the `tea.Cmd` pipeline so the spinner + error chrome work.

### 9. Register the `TabSpec`

`internal/ui/tab_registry.go`, in `tabRegistry()`. Put it next to similar tabs alphabetically.

```go
TabFoo: {
    Tab:         TabFoo,
    Stem:        TabFoo,
    Renderer:    Model.renderFoos,
    Chips:       &foosChipSurface,
    Open:        &foosOpenSurface,
    List:        &foosListSurface,
    Identity:    identityFromFoosList,
    Sidebar:     Model.sidebarFoos,
    BusyLabel:   busyFoos,
    ErrorLabel:  errFoos,
    EnsureData:  func(m *Model, d *orgData, o sf.Org) tea.Cmd { return d.Foos.Ensure(m.cache) },
    RefreshData: func(m Model, d *orgData) tea.Cmd { return d.Foos.Refresh(m.cache) },
},
```

For a **single-mode tab** the renderer is ~15 lines of orchestration. Match `tab_apex.go::renderApexClasses` for shape:

```go
func (m Model) renderFoos(w, innerH int) string {
    d, ok := m.activeOrgState()
    if !ok { return theme.Subtle.Render("  no org selected") }

    inner := w - 4
    chips := m.stripRows(domainFoos, "*")
    dash := m.renderDashboard("VIEWS", chips, m.foosChipIdx(), inner)

    var lines []string
    if dash != "" { lines = append(lines, dash) }

    // Empty / loading states.
    if d.Foos.FetchedAt().IsZero() {
        if d.Foos.Busy() {
            lines = append(lines, dimLine("  loading foos…", inner))
        } else {
            lines = append(lines, dimLine("  press r to load foos", inner))
        }
        return strings.Join(lines, "\n")
    }

    model, ok := foosListSurface.BuildRenderModel(m, d)
    if !ok {
        lines = append(lines, dimLine("  loading…", inner))
        return strings.Join(lines, "\n")
    }
    budget := innerH - len(lines)
    lines = append(lines, renderListModel(m, model, m.focus, inner, budget, len(lines))...)
    return strings.Join(lines, "\n")
}
```

For a **multi-subtab tab**, the top-level `Renderer` calls `dispatchSubtab` (see step 8) and you write one body-renderer per subtab. The body-renderers do NOT prepend the strip — `dispatchSubtab` owns that.

The **smoke test** (`render_smoke_test.go::TestRenderEveryTab`) walks the registry and renders every tab in two scenarios — your tab is automatically covered the moment it's registered. Don't write a per-tab render test unless you're testing something specific to that tab.

---

## What you get for free once you've done the steps

By following the registry, you get all of this without writing per-tab code:

| Feature | Source |
|---|---|
| Number-key nav (`2`) | TabsForNumbers slice + Stem field |
| Subtab nav (`Tab` / `Shift+Tab`) | `Subtabs` array + GetSubtabIdx/SetSubtabIdx |
| Subtab strip rendered consistently | `dispatchSubtab` (multi-subtab tabs only) |
| Search (`/`) with yellow border | `listSurface.SearchPtr` |
| Chip strip + cycle (`←/→`) | `chipSurface` + qchip Registry |
| Chip manager (`V`) | qchip Registry + chip-manager modal |
| New chip (`N`) | qchip Registry + chip-wizard modal |
| Chip overflow (`M`) | qchip Registry |
| Favourite chip (`F`) | qchip Registry |
| Project chip | `chipSurface.ApplyProjectChip` |
| Open in Lightning (`o` / `ctrl+o`) | Identity.Openable or openSurface |
| Yank URL (`y` / `ctrl+y`) | Same as above |
| Pin to project (`K` / `ctrl+k`) | Identity (Kind + Ref) |
| Tag (`#`) | Identity (Kind + Ref) |
| Refresh (`r`) | TabSpec.RefreshData |
| Auto-fetch on entry | TabSpec.EnsureData |
| Column mode (`c`) | listSurface.State |
| Sort (`s`) | listSurface.State (in column mode) |
| Resize columns (`[` / `]`) | listSurface.State + Cols |
| Snap-to-content (`}`) | listSurface.MeasureCell or BuildRenderModel.Cell |
| Horizontal scroll (`,` / `.`) | listSurface.State |
| Zen mode (`z`) | listSurface.State |
| Pagination mode (`P`) | listSurface.State (Paginated + Page) |
| Status-bar busy spinner | TabSpec.BusyLabel |
| Status-bar error pill | TabSpec.ErrorLabel |
| Smoke test coverage | TabSpec is enumerable; the test walks it |
| Registry completeness checks | `tab_registry_test.go` walks the registry |

---

## Common mistakes I've made and you should avoid

### 1. Putting list state on `Model` instead of `orgData`

**Symptom:** "saved queries are org-agnostic so they should live on Model."
**Reality:** the chip system + listSurface registry both thread `*orgData` through every callback. Putting your list elsewhere means inventing parallel dispatch (`ApplyChipModel`, etc.) — you'll fight every contract in the codebase.
**Rule:** every list goes on `orgData`. Even if the underlying data is shared across orgs, give each org its own ListView. Memory cost is trivial; design integrity is huge.

> **Subtlety:** for **shared-across-orgs** data (saved queries today; saved searches, plugin manifests in the future), each org's ListView still lives on `orgData`, but every mutation has to invalidate every org's snapshot. See "Shared-store lists" below for the current pattern + the planned long-term answer.

### 2. Using `chipMatcherFor[T]` without `T` implementing `query.Row`

**Symptom:** chip predicates silently match nothing.
**Reality:** `chipMatcherFor` walks the chip's query AST and calls `T.Field(name)` on each row. If `T` doesn't implement `query.Row`, the matcher returns false universally.
**Rule:** Step 3 above. Don't skip it.

### 3. Hand-rolling chip rendering instead of using the registry

**Symptom:** "I'll just draw a few chip pills inline." Then V doesn't work, N doesn't work, M doesn't work, project chip doesn't work, persistence doesn't work.
**Reality:** The chip strip you draw is ~5% of what users expect from a chip strip.
**Rule:** if your tab has chips at all, declare a `chipSurface` + `qchip.Builtins`. The first version takes one extra hour and gives you 100% of the standard vocabulary.

### 4. Doing expensive work inside `Cell`

**Symptom:** scrolling lags. Tag pills flicker.
**Reality:** `Cell(row, col)` is called once per visible cell per frame. If it queries the store, you're hitting SQLite N times per render.
**Rule:** do bulk lookups (tags, projects) once at the top of `BuildRenderModel`, capture the result map, read in `Cell`. See `bulkTagsForFlows` for the pattern.

### 5. Re-implementing busy / error chrome

**Symptom:** "I'll just check the resource state and render a banner."
**Reality:** `BusyLabel` + `ErrorLabel` on `TabSpec` give you the standard status-bar busy spinner + red error pill, consistent with every other tab.
**Rule:** declare them; let the global status bar render the chrome.

### 6. Forgetting to register your TabSpec entry

**Symptom:** smoke test passes (because you didn't register), but the tab silently doesn't work at runtime.
**Reality:** `tabRegistry()` is the source of truth. If your tab isn't there, dispatch falls back to the bespoke pre-registry path which is incomplete.
**Rule:** register first, then test. The completeness test (`tab_registry_test.go`) will tell you what's missing.

### 7. Passing `*Model` through new code paths

**Symptom:** "I just need to mutate this one field, I'll take a `*Model`."
**Reality:** Bubble Tea's whole concurrency model is value-Model. Every reducer returns a fresh `(Model, tea.Cmd)`. If you find yourself wanting `*Model` deep inside dispatch, you've usually put state in the wrong place. Move state to `orgData` (which is a pointer) or `m.activeOrgState()`'s receiver, then mutate through the pointer you already have.
**Rule:** if you're changing a function signature from `Model` to `*Model`, stop and reconsider where the state lives.

### 8. Hand-rolling the subtab strip in each branch

**Symptom:** "I'll just render the strip at the top of my main renderer and prepend it in each branch." Then you forget one branch (or change the strip later and miss a callsite), and that subtab silently loses the pills until the user cycles away with Tab and back. This shipped on /reports earlier today.
**Reality:** `dispatchSubtab` owns the strip. Branch closures should draw their body and nothing else; the dispatcher prepends the strip exactly once and reduces the budget the body sees by however many lines the strip ate.
**Rule:** if your tab has subtabs, route through `dispatchSubtab`. If the existing branch renderers take `subStrip` as a parameter, that's a smell — they predate the helper and should be migrated.

---

## When to escape the registry

Some tabs legitimately can't fit. Concrete examples in the codebase:

- **TabObjectDetail** has per-subtab chip strips, each with its own cursor (Schema/Validation/Records each have different chip sets). The single `chipSurface` shape doesn't fit; uses `TabSpec.CycleChip` (a bespoke `func(*Model, int) tea.Cmd`).
- **TabRecords** has picker-mode (sObject list) and list-mode (records of selected sObject). Single dispatcher hand-rolls the branch.
- **TabPermParentDetail** varies its subtabs by parent kind (PermSet / PSG / Profile). Subtabs are computed at render time, not declared.

The pattern: when a tab is the **only** one with a particular shape, escape into a bespoke hook on `TabSpec` (CycleChip, MoveCursor closure, etc.). When a shape recurs across tabs, lift it into the registry.

**Don't escape the registry to avoid 30 minutes of plumbing.** Escape it when the registry contract genuinely can't express what you're doing.

---

## Shared-store lists (the "saved queries" case)

**99% of lists are org-scoped** — sObjects describe metadata in *that* org, flows live in *that* org, etc. Each org has its own data, lives on its own `orgData`, and there's nothing more to think about.

The exception: **user artefacts shared across orgs**. Today that's saved SOQL queries. Future candidates: saved searches, user-defined chips, plugin manifests, project-bundle templates. The defining trait is "the user authored it once and expects to see it on every org."

### How it works today (current pattern)

The data lives in *one* SQLite table (`saved_queries`). Each `orgData` carries its own `ListView`, table state, and chip cursor — but every org's ListView is loaded fresh from the same store, so they hold equivalent data. Cursors and search buffers are intentionally per-org.

The wart: **mutations have to invalidate every org's snapshot**, otherwise switching orgs after a save shows stale data. The pattern is `invalidateSOQLSaved()`:

```go
func (m *Model) invalidateSOQLSaved() {
    for _, d := range m.data {
        if d != nil {
            d.SOQLSavedLoaded = false
        }
    }
}
```

Every mutation handler calls it. It's enforceable by code review but easy to forget.

**If you're adding a new shared-store list today**, follow this pattern:
1. Field on `orgData`: `<Thing>List`, `<Thing>Loaded`, `<Thing>TableState`, `<Thing>ChipIdx` — same as any other list.
2. Add a comment on the field: `// shared across orgs — call invalidate<Thing>() after any mutation`.
3. Write `invalidate<Thing>()` that walks `m.data` and sets every `<Thing>Loaded = false`.
4. Every Create / Update / Delete / Duplicate handler calls it.

### How we'll do it long-term (planned)

The right shape is **shared backing slice on `Model`, ListView wrappers on `orgData`**. `ListView` gains a `Wrap(*[]T)` mode (alongside the current `Set([]T)`):

```go
// Wrap binds the ListView to an external backing slice. Filtered()
// reads through the pointer, so updates to the backing slice are
// visible to every wrapping ListView immediately. Mutually
// exclusive with Set — a wrapped ListView can't be Set, and vice
// versa.
func (lv *ListView[T]) Wrap(items *[]T)
```

The data model becomes:

```go
type Model struct {
    soqlSavedItems []devproject.SavedQuery  // single source of truth
    soqlSavedDirty bool
    // ...
}

type orgData struct {
    // ListView wraps Model.soqlSavedItems — the field still
    // lives on orgData so per-org cursor / search / chip state
    // is preserved, but the items are shared.
    SOQLSavedList ListView[devproject.SavedQuery]
    // ... per-org table state, chip idx ...
}
```

In `newOrgData`:

```go
d.SOQLSavedList.Wrap(&m.soqlSavedItems)
```

Mutations become:

```go
func (m *Model) reloadSOQLSaved() {
    m.soqlSavedItems, _ = m.devProjects.ListSavedQueries()
}
// every mutation handler calls reload — every org's wrapping
// ListView sees the new data automatically. No invalidate-walk.
```

**Why this is right:** one source of truth in memory, no cross-org invalidation discipline, contracts (`chipSurface`, `listSurface`) stay unchanged because they still see `*orgData` and a normal `ListView`.

**Cost:** ~30 lines in `internal/ui/resource/listview.go` plus migrating the SOQL handlers.

### When to actually do the migration

Two triggers — until one fires, the current pattern is fine:

1. **Second shared-store list lands.** When a saved-search / macro / plugin-tab feature ships, the invalidate-everywhere pattern starts repeating. Two callers = formalize.
2. **A real bug from forgotten invalidation.** If somebody adds a new mutation path, forgets to call `invalidate<Thing>()`, and a user notices stale data — that's the signal to make the bug structurally impossible.

Don't pre-emptively migrate just for elegance. The current pattern works; the planned one is *less wrong*, but the difference is subtle in practice.

---

## Things on the to-do list (so you know they're not your fault)

- Some kinds (sObject, Records, ObjectDetail) have project-collect deep wizards. New kinds get the simple flow only — that's fine for v1.
- LWC bundle subtabs are dynamic (one per file in the bundle). If you ever need that shape, see `lwcDetailSubtabs`.
- `ListView.Wrap(*[]T)` for shared-store lists — see "Shared-store lists" above.

### Recently resolved (kept here for the historical record)

- ~~V/M/F dispatch in `update_keys.go` hardcoded per-tab.~~ Now registry-driven via `m.resolveChipSurface()`. Each `chipSurface` declares its own `ManagerTitle` + `ImportFromSF`.
- ~~`chipSurfaces()` map duplicated `tabRegistry()`.~~ Replaced by `allChipSurfaces()` which walks the registry.
- ~~Default subtab branch could forget to render the strip.~~ `dispatchSubtab` owns the strip — branches can't drop it.

---

## Checklist

Print this. Tick as you go.

```
Step 1: constants
  [ ] internal/ui/tab.go      — TabFoo declared
  [ ] internal/ui/subtab.go   — any SubtabFoo* declared

Step 2: orgData
  [ ] orgData struct          — Foos, FooList, FooTableState, FoosChipIdx
  [ ] newOrgData()            — Resource declared with Fetch
  [ ] newOrgData()            — d.FooList.SetMatch(...) wired
  [ ] model_sync.go           — SyncFoosList helper added
  [ ] model_sync.go           — SyncListViews fans into SyncFoosList
  [ ] update.go               — case "foos" routes to d.SyncFoosList()

Step 3: query.Row
  [ ] internal/sf/foo.go      — Field(name) implemented

Step 4: list_surface
  [ ] internal/ui/list_surface_foo.go — listSurface declared
  [ ] fooListCols, fooCellAt — defined

Step 5: chip_surface (skip if no chips)
  [ ] foosChipSurface declared
  [ ] ManagerTitle closure set (so V's modal has a label)
  [ ] ImportFromSF set (only if you want SF-list-view import)

Step 6: chip domain + builtins (skip if no chips)
  [ ] chip_manager.go         — domainFoos constant
  [ ] qchip/builtins.go       — FooBuiltins
  [ ] model.go                — fooChips *qchip.Registry field
  [ ] model.go                — qchip.NewRegistry in New()
  [ ] model.go                — LoadFromSettings call
  [ ] active_state.go         — foosChipIdx + setFoosChipIdx

Step 7: open_surface (skip if Identity carries Openable)
  [ ] openSurface declared

Step 8: dispatchSubtab (skip if single-mode tab)
  [ ] subtabBranch map declared in renderFoos
  [ ] one body-renderer per subtab — none of them prepend the strip
  [ ] default branch declared (or nil for all-placeholder tabs)

Step 9: TabSpec
  [ ] tab_registry.go         — TabFoo entry in tabRegistry()
  [ ] renderFoos function written (calls dispatchSubtab if subtabs)
  [ ] sidebarFoos function written
  [ ] busyFoos / errFoos written
  [ ] identityFromFoosList written

Final
  [ ] go test ./...           — all green
  [ ] go build ./...          — clean
  [ ] manual smoke            — number-key nav works, search works, chips work,
                                K pins to project, # tags, V opens manager
```

If everything on the list is done and you can land on your new tab, search it, chip it, pin a row to a project, and tag it — you've wired it correctly. Welcome to the codebase.
