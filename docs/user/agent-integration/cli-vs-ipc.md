# CLI vs IPC

sf-deck exposes core operations over two transports, with intentional
surface-specific verbs. Pick the right one for the task and query the
registry for exact support.

## Use the CLI when

- **No sf-deck window is open.** Cold operations that just need to
  do the thing.
- **Single read or write.** Nothing benefits from session state.
- **Scripts.** Predictable input, predictable output, no socket
  setup.
- **CI / automation.** Easy to invoke, easy to capture.

```sh
sf-deck soql run --org dev --query "SELECT Id FROM Account LIMIT 5" --json
sf-deck bundle deploy --id <bid> --org uat --async --json
```

## Use IPC when

- **A user is sitting in front of a running sf-deck window** and
  you want to drive it.
- **You want the TUI to render state** between operations (so the
  user can see what's happening).
- **Multi-step flows** where each step depends on the live TUI's
  context (loaded project, active org, drilldown).
- **Operations that only make sense in a live TUI**: nav
  (`tab.open`), chip application (`chip.apply`), seeding the SOQL
  editor (`soql.seed`).

```sh
SOCK=~/.sf-deck/control-1.sock
echo '{"command":"tab.open","args":{"tab":"records","sobject":"Account"}}' | nc -U $SOCK
echo '{"command":"chip.apply","args":{"domain":"records","scope":"Account","id":"__sf_recent__"}}' | nc -U $SOCK
```

## Verbs only on one transport

Some verbs are intentionally one-transport.

### IPC-only

- `state.get` / `state.subscribe` — TUI state stream
- `tab.open` — driving nav
- `chip.preview` / `chip.preview.save` / `chip.preview.dismiss` —
  session-only chips
- `soql.seed` — pushing into the live TUI's editor
- `verbs.list --surface ipc` — discover via the socket

### CLI-only

- `instance.list` / `instance.kill` — process registry; the IPC
  instance IS the process
- `org.list` — bootstrapping; you call this before any sf-deck
  window exists
- `apex.snippet` — local snippet library management
- `soql.export` — large file output
- `report.export` — same

Why the asymmetry: some operations only make sense in one context.
`tab.open` is meaningless without a TUI. `instance.list` is
meaningless inside one.

## Discovering which transport a verb supports

```sh
sf-deck verbs list --json | jq '.data.verbs[] | select(.qualified == "soql.run") | {cli: (.cli != null), ipc: (.ipc != null)}'
```

Or the registry-driven [CLI](../reference/cli.md) and
[IPC](../reference/ipc.md) reference pages.

## Driving multiple instances

`sf-deck instance list --json` returns every running window with
its socket path. You can drive them independently.

Tip: launch each with `--label` and the badge shows which is which:

```sh
sf-deck --control --label "dev"   &
sf-deck --control --label "prod"  &
```

When an agent asks "which window?" and the user hasn't said, the
label disambiguates.

## When to drop to the CLI mid-IPC session

A few cases where you're in an IPC-driving agent but still want a
CLI subprocess:

- **Long-running Apex tests** — `sf apex run test --async` has no
  IPC equivalent yet. Use `sf` directly.
- **Re-authentication** — `sf org login web` is a browser flow.
- **Bulk metadata** — easier as one SOQL than many
  `metadata.get` calls.
- **File system** — IPC has no FS surface; the TUI does open
  files via the OS, but for arbitrary reads/writes use shell.

The registry tells you when only one transport supports a verb;
the `notes` field often explains why.
