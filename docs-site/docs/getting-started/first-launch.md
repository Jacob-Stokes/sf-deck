# First launch

Run `sf-deck` against your real orgs (or `sf-deck --demo` if you
just want to look around).

## What you see

The screen has four parts:

```
┌─[ tab strip ]──────────────────────────────────────────────┐
│ 1 home  2 soql  3 objects  4 flows  5 apex  6 users  ...   │
├─[ left rail ]──┬─[ main pane ]──────────────────[ sidebar ]┤
│ org names      │                                            │
│ + safety pill  │   This is where everything happens.        │
│                │                                            │
│                │                                            │
├────────────────┴──────────────────────────────[ status bar ]┤
└────────────────────────────────────────────────────────────┘
```

- **Tab strip** at the top. Each tab is numbered; press the digit to
  jump.
- **Left rail** shows your orgs. The active one is highlighted; its
  safety pill is the colour of its current level.
- **Main pane** is whatever the active tab shows.
- **Sidebar** (right) is context-sensitive — open with `\`. Shows
  the cursor's row in full, plus actions you can take.
- **Status bar** at the bottom carries the last flash message + a
  short hint of useful keys.

## Five gestures that get you 80% of the way

1. **Number key** (`1`-`9`, then `0` for the tenth) jumps to a tab.
2. **Enter** drills into the cursored row. **Esc** backs out one
   level.
3. **`/`** filters the current list. Type to narrow, Enter to
   commit, Esc to clear.
4. **`[`** and **`]`** cycle the saved views (chips) on any tab that
   has a chip strip. `L` toggles chip mode, `V` opens the chip
   manager, `F` favourites the current chip. More in
   [Concepts → Chips](../concepts/chips.md).
5. **`?`** shows every key the current screen understands. When you
   forget, that's the answer.

## Switch orgs

Press `'` (apostrophe) to focus the org rail, then move with `j` / `k`
(or the arrow keys) and press **Enter** to switch to the highlighted
org. `space` folds/unfolds an org group; `'` then `a` opens the
add-org picker.

The safety pill in the left rail changes colour as you switch — your
first signal that you're now looking at production.

To compare metadata *across* orgs without leaving the active one, use
the [`/compare` tab](../tasks/cross-org-workflow.md) rather than the
org rail.

## Where to go next

- [Keyboard basics](keyboard-basics.md) — the full first-line keymap.
- [Concepts → Panels](../concepts/panels.md) — what each pane is for.
- [Tasks → Find a record](../tasks/find-a-record.md) — your first
  recipe.
