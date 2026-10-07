---
name: sf-deck
description: Operate sf-deck over its headless CLI or live-instance IPC socket. Use for Salesforce work through sf-deck, including records, SOQL, reports, anonymous Apex, Tooling metadata, sfdx bundles, retrieve/validate/deploy, DevProjects, chips, tags, saved queries, org safety, update checks, and navigation of a running TUI. CLI and IPC share a backend and safety gates but have intentional surface-specific verbs. Always discover the current contract with `sf-deck verbs list --json` instead of relying on prose verb lists.
---

# sf-deck

sf-deck has three interfaces:

- A **TUI** the user runs interactively (records, flows, apex, deploys, etc.)
- A **headless CLI** (`sf-deck <noun> <verb> --json`) for one-shot operations
- A **control socket** at `~/.sf-deck/control-<N>.sock` for driving a running TUI window over IPC

The transports share a Backend, safety gates, and JSON envelope (`{ok, command, data, error}`), but some operations exist on only one surface by design. Always use the registry to choose a supported transport.

## Before accessing a real org

sf-deck requires the current privacy notice and user agreement to be
acknowledged before it enumerates or contacts Salesforce orgs. Check locally:

```bash
sf-deck legal status --json
```

If it is not accepted, show the user the returned privacy/terms URLs and ask
them to review and accept. Never run `sf-deck legal accept --yes` on the user's
behalf without their explicit approval. Demo mode is fully offline and does not
require acceptance.

## First thing to do — discover, don't enumerate

**The single source of truth is the verb registry, not this skill.** Verbs change; prose lists go stale; the registry never does.

For any question shaped like "what verbs do X?" / "is there a verb for X?" / "what arguments does X take?" — call:

```bash
sf-deck verbs list --json                  # everything
sf-deck verbs list --surface ipc --json    # only IPC-reachable verbs
sf-deck verbs list --surface cli --json    # only CLI-reachable verbs
```

Over IPC: `{"command":"verbs.list","args":{"surface":"ipc"}}`.

Each entry tells you:

- `qualified` — the noun.verb name (`record.update`)
- `summary` — one-line description
- `safety` — gate level (`read_only` / `records` / `metadata` / `full`). Absent means read-only. Anonymous Apex is gated by `full`; there is no separate `anonymous` level.
- `cli` — present when there's a CLI binding, with the usage string + flag list
- `ipc` — present when there's an IPC binding, with the command name + args list + `async` flag
- `notes` — when relevant (e.g. "IPC-only; pushes into the TUI editor")

**Filter the registry to find what you need.** Don't trust verb lists in any document — including this one. The few times this skill mentions specific verbs are illustrative; the registry is canonical.

## Pick your transport

| Use the CLI when... | Use IPC when... |
|---|---|
| You're scripting from outside the TUI | A user has a TUI window open and you want to drive it |
| Cold one-shot operations | A multi-step flow benefits from the TUI rendering state |
| No live session exists | You want navigation (`tab.open`, `chip.apply`) — IPC only |
| You want the simplest possible call | You want the TUI editor populated (`soql.seed`) — IPC only |

Some operations exist only on one transport by design — `verbs.list` carries an empty `cli` or empty `ipc` for those. The `notes` field explains why when the asymmetry isn't obvious.

### Finding the live IPC socket

```bash
sf-deck instance list --json
```

Returns one entry per running sf-deck process. Entries with a non-empty `socket` are reachable via IPC. The `label` field disambiguates multiple windows.

If the user has multiple instances open and didn't specify which one, ask before driving any of them. Don't pick "any."

## The 4-level safety model

Every Salesforce-touching write is gated by an effective safety level on the target org. Levels (lowest to highest):

| Level | Allows |
|---|---|
| `read_only` | reads only (SOQL, describe, record.get, …) |
| `records` | record DML (record.create, .update, .delete) |
| `metadata` | metadata CRUD (CustomField, ValidationRule, deploy, validate, …) |
| `full` | destructive metadata (metadata.delete) **and anonymous Apex** (apex.run / apex.execute) — the highest tier because it can do anything |

Every verb that mutates Salesforce declares its required level in the registry's `safety` field. A write below the org's effective level returns `safety_blocked` with `details.required_write_kind` so you know what to raise.

### Safety workflow

1. **Read current level**: `org.safety.get` — returns the effective level + whether it's an override or default.
2. **Check authorization**: a writable safety level is a guardrail, not permission. Perform a write only when the user requested it. Always ask before raising safety on a production org unless the current request explicitly authorizes that exact production change.
3. **Raise**: `org.safety.set` with one of the four levels.
4. **Do the work**: the gated verb now goes through.
5. **Drop back**: `org.safety.set` with `clear: true` to revert to the default.

