# Cross-org workflow

The reason sf-deck exists. Three real patterns that are awkward in
Lightning and quick in sf-deck.

## Pattern 1: dev → UAT → prod via bundles

The canonical change-management flow.

1. **Work in dev.** Build the feature, collect everything into a
   [dev project](../concepts/dev-projects.md) as you go.
2. **Bundle it.** `x` on the project, pick "sfdx skeleton +
   retrieve from org". sf-deck pulls the source from dev.
3. **Validate against UAT.** `sf-deck bundle validate --id <bundle-id>
   --org uat --async --json`. Poll until done.
4. **If validate passed, deploy to UAT.** `sf-deck bundle deploy --id
   <bundle-id> --org uat --async --json`. Poll.
5. **Test the feature in UAT.** sf-deck doesn't help here — go test
   the actual feature in the actual UAT browser session.
6. **Repeat 3-4 against prod.** With explicit human confirmation
   for the safety raise + the deploy itself.

See [Bundle and deploy](bundle-and-deploy.md) for the verb-level
walk-through.

## Pattern 2: spot-check the same thing across orgs

You want to know "is the `Account.Phone` field the same in dev,
UAT, and prod?" — or "is the `Shipment_Status_Change` flow active
everywhere?"

The fast way is the dedicated `/compare` tab: pick a source org and a
target org, choose the metadata types to diff, and sf-deck lays the two
side by side so you can scan for drift. For a single field or object,
open `/objects` on the source org, drill in, then run the comparison
from `/compare` against the other org.

## Pattern 3: pull data from one org, push to another

Records are not bundlable. To move data, use the export/import
flow:

```sh
# From the source org
sf-deck soql run \
  --org dev \
  --query "SELECT Id, Name, Status__c FROM Shipment__c WHERE Status__c = 'In Transit'" \
  --json > shipments.json

# Or use export for a nicer format
sf-deck soql export \
  --org dev \
  --query "SELECT Id, Name, Status__c FROM Shipment__c WHERE Status__c = 'In Transit'" \
  --output shipments.csv

# Massage the data...

# Push to the target org
# (Bulk import isn't an sf-deck verb yet — use the sf CLI directly:)
sf data import bulk \
  --target-org uat \
  --file shipments.csv \
  --sobject Shipment__c
```

`record.create` / `record.update` / `record.delete` work
per-record over both CLI and IPC, but for anything beyond ~50 rows
you want Bulk API, which is currently a `sf` CLI call.

## Multiple sf-deck windows

You can run more than one sf-deck instance at a time. Each one
gets:

- A unique instance number (the `◈N` badge in the top-left)
- Its own IPC socket (`~/.sf-deck/control-<N>.sock`) if launched
  with `--control`

Useful for side-by-side comparison: one window pointed at dev, one
at prod. Each has its own state, its own loaded project, its own
chip strip.

```sh
sf-deck --control --label "dev"   &
sf-deck --control --label "prod"  &
```

Labels show up in the badge so you don't mix them up.

## Driving multiple windows from one agent

The IPC controller skill walks through this. An agent can:

1. Call `sf-deck instance list --json` to discover open windows.
2. Pick one (by label, or by asking the user).
3. Send commands to its socket: navigate, apply a chip, fire a SOQL,
   etc.

See [Agent integration](../agent-integration/index.md).

## Related

- [Concepts → Dev projects](../concepts/dev-projects.md).
- [Concepts → Bundles](../concepts/bundles.md).
- [Tasks → Collect into project](collect-into-project.md).
- [Tasks → Bundle and deploy](bundle-and-deploy.md).
- [Agent integration](../agent-integration/index.md).
