# sf-deck QA checklist (generated)

Derived from tabRegistry by `go run ./cmd/inventory -qa` — do not
hand-edit; regenerate after feature work. Walk top to bottom on a
data-rich org, then spot-check empty states on a bare org and one
pass at ~100-col width.

## Global (once per session)

- [ ] cold launch: org list paints from cache instantly, then refreshes
- [ ] org rail: 0 focuses, j/k switches org live, quick-jump letters, fold/expand, M manager
- [ ] org switch mid-list: no cursor/scroll bleed (wheel gate)
- [ ] narrow terminal: footer shows +N… marker; ? lists the full set
- [ ] ? modal: Keybindings filter + edit, Tab → About page
- [ ] ctrl+f global search from cold launch
- [ ] ctrl+r global refresh; per-resource flashes
- [ ] settings modal: theme picker, limits, keybindings round-trip
- [ ] quit + relaunch: tabs/widths/chips/pins persist

## /home

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Landing

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Recently Viewed

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Notifications

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Limits

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Licenses

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Downloads

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /soql

- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Editor

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Saved

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### History

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /objects

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
- [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
- [ ] empty state renders a hint, not a blank pane
- [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /object

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Details

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Schema

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Records

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### FLS

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Validation

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Record Types

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Triggers

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Layouts

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

#### Flows

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
    - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

### drill: /field

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

### drill: /validation

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /recordtype

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /trigger

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /flows

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
- [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
- [ ] empty state renders a hint, not a blank pane
- [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /flow

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

### drill: /flow-version

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)


## /records

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /record

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)


## /packages

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
- [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
- [ ] empty state renders a hint, not a blank pane
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /projects

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /setup

- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /perms

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Permission Sets

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Permission Set Groups

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Profiles

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Queues

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Public Groups

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /perm

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

### drill: /queue

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /public-group

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /reports

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Reports

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Dashboards

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Report Types

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /report

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)


## /system

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org

### Logs

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Deploys

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Audit Trail

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Interviews

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Async Jobs

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Scheduled Jobs

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### API

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /deploy

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /dev-projects

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Projects

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Bundles

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /dev-project

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

#### Items

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

#### Bundles

    - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
    - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
    - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /bundle

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)


## /apex

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Classes

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Triggers

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /apex-class

  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)


## /components

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### LWC

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Aura

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /component

  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)


## /meta

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org

### Browse

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Custom Metadata

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Custom Labels

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Custom Settings

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Static Resources

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Named Credentials

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### Remote Sites

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)

### drill: /meta-type

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /tags

- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /tag

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /users

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Recent logins

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### All users

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Active

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] chips: ←/→ cycle, counts correct, `V` manager, `N` wizard round-trips, `F` pin
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /user-detail

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated
  - [ ] drilling records a Recently-viewed entry (check the recent chip on the parent)

### drill: /user-sessions

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated


## /exec

- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### Editor

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### Output

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### Saved

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### History

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill


## /compare

- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### New

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### Result

  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### Saved

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill

### History

  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill


## /communities

- [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
- [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
- [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
- [ ] empty state renders a hint, not a blank pane
- [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
- [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)
- [ ] sidebar matches the cursored row; updates on move; `i` inspect when truncated

### drill: /community

  - [ ] loads on entry (cold + from cache); `r` refreshes; error state readable on a broken org
  - [ ] `/` filters (and ranks sensibly); esc clears; cursor + scroll behave at list ends
  - [ ] `s` sort, `c` column mode, `}` snap, `z` zen — widths persist across relaunch
  - [ ] empty state renders a hint, not a blank pane
  - [ ] `enter` drills; `esc` returns to the same row; number-key back-nav restores drill
  - [ ] `o` opens the right Lightning page, `ctrl+o` lists every open target; `y` copies the URL, `ctrl+y` copies a value (label / API name / Id / SOQL …)


