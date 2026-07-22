# docs-site

The sf-deck user docs. Built with [mkdocs-material](https://squidfunk.github.io/mkdocs-material/).

## Local dev

```sh
brew install mkdocs-material      # or: pipx install mkdocs-material
cd docs-site
mkdocs serve                       # http://localhost:8000
```

## Build

```sh
mkdocs build --strict --site-dir site/docs
# docs output: docs-site/site/docs/
# the Pages workflow copies landing/ into docs-site/site/
```

## Regenerate reference pages

The CLI / IPC / keymap reference pages are auto-generated from the
verb + keymap registries:

```sh
go run ./cmd/sf-deck-docs
```

CI runs `go run ./cmd/sf-deck-docs -check` to fail the build if
docs are stale relative to the registry.

## Deploy

When the repository is public, every push to `main` that touches
`landing/**` or `docs-site/**` builds one Pages artifact: the marketing
site at `/sf-deck/` and these docs at `/sf-deck/docs/`. The workflow can
also be triggered manually.

First-time setup (one-time, by a repo admin):

1. **Settings → Pages → Source: GitHub Actions** in the
   `Jacob-Stokes/sf-deck` repo.
2. Make the repository public, then push a commit that touches
   `landing/` or `docs-site/` — the workflow runs.
3. The deploy URL appears in the workflow output.
4. (Optional) Point a custom domain at the Pages site and update the
   canonical URLs in `landing/index.html` and `docs-site/mkdocs.yml`.

## Layout

```text
docs-site/
├── mkdocs.yml                    site config + nav
├── docs/
│   ├── index.md                  landing page
│   ├── getting-started/          install + first launch + keys
│   ├── concepts/                 chips, dev projects, bundles, safety, tags
│   ├── tasks/                    cookbook recipes
│   ├── reference/                CLI / IPC / keymap (auto-generated)
│   └── agent-integration/        AI-agent-facing docs
└── site/                         built output (gitignored)
```
