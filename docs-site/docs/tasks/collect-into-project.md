# Collect into a project

Build a [dev project](../concepts/dev-projects.md) by stamping
items into it as you find them.

## The gesture

`K` on any list-shaped surface quick-collects the cursored row into
the loaded dev project (press again to remove it). To choose which
project — or when none is loaded — `Ctrl+K` opens the picker: pick
from the list, or create a new one inline.

```
3                (open /objects)
type: shipment   (filter)
Enter            (drill the matched sObject)
$                (jump to FLS subtab)
j                (move down to a field)
Ctrl+K           (collect this field into a project)
```

The picker shows every existing dev project plus a "Create new"
option. After selecting, you're back on the row you were on. The
project's item count went up by one.

## What gets collected

Whatever was selected:

- A row on `/objects` — that sObject
- A row on `/objects/<X>/Schema` — that field
- A row on `/flows` — the flow's current active version
- A row on `/apex` — the class
- A row on `/perms` — the permset / group / profile
- A row on the Records subtab — that record (specific to the org)
- A query on `/soql` — the query as a saved-query item
- An apex snippet — the snippet body

## Collecting containers

Some rows are containers: collecting a report folder adds every
report inside it in one go. There's no separate bulk-collect key —
narrow the list with a filter, then collect the rows you want.

## Inline-create a project

In the picker, the first option is **+ New project**. Pick it,
type a name, Enter. The item lands in the new project; the project
opens.

## What sf-deck stores

A reference, not a copy. The dev project remembers the
`(kind, ref, org_user)` triple. The actual metadata stays in the
org until you materialise a [bundle](../concepts/bundles.md).

This means the same item from different orgs is **two separate
collections**. `Account` from dev and `Account` from prod sit
side-by-side in the project — you see exactly which org each came
from.

## See what you collected

Open `/dev-projects`, drill the project. The items list is
filterable by kind (`[` / `]` to cycle the kind chips).

## Remove an item

On the project detail's items list, `d` on a row removes it.
Confirmation modal pops up. The project's count goes down by one.

## Driving from an agent

```sh
sf-deck project add-item \
  --project-id <pid> \
  --kind flow \
  --ref Shipment_Status_Change \
  --org-user <username> \
  --name "Shipment Status Change" \
  --json
```

The IPC equivalent (`project.add-item`) takes the same fields.

## Related

- [Concepts → Dev projects](../concepts/dev-projects.md).
- [Tasks → Bundle and deploy](bundle-and-deploy.md) — the next step
  after collecting.
