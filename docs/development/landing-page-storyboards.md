# Landing page storyboards

Status: production brief only. Do not record or publish from this document until
the preflight checks below pass.

The landing page should tell five short product stories. Each animation must prove
one claim with fictional Northwind data; it must not behave like a compressed
feature tour. The existing feature grid can remain later on the page as a
reference.

## Release blockers

- Replace `hero.gif`, `demo.gif`, `fls.gif`, and all three poster images.
- Remove `sf-deck — LSE-PROD · read-only` from the hero terminal chrome.
- Do not reuse any frame from the current `demo.gif` or `demo-poster.png`. They
  contain real LSE org information.
- Inspect every rendered GIF and poster at full size before publishing.
- Rewrite the public repository's history to remove the unsafe media and LSE
  string before making the repository public again.

## Recording contract

Every tape must begin from a fresh process and use the same presentation:

```text
Type "./sf-deck --demo"
Enter
Set Theme "TokyoNightStorm"
Set Width 1320
Set Height 760
Set FontSize 13
Set Framerate 15
```

Apply the visual settings before typing the launch command if the installed VHS
version requires settings to appear before interaction.

Rules for every animation:

- GIF is the canonical output. Do not publish WebM alternatives.
- Use only `./sf-deck --demo`; never launch an authenticated sf-deck session.
- Keep the visible `DEMO` badge in frame.
- Target 7–12 seconds for focused stories and 24–30 seconds for the hero reel;
  keep every GIF below 2 MB after optimization.
- Show no more than five meaningful beats.
- Hold the opening and closing frames for at least 1.5 seconds.
- Hold every destination long enough to read it, normally 1.5–2 seconds.
- Avoid cursor flutter, repeated scrolling, and transitions shorter than 750 ms.
- Use `Hide` and `Show` around startup or asynchronous redraws when needed.
- Never depend on row counts or timings from a live org.
- Generate the poster from the final, settled proof frame—not the first frame.
- Use the same fictional names and capitalization everywhere:
  `northwind-dev`, `northwind-uat`, `northwind`, `Shipment__c`, and
  `Shipment status revamp`.

The terminal title used by the website is specified per story. It belongs to the
HTML terminal chrome and is not baked into the GIF.

## Proposed page order

1. Hero
2. Trust strip
3. Every org, one workspace
4. The whole object in one place
5. Find anything quickly
6. Turn discoveries into a project
7. Safety levels
8. Feature reference grid
9. Coming soon: Compare
10. Headless and agents
11. Install and demo CTA

This order moves from the daily problem to the deeper workflow. The broad feature
grid becomes supporting evidence rather than the first explanation of the
product.

---

## Story 1 — Hero: launch and product reel

**Claim to prove:** sf-deck launches as one cohesive keyboard-first workspace
covering the daily Salesforce surfaces that normally require many browser tabs.

**Eyebrow:** Salesforce, without the tab maze

**Heading:** Every Salesforce org, one keystroke away.

**Body copy:**

> Explore schema, records, code, users, deploys, and org health from one
> keyboard-first terminal workspace. sf-deck uses your existing Salesforce CLI
> sessions and keeps its working state local.

**Primary CTA:** Install sf-deck

**Secondary CTA:** Try the fictional demo

**Secondary CTA command:** `sf-deck --demo`

**Terminal title:** `sf-deck — northwind-dev · DEMO`

**Alt text:** `Launching the fictional sf-deck demo, then touring SOQL, objects,
flows, Apex, components, users, deploys, and dev projects.`

**Target:** 24–30 seconds; poster on the settled Dev Projects overview.

### Timeline

| Time | Visible action | What it proves |
| --- | --- | --- |
| 0.0–2.0 | Type `./sf-deck --demo`, press Enter, then cut cleanly to the settled Home screen. | sf-deck is a real terminal application with a safe fictional demo. |
| 2.0–6.0 | Hold on Home, then open the populated SOQL History surface. | The landing workspace and query workflow are immediately useful. |
| 6.0–11.0 | Move through Objects and Flows, then drill into a flow version. | Metadata lists lead into detailed working views. |
| 11.0–18.0 | Open Apex and LWC source views. | Code is inspectable without leaving the workspace. |
| 18.0–23.0 | Show Users, System logs, and Deploys. | Operational work sits beside metadata work. |
| 23.0–28.0 | Finish on the populated Dev Projects overview. | Local projects tie discoveries together across resource types. |

