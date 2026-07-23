package ui

// Bubble Tea Update loop + resource message routing.
//
// Update() dispatches incoming messages to the right handler; the
// bulk of per-kind logic lives in sibling files:
//
//   update_keys.go   — handleKey, handleSearchInput, handleSOQLKey
//   update_nav.go    — moveCursor, activate, refreshCurrent,
//                       currentSearch, resetCursorForCurrentView,
//                       switchToViewIndex
//   update_open.go   — openDefault, yankDefault, cursorOpenable
//   util.go          — flash, itoa, onOff, trimLastWord

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/applog"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/qchip"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/resource"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/treechip/sources"
)

func (m Model) targetForUsername(username string) string {
	target := username
	for _, o := range m.orgs {
		if o.Username == username && o.Alias != "" {
			target = o.Alias
			break
		}
	}
	return target
}

// ensureOrgData returns the per-org state for `username`, creating it
// lazily. The current alias is used to wire Fetch closures; if the
// selected org doesn't have an alias, its username is used as the sf
// target. If the alias changes mid-session, rebuild the state so the
// resource closures stop talking to the stale target.
func (m *Model) ensureOrgData(username string) *orgData {
	target := m.targetForUsername(username)
	d, ok := m.data[username]
	if !ok || d.target != target {
		d = newOrgData(username, target, m.cache, m.settings)
		m.data[username] = d
	}
	// Lazy-load the persisted recent-visits list once per session.
	// Hitting settings on every ensureOrgData would be wasteful;
	// this guard keeps it to one read per org per process lifetime.
	if !d.RecentLoaded {
		loadRecent(m, d, username)
		d.RecentList.Set(d.Recent)
		// Same trigger: hydrate the loaded org-project Scope from
		// settings + the dev-project store. If the persisted id is
		// stale (project deleted), the helper clears settings and
		// leaves Scope nil so surfaces gracefully render no chip.
		m.hydrateLoadedProjectFromSettings(d, username)
		d.RecentLoaded = true
	}
	return d
}

// onOrgChanged / onTabChanged both trigger a data-ensure for whatever
// the active view needs. They also rewire the active chip's predicate
// onto the current org's list so cached-list filtering reflects the
// chip selection on first paint (the chip-cycle handler already does
// this on subsequent cycles).
func (m *Model) onOrgChanged() tea.Cmd {
	// Gate the chip registries to the new active org BEFORE any
	// render runs so the strip never flashes the previous org's
	// chips. Empty when no orgs are connected — strict fallback
	// in the Registry returns only built-ins.
	if len(m.orgs) > 0 {
		m.setActiveOrgOnChipRegistries(m.orgs[m.selected].Username)
	} else {
		m.setActiveOrgOnChipRegistries("")
	}
	// Warm notifications for the new org so the header bell shows an
	// unread count without the user having to visit /home first. Cached
	// (TTL-gated), so this only hits the network when stale.
	var notifCmd tea.Cmd
	if len(m.orgs) > 0 {
		o := m.orgs[m.selected]
		if canUseOrg(o) {
			d := m.ensureOrgData(o.Username)
			notifCmd = d.Notifications.Ensure(m.cache)
		}
	}
	return tea.Batch(m.tabRefreshCmd(), notifCmd)
}
func (m *Model) onTabChanged() tea.Cmd {
	m.recordDrillInForCurrentTab()
	return m.tabRefreshCmd()
}

func (m *Model) tabRefreshCmd() tea.Cmd {
	if len(m.orgs) > 0 {
		o := m.orgs[m.selected]
		if canUseOrg(o) {
			d := m.ensureOrgData(o.Username)
			m.applySelectedChipMatcher(d)
		}
	}
	// Restore the active surface's persisted column widths whenever
	// the surface (tab/org) changes — previously they only loaded on
	// the first column keypress, so saved widths weren't visible
	// after a restart until the user touched a resize key.
	m.activeListTableContext()
	cmd := m.ensureDataFor(m.tab())
	// Kick the home banner animation when entering /home, but ONLY
	// if there isn't already a tick chain in flight — otherwise
	// repeated entries (org switching, drill-back, etc.) stack N
	// timers that all advance the same frame counter and the
	// animation appears to speed up. The handler unsets the flag
	// when the user navigates away, so re-entry kicks a fresh
	// chain.
	if m.tab() == TabHome && !m.homeBadgeTickRunning &&
		!m.settings.DisableHomeBanner() && !m.settings.HideHomeBanner() {
		m.homeBadgeTickRunning = true
		interval := time.Duration(m.settings.HomeBannerIntervalMs()) * time.Millisecond
		tick := homeBannerTickCmd(interval)
		if tick == nil {
			return cmd
		}
		if cmd == nil {
			return tick
		}
		return tea.Batch(cmd, tick)
	}
	return cmd
}

// lensSubs gathers the per-org substitutions applied to lens SOQL
// fragments (the :userId placeholder, etc.). The user id comes from
// the cached HomeData; if Home hasn't been fetched yet for this org
// the substitution is empty and lenses that depend on it return an
// SF parsing error — by then the user has likely visited /home so
// the value is populated.
func chipSubs(d *orgData) qchip.Substitutions {
	if d == nil {
		return qchip.Substitutions{}
	}
	home := d.Home.Value()
	return qchip.Substitutions{
		UserID:   home.UserID,
		UserName: home.UserName,
	}
}

