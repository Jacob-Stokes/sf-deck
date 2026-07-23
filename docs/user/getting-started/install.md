# Install

## Prerequisites

- [Go 1.26.5+](https://go.dev/dl/)
- [Salesforce CLI](https://developer.salesforce.com/tools/salesforcecli)
  (`sf`), with at least one authenticated org (`sf org login web`)

If `sf org list` returns your orgs, you're ready.

## Homebrew (macOS / Linux)

```sh
brew install --cask Jacob-Stokes/tap/sf-deck
```

This installs the pre-built binary for your platform and keeps upgrades on
Homebrew's normal update path.

## Build from source

```sh
git clone https://github.com/Jacob-Stokes/sf-deck
cd sf-deck
go build -o sf-deck ./cmd/sf-deck
```

Drop the binary somewhere on your `PATH`:

```sh
mv sf-deck ~/.local/bin/
```

Or put it wherever you keep local binaries — sf-deck doesn't care.

## Windows (via WSL)

There's no native Windows binary yet, but sf-deck runs well in WSL2
under Windows Terminal — build or install the Linux binary inside WSL
exactly as above. One WSL-specific note:

**Install the Linux `sf` CLI inside WSL** and authenticate your orgs
from there (`sf org login web`). sf-deck uses the CLI's auth store in
the environment it runs in — a Windows-side `sf` install won't be
seen from WSL.

Everything else works unchanged: browser opens (`o`) detect WSL and
hand the URL to Windows directly via interop (no helper packages
needed), and caching, dev projects, and the IPC control socket all
behave as on native Linux.

## Verify

```sh
sf-deck --help
sf-deck verbs list --json | head -20
```

The first command shows top-level usage. The second confirms the
verb registry is loaded — that's what agents and scripts query.

## What sf-deck stores

sf-deck keeps state under `~/.sf-deck/`. You don't need to touch
these directly, but it's good to know they exist:

| Path | What |
|---|---|
| `~/.sf-deck/settings.toml` | chips, theme, per-org safety overrides |
| `~/.sf-deck/cache.db` | local read-cache: org list, describes, list results |
| `~/.sf-deck/devprojects.db` | dev projects, items, bundles, tags, saved queries, snippets |
| `~/.sf-deck/instances.json` | running-instance registry |
| `~/.sf-deck/control-<N>.sock` | per-instance IPC socket (when started with `--control`) |

Nothing leaves your machine. There is no telemetry. sf-deck talks
to Salesforce; that's it.

## Try the demo

Want to see what sf-deck looks like without pointing it at a real
org?

```sh
sf-deck --demo
```

Three fictional orgs, ~95 sObjects, dev projects, bundles, the lot.
No network calls. Quit with `Ctrl+C` when done.

## Next

[First launch →](first-launch.md)
