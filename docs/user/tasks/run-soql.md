# Run SOQL

## The editor

`2` opens `/soql`. The editor is a multi-line textarea with
autocomplete against the active org's schema.

```
2            (open /soql)
i            (enter input mode)
type: SELECT Id, Name FROM Account LIMIT 5
Enter        (run)
```

Autocomplete fires as you type:

- Field names — after `SELECT` and in `WHERE` / `ORDER BY`
- sObject names — after `FROM`
- Picklist values — `Ctrl+Space` on a `WHERE foo = ` cursor position
  fetches distinct values from the org
- Relationship names — typing `Account.` offers parent + child
  references

`Esc` leaves input mode without running. `Ctrl+C` aborts a running
query.

## Tooling vs. standard API

Some objects (`Flow`, `ApexClass`, `ValidationRule`, `FieldDefinition`,
…) live on the Tooling API, not the standard one. Press `T` on the
editor to toggle Tooling mode — sf-deck shows the active mode in
the editor border.

```sh
sf-deck soql run --org <alias> --tooling --query "SELECT Id FROM Flow LIMIT 5" --json
```

## Saving for later

After running a query, `s` opens a save dialog. Pick a name + an
optional description. Saved queries live in your saved-query
library, accessible from any sf-deck session.

```sh
sf-deck soql saved create --name "Open opps" --query "SELECT ..." --json
sf-deck soql saved list --json
```

## Recall

From `/soql`, `Shift+2` jumps to the Saved subtab. Cursor onto a
query, `Enter` to load it into the editor and run.

## Browsing history

`Shift+3` (or whatever subtab History sits on) shows every query
you've run recently — both from the TUI and from `sf-deck soql run`.
Each row has timestamp, duration, row count, and the error message
if it failed.

```sh
sf-deck verbs list --json | jq '.data.verbs[] | select(.qualified | startswith("soql."))'
```

## Driving the editor from an agent

The IPC verb `soql.seed` pushes a query straight into the live
TUI's editor. With `"run": true` it also fires the run, same as if
the user had pressed Enter.

```sh
echo '{"command":"soql.seed","args":{"query":"SELECT Id, Name FROM Account LIMIT 5","run":true}}' | nc -U ~/.sf-deck/control-1.sock
```

Useful for "I drafted this query; show it to the user so they can
edit + run" workflows.

## Export

Two ways to get results out:

- **From the editor** — `e` opens a format picker (CSV, XLSX, JSON,
  Bulk-API CSV for big result sets).
- **From the CLI** — `sf-deck soql export --org <a> --query <q> --output result.csv`.

## Related

- [Reference → CLI](../reference/cli.md) — every `soql.*` subcommand.
- [Reference → IPC](../reference/ipc.md) — `soql.seed`,
  `soql.run`, `soql.history.list`, `soql.saved.*`.
