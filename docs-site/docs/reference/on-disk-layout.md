# On-disk layout

sf-deck stores everything under `~/.sf-deck/`. Nothing leaves your
machine.

```
~/.sf-deck/
├── settings.toml             chips, theme, per-org safety overrides
├── cache.db                  local read-cache (SQLite)
├── devprojects.db            dev projects, items, bundles, tags, ...
├── keybindings.toml          optional user keymap overrides
├── instances.json            running-instance registry
├── control-<N>.sock          per-instance IPC socket (when --control is on)
└── logs/                     app log (gitignored by default)
```

## settings.toml

Plain TOML. Editable, but go through `sf-deck` commands rather
than hand-editing — sf-deck rewrites the file when settings
change and may overwrite manual edits.

Sections:

- `[ui]` — theme, sidebar defaults, font preferences
- `[ui.api]` — API timeouts (HTTP, CLI, retrieve/deploy, deploy
  polling, bulk polling)
- `[org.<username>]` — per-org overrides, including `safety = "..."`
  and any per-org pinned chips
- `[[chips.records]]` / `[[chips.objects]]` / `[[chips.flows]]` …
  — saved chips by domain
- `[[recent]]` — visited-row log per org

## cache.db

SQLite. Two top-level tables:

- **`orgs`** — one row per known org (alias, username,
  instance_url, sandbox flag, last_used, …). Populated when the TUI
  refreshes the org list via `sf org list`. Same source the CLI's
  org-resolver reads.
- **`kv`** — per-org key/value JSON blobs. Used to memoize
  describes, sObject lists, recently-viewed payloads, FLS grids,
  recent records. Composite primary key `(org_username, key)`.

Blobs are opaque from outside — the keys (`describe_v3:Account`,
`flows_v2`, `records:Shipment__c`, …) match what the loaders
expect. Don't hand-edit.

Safe to delete cache.db at any time; sf-deck rebuilds it on next
launch.

## devprojects.db

SQLite. Holds the modern feature set:

- **`dev_projects`** — your collected working sets
- **`dev_project_items`** — per-project items, keyed by `(project_id,
  org_user, kind, ref)`
- **`bundles`** — sfdx project directories linked to dev projects
- **`tags`** + **`tag_bindings`** — tags applied to items
- **`saved_queries`** — your SOQL library
- **`saved_apex`** — your apex snippet library
- **`soql_history`** — every SOQL run (both TUI and CLI)
- **`apex_history`** — every anonymous Apex execution

Schema migrations are applied automatically on `Store.Open`.

## keybindings.toml

Optional. If present, overrides individual key bindings.

Dump the current defaults as a starting template:

```sh
sf-deck --dump-keymap > ~/.sf-deck/keybindings.toml
```

Then edit the TOML; sf-deck loads it on next launch.

The full keymap reference is at [Reference → Keymap](keymap.md).

## instances.json

The running-instance registry. One entry per live sf-deck process,
written on startup and removed (best-effort) on clean shutdown.

```json
{
  "instances": [
    {
      "number": 1,
      "pid": 12345,
      "started_at": "2026-06-27T13:11:55Z",
      "socket": "/Users/you/.sf-deck/control-1.sock",
      "label": "dev"
    }
  ]
}
```

Read via `sf-deck instance list --json` — that's what an IPC-driving
agent uses to discover sockets.

Stale entries (pid no longer running) are pruned on read.

## Bundle directories

By default sf-deck writes bundles into
`~/sf-deck-bundles/<project>-<unix-ts>/`. Each bundle dir is a
complete sfdx project — you can `cd` into it and run the `sf` CLI
directly if you want.

You can override the default by passing `--path` to `bundle
create`, or by registering an existing directory with `bundle
link`.

## What sf-deck doesn't store

- Salesforce session tokens — those stay in the `sf` CLI's keychain
  / `~/.sfdx/`. sf-deck reuses the `sf` session.
- Org credentials — same.
- Salesforce data beyond what's been cached. Cache entries are
  shaped to be safe to drop at any time.

## Versioning the layout

| File | Schema version |
|---|---|
| `cache.db` | tracked in a `meta` table; auto-migrated |
| `devprojects.db` | same |
| `settings.toml` | unversioned; sf-deck reads only the keys it knows about |

Schema migrations are forward-only. Downgrading the binary may fail
to open a newer DB.
