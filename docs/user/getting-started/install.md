# Install

## Prerequisites

Demo mode has no external prerequisites. To connect a real org, install the
[Salesforce CLI](https://developer.salesforce.com/tools/salesforcecli) (`sf`)
and authenticate at least one org with `sf org login web`.

If `sf org list` returns your orgs, you're ready.

## Homebrew (macOS / Linux)

```sh
brew install --cask Jacob-Stokes/tap/sf-deck
```

This installs the pre-built binary for your platform and keeps upgrades on
Homebrew's normal update path.

## Linux packages

Each [GitHub release](https://github.com/Jacob-Stokes/sf-deck/releases)
includes packages for x86-64 (`amd64`) and ARM64 (`arm64`) Linux systems.
Replace `VERSION` and `ARCH` below with the values in the downloaded filename.

On Debian, Ubuntu, and derivatives, download the matching `.deb` and run:

```sh
sudo apt install ./sf-deck_VERSION_linux_ARCH.deb
```

On Fedora, RHEL, and derivatives, download the matching `.rpm` and run:

```sh
sudo dnf install ./sf-deck_VERSION_linux_ARCH.rpm
```

Both packages install the binary under `/usr/bin`. They do not add an APT or
DNF repository, so install a future upgrade by downloading its newer package.

## Portable archive

The release page also provides `.tar.gz` archives for macOS and Linux. Extract
the archive and place the binary on your `PATH`:

```sh
tar -xzf sf-deck_VERSION_linux_ARCH.tar.gz
sudo install -m 0755 sf-deck /usr/local/bin/sf-deck
```

## Build from source

Building requires [Go 1.26.6+](https://go.dev/dl/).

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
under Windows Terminal. Install the Linux `.deb`, portable archive, or source
build inside WSL exactly as above. One WSL-specific note:

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
| `~/.sf-deck/update-state.json` | timestamp and result of the last stable-release check |
| `~/.sf-deck/instances.json` | running-instance registry |
| `~/.sf-deck/control-<N>.sock` | per-instance IPC socket (when started with `--control`) |

There is no telemetry. Normal data traffic goes to Salesforce. By default,
release builds also make at most one anonymous request to GitHub Releases every
24 hours to discover newer stable sf-deck versions. The request does not include
your current version, and sf-deck never downloads or installs an update.

Manage this under **Settings → Updates**, or disable automatic checks for a
process:

```sh
SF_DECK_NO_UPDATE_CHECK=1 sf-deck
```

Check explicitly from a shell:

```sh
sf-deck update check
sf-deck update check --force --json
```

Homebrew users install a discovered update with:

```sh
brew upgrade --cask sf-deck
```

Linux package users download and install the newer `.deb` or `.rpm` from the
release page.

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
