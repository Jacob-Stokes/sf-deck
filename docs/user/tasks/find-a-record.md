# Find a record

Three ways, ordered by how much you already know about what you're
looking for.

## You know roughly what it's called

Press `Ctrl+F` from anywhere. The global search modal opens. Type a
substring of the name.

```
Ctrl+F
type: carrier
```

Results appear instantly across both metadata (flows, classes,
objects) and records (Account names, Case subjects, …). `Enter` on
a hit drills you straight in.

`Tab` flips between metadata and records mode. Records mode runs a
SOSL against the active org; metadata mode searches the local cache.

## You know what sObject it's on

Jump to that sObject's records list:

```
3            (open /objects)
type: shipment
Enter        (drill the matched sObject)
Shift+3      (jump to the Records subtab)
```

You're now on a list of all visible records for that sObject. From
here:

- **Chip strip** — try `Recently viewed`, `Mine`, or a saved chip.
  `[` / `]` to cycle.
- **`/`** — filter the visible list by typing.
- **`Enter`** on a row drills the record.

## You only have the id

```sh
sf-deck record get --org <alias> --id 001xx0000000Axyz --json
```

Or in the TUI: from anywhere, `Ctrl+F`, paste the id. sf-deck
recognises 15/18-char Salesforce ids and drills directly.

## Once you're on the record

The right sidebar shows every field. The main pane shows it broken
into sections. Useful keys:

| Key | What |
|---|---|
| `e` | Edit a field (if your safety level allows) |
| `t` | Tag this record |
| `Ctrl+K` | Collect into a dev project |
| `o` | Open in Lightning (browser) |
| `y` | Yank the record id to clipboard |
| `Ctrl+O` | Open menu (more targets) |
| `Esc` | Back to the records list |

## Related

- [Concepts → Chips](../concepts/chips.md) — saved filters.
- [Tasks → Run SOQL](run-soql.md) — when you want more than name
  search.
