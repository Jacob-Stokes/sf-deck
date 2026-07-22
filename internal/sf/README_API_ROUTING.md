# Salesforce API routing cheatsheet

This package talks to Salesforce over several different APIs. Each one has
distinct endpoints, auth, response shapes, and failure modes. Picking the
right one per feature is NOT optional — Salesforce enforces which entities
are available where.

## The four APIs we use

### 1. REST API — `/services/data/vNN/…`

Plain business-data CRUD + describes + SOQL. Every org's data plane.

- **Use for**: Account/Contact/opportunity read+write, SOQL queries on data
  sobjects, sobject describes, list-view execution.
- **Client**: `rest.go` → `c.get / c.patch / c.post / c.delete`.
- **Paging**: `QueryREST` follows `nextRecordsUrl` automatically.
- **Counts**: against `DailyApiRequests`.

### 2. Tooling API — `/services/data/vNN/tooling/…`

Metadata-as-data for **some** metadata entities. Fastest path when the
entity supports it, because edits are per-row REST PATCHes (no zip, no
async).

- **Writable entities** (exposed with `FullName` + `Metadata` virtual
  columns, PATCHable row-by-row):
    - `CustomField`
    - `ValidationRule`
    - `RecordType`
    - `ApexTrigger` (Body is top-level, not in Metadata; see
      `triggers.go`)
    - `ApexClass`
    - `FlexiPage` (Lightning Pages), `Layout`, `HomePageLayout`
    - `WorkflowRule`, `WorkflowFieldUpdate`, …
    - Full list:
      https://developer.salesforce.com/docs/atlas.en-us.api_tooling.meta/api_tooling/tooling_api_objects.htm
- **Read-only entities** (queryable but can't PATCH — still useful for
  browsing / inventory):
    - `CustomObject` ← **the gotcha** — parent of CustomField but read-only
      via Tooling
    - `EntityDefinition`
    - `FieldDefinition`
    - `FlowDefinition`, `Flow`
    - most `Organization`-scoped singletons
- **Client**: same as REST, via `c.ToolingPath(…)`.
- **Quirks**:
    - Tooling SOQL caps responses at 500 rows regardless of `LIMIT`
      — `QueryREST` follows `nextRecordsUrl` to reassemble.
    - `EntityDefinition` doesn't support `queryMore` at all; we page
      manually by `DurableId` cursor (see `sobjects.go`).
    - `ORDER BY` uses case-insensitive collation, `WHERE X > Y` uses
      binary — never cursor on a `string` column for pagination; use
      `Id` / `DurableId`.
- **Counts**: against `DailyApiRequests`.

### 3. Metadata API (REST) — `/services/data/vNN/metadata/…`

The "real" metadata deployment surface. Required for anything
Tooling can't do: CustomObject edits (labels, descriptions, feature
toggles), Profile / PermissionSet changes, new CustomField creation,
Picklist value CRUD, Page Layouts, Lightning Record Pages, etc.

- **Use for**: Any write where the entity is NOT listed as Tooling-
  writable above. Rule of thumb: if `GET /tooling/sobjects/X/describe`
  says every field is `updateable: false`, you need Metadata API.
- **Client**: `metadata.go` → `DeployMetadata(target, files)`.
- **Shape**: upload a ZIP of XML files + `package.xml` manifest, poll
  the deploy job until done. Async.
- **Counts**: deploy submissions count as API calls; the poll loop
  counts each GET.

### 4. sf CLI shell-out — `sf <args...>`

The slow path. ~1s Node startup per invocation. Kept as a fallback
for:

- Initial org auth (`sf org display --verbose --json` bootstraps the
  REST client's token).
- Things we haven't bothered to REST-port yet (look for `runSF` call
  sites).
- User-facing "open this in Setup" (`sf org open -p <path>`).

Prefer REST-direct when possible. The CLI will vanish from hot paths
as we go.

## Decision tree for new write features

```
Can the entity be PATCHed via Tooling API?
├── Yes (CustomField, ValidationRule, RecordType, ApexTrigger, ApexClass,
│        FlexiPage, Layout, Workflow*, etc.)
│     └── Use tooling_metadata.go → UpdateToolingMetadata
│         (or entity-specific wrapper in <entity>.go)
│
└── No (CustomObject edits, new field creation, picklist CRUD, profile/
         permset, page layouts, lightning pages, approval processes,
         sharing rules, custom metadata types, etc.)
      └── Use metadata.go → DeployMetadata with an appropriate
          XML payload.  Helper builders for common cases live in
          objects_meta_deploy.go, fields_deploy.go, etc.
```

## Debugging tips

- **"INVALID_FIELD: No such column 'FullName' on sobject of type X"**
    — You're trying to use `UpdateToolingMetadata` on an entity that
    doesn't expose FullName / Metadata. Check the writable list above;
    if the entity isn't there, use Metadata API instead.
- **401 INVALID_SESSION_ID** — token expired. `rest.go`'s `doWithRetry`
  will auto-rebootstrap and retry once. If it keeps happening, the
  cached sf token is stale.
- **Query truncated at 500/2000 rows** — Tooling/REST page cap;
  `QueryREST` handles this via `nextRecordsUrl`. If it's still
  truncating, the outer caller might have an early-exit bug.
- **Deploy sits in "Pending" forever** — poll interval is 1s; typical
  deploys land in 2-5s. If >30s, check `deployRequest/<id>?includeDetails=true`
  for real status; deploy logs live there.
