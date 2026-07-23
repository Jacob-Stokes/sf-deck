<!-- markdownlint-disable MD013 MD033 MD041 -->
<!-- GitHub README: centered hero/badges, details blocks, and wide tables are intentional. -->

<div align="center">

# sf-deck

**A keyboard-first Salesforce workspace for admins, developers, architects, and consultants working across multiple orgs.**

<p>
  <a href="https://github.com/Jacob-Stokes/sf-deck/actions/workflows/ci.yml"><img src="https://github.com/Jacob-Stokes/sf-deck/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Jacob-Stokes/sf-deck/releases"><img src="https://img.shields.io/github/v/release/Jacob-Stokes/sf-deck" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License: Apache-2.0"></a>
</p>

<img src="docs/landing/assets/hero.gif" alt="Launching the fictional sf-deck demo and touring its main Salesforce workspaces" width="920">

<p>
  <a href="#install-and-try-it">Install</a> ·
  <a href="#keyboard-basics">Keyboard</a> ·
  <a href="#what-you-can-do">Capabilities</a> ·
  <a href="#automation-and-agents">Automation</a> ·
  <a href="https://sfdeck.dev/docs/">Docs</a>
</p>

</div>

sf-deck puts every Salesforce org you work with in one terminal. Switch orgs
without losing your place, investigate metadata and permissions, query records,
inspect code and automation, and jump directly into Lightning, Setup, Flow
Builder, or your editor when you need the full interface.

It uses the orgs already authenticated with Salesforce CLI. No managed package,
connected app, Setup changes, or extra credentials. If `sf org list` works,
sf-deck is ready.

## Install and try it

You need the [Salesforce CLI](https://developer.salesforce.com/tools/salesforcecli)
with at least one authenticated org.

```sh
brew install --cask Jacob-Stokes/tap/sf-deck
sf-deck
```

Or explore three fictional orgs without making a network call:

```sh
sf-deck --demo
```

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

</details>

## Keyboard basics

| Key | Action |
| --- | --- |
| `1`–`9` | Open a main workspace |
| `'` | Switch org |
| `/` | Filter the current list |
| `Enter` | Open the selected item |
| `Ctrl+F` | Search the active org |
| `[` / `]` | Cycle saved views or subtabs |
| `o` | Open the matching page in Lightning, Setup, or your editor |
| `?` | Show the keys available on the current screen |

The mouse also works. See the [complete keymap](https://sfdeck.dev/docs/reference/keymap/).

## What you can do

- **Move between orgs without losing your place.** Each org keeps its own
  workspace, filters, selection, and navigation state.
- **Find anything quickly.** Search objects, fields, records, Flows, Apex,
  components, and other loaded metadata from anywhere with `Ctrl+F`.
- **Jump into the right Salesforce tool.** Press `o` to open the selected item
  in Lightning, Setup, Flow Builder, or your editor. `Ctrl+O` shows every
  available destination; `y` copies its URL and `Ctrl+Y` copies the relevant
  ID, API name, or query.
- **Investigate an object end to end.** Browse fields, record types, validation
  rules, permissions, field-level security, records, automation, and related
  source without crossing a maze of Setup pages.
- **Query and work with data.** Write SOQL with metadata completion, reuse saved
  queries and history, inspect or edit records, and export CSV, XLSX, or JSON.
- **Inspect code and automation.** Explore Flows, Apex, triggers, Lightning
  components, tests, debug logs, and deployments from the same workspace.
- **Administer and diagnose orgs.** Review users, permission assignments, login
  activity, packages, limits, jobs, audit history, and system health.
- **Turn discoveries into deployable work.** Collect mixed metadata into tagged
  dev projects and sfdx bundles, then retrieve, validate, deploy, and follow the
  result. These workflows remain beta.

See the [task walkthroughs](https://sfdeck.dev/docs/tasks/find-a-record/) for
complete workflows.

## Automation and agents

Run `sf-deck` for interactive work. Use headless commands in scripts and CI,
or the local IPC socket to drive a running TUI.

Core commands return a stable JSON envelope and scriptable exit codes:

```sh
sf-deck soql run       --org dev --query "SELECT Id, Name FROM Account LIMIT 5" --json
sf-deck record get     --org dev --id 001... --json
sf-deck org safety get --org prod --json
```

List the commands supported by each surface:

```sh
sf-deck verbs list --surface cli --json
sf-deck verbs list --surface ipc --json
```

The bundled [`skills/sf-deck`](skills/sf-deck) package gives AI agents the same
command discovery and safety model. See the [agent integration guide](https://sfdeck.dev/docs/agent-integration/).

<details>
<summary><strong>Install the Claude Code skill</strong></summary>

From a clone, install it for every project:

```sh
mkdir -p ~/.claude/skills
cp -R skills/sf-deck ~/.claude/skills/
```

Use `.claude/skills/` instead for one project. Run `/skills` to confirm the
installation; restart Claude Code if the new directory is not detected. Repeat
the copy after updating the repository.

</details>

## Platform support and maturity

sf-deck v0.1 is young, solo-maintained, and used daily against real orgs.

| Status | Areas |
| --- | --- |
| **Stable** | Home, objects/schema, records, users, permissions, SOQL, metadata browsing, packages, tags, system diagnostics |
| **Beta** | Reports, deploys and metadata writes, dev projects/bundles, cross-org compare, find-in-another-org |
| **Partial / planned** | System API-usage detail, dashboard viewing, native Windows support |

Release builds support macOS and Linux on arm64 and amd64. Windows users can
run the Linux build and Salesforce CLI together inside WSL2.

## Documentation

- [Install and first launch](https://sfdeck.dev/docs/getting-started/install/)
- [Keyboard basics](https://sfdeck.dev/docs/getting-started/keyboard-basics/)
- [Concepts: panels, chips, projects, bundles, tags, and safety](https://sfdeck.dev/docs/concepts/panels/)
- [Task walkthroughs](https://sfdeck.dev/docs/tasks/cross-org-workflow/)
- [CLI and IPC reference](https://sfdeck.dev/docs/reference/cli/)
- [Agent integration](https://sfdeck.dev/docs/agent-integration/)

## Privacy, security, and safety

sf-deck uses existing Salesforce CLI sessions and keeps its working state
local. It has no telemetry, hosted backend, or sf-deck account; Salesforce
record and query results are not written to its persistent cache. Production
orgs start read-only, with explicit per-org safety levels for writes.

Before connecting a real org, read the [user agreement](USER_AGREEMENT.md),
[privacy notice](PRIVACY.md), [safety model](https://sfdeck.dev/docs/concepts/safety/),
[on-disk layout](https://sfdeck.dev/docs/reference/on-disk-layout/), and
[security policy](.github/SECURITY.md).

## Contributing

Bug reports, focused fixes, and documentation improvements are welcome. Open
an issue before starting a large feature.

See [CONTRIBUTING.md](.github/CONTRIBUTING.md) for setup, tests, releases, and
architectural conventions.

## License

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE). Dependency license
texts included with release binaries live under
[`docs/third_party_licenses/`](docs/third_party_licenses/).

"Salesforce" is a registered trademark of Salesforce, Inc. This project is
not affiliated with, endorsed by, or sponsored by Salesforce, Inc.
