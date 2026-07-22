# Limitations

What sf-deck deliberately doesn't do — so an agent doesn't waste
cycles trying to make it.

## Bundles

- **One project per bundle row.** Bundles attach to exactly one
  DevProject. No sharing across projects. Re-link (`bundle.link`)
  the same on-disk dir to a different project if you need that
  perspective.
- **No deploy-result digest.** `bundle.deploy` and `bundle.validate`
  return the raw `sf project deploy --json` envelope embedded as a
  string in `data.sf_output`. To surface component / test failures,
  parse that. `bundle.report` adds a parsed summary
  (status / done / success / counts) but NOT the failures list —
  that's still in the embedded JSON.
- **No git auto-init.** `bundle.create` writes an sfdx project but
  doesn't `git init`. If you want history, run it yourself after
  the create returns.
- **No bundle-vs-org diff over IPC.** The TUI has a "what would
  deploy?" view (`p` on `/bundles`) using `sf project deploy preview`.
  There's no IPC equivalent — parse the validate output or shell out
  to `sf` directly.
- **Records aren't bundlable.** DevProject items with `kind=record`
  are skipped from `package.xml`. Move record data via the CSV / XLSX
  export path or via `soql.run`.

## Apex tests

- **No async test runner.** `sf apex run test --async` doesn't have a
  poll-able IPC verb. For long test runs, shell out to `sf` directly.
- **`--tests NoTestRun` is sandbox-only.** Salesforce rejects it
  against production. Use `RunLocalTests` or `RunSpecifiedTests` for
  prod validates/deploys.

## File system

- **No FS surface in IPC.** Can't read or write arbitrary files via
  the socket. Bundle paths returned by `bundle.show` give the agent
  a working directory it CAN read from (since the file system is
  there) but the path is meta — the IPC layer itself doesn't expose
  file operations.

## Records

- **No bulk record API in CLI/IPC.** Per-record `record.create` /
  `update` / `delete` only. Bulk operations need the Bulk API; not
  exposed.

## Tooling metadata

- **Metadata.update is a GET-merge-PUT.** Tooling rejects partial
  PATCHes, so the service fetches current state, overlays your patch,
  and writes the full object back. Concurrent writes can lose data —
  serialise your updates.
- **Known types are limited.** The registry's
  `metadata.get/create/update/delete` verbs gate against a closed set
  of metadata types (CustomField, CustomObject, ValidationRule,
  RecordType, ApexTrigger, FlexiPage, FieldSet, WebLink, WorkflowRule,
  PermissionSet, CustomTab, CustomLabel, CustomPermission,
  FlowDefinition, Layout). Pulling other types means using
  `bundle.retrieve` instead.

## Auth

- **No auth refresh over IPC.** `sf org login web` is a browser flow.
  When auth expires, IPC verbs return `auth_required` — the user has
  to re-auth manually.

## TUI features without IPC equivalents

- **Compare feature** (`/compare` org-to-org metadata compare) —
  TUI-only.
- **Inline record edit** — TUI-only.
- **Most chip surfaces** (perms, queues, public groups) — TUI-only
  except for the bare `chip.apply` driver.
- **Code viewer / syntax highlighting** — TUI-only. The IPC layer
  returns raw source bodies.
