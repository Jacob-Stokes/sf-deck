# Bundles

A bundle is a [dev project](dev-projects.md) materialised as an sfdx
project directory on disk — `sfdx-project.json`, `package.xml`, a
`force-app/` tree of metadata files.

## Why they exist

A dev project is a list of references. To actually ship those
references — validate them against an org, deploy them, commit them
to git — you need them as files. That's what a bundle is.

The same project can have multiple bundles: one for the source org's
current state, another for a feature branch, another for the version
you shipped last week.

## How to make one

From `/dev-projects`, drill a project. On its detail page, press `x`
(or `Ctrl+X`). Pick a format:

| Format | What you get |
|---|---|
| **Bundle: manifest only** | Just a `package.xml`. No source. |
| **Bundle: sfdx skeleton** | `sfdx-project.json` + `package.xml` + empty `force-app/`. Ready for you to `cd` in and run `sf project retrieve`. |
| **Bundle: sfdx skeleton + retrieve from org** | Same as above, but sf-deck also runs `sf project retrieve start` for you. The common path. |
| **CSV / XLSX / JSON** | Flat exports of the project's item list. Not a bundle; just a reference document. |

The first three create a bundle row tracked in
`~/.sf-deck/devprojects.db`. The last three don't — they're one-shot
exports.

## What the bundle pipeline does

Once a bundle exists, you can:

- **Retrieve** — pull source from an org into the bundle dir.
- **Validate** — server-side check-only deploy. Catches Apex test
  failures, missing dependencies, validation rule violations.
- **Deploy** — actually push to the org.
- **Report** — poll an async validate or deploy by its DeployRequest
  id.

Validate and deploy both default to `--async` — a non-trivial deploy
takes 5–20 minutes (queue time + Apex test runs), and you don't want
to hold the terminal open. The async path returns a `deploy_id`
immediately; poll it via `bundle report` until done.

## Why `--async` matters

Salesforce queues deploys server-side. A first attempt can take 7
minutes purely on queue waiting. Sync calls will time out before
that finishes. The async + poll pattern is what makes the pipeline
reliable.

## Linking an existing directory

If you've already got an sfdx project (a git checkout, a colleague's
export), you can register it as a bundle without scaffolding
anything:

```sh
sf-deck bundle link \
  --project-id <project-id> \
  --path /path/to/existing/sfdx-project \
  --org <default-target-org>
```

The bundle row points at the directory; sf-deck doesn't touch the
contents.

## The detail view

Drilling a bundle opens a split view:

- **Components** — the manifest's contents. Each row is one
  retrievable component. Sort by kind, member, action.
- **Bundle files** — the on-disk directory, browseable like a file
  tree. cd into `force-app/`, into `flows/`, see every file.

Switch between the two with `[` / `]`. On a Components row, press
`Enter` to drill into the matching detail tab (Flow → `/flows`
detail, ApexClass → `/apex` detail, …). On a Files row, press `o` to
open the file in your default app.

## Limitations

- **Records aren't bundlable.** A bundle is metadata; records are
  data. To move records between orgs, use the CSV/XLSX export.
- **A bundle attaches to one project.** No sharing across projects.
- **No deploy-result digest yet.** `bundle deploy` returns the raw
  `sf` JSON envelope; agents parse what they need from it.
- **No bundle-vs-org diff over IPC.** The TUI has a "what would
  deploy?" preview, but it isn't exposed as an IPC verb yet.

See the Limitations section above for the headline gaps.

## Related

- [Dev projects](dev-projects.md) — the precursor concept.
- [Tasks → Bundle and deploy](../tasks/bundle-and-deploy.md) — the
  recipe.
- [Tasks → Cross-org workflow](../tasks/cross-org-workflow.md).
