# Changelog

All notable changes to sf-deck are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The default navigation jump is now ten rows with `Ctrl+Arrow`, `J`, or `K`.
- Deploy-job polling now settles at ten-second intervals, while the live
  deploys view refreshes every twenty seconds to reduce Salesforce API usage.

## [0.1.6] - 2026-08-18

### Fixed

- Importing the built-in demo now writes its cache in one atomic transaction.
  This prevents the TUI from freezing during hundreds of individual disk
  commits on slower Linux systems and avoids partially imported demo data if
  the process exits.

## [0.1.5] - 2026-08-18

### Added

- Native `.deb` and `.rpm` packages are attached to GitHub releases for
  straightforward installation on Debian, Ubuntu, Fedora, and related Linux
  distributions.

### Fixed

- `Ctrl+C` now quits during first-launch modal loading and saving transitions.

## [0.1.4] - 2026-08-17

### Security

- Release binaries are built with Go 1.26.6, incorporating the latest Go
  standard-library security fixes.

### Changed

- Redundant implementation comments were removed without changing runtime
  behaviour.

## [0.1.3] - 2026-08-13

### Security

- Demo and documentation assets are built exclusively from fictional
  Northwind data.
- Notification and Chatter response bodies are excluded from diagnostic dump
  files.
- Dev-project record sidecars use atomic writes with owner-only
  permissions.

## [0.1.2] - 2026-07-23

### Added

- A versioned first-run acknowledgement appears before sf-deck discovers or
  contacts a real Salesforce org. Headless users can inspect and accept the
  same privacy notice and user agreement with `sf-deck legal`.
- `sf-deck data inspect` documents local storage boundaries, while
  `sf-deck data erase --yes` removes sf-deck-owned application state after all
  running instances are closed.
- `sf-deck org logout --org <target> --yes` removes a selected local
  Salesforce CLI authorization.
- Privacy and user-agreement pages are linked from the website, README,
  documentation, Settings, and About screen.

### Changed

- Salesforce record lists/details, SOQL and report result rows, list-view
  results, related-record lookups, and Salesforce RecentlyViewed rows remain
  process-memory only. Startup enforces that boundary by clearing the reserved
  record-data cache prefixes.
- The agent skill requires policy-status discovery and explicit user
  approval before accepting terms or erasing local data.

## [0.1.1] - 2026-07-23

### Compatibility

- `sf-deck object describe` accepts the documented `--sobject` flag and the
  compatible `--name` spelling; conflicting values are rejected.
- JSON responses from `object describe` identify the command as
  `object.describe`, matching the verb registry and generated documentation.

## [0.1.0] - 2026-07-23

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
- **Cross-org comparison is unfinished beta functionality** and is
  not part of the initial launch promise.

See the [documentation](https://sfdeck.dev/docs/) for
feature-specific limitations.

[Unreleased]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.6...HEAD
[0.1.6]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/Jacob-Stokes/sf-deck/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Jacob-Stokes/sf-deck/releases/tag/v0.1.1
[0.1.0]: https://github.com/Jacob-Stokes/sf-deck/releases/tag/v0.1.0
