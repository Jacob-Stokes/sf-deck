# CLI reference

Every sf-deck CLI noun and verb. Auto-generated from
`internal/verbs/registry.go`. Re-run

    go run ./cmd/sf-deck-docs

after editing the registry.

Pass `--json` on every command. The JSON envelope is the
stable contract; text mode is for humans.

## `apex`

### `apex.execute`

Run anonymous Apex.

- Safety gate: `full`
- Usage: `sf-deck apex execute --org <a> --body "<apex>" --json`

### `apex.snippet`

Manage saved Apex snippets (list/show/create/update/delete/run).

- Usage: `sf-deck apex snippet list --json`

## `bundle`

### `bundle.create`

Scaffold a new sfdx project + (optionally) retrieve metadata from an org.

- Usage: `sf-deck bundle create --project-id <id> --org <alias> [--path <dir>] [--retrieve=false] --json`
- IPC equivalent: `bundle.create`

### `bundle.delete`

Unlink a bundle row (does not touch the on-disk directory).

- Usage: `sf-deck bundle delete --id <bundle-id> --json`
- IPC equivalent: `bundle.delete`

### `bundle.deploy`

Real deploy. Same async/tests flags as validate.

- Safety gate: `metadata`
- Usage: `sf-deck bundle deploy --id <bundle-id> --org <alias> [--async] [--tests <level>] --json`
- IPC equivalent: `bundle.deploy`

### `bundle.link`

Register an existing sfdx project directory as a bundle without overwriting it.

- Usage: `sf-deck bundle link --project-id <id> --path <dir> [--org <a>] --json`
- IPC equivalent: `bundle.link`

### `bundle.list`

List bundles (optionally for one DevProject).

- Usage: `sf-deck bundle list [--project-id <id>] --json`
- IPC equivalent: `bundle.list`

### `bundle.report`

Poll an async validate/deploy job by DeployRequest.Id.

- Usage: `sf-deck bundle report --id <bundle-id> --org <alias> --deploy-id <0Af...> --json`
- IPC equivalent: `bundle.report`

### `bundle.retrieve`

Pull source from the org into the bundle's working directory.

- Usage: `sf-deck bundle retrieve --id <bundle-id> --org <alias> --json`
- IPC equivalent: `bundle.retrieve`

### `bundle.show`

Show one bundle.

- Usage: `sf-deck bundle show --id <bundle-id> --json`
- IPC equivalent: `bundle.show`

### `bundle.validate`

Check-only deploy (validation rules + Apex tests).

- Safety gate: `metadata`
- Usage: `sf-deck bundle validate --id <bundle-id> --org <alias> [--async] [--tests <level>] --json`
- IPC equivalent: `bundle.validate`

## `chip`

### `chip.columns`

Update a chip's column ordering.

- Usage: `sf-deck chip columns --id <id> --columns A,B,C --json`

### `chip.create`

Create a new chip (filter view) in settings.toml.

- Usage: `sf-deck chip create --id <id> --domain <d> --columns <cols> --clauses <c> --json`

### `chip.delete`

Remove a chip from settings.toml.

- Usage: `sf-deck chip delete --id <id> --json`

### `chip.favourite`

Toggle a chip's favourite flag.

- Usage: `sf-deck chip favourite --id <id> --value true --json`

### `chip.list`

List all defined chips.

- Usage: `sf-deck chip list --json`

### `chip.show`

Show one chip by id.

- Usage: `sf-deck chip show --id <chip-id> --json`

### `chip.update`

Patch a chip's columns/clauses/label.

- Usage: `sf-deck chip update --id <id> [--columns <c>] [--clauses <c>] --json`

## `instance`

### `instance.kill`

Send SIGTERM to a running sf-deck instance.

- Usage: `sf-deck instance kill --number <n> --json`

### `instance.list`

List running sf-deck instances + their control sockets.

- Usage: `sf-deck instance list --json`

## `metadata`

### `metadata.create`

Create a new Tooling sobject row.

- Safety gate: `metadata`
- Usage: `sf-deck metadata create --org <a> --type ValidationRule --full-name <n> --patch <json> --json`
- IPC equivalent: `metadata.create`