// ensureDataFor returns the commands to populate whichever Resources
// view v needs for the currently-selected org.
//
// Data lifecycle is registry-driven: each tab declares TabSpec.EnsureData
// in tab_registry.go, and subtabs MAY declare their own SubtabSpec
// .EnsureData for subtab-scoped fetches (e.g. /users · All users
// fetches its per-chip Resource here). Both run when present — tab-
// level first (always needed), then subtab-level layered on top.
//
// Tabs / subtabs without an EnsureData hook are data-less on entry.
func (m *Model) ensureDataFor(v Tab) tea.Cmd {
	if len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	spec := lookupTabSpec(v)
	if spec == nil {
		return nil
	}
	if !canUseOrg(o) && !spec.OrgIndependent {
		// Disconnected org: don't fire fetches that can only fail.
		// Cached values stay in place (hidden by the renderMain
		// gate) and re-ensure on reconnect.
		return nil
	}
	d := m.ensureOrgData(o.Username)
	var cmds []tea.Cmd
	if spec.EnsureData != nil {
		if cmd := spec.EnsureData(m, d, o); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if sub := spec.activeSubtabSpec(*m); sub != nil && sub.EnsureData != nil {
		if cmd := sub.EnsureData(m, d, o); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

// Update is Bubble Tea's dispatch loop. The heavy lifting — resource
// routing, key handling, nav — lives in dedicated sibling files.
//
// Feature-cluster dispatchers (dispatchExportMsg, …) run first so the
// main switch stays focused on cross-cutting / not-yet-extracted
// messages. Each cluster lives in its own update_*_dispatch.go file
// and returns handled=true when it matched.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Preserve walkthrough completion before this message can close or
	// otherwise undo the state that satisfied the active step (for example,
	// dismissing global search or toggling zen back off).
	m.observeWalkthrough()

	// Inbound control-channel messages dispatch first so that an IPC
	// agent can drive the TUI even when another modal/dialogue is
	// open. The handlers themselves call into the same code keystrokes
	// would. After handling, re-arm the writes pump so the next
	// inbound message gets delivered.
	switch m2 := msg.(type) {
	case updateCheckMsg:
		return m, m.applyUpdateCheck(m2)
	case legalModalMsg:
		return m.applyLegalModal()
	case legalAcceptedMsg:
		return m.applyLegalAccepted()
	case welcomeModalMsg:
		return m.applyWelcomeModal()
	case welcomeActionMsg:
		return m.applyWelcomeAction(m2)
	case controlOpenTabMsg:
		mm, cmd := m.applyControlOpenTab(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlChipApplyMsg:
		mm, cmd := m.applyControlChipApply(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlSwitchOrgMsg:
		mm, cmd := m.applyControlSwitchOrg(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlLoadProjectMsg:
		mm, cmd := m.applyControlLoadProject(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlPreviewChipMsg:
		mm, cmd := m.applyControlPreviewChip(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlPreviewSaveChipMsg:
		mm, cmd := m.applyControlPreviewSaveChip(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlPreviewDismissChipMsg:
		mm, cmd := m.applyControlPreviewDismissChip(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	case controlSeedSOQLMsg:
		mm, cmd := m.applyControlSeedSOQL(m2)
		return mm, tea.Batch(cmd, mm.ControlWritesCmd())
	}
	if cmd, handled := (&m).dispatchExportMsg(msg); handled {
		return m, cmd
	}
	if mm, cmd, handled := m.dispatchModalMsg(msg); handled {
		return mm, cmd
	}
	if mm, cmd, handled := m.dispatchPermsMsg(msg); handled {
		return mm, cmd
	}
	if mm, cmd, handled := m.dispatchOrgsMsg(msg); handled {
		return mm, cmd
	}
	if mm, cmd, handled := m.dispatchDrillMsg(msg); handled {
		return mm, cmd
	}
	if mm, cmd, handled := m.dispatchBundlesMsg(msg); handled {
		return mm, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applyStartupAutoLayout()
		return m, nil

	case tea.MouseWheelMsg:
		// SOQL autocomplete popup takes wheel priority when the
		// editor is open with a non-empty suggestion list. The
		// popup is overlaid on top of the editor (same visual
		// plane as a modal) so the wheel scrolling the underlying
		// results table while a popup is on screen would be
		// disorienting — the popup is what the user is looking at.
		if s := m.activeAutocompleteSession(); s != nil {
			// Popup wheel scroll: route through the same rate-limited
			// accumulator the listtable uses. View() is heavy enough
			// that feeding raw wheel events at ~120Hz queues Updates
			// and produces the "lagged + overshoot" feel we already
			// fixed for list views. See feedback_bubbletea_v2_wheel_scroll
			// for the full diagnosis. wheelStep batches events into
			// a single drained delta per minInterval; 0 returns mean
			// "skip render, accumulate more."
			step := m.wheelStep(msg)
			if step == 0 {
				m.skipNextFrameRender()
				return m, nil
			}
			ac := s.autocomplete
			n := len(ac.Items)
			if n > 0 {
				next := ac.Cursor + step
				if next < 0 {
					next = 0
				}
				if next > n-1 {
					next = n - 1
				}
				ac.Cursor = next
			}
			return m, nil
		}
		// One row per wheel tick. Inertial scroll on macOS sends 100s
		// of events; each maps to one cursor step. Direction-change
		// "stickiness" (cursor doesn't move when reversing at the
		// edge) is genuine but unfixable in user-space — the
		// trackpad's inertial buffer is OS-level.
		//
		// When a modal is active, the wheel must steer the modal's
		// own list — not the surface behind it. Synthesize an arrow
		// key and route it through the same handler the modal uses
		// for the real key, so each modal's existing up/down logic
		// (cursor clamps, selectability skips, etc.) just works.
		// wheelStep accumulates events into a pending delta. A
		// deferred event returns 0 (skip render, no cursor move);
		// an accepted event returns the drained delta — the cursor
		// jumps by however many wheel events the user produced
		// since the last accepted tick. So a fast trackpad flick
		// translates faithfully to "advance N rows," not "advance
		// minInterval-rate-limited steps."
		if m.anyModalActive() {
			step := m.wheelStep(msg)
			if step == 0 {
				m.skipNextFrameRender()
				return m, nil
			}
			// Modal cursors take one arrow-key per row. For
			// accumulated bursts re-route |step| KeyPresses so
			// the modal's existing key handler does the
			// clamping / selectability work it already does.
			code := tea.KeyDown
			if step < 0 {
				code = tea.KeyUp
				step = -step
			}
			model := tea.Model(m)
			var cmds []tea.Cmd
			for i := 0; i < step; i++ {
				next, cmd := model.Update(tea.KeyPressMsg{Code: code})
				model = next
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return model, tea.Batch(cmds...)
		}
		// Two distinct wheel semantics depending on the active list-
		// table mode:
		//
		//   Continuous: wheel = "scroll through the data set." A
		//   fast trackpad flick advances the cursor across many
		//   rows — the accumulator drains drainPending(... cap)
		//   per accepted tick so finger speed maps to scroll
		//   speed.
		//
		//   Paginated: wheel = "scroll within THIS page." The
		//   cursor stays on the visible page; passing the bottom
		//   row stops at the bottom (no auto-advance). A separate
		//   shortcut moves between pages. This matches how
		//   less / vim / most pagers work and avoids the
		//   "teleporting through pages" feel that happens when an
		//   inertial flick auto-advances ten pages.
		if m.activeListPaginated() {
			return m.handleWheelPaginated(msg)
		}
		return m.handleWheelContinuous(msg)

	case tea.MouseClickMsg:
		m.orgQuickJumpActive = false
		return m.handleMouseClick(msg)

	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return m, nil

	case resource.UpdatedMsg:
		return m.applyResourceMsg(msg)

	case homeBannerTickMsg:
		// Single-flight: when the user has navigated away, clear the
		// running flag and stop scheduling. Returning to /home will
		// kick a fresh tick via tabRefreshCmd. While ON /home, advance
		// the frame and reschedule — the flag stays true and the
		// chain continues.
		if m.tab() != TabHome || m.settings.DisableHomeBanner() {
			m.homeBadgeTickRunning = false
			return m, nil
		}
		m.homeBadgeFrame++
		interval := time.Duration(m.settings.HomeBannerIntervalMs()) * time.Millisecond
		return m, homeBannerTickCmd(interval)

	case deployWatchTickMsg:
		return m, m.applyDeployWatchTick()

	case exportActivityTickMsg:
		// Self-rescheduling tick that drives the status-bar activity
		// indicator while exports are running. Stops re-arming when
		// the registry is empty so we don't spin forever after the
		// last export finishes.
		m.exportTickRunning = false
		m.exportActivityFrame++
		if m.exports != nil && m.exports.hasInflight() {
			return m, m.exportActivityTickCmd()
		}
		return m, nil

	case soqlSavedChangedMsg:
		m.invalidateSOQLSaved()
		return m, nil

	case fieldDescriptionLoadedMsg:
		// Lazy field-description fetch (Tooling) completed — cache it so
		// the field-detail page shows the real value instead of a
		// placeholder. Errors cache "" so we don't refetch every frame.
		(&m).applyFieldDescriptionLoaded(msg)
		return m, nil

	case scheduledJobClassResolvedMsg:
		// Enter on a scheduled-job row resolved (or didn't) an Apex class.
		if msg.err != nil {
			m.flash("couldn't resolve class: " + msg.err.Error())
			return m, nil
		}
		if msg.classID == "" {
			m.flash("this scheduled job isn't Apex-backed")
			return m, nil
		}
		if d := m.activeOrgData(); d != nil {
			rememberDrillReturn(d, TabApexDetail, TabSystem)
		}
		return m, (&m).triggerOpenApexClass(msg.classID)

	case sidebarPositionChangedMsg:
		// Persist the choice and apply it live. "auto" is a no-op today
		// (coming soon) — leave the sidebar where it is; rhs/bottom move
		// it immediately so the setting has visible effect.
		if m.settings != nil {
			m.settings.SetSidebarPosition(msg.pos)
			_ = m.settings.Save()
		}
		switch msg.pos {
		case settings.SidebarPositionRHS:
			m.sidebarStacked = false
			m.flash("sidebar: right")
		case settings.SidebarPositionBottom:
			m.sidebarStacked = true
			m.flash("sidebar: bottom")
		case settings.SidebarPositionAuto:
			m.flash("sidebar: auto (coming soon) — position unchanged for now")
		}
		return m, nil

	case flowChangedMsg:
		// A flow-detail write landed (rename / version delete). Refresh
		// the version list (the changed row) and the Flows list (a label
		// change surfaces there too).
		d := m.data[msg.username]
		if d == nil {
			return m, nil
		}
		var cmds []tea.Cmd
		if r, ok := d.FlowVersions[d.FlowCur]; ok {
			cmds = append(cmds, r.Refresh(m.cache))
		}
		cmds = append(cmds, d.Flows.Refresh(m.cache))
		return m, tea.Batch(cmds...)

	case soqlResultMsg:
		session := (&m).soqlSessionForTarget(msg.session)
		if session == nil {
			return m, nil
		}
		if msg.sessionID != 0 && session.id != 0 && msg.sessionID != session.id {
			return m, nil
		}
		// Stale result?  Happens when the user cancelled (ctrl+c)
		// or started a new query while this one was still in
		// flight.  Drop the message so the modal's idle state
		// isn't clobbered.
		if msg.gen != 0 && msg.gen != session.soqlRunGen {
			return m, nil
		}
		session.soqlRunning = false
		session.soqlCancel = nil
		session.soqlErr = msg.err
		if msg.err == nil {
			session.soqlResult = msg.data
			session.soqlRowCur = 0
			if msg.session == soqlSessionTab || msg.session == "" {
				if len(session.soqlHistory) == 0 || session.soqlHistory[len(session.soqlHistory)-1] != msg.soql {
					session.soqlHistory = append(session.soqlHistory, msg.soql)
					if len(session.soqlHistory) > 50 {
						session.soqlHistory = session.soqlHistory[len(session.soqlHistory)-50:]
					}
				}
			}
		}
		if msg.session == soqlSessionTab || msg.session == "" {
			m.persistSOQLHistory(msg)
		}
		return m, nil

	case autocompleteValuesMsg:
		(&m).applyAutocompleteValues(msg)
		return m, nil

	case searchDebounceTickMsg:
		// Debounce window elapsed since the last buffer mutation:
		// promote Buffer → Effective on the current view's search
		// state. The render that follows this tick is the one
		// that actually re-runs the (now-batched) filter pass.
		if s := (&m).currentSearch(); s != nil && s.DebouncePending() {
			s.SyncEffective()
		}
		return m, nil

	case execResultMsg:
		m.execRunning = false
		m.execErr = msg.err
		if msg.err == nil {
			m.execResult = msg.data
			// Auto-flip to Output subtab on success when log was
			// captured — saves a keystroke to see the debug output.
			if msg.data.Success && msg.data.LogBody != "" {
				m.execSubtabIdx = execSubtabIndex(SubtabExecOutput)
			}
		}
		m.persistExecHistory(msg)
		return m, nil

	case execEditorClosedMsg:
		if msg.err != nil {
			m.flash(msg.err.Error())
			return m, nil
		}
		m.execInput.SetValue(msg.body)
		return m, nil

	case demoFlashMsg:
		m.flashFor(msg.text, 5*time.Second)
		return m, nil

	case compareInventoryMsg:
		if d, ok := m.data[msg.OrgKey]; ok && d.Run != nil {
			if msg.Err != nil {
				d.Run.Phase = comparePhaseSetup
				d.Run.Err = msg.Err
			} else {
				d.Run.Inv = msg.Inv
				d.Run.snapA = msg.SnapA
				d.Run.snapB = msg.SnapB
				d.Run.Phase = comparePhaseInventory
				d.Run.Err = nil
				d.syncInventoryList()
				d.InventoryList.ResetCursor()
				d.recordCompareRun(d.Run)
			}
		}
		return m, nil

	case compareTypeDoneMsg:
		if d, ok := m.data[msg.OrgKey]; ok {
			return m, (&m).applyCompareTypeDone(d, msg)
		}
		return m, nil

	case compareObjectsDoneMsg:
		if d, ok := m.data[msg.OrgKey]; ok {
			return m, (&m).applyCompareObjectsDone(d, msg)
		}
		return m, nil

	case compareBodyFetchedMsg:
		// A dropped (over-budget) body was re-fetched for a drill-in.
		(&m).applyCompareBodyFetched(msg)
		return m, nil

	case comparePreviewFetchedMsg:
		// A dropped body was re-fetched for the side-panel preview.
		(&m).applyComparePreviewFetched(msg)
		return m, nil

	case compareTypesLoadedMsg:
		// The scope modal's async type-catalog fetch returned; fill the
		// open modal (ignored if stale / modal closed).
		(&m).applyCompareTypesLoaded(msg)
		return m, nil

	case compareTickMsg:
		// Self-rescheduling animation tick for the retrieving screen.
		// Stops re-arming once no run is in flight, so it doesn't spin
		// forever after the comparison finishes.
		m.compareTickRunning = false
		m.compareFrame++
		if m.compareRetrieveInFlight() {
			return m, m.compareTickCmd()
		}
		return m, nil

	case execProdConfirmMsg:
		// User cleared the production confirmation modal. Fire the
		// actual run with the body the gate captured at open-time
		// (so even if they typed more into the editor after opening
		// the modal, we run what they confirmed on).
		if len(m.orgs) == 0 {
			return m, nil
		}
		o := m.orgs[m.selected]
		return m, m.runExecConfirmed(o, msg.body)

	case recordEditSaveMsg:
		// PATCH /sobjects/.../<id> completed. applyRecordEditSave
		// processes success (clear session + refetch the record so
		// server-side formula / audit / trigger updates land) or
		// failure (stash per-field errors so the row renders them
		// inline + leave dirty intact for the user to fix + retry).
		return m, m.applyRecordEditSave(msg)

	case referenceSearchMsg:
		// User pressed Enter on a reference editor's query line.
		// SOSL the target object; result lands as
		// referenceSearchResultMsg below.
		return m, m.applyReferenceSearch(msg)

	case referenceSearchResultMsg:
		m.applyReferenceSearchResult(msg)
		return m, nil

	case sources.ReportFoldersLoadedMsg:
		// Source's Apply hydrates the index. Then we hydrate the
		// persisted last-path, which couldn't run before the source
		// had data.
		msg.Source.Apply(msg)
		if msg.Err != nil {
			m.flash("folders: " + msg.Err.Error())
		}
		// Find the registry that owns this source so we can hydrate
		// the path. Walk the orgs map; cheap.
		for _, d := range m.data {
			if d.ReportFoldersSrc == msg.Source && d.ReportFolders != nil {
				if persist := sources.NewSettingsPersister(m.settings, d.username, "report-folders"); persist != nil {
					_, lastPath := persist.Load()
					if len(lastPath) > 0 {
						d.ReportFolders.HydrateLastPath(lastPath)
					}
				}
				break
			}
		}
		return m, nil

	case openSettingsSubmenuMsg:
		return m, m.dispatchSettingsPick(msg.pick)

	case tea.KeyMsg:
		// Walkthrough control keys, intercepted while the tour is active
		// and no modal is open. The tour never auto-advances: the user
		// does the action (the panel shows a ✓ when the step's predicate
		// is satisfied) and presses 'w' to move on when THEY choose —
		// so they can linger, try variations, or just read.
		//   w      → next step
		//   ctrl+w → exit the tour
		//   esc    → deliberately NOT a tour key (it's "go back a level,"
		//            which the tour itself asks the user to press).
		if m.walkthroughActive() && !m.anyModalActive() {
			switch msg.String() {
			case "ctrl+w":
				m.exitWalkthrough()
				return m, nil
			case "w":
				m.advanceWalkthrough()
				return m, nil
			}
		}
		return m.handleKey(msg)

	case tea.PasteMsg:
		// Terminal-level bracketed-paste delivery. Forward to whichever
		// text input is active so ctrl+v / cmd+v works inside edit
		// modals + the SOQL editor. Anywhere else it's a no-op.
		if m.exportSave != nil {
			// Paste into the export-save path field (the most likely
			// interaction with this modal). Only when the path field
			// has focus; ignored on the checkbox.
			if m.exportSave.focus == 0 {
				m.exportSave.insertAtCursor(msg.Content)
			}
			return m, nil
		}
		if m.themePicker != nil && m.themePicker.search.Active {
			newInput, cmd := m.themePicker.search.Input.Update(msg)
			m.themePicker.search.Input = newInput
			mm, _ := m.applyThemePickerSearch()
			return mm, cmd
		}
		if m.chipWizard != nil {
			st := m.chipWizard
			if st.Cursor == -1 {
				newInput, cmd := st.labelInput.Update(msg)
				st.labelInput = newInput
				return m, cmd
			}
			if st.Advanced {
				newInput, cmd := st.advancedText.Update(msg)
				st.advancedText = newInput
				return m, cmd
			}
			if st.Cursor >= 0 && st.Cursor < len(st.criteria) {
				cur := &st.criteria[st.Cursor]
				switch cur.Kind {
				case cwText, cwInt, cwDate:
					newInput, cmd := cur.input.Update(msg)
					cur.input = newInput
					return m, cmd
				case cwLimit:
					// Only delegate paste/etc to the embedded input
					// when manual mode is active; otherwise the row
					// is a static label.
					if cur.triValue != nil && *cur.triValue {
						newInput, cmd := cur.input.Update(msg)
						cur.input = newInput
						return m, cmd
					}
				}
			}
		}
		if m.editModal != nil && m.editModal.editor != nil {
			newEditor, cmd := m.editModal.editor.Update(msg)
			m.editModal.editor = &newEditor
			return m, cmd
		}
		if m.soqlEditing {
			newInput, cmd := m.soqlInput.Update(msg)
			m.soqlInput = newInput
			return m, cmd
		}
		if m.globalSearch != nil {
			newInput, cmd := m.globalSearch.input.Update(msg)
			m.globalSearch.input = newInput
			return m, cmd
		}
		// Search buffer on the current view — /objects, /perms, etc.
		if s := m.currentSearch(); s != nil && s.Active {
			newInput, cmd := s.Input.Update(msg)
			s.Input = newInput
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// applyEditModalLoaded folds the outcome of the async LoadCurrent
// closure into the modal: buffer gets the current value, Loading
// clears, Err is stashed if the load failed (user can still type).
func (m Model) applyEditModalLoaded(msg editModalLoadedMsg) (tea.Model, tea.Cmd) {
	if m.editModal == nil {
		return m, nil
	}
	m.editModal.Loading = false
	if msg.Err != nil {
		if typed := sf.AsSFError(msg.Err); typed != nil {
			m.editModal.Err = typed.Error()
		} else {
			m.editModal.Err = msg.Err.Error()
		}
		return m, nil
	}
	if m.editModal.editor != nil {
		m.editModal.editor.SetValue(msg.Value)
		m.editModal.editor.CursorEnd()
	}
	return m, nil
}

// applyEditModalPreview folds the outcome of the Preview closure:
// on success, transition the modal into the Confirming phase with
// the diff lines + baseline token; on error, stay in editing and
// surface the error message. Either way Previewing flips off.
func (m Model) applyEditModalPreview(msg editModalPreviewMsg) (tea.Model, tea.Cmd) {
	if m.editModal == nil {
		return m, nil
	}
	m.editModal.Previewing = false
	if msg.Err != nil {
		if typed := sf.AsSFError(msg.Err); typed != nil {
			m.editModal.Err = typed.Error()
		} else {
			m.editModal.Err = msg.Err.Error()
		}
		return m, nil
	}
	m.editModal.Confirming = true
	m.editModal.PreviewLines = msg.Result.Lines
	m.editModal.PreviewBaseline = msg.Result.Baseline
	return m, nil
}

// applyEditModalResult folds the outcome of an edit-modal save back
// into the model. On success: close the modal, flash the caller's
// SuccessMsg (if any), and fire the OnSuccess tea.Cmd (typically a
// Resource refresh so the new value appears in the surrounding view).
// On failure: keep the modal open, unset Saving, render the typed
// error message so the user can edit + retry.
func (m Model) applyEditModalResult(msg editModalResultMsg) (tea.Model, tea.Cmd) {
	if m.editModal == nil {
		return m, nil
	}
	em := m.editModal
	if msg.Err == nil {
		onSuccess := em.OnSuccess
		successMsg := em.SuccessMsg
		m.editModal = nil
		if successMsg != "" {
			m.flash(successMsg)
		}
		if onSuccess != nil {
			return m, onSuccess()
		}
		return m, nil
	}
	// Failure: surface typed SFError message + hint if we can.
	if typed := sf.AsSFError(msg.Err); typed != nil {
		hint := ""
		if typed.Hint != "" {
			hint = "  (" + typed.Hint + ")"
		}
		em.Err = typed.Error() + hint
	} else {
		em.Err = msg.Err.Error()
	}
	em.Saving = false
	return m, nil
}

// applyChoiceModalLoaded folds LoadCurrent's result into the choice
// modal: positions the cursor on whichever option matches the loaded
// value, clears Loading, or surfaces the error.
func (m Model) applyChoiceModalLoaded(msg choiceModalLoadedMsg) (tea.Model, tea.Cmd) {
	if m.choiceModal == nil {
		return m, nil
	}
	m.choiceModal.Loading = false
	if msg.Err != nil {
		if typed := sf.AsSFError(msg.Err); typed != nil {
			m.choiceModal.Err = typed.Error()
		} else {
			m.choiceModal.Err = msg.Err.Error()
		}
		return m, nil
	}
	// Position the cursor on the matching option (shallow equality is
	// fine — options are primitives or small structs).
	for i, opt := range m.choiceModal.Options {
		if opt.Value == msg.Value {
			m.choiceModal.Cursor = i
			break
		}
	}
	return m, nil
}

// applyChoiceModalResult mirrors applyEditModalResult for the choice
// variant. Close-and-flash on success, keep-open-with-error on fail.
func (m Model) applyChoiceModalResult(msg choiceModalResultMsg) (tea.Model, tea.Cmd) {
	if m.choiceModal == nil {
		return m, nil
	}
	cm := m.choiceModal
	if msg.Err == nil {
		onSuccess := cm.OnSuccess
		onSuccessTyped := cm.OnSuccessTyped
		successMsg := cm.SuccessMsg
		m.choiceModal = nil
		if successMsg != "" {
			m.flash(successMsg)
		}
		if onSuccessTyped != nil {
			return m, onSuccessTyped(msg.Value)
		}
		if onSuccess != nil {
			return m, onSuccess()
		}
		return m, nil
	}
	if typed := sf.AsSFError(msg.Err); typed != nil {
		hint := ""
		if typed.Hint != "" {
			hint = "  (" + typed.Hint + ")"
		}
		cm.Err = typed.Error() + hint
	} else {
		cm.Err = msg.Err.Error()
	}
	cm.Saving = false
	return m, nil
}

// applyResourceMsg routes a resource update to the right Resource by
// (scope, key). Global scope → the orgsRes/projectsRes on the model.
// Per-org scope → the matching Resource on orgData, then a ListView
// sync so the view renders the latest items.
//
// If the global-search modal is open, every successful apply triggers
// an index rebuild so lazy scope-in fetches surface as soon as they
// land.
func (m Model) applyResourceMsg(msg resource.UpdatedMsg) (tea.Model, tea.Cmd) {
	// Surface fetch errors as a flash. Cache-load errors are silent
	// (they usually mean "no cache yet"); fresh-fetch errors are real
	// failures the user wants to see — without this every fetch error
	// looked like "data simply not loaded yet" because the resource
	// stays at zero with no visible indicator.
	//
	// Three suppressions:
	//   1. orgs fetch on fresh install — the /home onboarding panel
	//      already communicates the "no orgs / sf missing" state, so
	//      flashing the same message duplicates it.
	//   2. demo-mode "live Salesforce calls disabled" errors — these
	//      are expected in --demo and would otherwise spam every
	//      resource-fetch with the same noise message.
	suppressFlash := msg.Scope == "global" && msg.Key == "orgs" && len(m.orgs) == 0
	if msg.Err != nil && strings.Contains(msg.Err.Error(), "demo mode:") {
		suppressFlash = true
	}
	if !msg.FromCache && msg.Err != nil && !suppressFlash {
		m.flash(resourceFetchErrorMsg(msg.Key, msg.Err))
	}
	defer m.rebuildGlobalSearchIndexForKey(msg.Key)
	if msg.Scope == "global" {
		switch msg.Key {
		case "orgs":
			if m.orgsRes.Apply(msg) {
				m.orgs = m.orgsRes.Value()
				// Re-inject the demo org after every live org reload so a
				// refresh doesn't drop it from the panel (it isn't in the
				// sf-CLI org list).
				m.orgs = mergeDemoOrgs(m.orgs, m.settings.DemoOrgImported())
				// A live orgs refetch is the only signal we get that an
				// alias may have been repointed to a different org (via
				// `sf alias set` in another terminal). The REST client
				// registry is keyed by alias and caches instanceURL+token,
				// so without this a repointed alias keeps serving — and
				// persisting to disk — the OLD org's data under the new
				// org's label. Drop the cached clients so they re-bootstrap
				// against the current alias→org mapping. Cache loads can't
				// reflect an external repoint, so skip them.
				if !msg.FromCache {
					// Reconcile — don't nuke. A routine live orgs refetch
					// used to InvalidateRESTClients() unconditionally,
					// discarding every good token and forcing a full `sf`
					// re-bootstrap (two subprocess spawns on redacting
					// CLIs, ~2.5s) on the next data call — the cause of
					// "refresh got slow." Reconciling keeps every client
					// whose alias→instanceURL mapping is unchanged and
					// drops only aliases that vanished or were repointed
					// externally (the case the blanket call guarded).
					want := make(map[string]string, len(m.orgs))
					for _, o := range m.orgs {
						want[targetArg(o)] = o.InstanceURL
					}
					sf.ReconcileRESTClients(want)
					// Free the in-memory orgData for orgs that vanished
					// from the LIVE list (logged out via the CLI in
					// another terminal). Without this, m.data kept the
					// old session's UI state forever — a slow leak, and
					// if the org was later re-authed, ensureOrgData
					// reused the stale cursors/lists as if the session
					// had never ended. Live-only: a stale cache load
					// must not evict data for orgs that still exist.
					// (m.orgs already includes re-merged demo orgs, so
					// demo data survives.)
					present := make(map[string]bool, len(m.orgs))
					for _, o := range m.orgs {
						present[o.Username] = true
					}
					for user := range m.data {
						if !present[user] {
							delete(m.data, user)
						}
					}
					// Persist the live result to cache.db so OTHER
					// surfaces that need the org list (the IPC
					// listener's bundle / project verbs, future cold
					// launches) see the same orgs the TUI sees. Without
					// this the only writer was the demo seed, leaving
					// real-world caches frozen at whatever the last
					// demo run wrote — so any org added since then
					// would never make it into the cache, and
					// app.ResolveOrg(alias) would return "not found"
					// even after the TUI had successfully shelled out
					// and rendered the org in its left rail.
					if m.cache != nil {
						if err := m.cache.PutOrgs(orgsToRows(m.orgs)); err != nil {
							applog.Warn("ui.orgs_cache_put_failed",
								map[string]any{"err": err.Error()})
						}
					}
				}
				// Three-step selection resolution:
				//
				//   1. FIRST load only: if the user has a pinned default
				//      org, jump to it. Gated by pinnedDefaultRestored so
				//      a later refetch doesn't clobber the user's in-
				//      session switch (the previous "if selected == 0"
				//      heuristic was unreliable — it re-fired whenever
				//      the pinned org happened to land at index 0).
				//
				//   2. EVERY load: re-anchor m.selected to the row that
				//      matches selectedUsername. The orgs slice can
				//      reorder between fetches; without this re-anchor
				//      the index silently points at a different org and
				//      the user sees a phantom jump.
				//
				//   3. Fall back to index 0 only when the active username
				//      no longer exists in the list (org logged out via
				//      the CLI) or no selection has ever been made.
				if !m.pinnedDefaultRestored {
					pinned := ""
					if m.settings != nil {
						pinned = m.settings.DefaultOrgUsername()
					}
					pinnedFound := false
					if pinned != "" {
						for i, o := range m.orgs {
							if o.Username == pinned {
								(&m).setSelectedOrg(i)
								pinnedFound = true
								break
							}
						}
					}
					// Do not consume the one-shot startup-pin restore on
					// an empty/missing cache payload. If the cached org
					// list is stale and lacks the pinned org, let the live
					// refresh try once before falling back.
					if pinned == "" || pinnedFound || !msg.FromCache {
						m.pinnedDefaultRestored = true
					}
					// The LIVE list also lacks the pinned org (logged
					// out?): say so instead of silently landing on the
					// first org — a silent fallback reads as "my default
					// setting stopped working."
					if pinned != "" && !pinnedFound && !msg.FromCache {
						m.flash("default org " + pinned + " not found — using first org")
					}
				} else if m.selectedUsername != "" {
					found := false
					for i, o := range m.orgs {
						if o.Username == m.selectedUsername {
							m.selected = i
							found = true
							break
						}
					}
					// The active org was logged out mid-list (e.g. via the
					// CLI) and the list refetched. A bare `m.selected` is now
					// in-bounds but points at a DIFFERENT org — re-anchor to
					// index 0 (which also resets selectedUsername) so we don't
					// silently teleport the active context to the wrong org.
					if !found && len(m.orgs) > 0 {
						(&m).setSelectedOrg(0)
					}
				}
				if m.selected >= len(m.orgs) {
					(&m).setSelectedOrg(0)
				} else if m.selectedUsername == "" && len(m.orgs) > 0 {
					// No prior selection AND no pinned default — adopt
					// whatever index 0 is so future refetches re-anchor.
					(&m).setSelectedOrg(m.selected)
				}
				// Prune stale group members — orgs the user has logged
				// out of via the CLI while sf-deck wasn't running. Save
				// only when something actually changed so we don't
				// rewrite settings.toml on every list refresh.
				authed := make(map[string]bool, len(m.orgs))
				for _, o := range m.orgs {
					authed[o.Username] = true
				}
				if m.settings.PruneOrgGroupMembers(authed) {
					m.saveSettings("")
				}
				// Keep the rail cursor aligned to the resolved active
				// org. Clamping from the old cursor would let the rail's
				// default row 0 overwrite a startup pin before first
				// paint.
				m.syncOrgRailCursorToSelected()
				// Kicking off the selected-org's data is a side-effect
				// of the org list arriving.
				cmd := m.onOrgChanged()
				if msg.FromCache {
					cmd = tea.Batch(cmd, m.orgsRes.MaybeRefreshAfterCacheLoad(m.cache))
				}
				return m, cmd
			}
		case "projects":
			if m.projectsRes.Apply(msg) {
				m.projectList.Set(m.projectsRes.Value())
				if msg.FromCache {
					return m, m.projectsRes.MaybeRefreshAfterCacheLoad(m.cache)
				}
			}
		}
		return m, nil
	}

	d, ok := m.data[msg.Scope]
	if !ok {
		return m, nil
	}
	if handled, refresh := m.applyOrgPrefixResourceMsg(d, msg); handled {
		return m, refresh
	}
	// Child-entity resources (list + detail) route themselves via
	// SObjectChildren.ApplyAndMaybeRefresh. Each children struct knows
	// its own key prefixes; first match wins. The refresh cmd is the
	// post-cache-load kick that fires the network fetch when the
	// cached payload is missing/stale (otherwise drill-in screens
	// would sit on "loading…" forever).
	if handled, refresh := d.ValidationRules.ApplyAndMaybeRefresh(msg, m.cache); handled {
		return m, refresh
	}
	if handled, refresh := d.RecordTypes.ApplyAndMaybeRefresh(msg, m.cache); handled {
		return m, refresh
	}
	if handled, refresh := d.Triggers.ApplyAndMaybeRefresh(msg, m.cache); handled {
		return m, refresh
	}
	if handled, refresh := d.PageLayouts.ApplyAndMaybeRefresh(msg, m.cache); handled {
		return m, refresh
	}
	if handled, refresh := d.ObjectFlows.ApplyAndMaybeRefresh(msg, m.cache); handled {
		return m, refresh
	}
	// Per-key Apply + (FromCache only) follow-up refresh-if-stale.
	// Cache loads complete instantly; this conditional refresh fires
	// the network call only when the cached payload is stale, instead
	// of running unconditionally in parallel with the cache read.
	var refresh tea.Cmd
	switch msg.Key {
	case "home":
		if d.Home.Apply(msg) {
			d.SyncHomeLists()
		}
		if msg.FromCache {
			refresh = d.Home.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "org_info":
		// Org identity singleton — drives the Home banner + ORG card.
		// Read directly via d.OrgInfo.Value(), so there's no derived
		// list to sync; just Apply (clears Busy) + the cache-load
		// refresh kick. Without this case the UpdatedMsg was dropped
		// and the resource stayed Busy forever, so the banner / ORG
		// card couldn't reliably populate.
		d.OrgInfo.Apply(msg)
		if msg.FromCache {
			refresh = d.OrgInfo.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "networks":
		if d.Networks.Apply(msg) {
			// Nothing to sync — Networks is read directly by the
			// Contact ^O menu builder; no derived view needs a kick.
		}
		if msg.FromCache {
			refresh = d.Networks.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "permsets":
		landed := d.PermissionSets.Apply(msg)
		if msg.FromCache {
			refresh = d.PermissionSets.MaybeRefreshAfterCacheLoad(m.cache)
		}
		// Auto-select the first permset for the FLS grid on first
		// landing — used to be done at render time, which fired AFTER
		// ensureDataFor and therefore left the grid stuck on
		// "fetching…" until a navigation event re-triggered the
		// dispatcher. Now: as soon as permsets land, pick a default
		// parent and fire the FLS ensure so the grid populates without
		// further user input.
		if landed && d.FLSParentID == "" && d.DescribeCur != "" {
			perms := d.PermissionSets.Value()
			if len(perms) > 0 {
				d.FLSParentID = perms[0].ID
				if len(m.orgs) > 0 && m.tab() == TabObjectDetail && m.currentSubtab() == SubtabFLS {
					o := m.orgs[m.selected]
					if canUseOrg(o) {
						r := d.EnsureFLS(targetArg(o), d.DescribeCur, d.FLSParentID)
						flsCmd := r.Ensure(m.cache)
						if refresh == nil {
							refresh = flsCmd
						} else {
							refresh = tea.Batch(refresh, flsCmd)
						}
					}
				}
			}
		}
	case "sobjects_v5":
		if d.SObjects.Apply(msg) {
			d.SyncSObjectsList()
			// SOQL autocomplete may be waiting on this catalog
			// to suggest sObjects after FROM. Force a refresh
			// (memo-bust) so the next render sees the loaded list.
			(&m).autocompleteInvalidate()
		}
		if msg.FromCache {
			refresh = d.SObjects.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "deploys_v2":
		// Snapshot non-terminal ids pre-apply so we can flash a
		// completion banner for anything that just finished, then
		// (re-)arm the live-watch tick if the fresh window still
		// holds in-flight rows.
		inflight := map[string]bool{}
		for _, r := range d.Deploys.Value() {
			if r.InFlight() {
				inflight[r.ID] = true
			}
		}
		if d.Deploys.Apply(msg) {
			d.SyncDeploysList()
			m.flashFinishedDeploys(inflight, d)
		}
		if msg.FromCache {
			refresh = d.Deploys.MaybeRefreshAfterCacheLoad(m.cache)
		}
		if watch := m.deployWatchTickCmd(); watch != nil {
			if refresh == nil {
				refresh = watch
			} else {
				refresh = tea.Batch(refresh, watch)
			}
		}
	case "notifications":
		if d.Notifications.Apply(msg) {
			d.SyncNotificationsList()
		}
		if msg.FromCache {
			refresh = d.Notifications.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "recently_viewed":
		if d.RecentlyViewed.Apply(msg) {
			d.RecentlyViewedList.Set(d.RecentlyViewed.Value())
			d.recentGen++
			// d.RecentSFList lazy-syncs in the /home Recent render
			// surface; no eager sync needed here.  The synthetic SF
			// Recently Viewed chip on /records reads from the
			// per-sObject d.RecentlyViewedPerSObject payload (see
			// the `recently_viewed_per_sobject:` prefix route), so
			// this global apply no longer needs to re-fire chip
			// records — that's handled in update_resource_helpers.go.
		}
		if msg.FromCache {
			refresh = d.RecentlyViewed.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "permsets_full_v2":
		if d.PermSets.Apply(msg) {
			d.SyncPermSetsList()
		}
		if msg.FromCache {
			refresh = d.PermSets.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "psgs_v2":
		if d.PSGs.Apply(msg) {
			d.SyncPSGsList()
		}
		if msg.FromCache {
			refresh = d.PSGs.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "profiles_v2":
		if d.Profiles.Apply(msg) {
			d.SyncProfilesList()
		}
		if msg.FromCache {
			refresh = d.Profiles.MaybeRefreshAfterCacheLoad(m.cache)
		}
	default:
		// Every list-backed resource with the plain Apply→sync→refresh
		// shape is handled generically from its registration (see
		// list_resource_registrations.go). The explicit cases above are
		// only the ones with bespoke apply logic. Keyed-prefix resources
		// (groupmembers:, usersessions:, …) were already handled by
		// applyOrgPrefixResourceMsg before this switch, so anything
		// reaching here is either a registered generic resource or an
		// unknown key (harmless no-op).
		if handled, r := m.routeListResource(d, msg); handled {
			refresh = r
		}
	}
	// A resource just landed on the active org — if a Move-to-org is
	// armed and waiting on this org's data, try to complete it now.
	// resolvePendingMove is a no-op when nothing is pending or the
	// target list still hasn't loaded.
	if drill := m.resolvePendingMove(); drill != nil {
		if refresh != nil {
			return m, tea.Batch(refresh, drill)
		}
		return m, drill
	}
	return m, refresh
}

// applyStartupAutoLayout decides sidebar placement from the terminal
// width on the FIRST WindowSizeMsg, when the auto-layout setting is
// on. Wide terminal → sidebar on the right (beside main, room for
// many columns); narrow → sidebar stacked below main so columns get
// the full width.
//
// One-shot by design (latched via startupLayoutDone): this is a
// startup convenience, NOT reactive layout. Resizing the window
// afterwards never moves the sidebar back — the user's manual ctrl+\
// choice always wins after launch. Fires only when the sidebar is
// actually open (stacked-vs-beside is meaningless when hidden).
func (m *Model) applyStartupAutoLayout() {
	if m.startupLayoutDone {
		return
	}
	// Need a real width before we can decide. The very first
	// WindowSizeMsg carries it; guard against a 0 that would
	// mis-classify every terminal as "narrow".
	if m.width <= 0 {
		return
	}
	m.startupLayoutDone = true
	if m.settings == nil {
		return
	}
	// Sidebar placement is now driven by the single SidebarPosition
	// setting (applied at construction: rhs → unstacked, bottom →
	// stacked). "auto" is reserved for future reactive placement and is
	// a deliberate no-op today — the boot default (RHS) stands. There's
	// nothing width-dependent left to decide here; kept as the one-shot
	// hook so a real "auto" implementation can slot in without rewiring.
}
