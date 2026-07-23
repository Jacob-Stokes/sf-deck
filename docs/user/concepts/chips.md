# Chips

Chips are saved filters. Most list-shaped tabs have a chip strip
above the table; flipping a chip changes what's visible in the list
below.

## Why they exist

Salesforce list views work, but they're per-object, web-only, and
slow to author. sf-deck's chips are the same idea — "show me only
the rows that match this rule" — but:

- They live in `~/.sf-deck/settings.toml`, version-controllable.
- They work the same across every list-shaped surface (records,
  objects, flows, apex, perms, …).
- A chip can include columns, clauses, an ORDER BY, a limit, and a
  name.
- You can cycle them with `[` / `]` instead of clicking a dropdown.
- Some chips are session-only (ephemeral) and never written to
  disk.

## How they work

Each chip belongs to a **domain** (`records`, `objects`, `flows`,
…) and, for records-domain chips, a **scope** (the sObject API name;
`*` matches every sObject).

The active chip is highlighted at the top of the strip. Cycling
applies a different filter to the list below — sf-deck doesn't
re-fetch from the org unless the chip's clauses changed enough to
need new data.

## Built-in chips

Every list ships with a few defaults you can't delete:

| Chip | What |
|---|---|
| `__project__` | Items from the loaded dev project |
| `__visited__` | Things you've drilled into recently |
| `__sf_recent__` | The Salesforce "Recently Viewed" list (records only) |

## Creating chips

Two ways:

1. **From the TUI** — press `V` on any chip-bearing surface to open
   the chip manager. Pick a name, columns, clauses, optional ORDER
   BY + LIMIT.
2. **From the CLI** — `sf-deck chip create --domain records --id
   open-accounts --scope Account --columns Id,Name --clauses
   "WHERE IsClosed = false"`. Useful for scripting.

Either way the chip goes into your settings.toml and is available
on subsequent launches.

## Ephemeral chips

When you (or an agent driving sf-deck) want to filter for one
session but not pollute settings, use **ephemeral chips**. They
appear in the strip with a `~` prefix and dotted border and vanish
on quit.

Drop one over IPC: `{"command":"chip.preview","args":{...}}`.

If the user decides to keep it, promote with `chip.preview.save` —
that writes it to settings.toml with whatever id you give it.

## Discovering what's there

```sh
sf-deck chip list --json
sf-deck chip list --domain records --json
```

Or from inside the TUI, `V` opens the chip manager which shows
everything available on the current surface.

## Related

- [Tasks → Find a record](../tasks/find-a-record.md) — chips in
  context.
- [Reference → CLI](../reference/cli.md) — every `chip` subcommand.
