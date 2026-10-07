# Tags

Tags are user-defined labels you can apply to any item across
Salesforce orgs — a record, a flow, a permission set, a custom
field, a saved query. They live in `~/.sf-deck/devprojects.db`
and persist across sessions.

## Why they exist

Salesforce doesn't have a portable annotation layer. You can put a
description on most metadata objects, but you can't tag a record,
and the description field is org-specific. Tags in sf-deck are
**yours** — they apply across every org you connect, and they're
indexed for "show me everything tagged X."

Typical uses:

- **Workflow state** — `to-review`, `in-progress`, `done`,
  `blocked`.
- **Ownership** — `mine`, `team-billing`, `external-vendor`.
- **Triage** — `tech-debt`, `flaky`, `needs-tests`, `dead-code`.

## How to use them

| Action | How |
|---|---|
| **Apply a tag** | Press `t` on any list. The tag picker opens; pick or create. |
| **Bulk-tag a list** | Press `T` to apply a tag to every visible row. |
| **See an item's tags** | The item's sidebar shows them. List rows render a coloured dot in the gutter when tagged. |
| **Show everything with a tag** | Open `/tags`, drill into the tag, see every bound item across all orgs. |
| **Remove a tag** | From the tag picker on the item, untick. Or `sf-deck tag remove --id ... --kind ... --ref ...`. |

## Creating tags

From the TUI: open `/tags`, press `n`. Pick a name, a colour, an
optional icon (a glyph that renders inline next to the name).

From the CLI:

```sh
sf-deck tag create --name "needs-tests" --color cyan --icon T --json
```

## What a tag binds to

Each tag binding is `(tag_id, kind, ref, org_user)`. So:

- **Records** are bound per (sobject, record id, org). The same
  record id in two orgs gets two separate bindings.
- **Metadata** (flows, classes, fields, …) is bound per (kind, ref,
  org).
- **Org-agnostic items** (saved SOQL queries, apex snippets) bind
  with `org_user=""` — one binding regardless of which org spawned
  them.

This means a tag like `to-review` can span every kind of thing on
every org you connect — and `/tags` shows the full picture.

## Tag column on lists

Some surfaces render a tag column in the table (toggleable with
`Ctrl+T`). Others show tags only in the sidebar. The bulk
apply/remove modal (`T`) is everywhere.

## Tags vs. dev projects

Both let you group things across orgs. The difference:

- **Tags** are lightweight, multi-binding (one item can have many
  tags), no structure. Good for cross-cutting state ("everything
  I'm reviewing this week").
- **Dev projects** are structured working sets (one project per
  piece of work). Good for "the things this feature touches."

Use both. They don't conflict.

## Related

- [Concepts → Dev projects](dev-projects.md) — the other grouping
  mechanism.
- [Reference → CLI](../reference/cli.md) — every `tag` subcommand.
