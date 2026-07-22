# Cache architecture — full deep dive

This is the complete map of how caching works in sf-deck. Read it
when:

- You're adding a new piece of data and need to know which layer
  to use
- A user reports stale / fresh / wrong-data and you need to trace
  why
- You're debugging an "expected the cache to refresh but it
  didn't" symptom
- You want to understand why some data ends up on disk and other
  data doesn't

The companion [scrolling and render performance guide](scrolling-architecture.md)
covers the performance-sensitive render-time memos (Layer 3 below) in
more detail. This document covers all four layers and the machinery that
drives them.

## TL;DR

Four caches at descending levels of permanence:

| # | Layer | Where | Lifetime | Holds |
|---|---|---|---|---|
| 1 | On-disk SQLite | `~/.sf-deck/cache.db` | survives reboot | raw API responses as JSON blobs |
| 2 | Per-process in-memory | `Resource[T]` on `orgData` / `Model` | process | typed Go values + freshness state |
| 3 | Per-render memos | `renderCache`, `Filtered()` memo, gutter cache, projection cache | until input changes | computed render artifacts |
| 4 | HTTP client cache | `sf.RESTClient` registry | process | authenticated REST clients (instance URL + token) |

Driving mechanisms:

- **TTL** (per Resource) — passive staleness threshold
- **Ensure / Refresh / Apply** — Resource lifecycle (Layer 2 ↔ Layer 1)
- **`r` keymap** — user-forced refresh
- **`OnSuccess` hooks** — write-path refreshes after mutations
- **`devproject.Generation`** — counter that invalidates Layer 3 memos when tags / projects change
- **Lifecycle events** (`orgsChangedMsg`, alias change) — refresh + `InvalidateRESTClients()`
- **Startup wipes** — `DeleteKeyPrefix` for keys that should never have been cached

## Layer 1 — SQLite on disk

**File:** `~/.sf-deck/cache.db`
**Package:** `internal/cache/`
**Schema:** `internal/cache/cache.go:45`

Two tables. Both have a `cached_at` Unix timestamp.

### `orgs` table

Holds the `sf org list` output. One row per Salesforce org the
user has authenticated to via the `sf` CLI. Drives the org rail.

```sql
CREATE TABLE orgs (
  username     TEXT PRIMARY KEY,
  alias        TEXT,
  instance_url TEXT,
  org_id       TEXT,
  is_sandbox   INTEGER,
  is_scratch   INTEGER,
  is_devhub    INTEGER,
  status       TEXT,
  last_used    TEXT,
  cached_at    INTEGER NOT NULL
);
```

Refreshed via `orgsChangedMsg` (auth/login/logout/alias actions
in the org-manage modal) or by `r` on the org rail. Written by
`Cache.UpsertOrgs`, read by `Cache.AllOrgs`.

### `kv` table

The general-purpose blob store. Every other Resource lives here.

```sql
CREATE TABLE kv (
  org_username TEXT NOT NULL,  -- scope: org username, or "global"
  key          TEXT NOT NULL,  -- resource key (see "Key naming")
  value        TEXT NOT NULL,  -- JSON-serialized payload
  cached_at    INTEGER NOT NULL,
  PRIMARY KEY (org_username, key)
);
```

Mutation APIs:

- `Cache.Set(scope, key, payload) error` — write JSON-serialised value
- `Cache.GetJSON(scope, key, dst any) (time.Time, bool, error)` — read + deserialise
- `Cache.DeleteKeyPrefix(prefix string) (int, error)` — bulk delete by key prefix

**No automatic eviction.** Rows live forever unless explicitly
deleted. We trade disk space for predictable freshness behaviour.
Typical real-world cache.db sizes: 1-50 MB, growing with the
number of describes/lwcs/auras fetched.

### Key naming conventions

The key shape encodes (1) what the data is and (2) what
constrains it. Examples from the codebase:

