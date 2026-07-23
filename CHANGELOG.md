# Changelog

All notable changes to sf-deck are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] — first public release

The initial public release. sf-deck has been in private use for
several months; this release wraps up what was already there into
a first installable version.

### Highlights

- **Multi-org TUI** spanning every org you're authenticated to via
  the `sf` CLI. Switch orgs with a keystroke; safety-level pill in
  the header shows what you can write to.
- **Records, schema, FLS, flows, apex, deploys, users, perms** all
  reachable from a numbered tab strip. Drill, filter, search,
  chip-cycle.
- **Chips** — saved filter views per surface, cyclable, cross-org,
  optionally session-only (ephemeral).
- **Dev projects** — collect items from anywhere into a named
  working set that spans orgs.
- **Bundles** — materialise a dev project as an sfdx project
  directory and retrieve / validate / deploy from inside sf-deck.
  Async + report pattern for long-running deploys.
- **Tags** — apply your own tags to any item across any org.
- **SOQL editor** — multi-line, autocomplete against the org's
  schema, saved library, history.
- **Compare** — Apex, Flows, and metadata diffed org-to-org.
- **Headless CLI** — core automation runs as `sf-deck <noun>
  <verb> --json`, with a stable JSON envelope and exit codes.
- **IPC socket** — a running sf-deck window exposes a Unix-domain
  socket for agent-driven automation, including live-only navigation
  and editor state. CLI and IPC share a backend and safety gate but
  intentionally differ for some verbs.
- **Verb registry** — single source of truth for what sf-deck can
  do. Discoverable at runtime via `sf-deck verbs list --json`;
  drives both transports and the docs.
- **Safety model** — four levels (read-only, records, metadata,
  full). Per-org, gates every write before the API call; anonymous
  Apex requires full.
- **Agent skill** — `skills/sf-deck/` packages the contract for AI
  agents: discover via the registry, gate writes through safety,
  parse the JSON envelope.
- **Demo mode** — `sf-deck --demo` boots against fictional
  Northwind orgs with no network calls. Deterministic fixtures.

### Install

```sh
brew install --cask Jacob-Stokes/tap/sf-deck
```

Or download a binary from the
[release page](https://github.com/Jacob-Stokes/sf-deck/releases/latest).

### Known limitations

- **Windows not supported.** Uses POSIX file locking and AF_UNIX
  sockets. WSL works fine via the Linux binary.
- **No bulk record API.** `record.create / .update / .delete` are
  per-record. Use the `sf` CLI for bulk operations.
- **No async Apex test runner over IPC.** Long test runs fall
  through to the `sf` CLI.
- **No bundle-vs-org diff over IPC.** The TUI has the preview; the
  IPC layer doesn't expose it yet.

See [Limitations](docs/user/concepts/) in the docs site for
the full list.

[Unreleased]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Jacob-Stokes/sf-deck/releases/tag/v0.1.0
