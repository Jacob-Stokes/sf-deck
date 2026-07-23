# Keymap reference

Every sf-deck key binding. Auto-generated from
`internal/ui/keymap/commands.go`.

Press `?` on any screen for the live keymap (context-
sensitive — shows only what works here).

To override any binding, run `sf-deck --dump-keymap > ~/.sf-deck/keybindings.toml` and
edit the file.

## Bundles

| Action | Default key(s) | Where |
|---|---|---|
| Deploy from bundle | `D` | tab=bundles |
| Force-refresh diff | `R` | tab=bundle-detail |
| Open bundle directory / file on disk | `o` | tab=bundles,bundle |
| Retrieve into bundle | `r` | tab=bundles |
| Unlink bundle (leaves directory) | `d` | tab=bundles |
| Validate-only deploy | `v` | tab=bundle-detail |
| Yank bundle path | `y` | tab=bundle-detail |

## Cache Settings

| Action | Default key(s) | Where |
|---|---|---|
| Reset TTL to default | `r` | modal=cache-settings |

## Chip Wizard

| Action | Default key(s) | Where |
|---|---|---|
| Delete current row | `ctrl+x` | modal=chip-wizard |
| Open lookup | `ctrl+l` | modal=chip-wizard |
| Save chip | `ctrl+s` | modal=chip-wizard |
| Toggle row mode | `ctrl+t` | modal=chip-wizard |

## Chips

| Action | Default key(s) | Where |
|---|---|---|
| Hide SOQL query line | `ctrl+-` | tab=records-shaped |
| Hide chip strip (maximize list) | `ctrl+=` | tab=has-dashboard |
| Next chip | `], shift+right` | global |
| Open chip manager | `V` | tab=chip-bearing |
| Open chip overflow | `M` | tab=chip-bearing |
| Previous chip | `[, shift+left` | global |
| Toggle chip favourite | `F` | tab=chip-bearing |
| Toggle chip mode (sf-deck ↔ Salesforce list views) | `L` | tab=records-shaped |

## Dev Projects

