# Contributing

Thanks for taking a look. sf-deck is solo-maintained but issues and
PRs are welcome.

## Before you start

Open an issue first for anything bigger than a one-line fix. Two
reasons:

- A 200-line PR that doesn't fit the project's direction is
  painful to review and painful to close. A 5-line issue is
  cheap.
- Some things look like missing features but are intentional
  (see [What sf-deck can't do](docs-site/docs/concepts/) — the
  Limitations sections per page). Worth checking before you
  build.

Small things — typo fixes, tightening copy, a new CLI verb that
already has an IPC counterpart — feel free to PR directly.

## Dev setup

```sh
git clone https://github.com/Jacob-Stokes/sf-deck
cd sf-deck
go build -o sf-deck ./cmd/sf-deck
```

Requirements:

- Go 1.26+
- `sf` CLI installed and authenticated to at least one org (for
  anything beyond `--demo`)
- macOS or Linux (Windows isn't supported yet — see
  [#1](https://github.com/Jacob-Stokes/sf-deck/issues) if it
  becomes a real ask)

Run against the seeded demo for any work that doesn't need a real
org:

```sh
./sf-deck --demo
```

## The load-bearing patterns

sf-deck has a few patterns the rest of the codebase depends on.
Respect them and review feedback stays short.

### 1. The verb registry is the single source of truth

Every CLI noun.verb and every IPC command is declared in
`internal/verbs/registry.go`. Adding a verb means editing that
file first; the drift test (`go test ./internal/verbs/`) catches
you if the implementation isn't there.

The CLI and IPC reference pages in the docs site are
auto-generated from this registry. After editing, run:

```sh
go run ./cmd/sf-deck-docs
```

CI runs `go run ./cmd/sf-deck-docs -check` to catch drift.

### 2. List-shaped surfaces reuse the list engine

`internal/ui/list_surface*.go` files declare list surfaces with
columns, sort, search, scroll. Don't hand-roll a new list
renderer; reuse `listSurface` + `ListView[T]` + `renderListModel`.
See `docs/surfaces.md` for the cookbook recipe.

### 3. Every write goes through the safety gate

`internal/settings/safety.go` + `internal/app/safety.go` define
the 4-level model. Any verb that mutates Salesforce declares its
required level on the registry `Spec.Safety` field. The gate
fires before the network call.

If you're adding a write verb that doesn't fit the existing
levels, open an issue first — we don't want gate creep.

### 4. JSON envelope contract is stable

CLI and IPC both return `{ok, command, data, error}`. Don't add
fields that break older consumers. Adding new optional fields is
fine; renaming or removing is a breaking change.

### 5. The skill is the agent contract

`skills/sf-deck/SKILL.md` is what AI agents read. It tells them to
discover via the registry, gate writes through safety, parse JSON.
Keep behaviour consistent with what the skill claims.

## Tests

```sh
go test ./...
```

Conventions:

- **Integration tests against real orgs only run on a dedicated
  throwaway sandbox.** Never write to a real production org or
  any org you don't fully own from a test. See
  `docs/qa-checklist.md`.
- **Coverage ratchet** (`scripts/coverage-ratchet.sh`) is enforced
  in CI. A drop fails the build; you can either add tests or, with
  justification in the commit message, raise the baseline via
  `scripts/coverage-ratchet.sh --update`.
- **No mocks for SF write tests.** Real org, throwaway artifacts,
  cleanup after.

## Style

- **Comments explain WHY, not WHAT.** Code says what it does. A
  comment that just describes the next three lines is noise.
- **No emojis in code, commits, or commit messages.** Strong
  preference; see existing commits for the tone.
- **Commit message format:** subject line is one line, present
  tense, lowercased noun.verb. Body explains the why. See `git log`
  for the house style.

## Architecture docs

Internal architecture notes live in [`docs/`](docs/) — render
pipeline, cache layout, scrolling, and the list-surface cookbook.
The user-facing docs site lives in `docs-site/`. Read
the architecture notes when you need to; they're not required
reading to make a small change.

## Releasing

Maintainer-only. Tag + push triggers goreleaser:

```sh
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions builds binaries for macOS arm64/amd64 + Linux
amd64/arm64, attaches them to a release, and publishes a Homebrew
cask to `Jacob-Stokes/homebrew-tap`. See `.goreleaser.yaml` +
`.github/workflows/release.yml`.

## Questions

Open a GitHub issue and tag it `question`. For anything sensitive,
see [SECURITY.md](SECURITY.md).
