# Documentation and website

Everything published or maintained as documentation lives here. The
user guide is built with
[MkDocs Material](https://squidfunk.github.io/mkdocs-material/), while
the marketing landing page is plain HTML, CSS, and JavaScript.

## Local development

Install the locked Python dependencies and serve the user guide:

```sh
python -m pip install --require-hashes -r docs/requirements.txt
cd docs
mkdocs serve
```

Open `docs/landing/index.html` directly to preview the marketing page.

## Build

From the repository root:

```sh
cd docs
mkdocs build --strict --site-dir site/docs
cp -R landing/. site/
```

The resulting Pages artifact is in `docs/site/`, which is gitignored.
The marketing page is served at `https://sfdeck.dev/` and the MkDocs
user guide at `https://sfdeck.dev/docs/`.

## Regenerate reference pages

The CLI, IPC, and keymap reference pages are generated from the verb
and keymap registries:

```sh
go run ./cmd/sf-deck-docs
```

CI runs `go run ./cmd/sf-deck-docs -check` to fail when generated docs
are stale.

## Deploy

When the repository is public, pushes to `main` that touch `docs/**`
build and deploy one GitHub Pages artifact. The workflow can also be
triggered manually when the repository plan supports private Pages.

## Layout

```text
docs/
├── development/                  internal architecture and QA notes
├── landing/                      marketing page and GIF assets
├── third_party_licenses/         dependency notices verified in every release
├── user/                         MkDocs user-guide source
│   ├── getting-started/
│   ├── concepts/
│   ├── tasks/
│   ├── reference/                CLI / IPC / keymap (auto-generated)
│   └── agent-integration/
├── mkdocs.yml                    site config and navigation
├── requirements.in              direct documentation dependencies
├── requirements.txt             fully locked documentation dependencies
└── site/                         generated output (gitignored)
```
