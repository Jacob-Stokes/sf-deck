package ui

// Keyboard dispatch. Three modes routed first-come-first-served:
//
//   1. Open-menu overlay visible → handleOpenMenuKey (see menu.go)
//   2. SOQL editor active        → handleSOQLKey
//   3. Search input active        → handleSearchInput
//   4. Default                    → the big matches(key, Keys.X) switch
//
// Every shortcut resolves through the Keys struct (keymap.go), so user
// overrides in ~/.sf-deck/keybindings.toml take effect without any
// changes to this file.

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/exporters/bulk"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleOverlayKey(msg); handled {
		return next, cmd
	}
	// Chord mode: the next key is the second letter of a q-<letter> chord
	// (or q/esc to cancel). Checked before the input guard because chord
	// mode is only ever entered from normal-nav mode, and while it's
	// active the keystroke belongs to the chord, not any input.
	if m.chordActive {
		mm, cmd := m.handleChordKey(msg)
		return mm, cmd
	}
	if next, cmd, handled := m.handleInputModeKey(msg); handled {
		return next, cmd
	}

	key := msg.String()

	// Leader key: q in normal-nav mode enters chord mode. AFTER the input
	// guard above, so a q typed into a search box / editor is a literal
	// character and never reaches here.
	if key == "q" {
		return m.enterChordMode()
	}

	// /compare inventory side-panel preview keys (n/N hunk nav, f fetch a
	// dropped body). Intercepted before the main switch so they win on the
	// Result subtab when the preview is showing.
	if next, cmd, handled := m.handleComparePreviewKey(key); handled {
		return next, cmd
	}

	// Org quick-jump ultra-shortcut. While orgQuickJumpActive is set
	// (the overlay is showing QWERTY letters next to each org row),
	// the next keystroke is either:
	//   - a quick-jump letter → select that org + return focus to
	//     main + dismiss the overlay, all in one gesture; OR
	//   - anything else → dismiss the overlay and fall through to
	//     normal key handling so the user's gesture (j/k, esc,
	//     scroll, etc.) does its usual thing.
	// Either way, the overlay only lives until the next keystroke.
	if m.orgQuickJumpActive {
		m.orgQuickJumpActive = false
		if idx := orgQuickJumpIndexFor(key); idx >= 0 && idx < len(m.orgs) {
			(&m).setSelectedOrg(idx)
			m.syncOrgRailCursorToSelected()
			m.focus = focusMain
			// Quick-jump picked an org and returned to main — collapse
			// the transient rail like Enter-on-org does (unless pinned).
			if !m.leftPinned {
				m.leftOpen = false
			}
			return m, m.onOrgChanged()
		}
		// Fall through — normal dispatch handles this key.
	}

	if next, cmd, handled := m.handlePreGlobalTabKey(key); handled {
		return next, cmd
	}

	// Orgs panel intercept — runs before the global switch when the
	// user is focused on the left rail's Orgs utility. Owns the keys
	// for org grouping (n / R / x / space / [ / ] / < / >) and auth
	// lifecycle (A / D / * / =). Falls through (consumed=false) for
	// j / k / arrow / esc / 0…9 so the global handlers below still
	// drive cursor movement, focus toggling, and tab switches.
	if consumed, cmd := m.onOrgsKey(key); consumed {
		return m, cmd
	}

	// SOQL run cancel: ctrl+c while a SOQL is in flight aborts the
	// HTTP request via the stored context-cancel func and bumps the
	// run generation so any late-arriving result gets dropped.
	// Modal returns to idle immediately; the server may keep
	// running the query for a fraction of a second (REST path) or
	// be cancelled cleanly (Bulk path's job-delete).
	if m.soqlRunning && m.soqlCancel != nil && key == "ctrl+c" {
		m.soqlCancel()
		m.soqlCancel = nil
		m.soqlRunning = false
		m.soqlRunGen++
		m.flash("query cancelled")
		return m, nil
	}

	// Bulk-export cancel: ctrl+c during a running Bulk-API job sends
	// a cancel msg. The dispatcher calls the stored cancel func,
	// which propagates to sf.BulkQuery and aborts the SF job. When
	// no export is running this falls through to the global handlers
	// below (so ctrl+c keeps its normal meaning elsewhere).
	if (&m).Flight() != nil && key == "ctrl+c" {
		return m, func() tea.Msg { return bulk.CancelMsg{} }
	}

	// All global shortcuts are resolved through the configurable keymap
	// (keymap.go); nothing below hardcodes a key string. Keep the checks
	// in intention order: destructive / mode-changing first, then
	// navigation, then per-view extras.

	switch {
	case matches(key, Keys.Quit):
		return m, tea.Quit

	case matches(key, Keys.FocusOrgs):
		// Jump to the Orgs utility in the left rail. If another utility
		// is currently shown, switch to Orgs; if the widget pane is
		// closed, open it. Focus moves to the left rail either way so
		// j/k immediately move the org cursor.
		//
		// orgQuickJumpActive: arms the QWERTY ultra-shortcut overlay so
		// the next keypress can be "q/w/e/…" to select an org directly
		// AND return focus to main in one gesture. Cleared by any
		// nav action below (move, page, scroll) and by the quick-jump
		// dispatch itself.
		m.focus = focusOrgs
		m.leftUtilityIdx = orgsUtilityIdx()
		m.leftOpen = true
		m.orgQuickJumpActive = true
		m.syncOrgRailCursorToSelected()

	case matches(key, Keys.FocusBookmarks):
		// Legacy FocusBookmarks shortcut — Bookmarks/Dev Projects no
		// longer lives in the left rail (it has its own right-rail
		// nav pill now). Repurpose the keypress as the new
		// OpenDevProjects shortcut so users with the legacy binding
		// don't lose the affordance entirely.
		m.setTab(TabDevProjects)
		m.focus = focusMain
		return m, m.onTabChanged()

	case matches(key, Keys.Back):
		// Zen mode exits first under any condition. Clear EVERY zen
		// flag we know about (active list-table state, global
		// fallback, AND any list-table state that might be stuck on
		// a non-active surface) so the user is never trapped after
		// navigating across surfaces while zen was on. Esc is the
		// universal "get me out" key — restore base interaction
		// before any drill-back logic runs.
		exitedZen := false
		if state := (&m).activeListTableState(); state != nil {
			if state.Zen {
				state.Zen = false
				exitedZen = true
			}
		}
		if m.zenMode {
			m.zenMode = false
			exitedZen = true
		}
		if exitedZen {
			// Also defensively clear stale Zen flags on every list
			// state we know about — a tab switch while zen was on
			// could otherwise leave state.Zen=true on a surface the
			// user can't currently see, making the next visit
			// jarring.
			m.clearAllZenFlags()
			return m, nil
		}
		// /compare Result Esc steps back within the results view:
		// body-diff → inventory, inventory → New subtab (the setup form).
		if m.tab() == TabCompare && m.currentSubtab() == SubtabCompareResult {
			if d := m.activeOrgData(); d != nil && d.Run != nil {
				if d.Diff != nil {
					(&m).closeCompareDiff(d)
					return m, nil
				}
				if d.Run.Phase == comparePhaseInventory || d.Run.Phase == comparePhaseRetrieving {
					m.compareSubtabIdx = compareSubtabNewIdx
					return m, nil
				}
			}
		}
		// /reports has its own Esc semantics: when there's a folder
		// path, pop one level. Only when AT root does Esc actually
		// leave the tab (handled below via focus-fallback).
		if m.onReportsBrowser() && len(m.orgs) > 0 {
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			if d.ReportFolders != nil && d.ReportFolders.Path().Depth() > 0 {
				d.ReportFolders.Up()
				return m, nil
			}
		}
		// Dynamic return-tab first: drillByKind stamps the originator
		// onto d.DrillReturnTab so a drill from /home Recent (or
		// future global search) returns to the originator instead of
		// the detail tab's static stem.  Falls through to the static
		// EscBack below when no dynamic return is recorded.
		if d := m.activeOrgData(); d != nil && d.DrillReturnTab != nil {
			if back, ok := d.DrillReturnTab[m.tab()]; ok && back != m.tab() {
				delete(d.DrillReturnTab, m.tab())
				m.setTab(back)
				return m, m.onTabChanged()
			}
		}
		// Registry-first: tabs that declare an EscBack target in their
		// TabSpec route here. Idiosyncratic tabs (TabRecords' picker/
		// list duality, TabRecordDetail's per-context recordDetailReturnTab,
		// TabPermParentDetail's FLS-drill nuance) keep their own logic
		// below. New drill tabs only need EscBack populated.
		if spec := lookupTabSpec(m.tab()); spec != nil && spec.EscBack != 0 {
			m.setTab(spec.EscBack)
			return m, m.onTabChanged()
		}
		if m.tab() == TabFieldDetail {
			m.setTab(TabObjectDetail)
			return m, m.onTabChanged()
		}
		if m.tab() == TabValidationDetail {
			m.setTab(TabObjectDetail)
			return m, m.onTabChanged()
		}
		if m.tab() == TabRecordTypeDetail {
			m.setTab(TabObjectDetail)
			return m, m.onTabChanged()
		}
		if m.tab() == TabTriggerDetail {
			m.setTab(m.triggerDetailBackTab())
			return m, m.onTabChanged()
		}
		if m.tab() == TabObjectDetail {
			m.setTab(TabObjects)
			return m, m.onTabChanged()
		}
		if m.tab() == TabRecordDetail {
			// Drill-stack pop: when the user drilled record-to-record
			// via Enter on a reference field, esc unwinds one level
			// at a time. Once the stack is empty we fall through to
			// the original returnTab.
			if frame, ok := (&m).popRecordDrillStack(); ok {
				d := m.activeOrgData()
				if d != nil {
					d.RecordDetailCur = frame.SObject + ":" + frame.ID
					// Don't re-Ensure — the parent record is already
					// cached from the original drill. Renderer reads
					// from d.RecordDetails[…] directly.
				}
				return m, nil
			}
			back := m.recordDetailReturnTab
			if back == TabRecordDetail {
				// Defensive: don't loop back into the same tab.
				// TabHome (== iota 0) is a valid return target.
				back = TabRecords
			}
			m.setTab(back)
			return m, m.onTabChanged()
		}
		if m.tab() == TabPermParentDetail {
			// If the user drilled from Objects into Fields (per-sobject
			// FLS), Esc pops back to the Objects grid. Otherwise it
			// pops up to the parent /perms list.
			if len(m.orgs) > 0 {
				d := m.ensureOrgData(m.orgs[m.selected].Username)
				if m.currentSubtab() == SubtabParentObjects && d.PermFieldsSObject != "" {
					d.PermFieldsSObject = ""
					return m, nil
				}
			}
			m.setTab(TabPerms)
			return m, m.onTabChanged()
		}
		// /records has two modes (sObject picker → record list). Esc
		// from the record list pops back to the picker; esc from the
		// picker itself is a no-op (top level of the tab).
		if m.tab() == TabRecords && len(m.orgs) > 0 {
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			if d.RecordsSObjectCur != "" {
				d.RecordsSObjectCur = ""
				return m, nil
			}
		}
		// Top-level fallback: nothing to back out of. If a committed
		// search is narrowing the current view, treat Esc as a clear —
		// matches what users expect from VS Code / IntelliJ. Drill-in
		// surfaces handled their own Esc above; this only fires when
		// the user is already at the top of a tab. `C` still clears at
		// any level (see Keys.SearchClear).
		if cleared := m.clearCommittedSearch(); cleared {
			return m, nil
		}
		m.focus = focusMain

	case matches(key, Keys.TogglePane):
		if m.focus == focusOrgs {
			m.focus = focusMain
			// Collapse the rail on the way out unless the user pinned
			// it open with `|`. Matches the auto-collapse on org-pick and
			// tab-switch — a transient rail shouldn't linger once focus
			// returns to the main panel. (This was the "opened it, went
			// to main, it didn't disappear" inconsistency.)
			if !m.leftPinned {
				m.leftOpen = false
			}
		} else {
			m.focus = focusOrgs
			m.leftOpen = true
			m.syncOrgRailCursorToSelected()
		}

	// /compare retrieving screen: the per-type progress list can be ~600
	// rows, so scroll it (failures may be off-screen). Intercept nav keys
	// while a comparison is fetching; otherwise fall through to the
	// normal cursor movement.
	case m.compareRetrieveScrollActive() &&
		(matches(key, Keys.MoveDown) || matches(key, Keys.MoveUp) ||
			matches(key, Keys.PageDown) || matches(key, Keys.PageUp) ||
			matches(key, Keys.JumpDown) || matches(key, Keys.JumpUp)):
		delta := 1
		switch {
		case matches(key, Keys.MoveUp):
			delta = -1
		case matches(key, Keys.PageDown):
			delta = pageJump(m.height)
		case matches(key, Keys.PageUp):
			delta = -pageJump(m.height)
		case matches(key, Keys.JumpDown):
			delta = m.jumpRows()
		case matches(key, Keys.JumpUp):
			delta = -m.jumpRows()
		}
		if d := m.activeOrgData(); d != nil && d.Run != nil {
			d.Run.RetrieveScroll += delta
			if d.Run.RetrieveScroll < 0 {
				d.Run.RetrieveScroll = 0
			}
		}
		return m, nil

	case matches(key, Keys.MoveDown):
		return m.moveCursor(1)

	case matches(key, Keys.MoveUp):
		return m.moveCursor(-1)

	case matches(key, Keys.JumpDown):
		return m.moveCursor(m.jumpRows())

	case matches(key, Keys.JumpUp):
		return m.moveCursor(-m.jumpRows())

	case matches(key, Keys.PageDown):
		// In paginated mode the renderer auto-tracks the cursor's
		// page; PageDown still does its standard "jump cursor by
		// pane height" — and the cursor crossing the boundary
		// flips the page. Same key, no special handling.
		return m.moveCursor(pageJump(m.height))

	case matches(key, Keys.PageUp):
		return m.moveCursor(-pageJump(m.height))

	case matches(key, Keys.GoBottom):
		return m.moveCursor(1 << 20)

	case matches(key, Keys.GoTop):
		return m.moveCursor(-(1 << 20))

	case matches(key, Keys.Drill):
		return m.activate()

	case matches(key, Keys.Refresh) && !m.objPermReadContext():
		return m.refreshCurrent()

	case matches(key, Keys.GlobalRefresh):
		// Global refresh: re-fetch every loaded resource for the active
		// org (vs `r` which refreshes only the current view). ctrl+r is
		// otherwise only bound inside the global-search modal, which is
		// handled by the modal key router before reaching here.
		return (&m).refreshAllLoaded()

	case matches(key, Keys.ReportExport) && (m.onReportsBrowser() || m.tab() == TabReportDetail):
		return m.triggerReportExport()

	case matches(key, Keys.NewProject) && projectsContext(m):
		return m, m.triggerNewProject()

	case matches(key, Keys.EditProject) && projectsContext(m):
		return m, m.triggerEditProject()

	case matches(key, Keys.DeleteProject) && projectsContext(m):
		return m, m.triggerDeleteProject(false)
	case matches(key, Keys.DeleteProjectForce) && projectsContext(m):
		return m, m.triggerDeleteProject(true)
	case matches(key, Keys.DeleteProject) && m.tab() == TabDevProjectDetail:
		return m, m.triggerDeleteProjectItem()

	case matches(key, Keys.ExportProject) &&
		(projectsContext(m) || m.tab() == TabDevProjectDetail):
		return m, m.triggerExportProject()

	case matches(key, Keys.OpenBundles) && m.tab() == TabDevProjectDetail:
		// Jump straight to the Bundles subtab on the per-project
		// detail view — saves navigating with [/] when you already
		// know that's where you want to be.
		m.setDevProjectDetailSubtab(1)
		return m, nil
	case matches(key, Keys.OpenBundles) && m.tab() == TabDevProjects:
		// Top-level /dev-projects: jump to the all-bundles subtab.
		m.setDevProjectsSubtab(1)
		return m, nil

	case matches(key, Keys.CollectItem):
		// ctrl+k: quick-collect to the loaded project (toggle), picker
		// fallback when nothing is loaded.
		return m, m.triggerCollect(false)

	case matches(key, Keys.CollectItemPick):
		// K: always open the picker to choose a project.
		return m, m.triggerCollect(true)

	case matches(key, Keys.OpenDevProjects):
		// `-` opens the master dev-projects list. Right-rail nav pill
		// always offers this affordance; pressing the key is the
		// keyboard equivalent.
		m.setTab(TabDevProjects)
		m.focus = focusMain
		return m, m.onTabChanged()

	case matches(key, Keys.OpenTags):
		// `#` opens the tag manager. Always available since the right-
		// rail Tags pill is always present.
		m.setTab(TabTags)
		m.focus = focusMain
		return m, m.onTabChanged()

	// Tag manager actions — only on TabTags.
	case matches(key, Keys.NewProject) && m.tab() == TabTags:
		return m, m.triggerTagNew()
	// Edit the cursored tag (name / colour / icon). Bound to `e`, NOT
	// Enter: Enter is the global Drill (line above), which drills into
	// the tag's items — so this case was unreachable on Enter and the
	// tag editor (the only way to change a tag's colour) could never be
	// opened. `e` matches the edit convention on every other list.
	case matches(key, Keys.EditProject) && m.tab() == TabTags:
		return m, m.triggerTagEdit()
	case matches(key, Keys.DeleteProject) && m.tab() == TabTags:
		return m, m.triggerTagDelete()

	case matches(key, Keys.LoadOrgProject) && m.tab() == TabDevProjects:
		// On the master list, `_` toggles load/unload for the cursored
		// project — the only place where it makes sense to bind a
		// project as the active org's loaded slot.
		return m.toggleLoadDevProject()
	case matches(key, Keys.LoadOrgProject):
		// Anywhere else: jump to the loaded project's detail. The
		// right-rail "_ <project-name>" pill is the visible affordance
		// for this; pressing the key is the keyboard equivalent. No-op
		// (with a flash) when nothing's loaded — the pill wouldn't be
		// visible to suggest the binding in that state.
		if scope := m.activeScope(); scope.Loaded() {
			d := m.activeOrgData()
			if d != nil && d.LoadedDevProjectID != "" {
				m.setActiveDevProject(d.LoadedDevProjectID)
				m.devProjectShowAllOrgs = false
				m.reloadDevProjectItems()
				m.setTab(TabDevProjectDetail)
				m.focus = focusMain
				return m, m.onTabChanged()
			}
		}
		m.flash("no project loaded — _ on /dev-projects to load")
		return m, nil

	case matches(key, Keys.ToggleProjectMode) && m.onReportsBrowser():
		return m.toggleReportsProjectMode()

	case matches(key, Keys.Tab1):
		return m.switchToViewIndex(0)
	case matches(key, Keys.Tab2):
		return m.switchToViewIndex(1)
	case matches(key, Keys.Tab3):
		return m.switchToViewIndex(2)
	case matches(key, Keys.Tab4):
		return m.switchToViewIndex(3)
	case matches(key, Keys.Tab5):
		return m.switchToViewIndex(4)
	case matches(key, Keys.Tab6):
		return m.switchToViewIndex(5)
	case matches(key, Keys.Tab7):
		return m.switchToViewIndex(6)
	case matches(key, Keys.Tab8):
		return m.switchToViewIndex(7)
	case matches(key, Keys.Tab9):
		// Slot 9 holds the most-recently-activated overflow tab.
		// No-op when nothing's been activated this session.
		if !m.overflowSet {
			m.flash("no overflow tab — press " + firstPretty(Keys.Tab0) + " to pick one")
			return m, nil
		}
		m.setTab(m.resolveStem(m.overflowTab))
		m.focus = focusMain
		return m, m.onTabChanged()
	case matches(key, Keys.Tab0):
		// Slot 0 is reserved for the "More…" overflow modal. Every
		// tab not in TabsForNumbers is reachable from there.
		return m, m.openTabOverflowModal()

	case matches(key, Keys.ColShrink):
		if mm, cmd, handled := m.handleColResize(-1); handled {
			return mm, cmd
		}
	case matches(key, Keys.ColGrow):
		if mm, cmd, handled := m.handleColResize(+1); handled {
			return mm, cmd
		}
	case matches(key, Keys.ColSnapMin):
		if mm, cmd, handled := m.handleColSnap(-1); handled {
			return mm, cmd
		}
	case matches(key, Keys.ColSnapMax):
		if mm, cmd, handled := m.handleColSnap(+1); handled {
			return mm, cmd
		}
	case matches(key, Keys.ColResetWidths):
		if mm, cmd, handled := m.handleColResetWidths(); handled {
			return mm, cmd
		}

	// Shift+1..9: jump to subtab N-1 of the current tab. No-op when
	// the current tab has ≤1 subtab — falls through (the typed shifted
	// character won't match any later case for a list-shaped tab).
	case matches(key, Keys.Subtab1):
		return m.switchToSubtabIndex(0)
	case matches(key, Keys.Subtab2):
		return m.switchToSubtabIndex(1)
	case matches(key, Keys.Subtab3):
		return m.switchToSubtabIndex(2)
	case matches(key, Keys.Subtab4):
		return m.switchToSubtabIndex(3)
	case matches(key, Keys.Subtab5):
		return m.switchToSubtabIndex(4)
	case matches(key, Keys.Subtab6):
		return m.switchToSubtabIndex(5)
	case matches(key, Keys.Subtab7):
		return m.switchToSubtabIndex(6)
	case matches(key, Keys.Subtab8):
		return m.switchToSubtabIndex(7)
	case matches(key, Keys.Subtab9):
		return m.switchToSubtabIndex(8)
	case matches(key, Keys.Subtab0):
		// shift+0 opens the subtab overflow modal when the active
		// tab has subtabs that don't fit on the strip. No-op
		// otherwise — keeps the keypress harmless on tabs without
		// overflow.
		if m.hasSubtabOverflow() {
			return m, m.openSubtabOverflowModal()
		}
		return m, nil

	case matches(key, Keys.FLSToggleRead) && m.inFLSGridContext():
		return m.flsToggleCell("read")

	case matches(key, Keys.FLSToggleEdit) && m.inFLSGridContext():
		return m.flsToggleCell("edit")

	case matches(key, Keys.ObjPermRead) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("read")

	case matches(key, Keys.ObjPermCreate) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("create")

	case matches(key, Keys.ObjPermEdit) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("edit")

	case matches(key, Keys.ObjPermDelete) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("delete")

	case matches(key, Keys.ObjPermViewAll) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("viewall")

	case matches(key, Keys.ObjPermModifyAll) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentObjects && !m.inFLSGridContext():
		return m.objPermToggleCell("modifyall")

	case matches(key, Keys.SysPermToggle) && m.tab() == TabPermParentDetail && m.currentSubtab() == SubtabParentSystem:
		return m.sysPermToggleCell()

	case matches(key, Keys.RecordEditField) && m.tab() == TabRecordDetail:
		// Has to match BEFORE EditCurrentView / SOQLEdit since `e` is
		// the default for all three — earlier cases that no-op on
		// TabRecordDetail would otherwise eat the keystroke and the
		// inline field editor would never fire.
		return m.handleRecordEditEnter()

	// /compare: `R` on an opened SAVED comparison's inventory re-runs it
	// (fresh fetch) — the staleness banner's action. Live runs (no
	// OpenedSavedAt) ignore it.
	case key == "R" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareResult:
		if d := m.activeOrgData(); d != nil && d.Run != nil &&
			d.Run.Phase == comparePhaseInventory && d.Diff == nil && !d.Run.OpenedSavedAt.IsZero() {
			d.Run.OpenedSavedAt = time.Time{} // clear the banner; it's live now
			return m, (&m).startCompare(d)
		}
		return m, nil

	// /compare: `e` on a loaded inventory (e.g. an opened saved
	// comparison) drops to the prefilled setup form to edit + re-run.
	// Placed before EditCurrentView (also `e`, When=list-table).
	case key == "e" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareResult:
		if d := m.activeOrgData(); d != nil && d.Run != nil &&
			d.Run.Phase == comparePhaseInventory && d.Diff == nil {
			return m, (&m).editCurrentCompareInSetup(d)
		}
		return m, nil

	// /compare Saved-subtab row actions — placed BEFORE EditCurrentView
	// (also `e`, When=list-table) so they win on the compare Saved
	// subtab, which is itself a list-table. ↵ open · e rerun/edit · d
	// delete · t →template · rename (existing key).
	case key == "e" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareSaved:
		if d := m.activeOrgData(); d != nil {
			return m, (&m).rerunEditSelected(d)
		}
		return m, nil
	case key == "d" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareSaved:
		if d := m.activeOrgData(); d != nil {
			return m, (&m).deleteSelectedSaved(d)
		}
		return m, nil
	case key == "t" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareSaved:
		if d := m.activeOrgData(); d != nil {
			return m, (&m).saveSelectedAsTemplate(d)
		}
		return m, nil
	case matches(key, Keys.SOQLRename) && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareSaved:
		if d := m.activeOrgData(); d != nil {
			return m, (&m).renameSelectedSaved(d)
		}
		return m, nil

	case matches(key, Keys.EditCurrentView):
		// `e` edits the underlying view definition — but only when
		// the user has explicitly entered column-mode (`c`). Column-
		// mode signals "I'm working on the table's structure", and
		// editing the SOQL fits naturally there. Outside column-mode
		// `e` falls through to its other surface-scoped meanings
		// (SOQLEdit on /soql, ReportExport on /reports).
		// e on a list-table edits the active chip's view definition.
		// Falls through to SOQLEdit / ReportExport when no list-
		// table is active OR no chip is editable on this surface.
		if state := (&m).activeListTableState(); state != nil {
			if cmd, handled := m.editActiveChip(); handled {
				return m, cmd
			}
		}
		fallthrough

	case matches(key, Keys.SOQLEdit):
		if m.tab() == TabSOQL {
			m.soqlEditing = true
			m.soqlInput.Focus()
			// Populate suggestions immediately so the popup isn't
			// empty on first paint after entering edit mode.
			m.autocompleteRefresh(&m.soqlSession)
		}

	case matches(key, Keys.SOQLToggleTooling) && m.tab() == TabSOQL:
		m.soqlTooling = !m.soqlTooling
		if m.soqlTooling && m.soqlBulk {
			// Bulk API doesn't support Tooling queries — flip the
			// other flag off so the next run resolves cleanly.
			m.soqlBulk = false
			m.flash("tooling api: on (bulk off — bulk doesn't support tooling)")
		} else {
			m.flash("tooling api: " + onOff(m.soqlTooling))
		}

	case matches(key, Keys.SOQLToggleBulk) && m.tab() == TabSOQL:
		m.soqlBulk = !m.soqlBulk
		if m.soqlBulk && m.soqlTooling {
			m.soqlTooling = false
			m.flash("bulk api: on (tooling off — bulk doesn't support tooling)")
		} else {
			m.flash("bulk api: " + onOff(m.soqlBulk))
		}

	case matches(key, Keys.RecordEditSave) && m.tab() == TabRecordDetail:
		return m, m.triggerRecordEditSave()

	case matches(key, Keys.RecordEditCancelAll) && m.tab() == TabRecordDetail:
		mm := m
		(&mm).discardAllEdits()
		mm.flash("discarded all edits")
		return mm, nil

	case matches(key, Keys.ExecEdit) && m.tab() == TabExec && m.currentSubtab() == SubtabExecEditor:
		m.execEditing = true
		m.execInput.Focus()

	case matches(key, Keys.ExecToggleLog) && m.tab() == TabExec:
		m.execCaptureLog = !m.execCaptureLog
		m.flash("debug log capture: " + onOff(m.execCaptureLog))

	case matches(key, Keys.ExecExternalEditor) && m.tab() == TabExec:
		return m.handleExecExternalEditor()

	case matches(key, Keys.ExecSave) && m.tab() == TabExec:
		return m.handleExecSave()

	case matches(key, Keys.ExecDelete) && m.tab() == TabExec && m.currentSubtab() == SubtabExecSaved:
		return m.handleExecLibraryDelete()

	case matches(key, Keys.ExecDuplicate) && m.tab() == TabExec && m.currentSubtab() == SubtabExecSaved:
		return m.handleExecDuplicate()

	case matches(key, Keys.ExecRename) && m.tab() == TabExec && m.currentSubtab() == SubtabExecSaved:
		return m.handleExecRename()

	// --- /compare contextual keys (placed before the generic subtab-
	// cycle cases so they intercept [ / ] while the body diff is open) ---
	case key == "u" && m.tab() == TabCompare && m.compareDiffOpen():
		if d := m.activeOrgData(); d != nil && d.Diff != nil {
			d.Diff.Unified = !d.Diff.Unified
		}
		return m, nil
	case matches(key, Keys.NextSubtab) && m.tab() == TabCompare && m.compareDiffOpen():
		if d := m.activeOrgData(); d != nil {
			return m, (&m).stepCompareComponent(d, +1)
		}
		return m, nil
	case matches(key, Keys.PrevSubtab) && m.tab() == TabCompare && m.compareDiffOpen():
		if d := m.activeOrgData(); d != nil {
			return m, (&m).stepCompareComponent(d, -1)
		}
		return m, nil
	case key == "ctrl+s" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareResult:
		// On the Result subtab ctrl+s saves the whole comparison WITH its
		// data (the result is shown there).
		if d := m.activeOrgData(); d != nil && d.Run != nil &&
			d.Run.Phase == comparePhaseInventory && d.Diff == nil {
			return m, (&m).saveCurrentComparison(d)
		}
		return m, nil
	case key == "ctrl+s" && m.tab() == TabCompare && m.currentSubtab() == SubtabCompareNew:
		// On the setup form ctrl+s saves a data-less template (recipe).
		if d := m.activeOrgData(); d != nil {
			return m, (&m).saveCurrentCompareAsTemplate(d)
		}
		return m, nil

	case matches(key, Keys.SOQLSave) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLEditor:
		// Save the current editor body as a SavedQuery. Only on the
		// Editor subtab — on Saved/History the same key would
		// silently save the EDITOR's body while the user thinks
		// they're acting on the cursored row.
		// Uses the
		// query body itself as the default name (collapsed) — the
		// user can rename via the Library later. Future: open a
		// modal to capture name + description before saving.
		return m.handleSOQLSave()

	case matches(key, Keys.SOQLDelete) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLSaved:
		// Delete the cursored Saved query.
		return m.handleSOQLLibraryDelete()

	case matches(key, Keys.SOQLDuplicate) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLSaved:
		// Duplicate the cursored Saved query as "Copy of …".
		return m.handleSOQLDuplicate()

	case matches(key, Keys.SOQLRename) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLSaved:
		// R — rename / edit description of the cursored Saved query.
		return m.handleSOQLRename()

	case matches(key, Keys.FlowRename) && m.tab() == TabFlowDetail:
		// e — rename the flow's display label (metadata write).
		return m.handleFlowRename()

	case matches(key, Keys.FlowVersionDelete) && m.tab() == TabFlowDetail:
		// D — delete the cursored inactive flow version (metadata write).
		return m.handleFlowVersionDelete()

	case matches(key, Keys.SOQLSaveAs) && m.tab() == TabSOQL:
		// shift+S — always save as new, even if editor holds a
		// loaded saved query. The duplicate-then-modify flow.
		return m.handleSOQLSaveAs()

	case matches(key, Keys.SOQLExport) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLEditor:
		// x — export current results. Only valid on the Editor
		// subtab where soqlResult holds the rows.
		return m.triggerSOQLExport()

	case matches(key, Keys.RecordsExport) && m.recordsExportSurfaceActive():
		// x on a records-shaped surface (TabRecords list mode,
		// TabObjectDetail records subtab, TabUsers · All users)
		// — opens the format/path picker for the active list.
		return m.triggerRecordsExport()

	case matches(key, Keys.SOQLYankCell) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLEditor:
		// y — copy cursored cell. Overrides the global YankDefault
		// because the SOQL Editor results don't have URLs to yank.
		return m.handleSOQLYankCell()

	case matches(key, Keys.SOQLYankRow) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLEditor:
		// Y — copy cursored row as TSV.
		return m.handleSOQLYankRow()

	case matches(key, Keys.SOQLYankColumn) && m.tab() == TabSOQL && m.currentSubtab() == SubtabSOQLEditor:
		// ctrl+y — copy column as ('id1',…) IN-clause-ready string.
		return m.handleSOQLYankColumn()

	case matches(key, Keys.FilterCycle):
		// Legacy: f still cycles views on the Objects tab (same as
		// right-arrow now). Kept for muscle memory of alpha users.
		return m.cycleChip(+1)

	case matches(key, Keys.NextView):
		// Dev-project detail's Items subtab has its own kind-filter
		// chip strip — intercept before falling through to the
		// generic chip-system cycler.
		if m.tab() == TabDevProjectDetail && m.currentSubtab() == SubtabDevProjectItems {
			return m.cycleDevProjectKindChip(+1)
		}
		if m.tab() == TabTagDetail {
			return m.cycleTagKindChip(+1)
		}
		// Bundle detail has a custom view-mode switcher
		// (components ↔ files); the chip cycle keys also drive
		// it because /bundle has no real chip strip.
		if m.tab() == TabBundleDetail {
			return m.cycleBundleDetailView(+1)
		}
		return m.cycleChip(+1)
	case matches(key, Keys.PrevView):
		if m.tab() == TabDevProjectDetail && m.currentSubtab() == SubtabDevProjectItems {
			return m.cycleDevProjectKindChip(-1)
		}
		if m.tab() == TabTagDetail {
			return m.cycleTagKindChip(-1)
		}
		if m.tab() == TabBundleDetail {
			return m.cycleBundleDetailView(-1)
		}
		return m.cycleChip(-1)

	case matches(key, Keys.NextSubtab):
		// SidebarFocusable tabs have no subtab strip; Tab swaps focus
		// between the main pane and the sidebar action menu so j/k/
		// Enter route to whichever pane is active.
		if spec := lookupTabSpec(m.tab()); spec != nil && spec.SidebarFocusable {
			m.bodyFocus = !m.bodyFocus
			return m, nil
		}
		return m.cycleSubtab(+1)
	case matches(key, Keys.PrevSubtab):
		if spec := lookupTabSpec(m.tab()); spec != nil && spec.SidebarFocusable {
			m.bodyFocus = !m.bodyFocus
			return m, nil
		}
		return m.cycleSubtab(-1)

	case matches(key, Keys.ToggleProjectScope) && m.tab() == TabDevProjectDetail:
		// Toggle "this org / all orgs" view of the project's items.
		// Was previously bound to Tab/Shift+Tab but those collide with
		// the new Items/Bundles subtab cycling.
		m.devProjectShowAllOrgs = !m.devProjectShowAllOrgs
		// Kind-chip filter reset — the visible item set just shifted
		// (different orgs contribute different kinds), so the cursor
		// could be pointing at a now-empty chip.
		m.devProjectKindChip = ""
		m.devProjectKindChipCursor = 0
		m.reloadDevProjectItems()
		return m, nil

	case matches(key, Keys.ToggleDashboard):
		m.dashboardCollapsed = !m.dashboardCollapsed

	case matches(key, Keys.ToggleQueryLine):
		m.queryLineHidden = !m.queryLineHidden

	case matches(key, Keys.CommandPalette):
		// Global fuzzy-find. Walks tabSpecs() + paletteCommands to
		// build the entry list — new tabs/commands appear here for
		// free without per-feature wiring.
		mm := m
		cmd := mm.openCommandPalette()
		return mm, cmd

	case matches(key, Keys.OpenSettings):
		return m, m.openSettingsModal()

	case matches(key, Keys.OpenAPILog):
		m.openAPILogModal()
		return m, nil

	case matches(key, Keys.OpenDownloads):
		m.openDownloadsModal()
		return m, nil

	case matches(key, Keys.LensModeToggle): // historical name; toggles ChipMode
		// Toggle sf-deck lenses ↔ Salesforce list-views on the active
		// records-shaped surface. No-op on other tabs.
		return m.toggleChipMode()

	case matches(key, Keys.OpenLensManager):
		// V opens the unified chip manager. Resolves the domain +
		// title via the active chipSurface (registry-driven) so new
		// chip-bearing tabs don't need a hardcoded case here.
		// Records is the only bespoke fallback — its scope is
		// per-sObject and doesn't fit the standard surface contract.
		if surf := m.resolveChipSurface(); surf != nil {
			return m, m.openChipManagerFor(
				surf.Domain,
				surfaceManagerScope(*surf, m),
				surfaceManagerTitle(*surf, m),
				surf.ImportFromSF,
			)
		}
		_, sobj := m.activeRecordsSObject()
		if sobj == "" {
			m.flash("view manager only available on chip-bearing surfaces")
			return m, nil
		}
		return m, m.openChipManagerFor(domainRecords, sobj, "Views · "+sobj, true)

	case matches(key, Keys.OpenChipOverflow):
		// Schema field-filter chips use a lightweight overflow chooser
		// (no transient/favourite/persistence machinery) — handled here
		// before the full chipSurface path.
		if m.tab() == TabObjectDetail && m.currentSubtab() == SubtabSchema {
			return m, m.openSchemaChipOverflow()
		}
		// M opens the "+ N more…" chip picker — every non-favourite
		// chip applicable to the active surface. Picks land on the
		// strip as a transient (distinct style); F pins to favourites.
		// Same registry-driven resolution as V; same Records fallback.
		if surf := m.resolveChipSurface(); surf != nil {
			return m, m.openChipOverflowFor(surf.Domain, surfaceManagerScope(*surf, m))
		}
		_, sobj := m.activeRecordsSObject()
		if sobj == "" {
			m.flash("view picker only available on chip-bearing surfaces")
			return m, nil
		}
		return m, m.openChipOverflowFor(domainRecords, sobj)

	case matches(key, Keys.ToggleChipFavourite):
		// F has two behaviours depending on the active surface:
		//   /reports — pin/unpin the CURRENT folder (the leaf of the
		//     breadcrumb path). Persists via the registry.
		//   chip-shaped tabs — flip the favourite flag on the active
		//     chip (promote transient → favourite or unpin a non-
		//     locked favourite).
		if m.onReportsBrowser() && len(m.orgs) > 0 {
			d := m.ensureOrgData(m.orgs[m.selected].Username)
			if d.ReportFolders != nil {
				path := d.ReportFolders.Path()
				if path.Depth() == 0 {
					m.flash("can't pin the root — drill into a folder first")
					return m, nil
				}
				cur := path.Nodes[len(path.Nodes)-1]
				pinned := d.ReportFolders.TogglePin(cur)
				if pinned {
					m.flash("★ pinned " + cur.Label)
				} else {
					m.flash("☆ unpinned " + cur.Label)
				}
				return m, nil
			}
		}
		return m, m.toggleActiveChipFavourite()

	case matches(key, Keys.SearchStart):
		if m.focus == focusMain {
			if s := m.currentSearch(); s != nil {
				// Anchor the pre-search cursor BEFORE flipping flags —
				// only on the not-applied → active transition, so re-
				// pressing / while a filter is up doesn't trample the
				// original anchor with the filtered cursor.
				if !s.Applied() {
					m.captureRecordsCursorAnchor()
				}
				s.EnsureInit()
				s.Input.Focus()
				s.Active = true
				s.Committed = false
			}
		}

	case matches(key, Keys.GlobalSearch):
		return m, m.openGlobalSearch()

	case matches(key, Keys.SearchClear):
		m.clearCommittedSearch()

	case matches(key, Keys.ToggleSidebar):
		m.sidebarOpen = !m.sidebarOpen

	case matches(key, Keys.ToggleSidebarStacked):
		// Stacked mode only makes sense when the sidebar is OPEN —
		// flipping the flag while the sidebar is hidden does nothing
		// visible. Auto-open the sidebar on the first toggle so the
		// gesture always has visible effect.
		if !m.sidebarOpen {
			m.sidebarOpen = true
		}
		m.sidebarStacked = !m.sidebarStacked

	case matches(key, Keys.Help):
		// Combined help: Keybindings page (searchable, editable) +
		// the per-view About page, tab-switched inside the modal.
		// Replaces the old info-only modal — narrow terminals drop
		// footer hints, so ? must surface the full key list.
		return m, (&m).openKeybindingsModal()

	case matches(key, Keys.InspectPanel):
		// Full sidebar/context info in a modal — the escape hatch
		// behind the "⚠ truncated" warning when the panel is too short.
		if modal, ok := m.inspectModalForCurrentView(); ok {
			m.showInfoModal(modal)
		}

	case matches(key, Keys.ToggleLeft):
		m.leftOpen = !m.leftOpen
		// `|` is the explicit pin/unpin toggle. When opening, mark
		// pinned so transient close-on-action paths (org-pick,
		// tab-switch) leave the rail alone. When closing, clear
		// the pin so the next transient open isn't sticky.
		m.leftPinned = m.leftOpen
		if !m.leftOpen && m.focus == focusOrgs {
			// Closing the widget pane while focused on it kicks focus
			// back to main so j/k doesn't keep eating keystrokes
			// invisibly. The icon strip is still visible either way.
			m.focus = focusMain
		}

	case matches(key, Keys.OpenDefault):
		return m.openDefault()

	case matches(key, Keys.OpenMenu):
		return m.requestOpenMenu(menuOpen)

	case matches(key, Keys.YankDefault) && m.tab() == TabFlowVersionDetail:
		// On the flow-version viewer y yanks the definition JSON, not
		// the record URL (that's what o / the open menu are for).
		return m, m.yankFlowVersionDefinition()

	case matches(key, Keys.YankDefault) && m.tab() == TabFieldDetail:
		// On a picklist VALUE row, plain y yanks that single value.
		// Elsewhere on the field detail, fall through to the normal
		// URL yank (ctrl+y still offers the whole value sub-menu).
		if v, ok := m.cursoredFieldDetailYankValue(); ok {
			m.flash("copied: " + ansiTrunc(v, 40))
			return m, yankValueCmd(v)
		}
		return m.yankDefault()

	case matches(key, Keys.YankDefault):
		return m.yankDefault()

	case matches(key, Keys.YankMenu):
		return m.requestOpenMenu(menuYank)

	case matches(key, Keys.ColScrollL):
		// /dev-project-detail tree uses ←/h to collapse the cursored
		// row. No list-table on that surface, so ColScroll falls
		// through to the tree-collapse handler instead of no-op.
		if m.tab() == TabDevProjectDetail {
			if mm, handled := m.collapseCursoredDevProjectRow(); handled {
				return mm, nil
			}
		}
		if mm, handled := m.handleColScroll(-1); handled {
			return mm, nil
		}
	case matches(key, Keys.ColScrollR):
		if m.tab() == TabDevProjectDetail {
			if mm, handled := m.expandCursoredDevProjectRow(); handled {
				return mm, nil
			}
		}
		if mm, handled := m.handleColScroll(+1); handled {
			return mm, nil
		}
	case matches(key, Keys.ColSort):
		if mm, handled := m.handleColSort(); handled {
			return mm, nil
		}
	case matches(key, Keys.ColSortClear):
		if mm, handled := m.handleColSortClear(); handled {
			return mm, nil
		}
	case matches(key, Keys.ZenMode):
		if mm, handled := m.handleZenToggle(); handled {
			return mm, nil
		}
	case matches(key, Keys.Paginate):
		if mm, handled := m.handlePaginateToggle(); handled {
			return mm, nil
		}
	case matches(key, Keys.Tag):
		if cmd := m.openTagPickerForCursored(); cmd != nil {
			return m, cmd
		}
		// Modal is non-cmd-returning; the open call mutates state.
		return m, nil
	case matches(key, Keys.TagAll):
		// Bulk: tag every row of the current filtered view. Only on
		// surfaces that expose per-row identities; the rest flash a
		// hint instead of silently doing nothing.
		return m.openBulkTagPickerForVisible()
	case matches(key, Keys.TagColumn):
		if m.settings == nil {
			return m, nil
		}
		// 3-state cycle: compact (dots) → expanded (pills) → hidden →
		// back to compact. Flash messages so the user sees what they
		// just landed on.
		var next, msg string
		switch m.settings.TagColumnDisplayMode() {
		case settings.TagColumnModeCompact:
			next = settings.TagColumnModeExpanded
			msg = "tag column: expanded pills"
		case settings.TagColumnModeExpanded:
			next = settings.TagColumnModeHidden
			msg = "tag column: hidden"
		default: // hidden or unknown
			next = settings.TagColumnModeCompact
			msg = "tag column: compact dots"
		}
		m.settings.SetTagColumnMode(next)
		m.saveSettings(msg)
		return m, nil
	case matches(key, Keys.ProjectColumn):
		if m.settings == nil {
			return m, nil
		}
		nowVisible := m.settings.ProjectColumnVisible()
		m.settings.SetProjectColumnHidden(nowVisible)
		if m.settings.ProjectColumnVisible() {
			m.saveSettings("project column shown")
		} else {
			m.saveSettings("project column hidden")
		}
		return m, nil
	case matches(key, Keys.FlagColumn):
		if m.settings == nil {
			return m, nil
		}
		// 3-state cycle: letter → full → hidden → letter.
		var next, msg string
		switch m.settings.FlagColumnDisplayMode() {
		case settings.FlagColumnModeLetter:
			next = settings.FlagColumnModeFull
			msg = "flags column: full labels"
		case settings.FlagColumnModeFull:
			next = settings.FlagColumnModeHidden
			msg = "flags column: hidden"
		default: // hidden or unknown
			next = settings.FlagColumnModeLetter
			msg = "flags column: letters"
		}
		m.settings.SetFlagColumnMode(next)
		m.saveSettings(msg)
		return m, nil
	}
	return m, nil
}