### Tape notes

- Only the hero shows a shell command. All focused stories begin with sf-deck
  already open and settled.
- Set capture environment variables with VHS `Env` directives so the visible
  launch line is exactly `./sf-deck --demo`.
- Hide the startup wait after Enter and resume only when
  `LIGHTNING DESTINATIONS` proves Home has rendered.
- Use direct number keys for the main tour; do not drive the hero through the
  command palette.
- Start and remain on `northwind-dev`; the org story owns cross-org switching.

### Acceptance

- The launch command is readable and contains no capture-only environment
  prefix.
- Every destination is populated; no empty chip, loading error, or demo-mode
  network warning is visible.
- A viewer sees Home, SOQL, Objects, Flows, code, users, deploys, and projects.
- The final Dev Projects frame is stable enough to serve as the poster.

---

## Story 2 — Every org, one workspace

**Claim to prove:** each org preserves its own working context, and switching
between them keeps the active safety level visible.

**Eyebrow:** Multi-org by design

**Heading:** Switch orgs. Each keeps its place.

**Body copy:**

> Move from development to UAT to production without rebuilding your workspace.
> Each org remembers its own context, while the active safety level stays
> visible.

**CTA:** Follow the cross-org workflow

**CTA URL:** `/docs/tasks/cross-org-workflow/`

**Terminal title:** `three fictional Northwind orgs`

**Alt text:** `Switching between fictional Northwind development, UAT, and
production orgs, then returning to the preserved Shipment__c schema context.`

**Target:** 13 seconds; poster at 11.5 seconds.

### Timeline

| Time | Visible action | What it proves |
| --- | --- | --- |
| 0.0–2.0 | Hold on `Shipment__c` → Schema in `northwind-dev`; `FULL` is visible. | The starting context and safety level are unambiguous. |
| 2.0–5.0 | Use the org rail to open `northwind-uat`; hold on its previously prepared Flows context with `META` visible. | Each org owns an independent workspace state. |
| 5.0–8.5 | Use the org rail to open `northwind`; hold on Home with `READ` visible. | Production opens at a stricter safety level. |
| 8.5–13.0 | Return to `northwind-dev` and hold on `Shipment__c` → Schema. | Returning to an org restores the context left there. |

### Tape notes

- Prepare dev on `Shipment__c` → Schema and UAT on Flows before the visible
  take; hide this setup navigation from the final output.
- The fixture assigns `FULL` to dev, `META` to UAT, and `READ` to production.
- Use the documented org-rail quick hops only after verifying all three,
  especially `q` for the first org.
- Wait for each org redraw to settle before the next key.

### Acceptance

- All three aliases and all three safety badges are readable.
- Returning to dev restores `Shipment__c` → Schema.
- No write action is attempted on production.

---

## Story 3 — The whole object in one place

**Claim to prove:** an object is more than a field list; sf-deck brings its
details, schema, records, validation, record types, and field-level security into
one navigable workspace.

**Eyebrow:** One object, the whole picture

**Heading:** Stop opening six Setup pages.

**Body copy:**

> Drill into an object once, then move through its fields, records, validation
> rules, record types, triggers, and field-level security. The context stays
> anchored to the object you are investigating.

**CTA:** Explore objects and records

**CTA URL:** `/docs/getting-started/first-launch/`

**Terminal title:** `Shipment__c — schema, records, permissions`

**Alt text:** `Moving through Shipment__c schema, records, validation rules, and
field-level security in the fictional Northwind demo.`

**Target:** 16 seconds; poster at 14.0 seconds.

### Timeline

