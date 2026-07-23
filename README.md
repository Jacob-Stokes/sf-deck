<!-- markdownlint-disable MD013 MD033 MD041 -->
<!-- GitHub README: centered hero/badges, details blocks, and wide tables are intentional. -->

<div align="center">

# sf-deck

**A keyboard-first Salesforce workspace for people working across multiple orgs.**

<p>
  <a href="https://github.com/Jacob-Stokes/sf-deck/actions/workflows/ci.yml"><img src="https://github.com/Jacob-Stokes/sf-deck/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Jacob-Stokes/sf-deck/releases"><img src="https://img.shields.io/github/v/release/Jacob-Stokes/sf-deck" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License: Apache-2.0"></a>
</p>

<img src="docs/landing/assets/hero.gif" alt="Launching the fictional sf-deck demo and touring its main Salesforce workspaces" width="920">

<p>
  <a href="#install-and-try-it">Install</a> ·
  <a href="#the-one-minute-tour">One-minute tour</a> ·
  <a href="#what-you-can-do">Capabilities</a> ·
  <a href="#safety-by-default">Safety</a> ·
  <a href="#automation-and-agents">Automation</a> ·
  <a href="https://sfdeck.dev/docs/">Docs</a>
</p>

</div>

sf-deck brings schema, records, permissions, Apex, Flows, reports,
deployments, and org-to-org comparison into one fast terminal interface.
Switch orgs with a keystroke, search by name, and open Lightning only when
you need its canvas.

It reuses the Salesforce CLI session already on your machine. There is no
connected app, managed package, Setup change, or second credential store.
If `sf org list` works, sf-deck is ready to use.

## Install and try it

