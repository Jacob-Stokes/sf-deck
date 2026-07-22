# Safety model

sf-deck gates every Salesforce write behind a per-org "safety level."
Four levels in ascending order:

| Level | Lets you... | Example verbs |
|---|---|---|
| `read_only` | only reads | `soql.run`, `record.get`, `object.describe`, `bundle.retrieve` |
| `records` | record DML | `record.create`, `record.update`, `record.delete` |
| `metadata` | metadata CRUD | `metadata.create`, `metadata.update`, `bundle.validate`, `bundle.deploy` |
| `full` | destructive metadata + anonymous Apex | `metadata.delete`, `apex.run`, `apex.execute` |

There are exactly **four** levels — anonymous Apex is gated by `full`,
not a separate tier.

The level is **per-org**: each org has either an explicit override
or falls back to a default (typically `read_only`). Production orgs
should NEVER be left above `read_only` for any longer than the
duration of one operation.

## Decision tree before any write

1. **What level does this verb require?** Check the `safety` field on
   the verb's registry entry. Verbs without a `safety` field are
   read-only and bypass the gate.

2. **What's the org's current effective level?**

   ```bash
   sf-deck org safety get --org <alias> --json
   ```

   The `effective` field is what gets checked. `override` shows
   whether the user has set it explicitly.

3. **Is the effective level already at-or-above what the verb needs?**
   If yes → just fire. If no → you need to raise it.

4. **Is the org a production org?** Check the user's memory for
   their list of production orgs + their list of pre-authorised
   sandboxes. ALWAYS ask before raising production safety;
   pre-authorised sandboxes can be raised without confirmation.

5. **Raise** to the required level, do the work, **drop back**:

   ```bash
   sf-deck org safety set --org <alias> --level metadata --json
   # ... do the work ...
   sf-deck org safety set --org <alias> --clear --json
   ```

## What happens when you skip a level

The verb returns:

```json
{
  "ok": false,
  "error": {
    "code": "safety_blocked",
    "message": "...",
    "details": {
      "required_write_kind": "metadata",
      "effective_safety": "read_only",
      "target": "&lt;org-alias&gt;"
    }
  }
}
```

You haven't burned an API call against the org — the gate fires
before sf-deck shells out. Raise safety + retry.

## The override file

Per-org overrides live in `~/.sf-deck/settings.toml`. Don't edit it
by hand — go through `org.safety.set`. The TUI reads the same file
and refreshes per-org safety badges on save.

## Production rules

The skill describes the model; the user's memory describes the
specific policy. Before any write, check memory for:

- Which org aliases the user treats as production
- Which sandbox aliases are pre-authorised for writes
- Any per-org confirmation rules the user has set

Default rule when memory is silent: treat every org as production
and ask before any write.