### `metadata.delete`

Delete a Tooling row by id.

- Safety gate: `full`
- Usage: `sf-deck metadata delete --org <a> --type <t> --id <id> --json`
- IPC equivalent: `metadata.delete`

### `metadata.get`

Read a Tooling sobject row's Metadata map.

- Usage: `sf-deck metadata get --org <a> --type CustomField --id <id> --json`
- IPC equivalent: `metadata.get`

### `metadata.update`

Patch the Metadata of an existing Tooling row.

- Safety gate: `metadata`
- Usage: `sf-deck metadata update --org <a> --type <t> --id <id> --patch <json> --json`
- IPC equivalent: `metadata.update`

## `notification`

### `notification.send`

Send a desktop notification via the configured backend.

- Usage: `sf-deck notification send --title <t> --body <b> --json`

## `object`

### `object.describe`

Return the cached SObjectDescribe for an sobject.

- Usage: `sf-deck object describe --org <a> --sobject Account --json`
- IPC equivalent: `object.describe`

## `org`

### `org.list`

List sf CLI-known orgs with their connection status.

- Usage: `sf-deck org list --json`

### `org.safety.get`

Read effective + override safety level for an org.

- Usage: `sf-deck org safety get --org <alias> --json`
- IPC equivalent: `org.safety.get`

### `org.safety.set`

Set or clear per-org safety override.

- Usage: `sf-deck org safety set --org <alias> --level metadata --json`
- IPC equivalent: `org.safety.set`

## `project`

### `project.add-item`

Add a single item (flow/field/class/etc.) to a DevProject.

- Usage: `sf-deck project add-item --project-id <id> --kind flow --ref <name> [--org-user <u>] --json`
- IPC equivalent: `project.add-item`

### `project.create`

Create a new DevProject.

- Usage: `sf-deck project create --name <n> --description <d> --json`
- IPC equivalent: `project.create`

### `project.delete`

Delete a DevProject (with --force to cascade items).

- Usage: `sf-deck project delete --id <id> [--force] --json`
- IPC equivalent: `project.delete`

### `project.import-bundle`

Parse a package.xml + add each member as a DevProject item.

- Usage: `sf-deck project import-bundle --project-id <id> --path <dir> [--org <a>] --json`
- IPC equivalent: `project.import-bundle`

### `project.items`

List items in a DevProject (optionally filtered to one org).

- Usage: `sf-deck project items --id <id> [--org-user <u>] --json`
- IPC equivalent: `project.items`

### `project.list`

List all DevProjects.

- Usage: `sf-deck project list --json`
- IPC equivalent: `project.list`

### `project.remove-item`

Remove an item from a DevProject.

- Usage: `sf-deck project remove-item --project-id <id> --kind flow --ref <name> --json`
- IPC equivalent: `project.remove-item`

### `project.show`

Show one DevProject by id or name.

- Usage: `sf-deck project show --id <id> --json`
- IPC equivalent: `project.show`

### `project.update`

Rename or re-describe a DevProject.

- Usage: `sf-deck project update --id <id> [--name <n>] [--description <d>] --json`
- IPC equivalent: `project.update`

## `record`

### `record.create`

Insert a new record.

- Safety gate: `records`
- Usage: `sf-deck record create --org <a> --object Account --field Name=Acme --json`
- IPC equivalent: `record.create`

### `record.delete`

Delete a record by id.

- Safety gate: `records`
- Usage: `sf-deck record delete --org <a> --id <id> --json`
- IPC equivalent: `record.delete`

### `record.get`

Fetch one record by sobject + id.

- Usage: `sf-deck record get --org <a> --object Account --id <id> --json`
- IPC equivalent: `record.get`

### `record.recent`

Recent records for an sobject (read-only).

- Usage: `sf-deck record recent --org <a> --object Account [--limit 50] --json`
- IPC equivalent: `record.recent`

### `record.update`

Patch a record's fields.

- Safety gate: `records`
- Usage: `sf-deck record update --org <a> --id <id> --field Phone=555-1234 --json`
- IPC equivalent: `record.update`

