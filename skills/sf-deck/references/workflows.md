# Workflows

Task-shaped recipes. Each one names the verbs in order and the
decisions you'll need to make. For exact flags/args, look up the
verb in the registry (`sf-deck verbs list --json`).

## 1. Build → validate → deploy a bundle across orgs

The bundle pipeline materialises a DevProject as an sfdx project on
disk, then ships its metadata to another org.

1. **Confirm the project exists** — `project.show` with `id` or `name`.
2. **Create the bundle, retrieving from the source org**:
   - `bundle.create` with `project_id` + `org_alias: <source>`.
   - Path defaults to `~/sf-deck-bundles/<project>-<unix-ts>/`.
   - Response carries `bundle.id` and `package_xml_path`.
3. **Inspect the manifest** — optional. The dir on disk has
   `package.xml`, `sfdx-project.json`, and the retrieved
   `force-app/main/default/...` tree.
4. **Raise safety on the target org** to `metadata` if it isn't already.
   See `safety.md`.
5. **Validate against the target**:
   - `bundle.validate` with `id: <bundle-id>`, `org_alias: <target>`,
     `async: true`.
   - Response carries `data.deploy_id` (a `0Af…` id).
   - For sandboxes blocked by broken Apex tests, pass
     `tests: "NoTestRun"` or `tests: "RunSpecifiedTests"` + the class
     list.
6. **Poll** — `bundle.report` with `id` + `org_alias` + `deploy_id`
   every 30–60 seconds. Watch `data.status.done` and
   `data.status.number_test_errors`.
7. **On Succeeded, deploy** — same shape as validate but the verb is
   `bundle.deploy`. Confirm with the user before deploying to prod.
8. **Drop safety back** — `org.safety.set` with `clear: true`.

Stale bundles (directory moved or deleted) error out with a clear
hint — re-link via `bundle.link` or delete the row with `bundle.delete`.

## 2. Author a SOQL query, save to library, recall later

The SOQL surface is the most-developed agent workflow.

1. **Seed the editor** so the user can see + edit the query:
   - `soql.seed` with `query: "..."`, `open: true`. The TUI navigates
     to `/soql` if it wasn't already there.
2. **Optionally run it immediately** — same verb, pass `run: true`.
3. **Save to the library** — `soql.saved.create` with `name`, `body`,
   optional `description`. Response carries the new id (`sq_…`).
4. **Recall later (any session)** — `soql.saved.show` by id OR by name.
5. **Modify** — `soql.saved.update` with pointer fields for any of
   `name` / `body` / `description`.
6. **Re-run** — load the body into the editor via `soql.seed` with
   `run: true`, or use `soql.run` directly for a CLI-only result.
7. **History** — `soql.history.list` shows every IPC- and TUI-fired
   run with timestamps, durations, row counts, error messages.

## 3. Collect items into a DevProject, then ship

DevProjects are working sets of metadata across orgs.

1. **Create the project** — `project.create` with `name` and optional
   `description`.
2. **Add items individually** — `project.add-item` with `kind` (`flow`,
   `field`, `apex_class`, etc.), `ref`, `org_alias`. Some kinds need
   `type` (e.g. for fields the parent sobject).
3. **Or bulk-import from an existing sfdx project** — `project.import-bundle`
   with `path: /some/sfdx/dir` parses its `package.xml` and adds every
   member. Idempotent — re-running skips duplicates.
4. **Inspect** — `project.items` returns the full item list.
5. **Continue with the bundle pipeline** (workflow 1) once you're
   ready to ship the items somewhere.

## 4. Ingest an existing sfdx project into sf-deck

When the user already has a Salesforce repo (git checkout, another
tool's export, an old bundle) and wants sf-deck to track it.

1. **Create or pick a DevProject** to attach to.
2. **Import items from the manifest** — `project.import-bundle` with
   the existing path. Reports `added` / `skipped` / `unknown` counts.
3. **Register the on-disk dir as a bundle row** — `bundle.link` with
   the same path. Now the bundle pipeline (validate, deploy, report)
   can target it without touching the files.

`bundle.link` deliberately doesn't scaffold anything; use it when the
contents of the directory are sacred.

## 5. Drive a live TUI to a specific view

When the user is sitting in front of an sf-deck window and wants the
agent to navigate them somewhere.

1. **Discover the instance** — `sf-deck instance list --json`. Pick by
   `label` if the user specified, otherwise ask if multiple.
2. **Connect to its socket** — `~/.sf-deck/control-<N>.sock`.
3. **Navigate** — `tab.open` with the tab name (`records`, `soql`,
   `flows`, `apex`, …).
4. **Apply a chip** — `chip.apply` with the chip's domain + id.
5. **Drop an ephemeral chip** if you want to show one without
   persisting it — `chip.preview`. Promote with `chip.preview.save`
   when the user wants to keep it.
6. **Switch orgs mid-session** — `org.switch` with `alias` or `org_user`.

State updates flow through `state.subscribe` if you want the live
feed.

## 6. Mutate Tooling metadata (CustomField / ValidationRule / etc.)

For one-off metadata changes that don't warrant a full bundle deploy.

1. **Check current state** — `metadata.get` with `type` + `id` (or
   `full_name`).
2. **Raise safety to `metadata`** on the target org.
3. **Mutate**:
   - Create: `metadata.create` with `type`, `full_name`, `patch`.
   - Update: `metadata.update` with `type`, `id`, `patch`.
   - Delete: `metadata.delete` with `type`, `id` — needs `full` safety,
     not just `metadata`.
4. **Drop safety back**.

Note: `metadata.update` does GET-merge-PUT internally because the
Tooling API rejects partial PATCHes. Your patch is overlaid on the
current state, not a full replacement.

## 7. Run anonymous Apex from an agent

Anonymous Apex is the "I can do anything" operation — it can DML
records, deploy metadata, call out. It's gated by the **`full`** safety
level (there is no separate `anonymous` level).

1. **Confirm with the user** before running anything non-trivial.
   Especially against production.
2. **Raise safety** to `full`.
3. **Fire** — `apex.run` (IPC) or `apex.execute` (CLI) with `body` or
   `body_file` or `snippet_id`.
4. **Check the response** — `compiled`, `success`, `compile_problem`,
   `exception_message`, `line`, `column`, `took_ms`.
5. **Drop safety back**.
