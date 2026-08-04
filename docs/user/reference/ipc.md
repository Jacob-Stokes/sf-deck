# IPC reference

Every sf-deck IPC command. Auto-generated from
`internal/verbs/registry.go`.

IPC reaches a running sf-deck instance over a Unix socket. Find the
socket via `sf-deck instance list --json`; it's at
`~/.sf-deck/control-<N>.sock`.

Wire format is one JSON object per line. Request:

    {"command": "noun.verb", "args": {...}}

Response (success):

    {"ok": true, "command": "noun.verb", "data": {...}, "changed": true}

Response (failure):

    {"ok": false, "command": "noun.verb", "error": {"code": "...", "message": "..."}}

## `apex.*`

### `apex.run`

Run anonymous Apex (IPC verb name; same as CLI's apex execute).

- Safety gate: `full`

## `bundle.*`

### `bundle.create`

Scaffold a new sfdx project + (optionally) retrieve metadata from an org.

- CLI equivalent: `sf-deck bundle create --project-id <id> --org <alias> [--path <dir>] [--retrieve=false] --json`

### `bundle.delete`

Unlink a bundle row (does not touch the on-disk directory).

- CLI equivalent: `sf-deck bundle delete --id <bundle-id> --json`

### `bundle.deploy`

Real deploy. Same async/tests flags as validate.

- Async: agent polls via the matching report verb
- Safety gate: `metadata`
- CLI equivalent: `sf-deck bundle deploy --id <bundle-id> --org <alias> [--async] [--tests <level>] --json`

### `bundle.link`

Register an existing sfdx project directory as a bundle without overwriting it.

- CLI equivalent: `sf-deck bundle link --project-id <id> --path <dir> [--org <a>] --json`

### `bundle.list`

List bundles (optionally for one DevProject).

- CLI equivalent: `sf-deck bundle list [--project-id <id>] --json`

### `bundle.report`

Poll an async validate/deploy job by DeployRequest.Id.

- CLI equivalent: `sf-deck bundle report --id <bundle-id> --org <alias> --deploy-id <0Af...> --json`

### `bundle.retrieve`

Pull source from the org into the bundle's working directory.

- CLI equivalent: `sf-deck bundle retrieve --id <bundle-id> --org <alias> --json`

### `bundle.show`

Show one bundle.

- CLI equivalent: `sf-deck bundle show --id <bundle-id> --json`

### `bundle.validate`

Check-only deploy (validation rules + Apex tests).

- Async: agent polls via the matching report verb
- Safety gate: `metadata`
- CLI equivalent: `sf-deck bundle validate --id <bundle-id> --org <alias> [--async] [--tests <level>] --json`

## `chip.*`

### `chip.apply`

Apply a chip in the live TUI (set as active view).

- Note: IPC-only — applies to a running TUI's view state.

### `chip.preview`

Drop a session-only chip (ephemeral) onto the strip.


### `chip.preview.dismiss`

Drop a previewed chip without saving.


### `chip.preview.save`

Promote a previewed chip to a persistent settings.toml entry.


## `metadata.*`

### `metadata.create`

Create a new Tooling sobject row.

- Safety gate: `metadata`
- CLI equivalent: `sf-deck metadata create --org <a> --type ValidationRule --full-name <n> --patch <json> --json`

### `metadata.delete`

Delete a Tooling row by id.

- Safety gate: `full`
- CLI equivalent: `sf-deck metadata delete --org <a> --type <t> --id <id> --json`

### `metadata.get`

Read a Tooling sobject row's Metadata map.

- CLI equivalent: `sf-deck metadata get --org <a> --type CustomField --id <id> --json`

### `metadata.update`

Patch the Metadata of an existing Tooling row.

- Safety gate: `metadata`
- CLI equivalent: `sf-deck metadata update --org <a> --type <t> --id <id> --patch <json> --json`

## `object.*`

### `object.describe`

Return the cached SObjectDescribe for an sobject.

- CLI equivalent: `sf-deck object describe --org <a> --sobject Account --json`

## `org.*`

### `org.safety.get`

Read effective + override safety level for an org.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | — |
    | `org_user` | `string` | no | — |

- CLI equivalent: `sf-deck org safety get --org <alias> --json`

### `org.safety.set`

Set or clear per-org safety override.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | — |
    | `org_user` | `string` | no | — |
    | `level` | `string` | no | — |
    | `clear` | `bool` | no | — |

- CLI equivalent: `sf-deck org safety set --org <alias> --level metadata --json`

### `org.switch`

Switch the active org in the running TUI.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_user` | `string` | no | canonical username; either this or alias |
    | `alias` | `string` | no | alias; either this or org_user |


## `project.*`

### `project.add-item`

Add a single item (flow/field/class/etc.) to a DevProject.

- CLI equivalent: `sf-deck project add-item --project-id <id> --kind flow --ref <name> [--org-user <u>] --json`

### `project.create`

Create a new DevProject.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `name` | `string` | yes | — |
    | `description` | `string` | no | — |

- CLI equivalent: `sf-deck project create --name <n> --description <d> --json`

### `project.delete`

Delete a DevProject (with --force to cascade items).

- CLI equivalent: `sf-deck project delete --id <id> [--force] --json`

### `project.import-bundle`

Parse a package.xml + add each member as a DevProject item.

- CLI equivalent: `sf-deck project import-bundle --project-id <id> --path <dir> [--org <a>] --json`

### `project.items`

List items in a DevProject (optionally filtered to one org).

- CLI equivalent: `sf-deck project items --id <id> [--org-user <u>] --json`

### `project.list`

List all DevProjects.

- CLI equivalent: `sf-deck project list --json`

### `project.load`

Make a DevProject the active context in the TUI.


### `project.remove-item`

Remove an item from a DevProject.

- CLI equivalent: `sf-deck project remove-item --project-id <id> --kind flow --ref <name> --json`

### `project.show`

Show one DevProject by id or name.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `id` | `string` | no | — |
    | `name` | `string` | no | — |

- CLI equivalent: `sf-deck project show --id <id> --json`

### `project.unload`

Clear the active DevProject context in the TUI.


### `project.update`

Rename or re-describe a DevProject.

- CLI equivalent: `sf-deck project update --id <id> [--name <n>] [--description <d>] --json`

## `record.*`

### `record.create`

Insert a new record.

- Safety gate: `records`
- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | target org alias |
    | `org_user` | `string` | no | target org username (alternative to org_alias) |
    | `sobject` | `string` | yes | sObject API name |
    | `fields` | `object` | yes | field name -> value map for the new record |

- CLI equivalent: `sf-deck record create --org <a> --object Account --field Name=Acme --json`

### `record.delete`

Delete a record by id.

- Safety gate: `records`
- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | target org alias |
    | `org_user` | `string` | no | target org username (alternative to org_alias) |
    | `sobject` | `string` | yes | sObject API name |
    | `id` | `string` | yes | record id to delete |

- CLI equivalent: `sf-deck record delete --org <a> --id <id> --json`

### `record.get`

Fetch one record by sobject + id.

- CLI equivalent: `sf-deck record get --org <a> --object Account --id <id> --json`

### `record.recent`

Recent records for an sobject (read-only).

- CLI equivalent: `sf-deck record recent --org <a> --object Account [--limit 50] --json`

### `record.update`

Patch a record's fields.

- Safety gate: `records`
- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | target org alias |
    | `org_user` | `string` | no | target org username (alternative to org_alias) |
    | `sobject` | `string` | yes | sObject API name |
    | `id` | `string` | yes | record id to patch |
    | `fields` | `object` | yes | field name -> value map of changes |

- CLI equivalent: `sf-deck record update --org <a> --id <id> --field Phone=555-1234 --json`

## `report.*`

### `report.list`

List reports (with optional name/folder filters).

- CLI equivalent: `sf-deck report list --org <a> [--contains <s>] [--folder <f>] --json`

### `report.run`

Execute a report (synchronous; cached result by default).

- CLI equivalent: `sf-deck report run --org <a> --id <report-id> [--force-rerun] --json`

## `soql.*`

### `soql.history.list`

Recent SOQL runs from soql_history.

- Note: IPC-only — no CLI counterpart yet.

### `soql.run`

Execute a SOQL query and return records.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `org_alias` | `string` | no | target org alias |
    | `org_user` | `string` | no | target org username (alternative to org_alias) |
    | `query` | `string` | no | SOQL string (or use query_file) |
    | `query_file` | `string` | no | path to a file with the SOQL ('-' for stdin) |
    | `tooling` | `bool` | no | run against the Tooling API |
    | `limit` | `int` | no | max rows (0 = default cap) |

- CLI equivalent: `sf-deck soql run --org <alias> --query <q> [--tooling] [--limit N] --json`

### `soql.saved.create`

Persist a new saved query.

- CLI equivalent: `sf-deck soql saved create --name <n> --query <q> [--description <d>] --json`

### `soql.saved.delete`

Remove a saved query.

- CLI equivalent: `sf-deck soql saved delete --id <id> --json`

### `soql.saved.list`

List saved queries.

- CLI equivalent: `sf-deck soql saved list --json`

### `soql.saved.show`

Show a saved query by id or name.

- CLI equivalent: `sf-deck soql saved show --id <id> --json`

### `soql.saved.update`

Patch a saved query (name/body/description).

- CLI equivalent: `sf-deck soql saved update --id <id> [--name <n>] [--query <q>] [--description <d>] --json`

### `soql.seed`

Push a query into the TUI editor (optional auto-run).

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `query` | `string` | yes | — |
    | `open` | `bool` | no | navigate to /soql first (default true) |
    | `run` | `bool` | no | also fire the query immediately |

- Note: IPC-only — pushes into the live TUI's textarea.

## `state.*`

### `state.get`

Read the live TUI state snapshot (tab, org, drilldown ids).


### `state.subscribe`

Subscribe to live TUI state updates over the socket.


## `tab.*`

### `tab.open`

Navigate the live TUI to the named tab.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `tab` | `string` | yes | tab id (home/records/flows/apex/...) |
    | `sobject` | `string` | no | drill into this sobject (records tab only) |
    | `org_user` | `string` | no | switch org before opening tab |


## `tag.*`

### `tag.apply`

Bind a tag to one (kind, ref, org_user) item.

- CLI equivalent: `sf-deck tag apply --id <tag-id> --kind flow --ref <name> [--org-user <u>] --json`

### `tag.create`

Create a new tag (name + optional color/icon).

- CLI equivalent: `sf-deck tag create --name <n> [--color <c>] [--icon <i>] --json`

### `tag.delete`

Remove a tag (cascades unbindings).

- CLI equivalent: `sf-deck tag delete --id <id> --json`

### `tag.list`

List all tags (optionally only ones in use).

- CLI equivalent: `sf-deck tag list [--usage-only] --json`

### `tag.remove`

Unbind one tag from one item.

- CLI equivalent: `sf-deck tag remove --id <tag-id> --kind flow --ref <name> --json`

### `tag.set`

Replace the full tag set on one item with the supplied list.

- CLI equivalent: `sf-deck tag set --kind flow --ref <name> --ids 1,3,5 --json`

### `tag.show`

Show a tag by id or name.

- CLI equivalent: `sf-deck tag show --id <id> --json`

### `tag.update`

Patch a tag's name/color/icon.

- CLI equivalent: `sf-deck tag update --id <id> [--name <n>] [--color <c>] [--icon <i>] --json`

## `verbs.*`

### `verbs.list`

Return the full verb registry — single source of truth.

- Arguments:

    | Name | Type | Required | Description |
    |---|---|---|---|
    | `surface` | `string` | no | filter to cli/ipc/tui (empty = all) |

- CLI equivalent: `sf-deck verbs list [--surface cli|ipc|tui] --json`
- Note: Agents call this to discover what sf-deck can do without parsing docs.