| Time | Visible action | What it proves |
| --- | --- | --- |
| 0.0–2.0 | Hold on the Shipment object Details subtab. | One object is the stable parent context. |
| 2.0–5.0 | Press `Tab` to Schema and hold on the field table. | Fields are immediately inspectable. |
| 5.0–8.0 | Press `Tab` to Records and hold on seeded Shipment rows. | Data is accessible from the same object. |
| 8.0–11.0 | Jump to the Validation or Record Types subtab and hold. | Object behavior is included, not just field definitions. |
| 11.0–16.0 | Open the FLS subtab, move between two permission parents, then toggle Edit once with `e`. Hold on the changed cell. | Permissions are part of the object workflow and can be edited in place when safety allows. |

### Tape notes

- Record in `northwind-dev`, whose demo safety level permits metadata writes.
- Use direct subtab shortcuts where they are stable; otherwise use `Tab` with a
  full pause between destinations.
- Exactly one FLS edit is enough. More toggles turn the story into visual noise.
- Because demo mode uses a throwaway settings/cache directory, the edit cannot
  affect a real org.

### Acceptance

- FLS appears as the conclusion of an object story, not as a standalone product
  category.
- At least four distinct object facets are readable.
- The single edit visibly changes one cell and no confirmation/error overlay
  obscures the final frame.

---

## Story 4 — Find it before you remember where Salesforce put it

**Claim to prove:** global search finds metadata across sf-deck from one
keyboard shortcut.

**Eyebrow:** Search across the workspace

**Heading:** Find it before you remember where Salesforce put it.

**Body copy:**

> Press `Ctrl+F` to search objects, fields, flows, Apex, users, and other loaded
> metadata from one place. Open the result directly and keep working.

**CTA:** Learn the keyboard workflow

**CTA URL:** `/docs/getting-started/first-launch/`

**Terminal title:** `global search — fictional metadata`

**Alt text:** `Using sf-deck global search to find Shipment-related objects,
fields, flows, and Apex in the fictional Northwind demo.`

**Target:** 12 seconds; poster at 10.0 seconds.

### Timeline

| Time | Visible action | What it proves |
| --- | --- | --- |
| 0.0–2.0 | Hold on Home in `northwind-dev`. | Search begins from an ordinary working screen. |
| 2.0–4.0 | Press `Ctrl+F`; hold briefly on the empty global-search modal. | Search is available globally. |
| 4.0–7.5 | Type `shipment` slowly enough to see the result set narrow. | One query spans multiple metadata kinds. |
| 7.5–9.5 | Move the cursor from `Shipment__c` to `ShipmentTriggerHandler` or `Carrier_Onboarding`. | Results are not limited to objects. |
| 9.5–12.0 | Press `Enter` and hold on the selected item's detail/source screen. | A result is a direct route back into work. |

### Tape notes

- Use metadata mode only. Do not switch to Records mode during this GIF.
- Demo mode intentionally does not execute live SOQL/SOSL. The copy must not say
  that this animation proves a live record search.
- Confirm the ordering of `shipment` results immediately before recording; pick
  a seeded cross-kind result whose row position is deterministic.
- If result order is not deterministic, add a demo-only stable sort before
  recording rather than encoding a fragile count of `j` presses.

### Acceptance

- At least three distinct result kinds are visible together.
- The selected result opens into a populated detail view.
- The wording does not imply that demo mode queried a live Salesforce org.

---

## Story 5 — Turn discoveries into a project

**Claim to prove:** items found across sf-deck can be collected into a
persistent, mixed working set.

**Eyebrow:** From investigation to working set

**Heading:** Turn discoveries into a project.

**Body copy:**

> Collect objects, fields, flows, Apex, records, and saved queries into a dev
> project as you investigate. Review the working set, tag it, and create an SFDX
> bundle when it is ready to move.

**CTA:** Collect work into a project

**CTA URL:** `/docs/tasks/collect-into-project/`

**Terminal title:** `Carrier consolidation — local project`

**Alt text:** `Adding a fictional Northwind flow to the Carrier consolidation
project and opening its mixed working set.`

**Target:** 15 seconds; poster at 13.0 seconds.

### Timeline

