# Discovering verbs

The verb registry is the single source of truth for what sf-deck
can do. Every CLI noun and every IPC command is declared once in
`internal/verbs/registry.go`. Both transports — and this docs site
— read from it.

## Why this matters for agents

Prose lists go stale. The registry doesn't. Agents that want to
keep working as sf-deck changes should query the registry at
runtime rather than rely on memorised verb lists.

## From the CLI

```sh
sf-deck verbs list --json
sf-deck verbs list --surface cli --json
sf-deck verbs list --surface ipc --json
```

## From IPC

```json
{"command": "verbs.list", "args": {"surface": "ipc"}}
```

## What you get

Each entry:

```json
{
  "noun": "bundle",
  "verb": "deploy",
  "qualified": "bundle.deploy",
  "summary": "Real deploy. Same async/tests flags as validate.",
  "stability": "stable",
  "safety": "metadata",
  "cli": {
    "usage": "sf-deck bundle deploy --id <bundle-id> --org <alias> [--async] [--tests <level>] --json",
    "flags": [...]
  },
  "ipc": {
    "command": "bundle.deploy",
    "args": [...],
    "async": true
  }
}
```

Fields:

- `qualified` — the canonical name (`noun.verb`).
- `summary` — one-line description for help text.
- `stability` — `stable`, `experimental`, or `deprecated`.
- `safety` — required write level (`read_only`, `records`,
  `metadata`, `full`). Absent means the verb is read-only. Anonymous
  Apex reports `full`; there is no separate `anonymous` level.
- `cli` / `ipc` — non-null when the verb is reachable via that
  transport. Each carries the verb-specific shape (CLI usage +
  flags, IPC command + arg schema).
- `notes` — optional longer-form note ("IPC-only — drives the
  live TUI editor").

## Filtering patterns

**Every verb that mutates Salesforce records:**

```sh
sf-deck verbs list --json \
  | jq '.data.verbs[] | select(.safety == "records")'
```

**Every IPC-only verb (no CLI equivalent):**

```sh
sf-deck verbs list --json \
  | jq '.data.verbs[] | select(.ipc != null and .cli == null)'
```

**Every async verb:**

```sh
sf-deck verbs list --json \
  | jq '.data.verbs[] | select(.ipc.async == true)'
```

**Verbs for one noun, on either surface:**

```sh
sf-deck verbs list --json \
  | jq '.data.verbs[] | select(.noun == "bundle")'
```

## Why agents should call this first

Compare two agent answers to "what verbs let me modify a record
over IPC?":

**Without the registry:** the agent remembers "record.update
exists" from a prose doc that may be months old. Misses
`record.create` and `record.delete`. Confidence: low. Result:
incomplete.

**With the registry:** the agent runs
`verbs list --surface ipc --json`, filters to `.noun == "record"`,
checks `.safety`. Returns `record.create`, `record.update`,
`record.delete`, all gated at `records`. Confidence: total.

That's the pattern. Discover, then act.

## What's NOT in the registry

The TUI's hand-rolled internals — chip-strip rendering, list
projections, sidebar layout. Those aren't user-facing verbs;
they're implementation details. The registry is the public
contract.

Anything an agent should ever do, you'll find here.