| Action | Default key(s) | Where |
|---|---|---|
| Collect item to loaded project (toggle) | `K` | global |
| Collect item — pick project | `ctrl+k` | global |
| Create bundle from project | `x, ctrl+x` | tab=dev-projects |
| Delete project | `d` | tab=dev-projects |
| Edit / rename project | `e` | tab=dev-projects |
| Force-delete project | `D` | tab=dev-projects |
| Load / open loaded project | `_` | global |
| New dev project | `n` | tab=dev-projects |
| Open bundles | `b` | tab=dev-project-detail |
| Open dev-projects list | `-` | global |
| Open tag manager | ``` | global |
| Toggle project chip | `p` | tab=reports |
| Toggle project scope | `O` | tab=dev-project-detail |

## Downloads

| Action | Default key(s) | Where |
|---|---|---|
| Open exported file | `o` | downloads |
| Remove from history | `d` | downloads |
| Reveal in Finder | `r` | downloads |
| Yank file path | `y` | downloads |

## Exec

| Action | Default key(s) | Where |
|---|---|---|
| Delete saved snippet | `D` | tab=exec-saved |
| Duplicate saved snippet | `c` | tab=exec-saved |
| Edit snippet | `e` | tab=exec |
| Open $EDITOR | `ctrl+e` | tab=exec |
| Rename saved snippet | `R` | tab=exec-saved |
| Save / update snippet | `S` | tab=exec |
| Save as new snippet | `ctrl+n` | tab=exec |
| Toggle debug-log capture | `ctrl+l` | tab=exec |

## Flows

| Action | Default key(s) | Where |
|---|---|---|
| Delete inactive flow version | `D` | tab=flow-detail |
| Rename flow label | `e` | tab=flow-detail |

## Legacy

| Action | Default key(s) | Where |
|---|---|---|
| Cycle filter (legacy) | `f` | tab=objects |

## List Table

| Action | Default key(s) | Where |
|---|---|---|
| Clear sort | `S` | list-table |
| Edit current view | `e` | list-table |
| Grow column | `>` | list-table |
| Reset column widths | `W` | list-table |
| Scroll columns left | `left, h, ,` | list-table |
| Scroll columns right | `right, l, .` | list-table |
| Shrink column | `<` | list-table |
| Snap column to header | `{` | list-table |
| Snap column to widest cell | `}` | list-table |
| Sort by cursored column | `s` | list-table |
| Toggle pagination | `P` | list-table |
| Toggle zen mode | `z` | list-table |

## Modals

| Action | Default key(s) | Where |
|---|---|---|
| Open API call log | `ctrl+a` | global |
| Open downloads | `ctrl+j` | global |
| Open settings | `ctrl+s` | global |

## Navigation

| Action | Default key(s) | Where |
|---|---|---|
| Drill / activate | `enter` | global |
| Go to bottom | `G, end` | global |
| Go to top | `g, home` | global |
| Jump down (~5 rows) | `ctrl+down` | global |
| Jump up (~5 rows) | `ctrl+up` | global |
| Move down | `j, down` | global |
| Move up | `k, up` | global |
| Page down | `ctrl+d, pgdown` | global |
| Page up | `ctrl+u, pgup` | global |
| Refresh all loaded data (active org) | `ctrl+r` | global |
| Refresh current view | `r` | global |

## Open & Yank

| Action | Default key(s) | Where |
|---|---|---|
| Open in Lightning | `o` | global |
| Open menu (multi-target) | `ctrl+o` | global |
| Yank URL | `y` | global |
| Yank menu (multi-target) | `ctrl+y` | global |

## Org Manager

| Action | Default key(s) | Where |
|---|---|---|
| Add org | `A` | modal=org-manage |
| Clear alias | `-` | modal=org-manage |
| Cycle safety level (read_only → records → metadata → full → inherit) | `s` | modal=org-manage |
| Delete group | `x` | modal=org-manage |
| Logout / disconnect org | `D` | modal=org-manage |
| Move group down | `]` | modal=org-manage |
| Move group up | `[` | modal=org-manage |
| Move org down (within / across groups) | `>` | modal=org-manage |
| Move org to group… | `g` | modal=org-manage |
| Move org up (within / across groups) | `<` | modal=org-manage |
| New group | `n` | modal=org-manage |
| Pin as sf-deck startup org | `P` | modal=org-manage |
| Re-authenticate org (login web, same alias) | `L` | modal=org-manage |
| Rename alias | `=` | modal=org-manage |
| Rename group | `R` | modal=org-manage |
| Set as default DevHub | `^` | modal=org-manage |
| Set as sf CLI default org | `*` | modal=org-manage |

## Orgs

| Action | Default key(s) | Where |
|---|---|---|
| Open org-management modal | `ctrl+e` | focus=orgs |
| Toggle group expand/collapse | ` ` | focus=orgs |

## Permissions

| Action | Default key(s) | Where |
|---|---|---|
| Toggle ModifyAllRecords | `m` | tab=perm-objects |
| Toggle ViewAllRecords | `v` | tab=perm-objects |
| Toggle field-level Edit | `e` | tab=fls |
| Toggle field-level Read | `r` | tab=fls |
| Toggle object Create | `c` | tab=perm-objects |
| Toggle object Delete | `d` | tab=perm-objects |
| Toggle object Edit | `e` | tab=perm-objects |
| Toggle object Read | `r` | tab=perm-objects |
| Toggle system permission | `space` | tab=perm-system |

## Process

| Action | Default key(s) | Where |
|---|---|---|
| Back / cancel | `esc` | global |
| Focus bookmarks panel | `—` | global |
| Focus orgs panel | `'` | global |
| Help / view info | `?` | global |
| Inspect — full sidebar info in a modal | `i` | global |
| Open command menu | `;` | global |
| Quit | `ctrl+c` | global |
| Toggle left rail | `ctrl+\` | global |
| Toggle pane focus | `—` | global |
| Toggle right sidebar | `\` | global |
| Toggle stacked sidebar (below vs beside main) | `|` | global |

## Record

| Action | Default key(s) | Where |
|---|---|---|
| Discard all dirty edits | `ctrl+x` | tab=record |
| Edit cursored field | `e` | tab=record |
| Save dirty fields | `ctrl+s` | tab=record |

## Records

| Action | Default key(s) | Where |
|---|---|---|
| Export records | `ctrl+x` | tab=records,object-detail,users |

## Reports

| Action | Default key(s) | Where |
|---|---|---|
| Export report | `ctrl+x` | tab=reports |

## SOQL

| Action | Default key(s) | Where |
|---|---|---|
| Delete saved query | `D` | tab=soql-saved |
| Duplicate saved query | `c` | tab=soql-saved |
| Edit query | `e` | tab=soql |
| Export results | `ctrl+x` | tab=soql-editor |
| Rename saved query | `R` | tab=soql-saved |
| Save / update query | `S` | tab=soql |
| Save as new | `ctrl+n` | tab=soql |
| Toggle Bulk API (1 call vs ~1/2k rows) | `ctrl+b` | tab=soql |
| Toggle Tooling API | `T` | tab=soql |
| Yank column as IN-clause | `ctrl+y` | tab=soql-editor |
| Yank cursored cell | `y` | tab=soql-editor |
| Yank cursored row (TSV) | `Y` | tab=soql-editor |

## Search

| Action | Default key(s) | Where |
|---|---|---|
| Clear search | `C` | global |
| Global search modal | `ctrl+f` | global |
| Start search | `/` | global |
| Toggle metadata/records mode (inside global search) | `ctrl+r` | modal |

## Tabs

| Action | Default key(s) | Where |
|---|---|---|
| Active overflow tab | `9` | global |
| More tabs (overflow modal) | `0` | global |
| Next subtab | `tab` | global |
| Previous subtab | `shift+tab` | global |
| Subtab 1 | `!, shift+1` | subtabbed |
| Subtab 2 | `@, ", shift+2` | subtabbed |
| Subtab 3 | `#, £, shift+3` | subtabbed |
| Subtab 4 | `$, shift+4` | subtabbed |
| Subtab 5 | `%, shift+5` | subtabbed |
| Subtab 6 | `^, shift+6` | subtabbed |
| Subtab 7 | `&, shift+7` | subtabbed |
| Subtab 8 | `*, shift+8` | subtabbed |
| Subtab 9 | `(, shift+9` | subtabbed |
| Subtab overflow (More…) | `), shift+0` | subtabbed |
| Tab 1 | `1` | global |
| Tab 2 | `2` | global |
| Tab 3 | `3` | global |
| Tab 4 | `4` | global |
| Tab 5 | `5` | global |
| Tab 6 | `6` | global |
| Tab 7 | `7` | global |
| Tab 8 | `8` | global |

## Tags

| Action | Default key(s) | Where |
|---|---|---|
| Cycle flag column (full/letter/hidden) | `ctrl+g` | global |
| Tag all visible rows | `T` | list surfaces |
| Tag picker | `t` | global |
| Toggle project column | `ctrl+p` | global |
| Toggle tag column | `ctrl+t` | global |

## Theme Picker

| Action | Default key(s) | Where |
|---|---|---|
| Clear search | `C` | modal=theme-picker |
| Toggle theme favourite | `f, F` | modal=theme-picker |