User-specific rules may live in the current request, project instructions, or saved agent context. Check them before any write. When no policy is available, treat the org as production and ask.

## Async + report pattern

Two verbs in the bundle pipeline take time: `bundle.validate` and `bundle.deploy`. Salesforce queues them, runs Apex tests, sometimes 10–20 minutes total.

Both verbs accept `async: true` (IPC) or `--async` (CLI). When async:

1. The call returns immediately with `data.deploy_id` (a `0Af…` DeployRequest id).
2. You poll `bundle.report` with that deploy id every 30–60 seconds.
3. The response has `data.status.done`, `data.status.success`, component/test error counts.
4. Loop until `done=true`.

**Always use async over IPC** — the socket should not hold a long deploy request open. CLI sync mode is available when the caller can wait.

For sandbox validates blocked by unrelated broken Apex tests, pass `tests: "NoTestRun"` (sandbox-only) or `tests: "RunSpecifiedTests"` + `test_classes: [...]` to scope the test run.

## Common workflows

See `references/workflows.md` for task-shaped recipes:

- Build → validate → deploy a bundle across orgs
- Author a SOQL query, save to library, recall in another session
- Collect items into a DevProject, materialise as bundle, deploy
- Ingest an existing sfdx project as a DevProject
- List, run, and export reports
- Check for a newer sf-deck release without installing it

The recipes reference verbs by name; for exact flags/args use the registry.

## On-disk state

sf-deck persists under `~/.sf-deck/`. The agent must NEVER edit these directly — every command returns affected paths in the response envelope.

| Path | Purpose |
|---|---|
| `~/.sf-deck/settings.toml` | chips, favourites, org safety overrides, theme |
| `~/.sf-deck/cache.db` | SQLite read-cache: org catalogue + metadata/schema data |
| `~/.sf-deck/devprojects.db` | SQLite: DevProjects, items, tags, saved queries, snippets, history |
| `~/.sf-deck/keybindings.toml` | optional user keymap overrides |
| `~/.sf-deck/instances.json` | running-instance registry (read by `instance list`) |
| `~/.sf-deck/control-<N>.sock` | per-instance IPC socket (when `--control` is on) |
| `~/sf-deck-bundles/` | default bundle output directory (sfdx project per bundle) |

Salesforce record lists/details, SOQL/report result rows, list-view results,
RecentlyViewed rows, and related-record lookups are process-memory only and are
not written to the persistent response cache. Saved query text/history is local
working state; returned rows are not persisted.

Use `sf-deck data inspect --json` to report known locations. Local deletion is
destructive: only run `sf-deck data erase --yes` after explicit user approval
and after all sf-deck instances are closed. It does not remove custom export
paths or Salesforce CLI credentials. Use `org.logout` when the user explicitly
asks to disconnect an org.

## What sf-deck can't do

See `references/limitations.md` for the full list. Headline items:

- A bundle row attaches to exactly one DevProject (no sharing).
- No bundle-vs-org diff verb over IPC (the TUI has it).
- No git auto-init on bundle directories.
- Records aren't bundlable (use record export for data movement).
- No async Apex test runner; long test suites need the CLI's sync path.
- No file-system surface in IPC — use bundle paths returned by `bundle.show`.

## Output discipline

- Always pass `--json` on the CLI. The text mode is for humans.
- On error, branch on `error.code`: `invalid_argument`, `safety_blocked`, `not_found`, `auth_required`, `partial_success`, `instance_busy`, `confirmation_required`, `method_not_implemented`, `internal_error`.
- Don't dump raw record payloads or Apex source back to the user unless they asked for the literal values.
- Do not infer authorization from the org's current safety level.
- For destructive operations (`bundle.delete`, `metadata.delete`, `record.delete`, `project.delete --force`), confirm with the user before firing.

## When to drop to the CLI from an IPC-driving agent

A few cases genuinely warrant a CLI subprocess even mid-IPC session:

- **Long-running Apex test runs** — IPC has no async-poll for `sf apex run test`.
- **Re-authentication** — `sf org login web` is a browser flow, not a sf-deck call.
- **Bulk metadata pulls** — easier as one SOQL than many `metadata.get` calls.
- **Filesystem operations** — IPC has no FS surface.

The registry's `cli` and `ipc` fields tell you when only one transport supports a verb.
