# Dev projects

A dev project is a working set of Salesforce things — fields, flows,
classes, records, saved queries — pulled together because they
belong to one piece of work.

## Why they exist

Salesforce changes don't live in one place. A typical "ship the new
shipment status flow" project touches:

- A custom field or two
- A validation rule
- A flow definition
- A test apex class
- A permission set adjustment
- Maybe a record type

These live in different tabs in Setup. In Lightning, you keep them
together by living in browser tabs and a notes file. In sf-deck, you
collect them into a **dev project** and they stay together — across
sessions, across orgs.

## How they're organised

A dev project has:

- A **name** + **description** (the bit you'd write in your ticket).
- A list of **items** — references to specific Salesforce things.
- Optional **bundles** — sfdx project directories linked to the
  project. More on that in [Bundles](bundles.md).

Items are **per-origin-org**. The same `Account` sObject can sit in
the same project under multiple orgs. This is the point: when you're
collecting context for a piece of work that touches dev + UAT + prod,
the project records what each org's version looks like.

## How to use them

1. **Create** — `sf-deck project create --name "Shipment revamp"` or
   `n` on `/dev-projects`.
2. **Collect** — press `Ctrl+K` on any list (a flow, a field, a
   record) to add the cursored row to a project. Pick from a list of
   existing projects or create a new one inline.
3. **Drill** — open `/dev-projects` and Enter on a project. You'll
   see every item, filterable by kind (sObject, field, flow, apex,
   LWC, permset, …).
4. **Filter by kind** — `[` `]` on the items list cycles the kind
   chips (`All` / `Objects` / `Fields` / `Flows` / `Apex` / …).
5. **Untag scope** — by default the items list shows only the
   active org's items. `\` toggles to "all orgs" so you can see the
   project's full cross-org reach.

## Cleanup and rename

- `e` on a project renames + re-describes it.
- `d` on a project deletes it (with confirmation when it has items).
- `d` on an item removes it from the project.

## Importing from an existing sfdx project

If you already have a Salesforce repo, sf-deck can ingest its
`package.xml` and create a dev project that mirrors it:

```sh
sf-deck project create --name "Existing repo"
sf-deck project import-bundle \
  --project-id <id> \
  --path /path/to/sfdx-project \
  --org <org-alias>
```

Idempotent — re-running skips duplicates.

## What's stored where

DevProject items live in `~/.sf-deck/devprojects.db`. Nothing leaves
your machine. The items are just references — sf-deck doesn't store
copies of the metadata itself unless you materialise a
[bundle](bundles.md).

## Related

- [Bundles](bundles.md) — how dev projects become deployable sfdx
  projects.
- [Tasks → Collect into project](../tasks/collect-into-project.md).
- [Tasks → Cross-org workflow](../tasks/cross-org-workflow.md).
