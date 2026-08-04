# Panels

sf-deck has four panels. Each has one job.

## Tab strip

Top of the screen. Each tab gets a number. Press the digit to jump.

Tabs that don't fit get folded into a "more" overflow you open with
`0`. The order is configurable — pin your most-used tabs first via
the settings modal (`Ctrl+S`).

Some tabs have subtabs — a second strip just below the main one.
Shift + a digit (`Shift+1`, `Shift+2`, …) jumps to the matching
subtab.

## Left rail

Lists every org you're authenticated to (as `sf org list` knows
them). The active one is highlighted. Next to each org is its safety
pill — read-only, records, metadata, or full.

The rail also surfaces the loaded dev project and any pinned tabs
beyond the visible strip.

To switch orgs, press `'` to focus the rail, move with `j` / `k`, and
`Enter` to hop. The rail is also there so you always know what you're
looking at.

Toggle the rail with `|` if you need the horizontal space.

## Main pane

Whatever the active tab shows. Most tabs are list-shaped: a chip
strip at the top, a table below, a footer hint at the bottom.

The cursor sits in the main pane. Up/down moves it; Enter drills.

## Sidebar (right)

Context-sensitive. Whatever's interesting about the cursored row.
For a record, it's every field. For a flow, it's the
last-modified-by + version count + a list of every sObject the flow
touches. For an apex class, it's a one-line summary + a list of
classes that reference it.

Toggle with `\`. Stack it below the main pane instead of beside it
with `Ctrl+\` (better on narrow terminals).

Press `i` on a row to expand the sidebar's content into a full
modal — same data, more room to read.

## Status bar

Bottom of the screen. Two parts:

- **Last flash message** — sf-deck's way of telling you what just
  happened. Errors, confirmations, brief hints.
- **Key hints** — the most relevant shortcuts for the current
  screen.

The status bar isn't a log. Messages are transient. If you want a
history of what sf-deck has done, see the `applog` tab on `/system`.