| Time | Visible action | What it proves |
| --- | --- | --- |
| 0.0–3.5 | Hold on `Carrier_Onboarding` in the Flows list. | The workflow starts from an item found during normal browsing. |
| 3.5–7.5 | Press `Ctrl+K`, choose `Carrier consolidation`, and confirm. Hold on the success flash. | An item can be collected into a chosen project without leaving its surface. |
| 7.5–11.0 | Press `-` to open Dev Projects and select `Carrier consolidation`. | Projects are first-class local working sets. |
| 11.0–15.0 | Press `Enter` and hold on the project's mixed collected items. | One project can contain several metadata kinds. |

### Tape notes

- Use `Ctrl+K` for the first collection so the chosen project is explicit.
  `K` is the fast toggle only after a project is loaded.
- Use `Carrier_Onboarding`, which is not initially present in `Carrier
  consolidation`; the final project should contain ten items.
- Do not open the Bundles subtab in this story. Demo bundles use a throwaway
  local path that does not belong in marketing media.

### Acceptance

- The project picker and mixed project contents are both visible.
- The story does not suggest that collecting an item deploys it.
- The final project contains mixed item kinds, not only fields from one object.

---

## Static supporting copy

These sections do not need another looping animation.

### Trust strip

- Uses your existing `sf` CLI sessions
- Local cache reduces repeat API calls
- No package or connected app to install in your org
- No application telemetry
- Optional cached release checks; never auto-installs

### Safety

**Eyebrow:** Per-org guardrails

**Heading:** Production stays read-only until you decide otherwise.

**Body:**

> Every org has an explicit safety level. Actions outside that level are hidden
> or blocked, and the active level stays visible while you work.

Keep the four existing level descriptions:

- Read-only — browse and export
- Records — allow record changes
- Metadata — allow metadata writes and deploys
- Full — allow destructive operations and anonymous Apex

### Feature reference

**Eyebrow:** The wider workspace

**Heading:** More of the Salesforce workday, in one terminal.

Keep the existing feature cards, but place them after the five usable product
stories. Retain `beta` labels on Deploys and Tags & dev projects until those
labels are intentionally removed from the product. Remove Compare from the
general feature grid so it appears only in the explicit Coming soon section.

### Coming soon

This is a static section. Do not show a GIF, screenshot, fake comparison result,
or release date.

**Eyebrow:** Coming soon · beta in progress

**Heading:** Compare orgs before you ship.

**Body:**

> Cross-org comparison is being built for reviewing Apex, flows, and selected
> metadata between environments. The goal is a focused changed-component list,
> inspectable diffs, and saved comparisons you can reopen during release review.

**Status line:**

> Compare is available as an unfinished beta and is not part of the initial
> launch promise yet.

**Optional CTA:** Follow development on GitHub

The section should be visually distinct but quiet: a small `COMING SOON` badge,
an abstract diff motif made with HTML/CSS, and no animated product capture. Do
not use “almost here,” “launching soon,” or a date until there is a committed
release milestone.

### Headless and agents

**Eyebrow:** The same guardrails, scriptable

**Heading:** Automate the workflow without inventing another API.

**Body:**

> Core operations expose versioned JSON, predictable exit codes, and the same
> per-org safety checks. Scripts and agents can also control a running sf-deck
> window through its local Unix socket.

### Final CTA

**Heading:** Try the whole workspace without an org.

**Body:**

> Launch the fully offline Northwind demo, explore three fictional orgs, and
> decide whether sf-deck fits your workflow before authenticating anything.

**Command:** `sf-deck --demo`

**Primary CTA:** Install with Homebrew

**Secondary CTA:** Read the first-launch guide

## Production sequence

1. Write one VHS tape per story from this brief.
2. Render GIFs and posters into a temporary, non-public directory.
3. Inspect every frame for real identifiers and visual glitches.
4. Optimize the approved GIFs and record their final dimensions and sizes.
5. Replace the landing assets and implement the copy/order above, including the
   static Coming soon section for Compare.
6. Test desktop, mobile, reduced-motion, keyboard navigation, and broken-image
   behavior locally.
7. Rewrite the unsafe media and LSE string out of Git history.
8. Run a final content/security review before making the repository public.
