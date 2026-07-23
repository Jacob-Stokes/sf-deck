# sf-deck

A terminal UI for working across your Salesforce orgs.

sf-deck shows you schema, field-level security, SOQL, records,
deploys, users, and metadata diffs — all on one screen, for every
org you're authenticated to. Switch orgs with a keystroke, and reach
most things in a few more.

It runs on the `sf` CLI session you already have. No connected app,
no Setup changes, no new credentials. If `sf org list` works on your
machine, you're already set up.

## What's here

- **[Getting started](getting-started/install.md)** — install, first
  launch, the dozen keys you'll use 90% of the time.
- **[Concepts](concepts/panels.md)** — chips, dev projects, bundles,
  safety levels, tags. The vocabulary sf-deck uses.
- **[Tasks](tasks/find-a-record.md)** — recipes for the things you'd
  actually want to do.
- **[Reference](reference/keymap.md)** — full keymap, CLI verbs, IPC
  protocol, error codes.
- **[Agent integration](agent-integration/index.md)** — how to drive
  sf-deck from a script or AI agent.

## Three things to know up front

**Every screen is keyboard-first.** Number keys jump tabs, Enter
drills, Esc backs out, `/` filters, `?` shows every key the current
screen understands.

**Every write is gated.** Each org has a safety level — read-only,
records, metadata, or full — shown next to the org name. sf-deck
won't offer a write the level disallows. You can set production
read-only here even if your user has full perms in the org.

**Core operations are scriptable.** CLI and IPC share the same JSON
envelope, backend, and safety gate, with intentional transport-specific
verbs for live TUI state and local process/file operations. Query the verb
registry for exact support.

## Status

sf-deck is at **v0.1** — built and used daily against real orgs, but
solo-maintained and young. Most surfaces are stable; a few are **beta**
(Compare, Reports export, Deploys/metadata writes, Dev projects &
bundles) and will hit rough edges. See the
[maturity table in the README](https://github.com/Jacob-Stokes/sf-deck#status--maturity)
for the per-area breakdown. Issues and PRs welcome on
[GitHub](https://github.com/Jacob-Stokes/sf-deck).