## `report`

### `report.export`

Export a report to XLSX.

- Usage: `sf-deck report export --org <a> --id <report-id> --output <path.xlsx> [--view formatted|details] [--force] --json`
- Note: Refuses to overwrite an existing output unless --force is supplied.

### `report.list`

List reports (with optional name/folder filters).

- Usage: `sf-deck report list --org <a> [--contains <s>] [--folder <f>] --json`
- IPC equivalent: `report.list`

### `report.run`

Execute a report (synchronous; cached result by default).

- Usage: `sf-deck report run --org <a> --id <report-id> [--force-rerun] --json`
- IPC equivalent: `report.run`

## `soql`

### `soql.export`

Run a query + export the result to CSV/XLSX/JSON.

- Usage: `sf-deck soql export --org <a> --query <q> --output <path> --format csv|xlsx|json [--force] --json`
- Note: Refuses to overwrite an existing output unless --force is supplied.

### `soql.run`

Execute a SOQL query and return records.

- Usage: `sf-deck soql run --org <alias> --query <q> [--tooling] [--limit N] --json`
- IPC equivalent: `soql.run`

### `soql.saved.create`

Persist a new saved query.

- Usage: `sf-deck soql saved create --name <n> --query <q> [--description <d>] --json`
- IPC equivalent: `soql.saved.create`

### `soql.saved.delete`

Remove a saved query.

- Usage: `sf-deck soql saved delete --id <id> --json`
- IPC equivalent: `soql.saved.delete`

### `soql.saved.list`

List saved queries.

- Usage: `sf-deck soql saved list --json`
- IPC equivalent: `soql.saved.list`

### `soql.saved.show`

Show a saved query by id or name.

- Usage: `sf-deck soql saved show --id <id> --json`
- IPC equivalent: `soql.saved.show`

### `soql.saved.update`

Patch a saved query (name/body/description).

- Usage: `sf-deck soql saved update --id <id> [--name <n>] [--query <q>] [--description <d>] --json`
- IPC equivalent: `soql.saved.update`

## `tag`

### `tag.apply`

Bind a tag to one (kind, ref, org_user) item.

- Usage: `sf-deck tag apply --id <tag-id> --kind flow --ref <name> [--org-user <u>] --json`
- IPC equivalent: `tag.apply`

### `tag.create`

Create a new tag (name + optional color/icon).

- Usage: `sf-deck tag create --name <n> [--color <c>] [--icon <i>] --json`
- IPC equivalent: `tag.create`

### `tag.delete`

Remove a tag (cascades unbindings).

- Usage: `sf-deck tag delete --id <id> --json`
- IPC equivalent: `tag.delete`

### `tag.items`

List items currently tagged with the supplied tag.

- Usage: `sf-deck tag items --id <tag-id> --json`

### `tag.list`

List all tags (optionally only ones in use).

- Usage: `sf-deck tag list [--usage-only] --json`
- IPC equivalent: `tag.list`

### `tag.of`

List tags applied to one (kind, ref, org_user) item.

- Usage: `sf-deck tag of --kind flow --ref <name> --json`

### `tag.remove`

Unbind one tag from one item.

- Usage: `sf-deck tag remove --id <tag-id> --kind flow --ref <name> --json`
- IPC equivalent: `tag.remove`

### `tag.set`

Replace the full tag set on one item with the supplied list.

- Usage: `sf-deck tag set --kind flow --ref <name> --ids 1,3,5 --json`
- IPC equivalent: `tag.set`

### `tag.show`

Show a tag by id or name.

- Usage: `sf-deck tag show --id <id> --json`
- IPC equivalent: `tag.show`

### `tag.update`

Patch a tag's name/color/icon.

- Usage: `sf-deck tag update --id <id> [--name <n>] [--color <c>] [--icon <i>] --json`
- IPC equivalent: `tag.update`

## `verbs`

### `verbs.list`

Return the full verb registry — single source of truth.

- Usage: `sf-deck verbs list [--surface cli|ipc|tui] --json`
- IPC equivalent: `verbs.list`
- Note: Agents call this to discover what sf-deck can do without parsing docs.

