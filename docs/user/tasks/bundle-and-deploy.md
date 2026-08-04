# Bundle and deploy

The end-to-end recipe for shipping a [dev project](../concepts/dev-projects.md)
from one org to another.

**Prerequisite**: a dev project with the items you want to ship.
If you haven't built one yet, see [Collect into project](collect-into-project.md)
first.

Scenario: you have a project called "Shipment revamp" populated
with the items you want to ship. The source org is your dev
sandbox; you want to deploy to UAT.

## 1. Make the bundle

On `/dev-projects`, drill the project. Press `x`. Pick **Bundle:
sfdx skeleton + retrieve from org**, supply a target directory (or
accept the default `~/sf-deck-bundles/<project>-<unix-ts>/`).

sf-deck:

1. Writes `sfdx-project.json` + `package.xml` into the directory.
2. Runs `sf project retrieve start` against the source org.
3. Creates a bundle row in `~/.sf-deck/devprojects.db` linking the
   directory to the project.

From the CLI:

```sh
sf-deck bundle create \
  --project-id <pid> \
  --org <source-org> \
  --json
```

## 2. Look at what landed

The bundle row is now visible on `/dev-projects` (drill the project,
Bundles subtab) and on `/bundles` (global view).

Drill it. The Components view shows every component in the
manifest. The Files view (`]` to switch) shows the on-disk
directory. Eyeball both.

If you want to edit something before shipping — say, tweak a flow
XML before deploy — open the on-disk file in your editor of choice
via `o` on a Files row.

## 3. Raise safety on the target

Bundle validate + deploy require `metadata` safety on the target
org.

```sh
sf-deck org safety get --org <target> --json
# If it's read_only:
sf-deck org safety set --org <target> --level metadata --json
```

For production: **always confirm with a human** before raising. For
sandboxes you've pre-authorised, raise without ceremony.

## 4. Validate

Validate runs a check-only deploy: validation rules + Apex tests
without committing changes. **Always use `--async`** for
non-trivial bundles — Salesforce queues + runs tests, which takes
5–20 minutes.

```sh
sf-deck bundle validate \
  --id <bundle-id> \
  --org <target> \
  --async \
  --json
```

The response carries `data.deploy_id` — the DeployRequest id you'll
poll.

For sandbox validates blocked by unrelated broken Apex tests, scope
the test run:

```sh
sf-deck bundle validate \
  --id <bundle-id> \
  --org <target> \
  --async \
  --tests NoTestRun \           # sandbox-only
  --json

sf-deck bundle validate \
  --id <bundle-id> \
  --org <target> \
  --async \
  --tests RunSpecifiedTests \
  --test-classes ShipmentTest,CarrierTest \
  --json
```

## 5. Poll

```sh
sf-deck bundle report \
  --id <bundle-id> \
  --org <target> \
  --deploy-id <0Af...> \
  --json
```

Loop every 30–60 seconds. Watch `data.status.done` and
`data.status.success`. When `done=true`, check:

- `number_component_errors` — manifest contents Salesforce rejected
- `number_test_errors` — Apex tests that failed

If either is non-zero, the raw `sf project deploy --json` envelope
in `data.sf_output` carries the per-failure details.

## 6. Deploy

Same shape as validate, but the verb is `bundle deploy`. Same async
+ poll pattern.

**Always confirm with a human before deploying to production**, even
when validate succeeded. Validate proves it's deployable; it
doesn't prove it's a good idea.

```sh
sf-deck bundle deploy \
  --id <bundle-id> \
  --org <target> \
  --async \
  --json
```

Poll the new deploy_id via `bundle report` until done.

## 7. Drop safety back

```sh
sf-deck org safety set --org <target> --clear --json
```

Reverts the override. Next sf-deck session sees the org at its
default level.

## Driving the whole flow from an agent

Same verbs work over IPC. The
[agent integration](../agent-integration/index.md) section walks
through the JSON envelopes.

## Related

- [Concepts → Bundles](../concepts/bundles.md) — what a bundle is.
- [Concepts → Safety](../concepts/safety.md) — the gate model.
- [Tasks → Cross-org workflow](cross-org-workflow.md) — the broader
  multi-org pattern.