| Pattern | Example | What it stores |
|---|---|---|
| `<name>` | `sobjects` | per-org list of sObjects |
| `<name>:<param>` | `describe:Account` | per-(org, sobject) describe |
| `<children-key>:<sobject>` | `validationrules:Account` | child list per sObject |
| `<detail-key>:<id>` | `recordtypedetail:01I580...` | one detail record |
| `<plural>:<sobject>` | `flowversions:0Eo580...` | versions per definition |
| `<chip-shaped>:<sobject>:<chipID>` | `chiprecords:Account:active_only` | records per (sObject, chip) |
| `object_baseline:<sobject>` | `object_baseline:Account` | Tooling CustomObject baseline |

The first colon-delimited segment is also the **routing prefix**
used by the Update-loop dispatcher (see Layer 2 → "Message
routing").

### What's in `kv` today (representative — not exhaustive)

| Prefix | Resource | Typical TTL |
|---|---|---|
| `home` | per-user home data | 10 min |
| `org_info` | Organization sObject | 1 hr |
| `sobjects` | EntityDefinition catalogue | 4 hr |
| `describe:` | per-sObject describe | 4 hr |
| `apex_classes` | flat Apex Class list | 30 min |
| `apex_class:` | per-class Apex source | 1 hr |
| `apex_triggers_flat` | flat trigger list | 30 min |
| `triggers:` | per-sObject trigger rows | 30 min |
| `triggerdetail:` | per-trigger detail | 30 min |
| `validationrules:` | per-sObject VR rows | 30 min |
| `validationruledetail:` | per-VR detail | 30 min |
| `recordtypes:` | per-sObject RT rows | 30 min |
| `recordtypedetail:` | per-RT detail | 30 min |
| `flows` | flat flow list | 15 min |
| `flowversions:` | per-definition versions | 5 min |
| `apex_logs` | log catalogue | 30 sec |
| `deploys` | recent deploys | 2 min |
| `packages` | installed packages | 2 hr |
| `lwc_bundles` | LWC bundle list | 1 hr |
| `lwc_bundle:` | per-LWC source | 4 hr |
| `aura_bundles` | Aura bundle list | 1 hr |
| `aura_bundle:` | per-Aura source | 4 hr |
| `permsets` | permission set list | 1 hr |
| `psgs` | permission set group list | 1 hr |
| `profiles` | profile list | 1 hr |
| `queues` | queue list | 30 min |
| `public_groups` | public group list | 1 hr |
| `notifications` | notification feed | 30 sec |
| `recently_viewed` | RecentlyViewed | 30 sec |
| `recent` | local /recent log | (no TTL, append-only) |
| `home_users` | recent logins | 5 min |
| `reports` | reports listing | 30 min |
| `report_folders` | folder tree | 1 hr |

Full source of truth: the `cacheResourceCatalog` slice in
`internal/ui/modal_cache_settings.go`.

### Things that are deliberately NOT cached to disk

These run as `NoCache:true` Resources — they're still in-memory
(Layer 2) for the session but never touch SQLite:

- **Records** (`records:`, `chiprecords:`) — actual SF record
  data. PII concerns; staleness misleads (record might have
  been edited/deleted).
- **Record details** (`recorddetail:`) — same reasoning.
- **Custom object baselines** (`object_baseline:`) — toggles can
  change via Setup outside our control; staleness misleads.
- **Report runs** (`reportrun:`) — point-in-time aggregates.
  Persisting would confuse "ran 3 days ago" vs "ran just now."
- **List views** (`listviews:`, `listview:`) — historically
  cached then disabled. Per-user sharing + live data. Startup
  also `DeleteKeyPrefix`es these (one-time cleanup of legacy
  cache rows).
- **Flow source bodies** — large per-flow XML; cheap to re-fetch
  on demand.

If you're adding a new Resource and unsure, default to
`NoCache:true`. Adding caching later is easy; removing it (and
explaining to users why their cached PII is on disk) is hard.

### Startup wipes

`cmd/sf-deck/main.go:69` proactively deletes leaked legacy keys
on every process start:

```go
_, _ = c.DeleteKeyPrefix("records:")
_, _ = c.DeleteKeyPrefix("listviews:")
_, _ = c.DeleteKeyPrefix("listview:")
```

These are belt-and-braces — the corresponding Resources are all
`NoCache:true` now, so no new writes happen. But older versions
of sf-deck did persist these; the deletes catch any stale rows
sitting on disk from before NoCache landed.

## Layer 2 — Per-process in-memory (`Resource[T]`)

**File:** `internal/ui/resource/resource.go`
**Type:** `Resource[T any]`

This is the core unit. Every piece of fetched data lives in a
Resource. The Resource is the bridge between Layer 1 (SQLite)
and the UI (renderer reads `.Value()`).

### Anatomy

```go
type Resource[T any] struct {
    Scope string      // "global" or org username — matches kv.org_username
    Key   string      // the lookup key — matches kv.key
    TTL   time.Duration

    NoCache bool      // true = skip Layer 1 (records, baselines, PII)
    Fetch   func() (T, error)            // standard fetcher
    FetchWithExisting func(existing T) (T, error)  // delta fetcher

    data      T          // the typed value
    fetchedAt time.Time  // zero = never loaded
    err       error
    busy      bool       // a fetch is in flight
}
```

The zero value is a valid "never loaded" Resource — callers can
declare and use one without explicit init beyond setting
Scope/Key/TTL/Fetch.

### Where Resources live

Three storage shapes:

1. **Singletons on `orgData`** — one field per data kind.
   Examples: `d.Home`, `d.SObjects`, `d.ApexClasses`, `d.Flows`.
   See `internal/ui/model.go` lines 119-220 for the full list.

2. **Per-key maps on `orgData`** — for data that varies by some
   identifier (sObject API name, chip ID, record ID, etc.):

   ```go
   Describes        map[string]*Resource[sf.SObjectDescribe]      // key: sobject
   FlowVersions     map[string]*Resource[[]sf.FlowVersion]        // key: definition ID
   ReportRuns       map[string]*Resource[sf.ReportRun]            // key: report ID
   ChipRecords      map[string]*Resource[sf.RecordsList]          // key: "sobject:chipID"
   ChipUsers        map[string]*Resource[sf.UsersList]            // key: chip ID
   CustomObjectBaselines map[string]*Resource[*sf.CustomObjectBaseline]
   // etc.
   ```

3. **Globals on `Model`** — org-agnostic:

   ```go
   orgsRes     Resource[[]sf.Org]
   projectsRes Resource[[]*project.Project]
   ```

### Lifecycle: Ensure / Apply / Refresh

The three operations every Resource supports. They drive the
transition between "cold" (never loaded), "warm" (loaded from
cache, possibly stale), and "fresh" (just fetched).

#### `Ensure(c *cache.Cache) tea.Cmd`

Called by tab `EnsureData` hooks. Returns the commands needed to
bring the Resource up-to-date. Safe to call on every render /
nav — idempotent when already fresh.

```
Ensure logic:
   NoCache:
     stale + not busy + can fetch → refreshCmd
     else                         → no-op
   Cacheable:
     never loaded + not busy      → loadCmd (Layer 1 read)
     loaded but stale + not busy  → refreshCmd
     fresh                        → no-op
```

`loadCmd` reads the SQLite blob and emits
`UpdatedMsg{FromCache: true}`. `refreshCmd` calls `Fetch()` in a
goroutine, writes the result to SQLite (unless NoCache), and
emits `UpdatedMsg{FromCache: false}`.

#### `Apply(msg UpdatedMsg) bool`

The single writer. Lives on the Update loop, so single-threaded
by construction. Folds an `UpdatedMsg` back into the Resource:

- **Cache load (FromCache:true)**: populates `data` + `fetchedAt`
  if not already populated (a fresh fetch may have raced and
  won). Doesn't touch `busy` or `err`.
- **Fresh fetch (FromCache:false)**: clears `busy`, sets `err`,
  updates `data` + `fetchedAt` to `time.Now()` if the fetch
  succeeded.

Returns `true` when the message was for this Resource (caller
stops routing).

#### `Refresh(c *cache.Cache) tea.Cmd`

Force-fetch regardless of TTL. Used by the `r` keymap and write-
path `OnSuccess` hooks. No-op if already busy.

#### `MaybeRefreshAfterCacheLoad(c) tea.Cmd`

Called by the dispatcher after a `FromCache:true` apply. Decides
whether to ALSO fire a network refresh:

```
if NoCache              → no
if busy or no fetcher   → no
if !Stale()             → no (cache was fresh, skip network)
otherwise               → yes, fire refreshCmd
```

This is what makes the "load from cache first, refresh if stale"
short-circuit work. Without it, the cold path on a fresh-but-on-
disk cache row would always do a redundant network fetch.

### Cold start trace (example: /objects on fresh launch)

1. User runs `sf-deck`, lands on /home (or wherever).
2. Eventually switches to /objects → tab dispatcher fires
   `ensureObjectsData` → calls `d.SObjects.Ensure(m.cache)`.
3. `fetchedAt` is zero, `NoCache` is false → `loadCmd` emits.
4. Goroutine reads `kv` row `(orgname, "sobjects")`. Either:
   - **Hit + fresh**: `UpdatedMsg{FromCache:true, Payload:&value, CachedAt:t}`.
     Apply populates. `MaybeRefreshAfterCacheLoad` sees `!Stale()`, no-op.
     Total cost: one SQLite SELECT.
   - **Hit + stale**: same as above for Apply.
     `MaybeRefreshAfterCacheLoad` sees `Stale()`, fires `refreshCmd`.
     User sees the cached data immediately; the screen updates
     when the fresh fetch lands.
   - **Miss**: `UpdatedMsg{FromCache:true, Payload:nil}`.
     Apply no-ops (FromCache + nil payload = "nothing to fold in").
     `MaybeRefreshAfterCacheLoad` sees `Stale()` (because never
     loaded), fires `refreshCmd`. User sees loading state.
5. `refreshCmd` (when triggered) calls `Fetch()` → `sf.ListSObjects(alias)` →
   shell to `sf` CLI → JSON → struct. Result written to SQLite,
   `UpdatedMsg{FromCache:false}` emitted. Apply updates in-memory data.
6. Next render reads `d.SObjects.Value()`. Done.

### Message routing — `applyResourceMsg`

When an `UpdatedMsg` arrives at Bubble Tea's Update loop, the
dispatcher in `internal/ui/update.go:809` routes it to the right
Resource.

Routing is by `(Scope, Key)`:

```go
case "global":
    if msg.Key == "orgs"      → m.orgsRes.Apply(msg)
    if msg.Key == "projects"  → m.projectsRes.Apply(msg)

case <username>:
    → walk applyOrgPrefixResourceMsg's route table
      to find the right per-key map by key prefix
```

The per-prefix route table lives in
`internal/ui/update_resource_helpers.go:applyOrgPrefixResourceMsg`.
Each entry is `(prefix, handler)`. Handlers do
`applyAndMaybeRefreshResource(m, d.SomeMap[suffix], msg)` which
both applies the message AND fires the cache-load post-refresh
if needed.

**When you add a new Resource map, you MUST add a prefix entry
here.** Otherwise its UpdatedMsg goes nowhere and the Resource
stays `fetchedAt=zero` forever (we hit this bug with
`object_baseline:` — symptom was "modal shows current state
unknown forever"). The route table is the loop-closer.

### TTL resolution

Each Resource declares a default TTL inline. Users can override
per-key in `settings.toml`:

```toml
[ui.cache]
default_ttl = "30m"

[ui.cache.ttl]
describe = "8h"
sobjects = "24h"
flows = "5m"
```

`Settings.CacheTTL(key, fallback)` resolves:

1. per-key override (if set and parseable)
2. default_ttl (if set)
3. fallback (hardcoded at the Resource declaration)

The cache-settings modal (`=` → "Cache & refresh policy") lets
users edit overrides interactively. Surfaces the catalogue in
`cacheResourceCatalog` (in `modal_cache_settings.go`) — adding a
new Resource means adding a row there too if you want it
user-tunable.

## Layer 3 — Per-render computed memos

**Not cross-fetch caches.** These cache the *computation* of
"what should we render right now" so we don't redo it twice per
frame.

All ephemeral. All in-memory. All keyed on some identity that
naturally changes when inputs change (slice pointer + version,
mostly).

### The five memos

1. **`Filtered()` memo** on `ListView[T]` — caches the filtered
   slice keyed on `(version, search buffer)`. Version bumps on
   `Set` / `SetExtra` / `SetMatch` / `SetScorer`. File:
   `internal/ui/resource/listview.go`.

2. **Gutter cache** — bulk tag/project map fetches per `(items
   slice pointer, devproject.Generation)`. Lives on `orgData`.
   File: `internal/ui/gutter_cache.go`. Eliminates SQLite
   round-trips for tag/project pills on the render hot path.

3. **Per-row paged-render cache** — caches rendered row strings
   per `(group key, rowIndex)`. Active in paginated mode only —
   continuous mode's sliding window means row indices aren't
   stable. Group key folds in 10+ inputs that affect row
   appearance. Cursor row never cached (its highlight changes
   per tick). File: `internal/ui/render_cache.go::pagedRows`.

4. **Records projection cache** — records-specific. Caches `(cols,
   cells [][]string)` so the auto-fit column-width walk runs
   once per data change rather than per frame. File:
   `internal/ui/tab_records.go::recordsProjectionFor`.

5. **Visible records memo** — records-specific. Memoises the
   `(visible, visibleIdx)` pair so multiple per-render callers
   (cursor logic, sidebar, breadcrumb) share one walk. File:
   `internal/ui/tab_records_dashboard.go::visibleRecordsAndIdx`.

Plus smaller per-frame caches in `render_cache.go`: tab bar rows,
left tab rows, status bars, dashboards.

For the full why-and-how of Layer 3, see the
[scrolling and render performance guide](scrolling-architecture.md).

## Layer 4 — REST client cache

**File:** `internal/sf/rest.go`
**Function:** `RESTClient(alias)`

Not a data cache — a client cache. The Salesforce REST path needs
an authenticated HTTP client (instance URL + access token + http
client). Bootstrapping one requires shelling to `sf org display
--verbose -o <alias> --json` which is slow (~200ms).

The `clients` registry (private global in `rest.go`) maps `alias
→ *Client`. First `RESTClient(alias)` call bootstraps; subsequent
calls return the cached pointer.

### Invalidation

`sf.InvalidateRESTClients()` drops every cached client. Called
when:

- Process startup (`cmd/sf-deck/main.go:80`) — defensive.
- Any auth/org-lifecycle event:
  - `orgsChangedMsg` (org-manage modal: login / logout / set-alias /
    set-default / add)
  - `orgLifecycleResultMsg{Refetch:true}` (post-action refetch
    requests)

Both also fire `m.orgsRes.Refresh()` to rebuild the org list.
Stale clients with old tokens / wrong instance URLs would get
401s indefinitely otherwise.

## Invalidation paths summary

Everything that can cause a Resource to refresh, in one table:

| Trigger | Action | What it invalidates |
|---|---|---|
| TTL expiry (passive) | next `Ensure` calls `refreshCmd` | the one Resource |
| `r` keymap | `tabSpec.RefreshData(d)` (or default `Resource.Refresh`) | per-surface Resources |
| `OnSuccess` on an action | hook runs after deploy/save succeeds | the affected Resource(s) |
| `devproject.Store.Generation()` bump | every tag/project write touches it | Layer 3 memos keyed on it (row cache, gutter cache) — Layer 2 unaffected |
| `orgsChangedMsg` | `m.orgsRes.Refresh()` + `InvalidateRESTClients()` | the org list + every REST client |
| `orgLifecycleResultMsg{Refetch:true}` | same as orgsChangedMsg | same |
| Alias change (per-org) | `ensureOrgData` rebuilds `orgData` | every Resource on that org (they captured the old alias) |
| Cache-miss on cold load | `Apply` + `MaybeRefreshAfterCacheLoad` | fires network refresh |
| Startup wipes | `DeleteKeyPrefix("records:" / "listviews:" / "listview:")` | leaked legacy cache rows |
| User runs `rm ~/.sf-deck/cache.db` | manual nuke | Layer 1 entirely; Layer 2/3 unaffected for current session |

## Adding a new Resource

The minimum recipe:

1. **Add the typed field** on `orgData` (or Model for global) in
   `internal/ui/model.go`. Singleton or map; pick the shape.

2. **Wire the Resource** in `newOrgData` (or `New()` for Model
   globals). Set `Scope`, `Key`, `TTL`, `Fetch`. Decide
   `NoCache` — default `true` if there's any doubt.

3. **Add a routing entry** in
   `internal/ui/update_resource_helpers.go:applyOrgPrefixResourceMsg`
   with the key prefix. Without this the Resource will fetch but
   never apply.

4. **Wire `Ensure` calls** in the relevant tab's `EnsureData`
   hook (in `tab_xxx_hooks.go` or `tab_registry.go`). Subtabs
   can declare their own `EnsureData` on `SubtabSpec` for
   subtab-scoped fetches — the dispatcher runs both tab-level
   and subtab-level hooks on entry, so per-subtab Resources
   auto-load alongside the tab-level ones.

5. **(Optional)** Add a row to `cacheResourceCatalog` in
   `modal_cache_settings.go` if you want users to be able to
   tune the TTL.

Test it by running with a cold cache (delete `~/.sf-deck/cache.db`
or use a fresh org), entering the surface, and confirming:

- First fetch shows loading state
- Data renders after fetch
- Re-entering the surface reads from cache (instant)
- TTL expiry triggers a background refresh (data updates on next
  render after fetch completes)

## Where data lives at any given moment

Picture a single sObject describe. Its journey:

```
       ┌────────────────────────────────────┐
       │ Salesforce describe API            │
       └─────────────┬──────────────────────┘
                     │ sf.Describe(alias, "Account")
                     ▼
       ┌────────────────────────────────────┐
       │ Layer 1 — kv:                      │
       │   org_username | "describe:Account"│
       │   value        | JSON blob         │
       │   cached_at    | unix ts           │
       └─────────────┬──────────────────────┘
                     │ loadCmd / refreshCmd
                     │ → UpdatedMsg
                     │ → applyResourceMsg
                     ▼
       ┌────────────────────────────────────┐
       │ Layer 2 — Resource[T]:             │
       │   d.Describes["Account"]           │
       │     .data       (sObjectDescribe)  │
       │     .fetchedAt  (time.Time)        │
       └─────────────┬──────────────────────┘
                     │ d.Describes["Account"].Value()
                     ▼
       ┌────────────────────────────────────┐
       │ Layer 3 — render memos:            │
       │   gutter cache reads describe rows │
       │   row cache snapshots cell strings │
       │   (these regenerate when describe  │
       │    re-fetches and slice ptr flips) │
       └─────────────┬──────────────────────┘
                     │
                     ▼
                 [rendered UI]
```

When the user hits `r` on /objects, the Resource refreshes →
fresh JSON lands → SQLite kv row updates → `Apply` sets new
`data` + `fetchedAt` → Layer 3 memos see a new slice pointer
next render → they rebuild → next frame shows fresh data.

## Performance notes

- **Layer 1 reads:** ~1ms typical. SQLite is fast.
- **Layer 1 writes:** ~5ms typical. Tolerable since they're async
  (goroutine after fetch completes).
- **Layer 2 ops:** essentially free (in-memory pointer
  dereferences + slice comparisons).
- **Layer 3 hits:** O(1) map lookups.
- **Network fetch:** 100ms to several seconds depending on the SF
  endpoint. Always async via Bubble Tea cmds.

The hot render path (Layer 3) avoids Layer 2 reads (via memos)
and Layer 1 entirely. The hot input path (wheel events) avoids
fetching but does touch Layer 2 (Resource.Value() for the cursor
move). The cold path (tab switch) goes all the way through:
Ensure → loadCmd → Apply → render.

## When to come back to this doc

Update when:

- You add a new Resource — add it to the table above
- You change a TTL default — update the table
- You add a new layer (e.g. an in-memory LRU for fetched flows)
- You change the invalidation contract (e.g. add a new event
  that should trigger refreshes)
- The on-disk schema gains a new table