You need the
[Salesforce CLI](https://developer.salesforce.com/tools/salesforcecli)
with at least one authenticated org. Then install sf-deck on macOS or Linux:

```sh
brew install --cask Jacob-Stokes/tap/sf-deck
sf-deck
```

Want to look around without connecting a real org?

```sh
sf-deck --demo
```

Demo mode boots three fictional orgs with populated records, metadata,
projects, bundles, and activity. It makes no network calls.

<details>
<summary><strong>Other installation options</strong></summary>

Download a macOS or Linux archive from the
[release page](https://github.com/Jacob-Stokes/sf-deck/releases),
or build from source with [Go 1.26.5+](https://go.dev/dl/):

```sh
git clone https://github.com/Jacob-Stokes/sf-deck
cd sf-deck
go build -o sf-deck ./cmd/sf-deck
```

There is no native Windows build yet. WSL2 is supported; install the Linux
`sf` CLI and sf-deck inside WSL so they share the same authentication store.

</details>

## Why sf-deck

- **Work across orgs without losing context.** Every authenticated org is one
  key away, and the header always shows the active org and its safety level.
- **Find things by name.** Search locally cached metadata or live records
  instead of walking through layers of Object Manager and Setup.
- **Keep related work together.** Tags and dev projects collect records and
  metadata from multiple orgs into one working set.
- **Use one tool interactively and in automation.** The TUI, headless CLI,
  and local IPC socket share the same backend and safety checks.

## The one-minute tour

| Key | Action |
| --- | --- |
| `1`–`9` | Open a main workspace |
| `'` | Open the org rail and switch org |
| `/` | Filter the current list; in a code viewer, find in the body |
| `Enter` | Drill into the selected item |
| `Esc` | Go back or close the current modal |
| `Ctrl+F` | Search metadata or records across the active org |
| `[` / `]` | Cycle saved view chips or subtabs, depending on context |
| `o` | Open the matching page in Lightning, Setup, or your editor |
| `?` | Show every key available on the current screen |

The mouse also works for tabs, subtabs, chips, rail buttons, and list
scrolling. The full generated keymap is in the
[reference documentation](https://sfdeck.dev/docs/reference/keymap/).

## What you can do

### Explore an org

- See API and storage limits, licenses, notifications, recent activity, and
  the active org's safety state from the home workspace.
- Browse sObjects, fields, record types, validation rules, triggers, page
  layouts, and record-triggered Flows in one drill path.
- Inspect field-level security by profile or permission set and edit it when
  the active safety level allows.
- Browse Apex, Flows, Lightning Web Components, Aura bundles, packages,
  custom metadata, labels, settings, static resources, named credentials,
  and remote sites.

### Query and work with data

- Write SOQL with schema-aware completion, saved queries, per-org history,
  Tooling API mode, and CSV/XLSX/JSON export.
- Browse and search records, inspect every field, edit values, and export the
  current view.
- Navigate report folders, preview reports, and export formatted or detailed
  CSV/XLSX output.

### Administer and troubleshoot

- Search users, inspect logins and permissions, freeze or unfreeze accounts,
  reset passwords, and review failed login history.
- Browse permission sets, permission set groups, profiles, queues, and public
  groups.
- Inspect deploy history, component failures, Apex test results, debug logs,
  audit activity, jobs, and Flow interviews.

### Build, compare, and ship

- Compare Apex, Flows, and metadata between two orgs to find drift.
- Tag any supported item and collect cross-org work into named dev projects.
- Materialise a dev project as an sfdx bundle, then retrieve, validate, and
  deploy it without leaving sf-deck.
- Open source files, Lightning records, Flow Builder, and Setup pages in the
  right external tool when the task needs a richer editor or canvas.

Task-oriented walkthroughs live in the
[documentation](https://sfdeck.dev/docs/tasks/find-a-record/).

## Safety by default

sf-deck adds a local safety gate in front of Salesforce's own permissions.
Each org has an explicit level shown in the header:

| Level | Permitted through sf-deck |
| --- | --- |
| `read-only` | Browse, query, compare, and export |
| `records` | Read-only actions plus record create/update/delete |
| `metadata` | Record actions plus metadata writes, validation, and deploys |
| `full` | Destructive metadata operations and anonymous Apex |

Production orgs default to read-only. Raising a level is a deliberate,
per-org settings action; unavailable writes do not appear in the TUI and are
rejected by the CLI and IPC backend. Salesforce still enforces the connected
user's permissions—the sf-deck gate is an additional guardrail, not a
replacement for platform security.

Read the full [safety model](https://sfdeck.dev/docs/concepts/safety/).

## Automation and agents

sf-deck has three surfaces with different jobs:

| Surface | Best for | Example |
| --- | --- | --- |
| TUI | Interactive exploration and review | `sf-deck` |
| CLI | One-shot commands, shell scripts, and CI | `sf-deck soql run ... --json` |
| IPC | Driving a running TUI and its live state | `tab.open`, `chip.apply`, `soql.seed` |

Core commands return a stable JSON envelope and scriptable exit codes:

```sh
sf-deck soql run       --org dev --query "SELECT Id, Name FROM Account LIMIT 5" --json
sf-deck record get     --org dev --id 001... --json
sf-deck org safety get --org prod --json
```

CLI and IPC deliberately differ where context demands it: navigation and
editor seeding require a live window, while some file and process operations
are CLI-only. Discover the exact current surface instead of relying on a
static list:

```sh
sf-deck verbs list --json
sf-deck verbs list --surface cli --json
sf-deck verbs list --surface ipc --json
```

The bundled [`skills/sf-deck`](skills/sf-deck) package teaches AI agents to
discover verbs, inspect safety before writes, ask before production changes,
and parse the JSON contract rather than terminal text. See the
[agent integration guide](https://sfdeck.dev/docs/agent-integration/).

## Local data and authentication

- Authentication remains owned by the Salesforce CLI. sf-deck requests the
  current access token, keeps it in process memory, and never writes it to its
  cache or logs.
- Cached org data, settings, tags, saved queries, and dev projects live under
  `~/.sf-deck/`. User-requested exports and bundles go to the path you choose.
- There is no telemetry, analytics, remote license check, or sf-deck cloud
  service. Normal data traffic goes to the selected Salesforce instance.
- Automatic update discovery makes at most one anonymous, version-free request
  to GitHub Releases every 24 hours. It only reports newer stable releases and
  never downloads or installs them. Disable it in **Settings → Updates** or set
  `SF_DECK_NO_UPDATE_CHECK=1`.
- The optional IPC socket is local and user-only. Diagnostics are opt-in,
  loopback-only, and authenticated.

See the [on-disk layout](https://sfdeck.dev/docs/reference/on-disk-layout/)
and [security policy](.github/SECURITY.md) for the complete details.

## Platform support and maturity

sf-deck v0.1 is young, solo-maintained, and already used daily against real
orgs. The maturity labels are intentionally conservative:

| Status | Areas |
| --- | --- |
| **Stable** | Home, objects/schema, records, users, permissions, SOQL, metadata browsing, packages, tags, system diagnostics |
| **Beta** | Reports, deploys and metadata writes, dev projects/bundles, cross-org compare, find-in-another-org |
| **Partial / planned** | System API-usage detail, dashboard viewing, native Windows support |

Supported release targets are macOS and Linux on arm64 and amd64. WSL2 is the
supported Windows path today. Beta means the workflow is implemented and in
regular use, but has had less real-world mileage and may still have rough
edges.

Current limitations:

- No native Windows binary; use WSL2.
- No bulk record mutation API; use the Salesforce CLI for bulk imports and
  updates.
- No async Apex test runner over IPC; long-running test workflows fall
  through to `sf`.
- Dashboard viewing is not implemented.

## FAQ

<details>
<summary><strong>Is anything installed in my Salesforce org?</strong></summary>

No. sf-deck uses the local Salesforce CLI session. There is no connected app,
managed package, permission set, or Setup change to remove later.

</details>

<details>
<summary><strong>How is this different from the Salesforce CLI or VS Code?</strong></summary>

The Salesforce CLI is command-oriented and VS Code is source-oriented.
sf-deck is org-oriented: it is designed for navigating live org state,
switching environments, comparing them, and assembling a working set. It
uses and complements both tools rather than replacing them.

</details>

<details>
<summary><strong>What happens when the Salesforce CLI refreshes my token?</strong></summary>

Newer Salesforce CLI releases expose the access token through a dedicated
non-interactive command. A cold token refresh therefore takes an extra CLI
round trip, and an org with MFA or passkey policy may ask you to verify again.
The header shows `getting new token…` during that work; subsequent API calls
reuse the in-memory token.

</details>

<details>
<summary><strong>Will this consume all of my org's API calls?</strong></summary>

sf-deck caches describes and list results, loads detail lazily, and does no
background polling. The header shows the active API count so the cost of an
action remains visible. You can clear or tune the local cache from Settings.

</details>

<details>
<summary><strong>How does sf-deck tell me about updates?</strong></summary>

Release builds check GitHub Releases asynchronously at most once every 24
hours. Patch, minor, and major stable releases are reported; prereleases are
ignored. The TUI shows a small notice and **Settings → Updates** has a manual
check. Scripts can use:

```sh
sf-deck update check --json
```

sf-deck never downloads or installs an update. Homebrew users upgrade with
`brew upgrade --cask sf-deck`. Automatic checks can be disabled in Settings or
with `SF_DECK_NO_UPDATE_CHECK=1`.

</details>

<details>
<summary><strong>Can a team share tags and dev projects?</strong></summary>

Not yet. They live in a local SQLite database and do not sync between users or
machines. Commit generated sfdx bundles to your normal source repository when
you need a shared, reviewable artifact.

</details>

## Documentation

- [Install and first launch](https://sfdeck.dev/docs/getting-started/install/)
- [Keyboard basics](https://sfdeck.dev/docs/getting-started/keyboard-basics/)
- [Concepts: panels, chips, projects, bundles, tags, and safety](https://sfdeck.dev/docs/concepts/panels/)
- [Task walkthroughs](https://sfdeck.dev/docs/tasks/cross-org-workflow/)
- [CLI and IPC reference](https://sfdeck.dev/docs/reference/cli/)
- [Agent integration](https://sfdeck.dev/docs/agent-integration/)

## Contributing

Bug reports, focused fixes, and documentation improvements are welcome. Open
an issue before a large feature so the design can be aligned with the existing
list-surface, verb-registry, and safety-gate architecture.

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for development setup, tests, release
process, and architectural conventions.

## License

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE). Dependency license
texts included with release binaries live under
[`docs/third_party_licenses/`](docs/third_party_licenses/).

"Salesforce" is a registered trademark of Salesforce, Inc. This project is
not affiliated with, endorsed by, or sponsored by Salesforce, Inc.
