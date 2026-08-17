package ui

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

func (m *Model) ensureOrgData(username string) *orgData {
	target := m.targetForUsername(username)
	d, ok := m.data[username]
	if !ok || d.target != target {
		d = newOrgData(username, target, m.cache, m.settings)
		m.data[username] = d
	}
	if !d.RecentLoaded {
		loadRecent(m, d, username)
		d.RecentList.Set(d.Recent)
		m.hydrateLoadedProjectFromSettings(d, username)
		d.RecentLoaded = true
	}
	return d
}

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
		if s := m.activeAutocompleteSession(); s != nil {
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
		(&m).applyFieldDescriptionLoaded(msg)
		return m, nil

	case scheduledJobClassResolvedMsg:
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
		// Persist the choice and apply it live. Normalise the retired
		// "auto" value to its historical effective layout so old messages
		// and settings cannot revive an unsupported picker option.
		pos := msg.pos
		if pos == settings.SidebarPositionAuto {
			pos = settings.SidebarPositionRHS
		}
		if m.settings != nil {
			m.settings.SetSidebarPosition(pos)
			_ = m.settings.Save()
		}
		switch pos {
		case settings.SidebarPositionRHS:
			m.sidebarStacked = false
			m.flash("sidebar: right")
		case settings.SidebarPositionBottom:
			m.sidebarStacked = true
			m.flash("sidebar: bottom")
		}
		return m, nil

	case flowChangedMsg:
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
		if s := (&m).currentSearch(); s != nil && s.DebouncePending() {
			s.SyncEffective()
		}
		return m, nil

	case execResultMsg:
		m.execRunning = false
		m.execErr = msg.err
		if msg.err == nil {
			m.execResult = msg.data
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
		(&m).applyCompareBodyFetched(msg)
		return m, nil

	case comparePreviewFetchedMsg:
		(&m).applyComparePreviewFetched(msg)
		return m, nil

	case compareTypesLoadedMsg:
		(&m).applyCompareTypesLoaded(msg)
		return m, nil

	case compareTickMsg:
		m.compareTickRunning = false
		m.compareFrame++
		if m.compareRetrieveInFlight() {
			return m, m.compareTickCmd()
		}
		return m, nil

	case execProdConfirmMsg:
		if len(m.orgs) == 0 {
			return m, nil
		}
		o := m.orgs[m.selected]
		return m, m.runExecConfirmed(o, msg.body)

	case recordEditSaveMsg:
		return m, m.applyRecordEditSave(msg)

	case referenceSearchMsg:
		return m, m.applyReferenceSearch(msg)

	case referenceSearchResultMsg:
		m.applyReferenceSearchResult(msg)
		return m, nil

	case sources.ReportFoldersLoadedMsg:
		msg.Source.Apply(msg)
		if msg.Err != nil {
			m.flash("folders: " + msg.Err.Error())
		}
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
		if m.exportSave != nil {
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
		if s := m.currentSearch(); s != nil && s.Active {
			newInput, cmd := s.Input.Update(msg)
			s.Input = newInput
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

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
	for i, opt := range m.choiceModal.Options {
		if opt.Value == msg.Value {
			m.choiceModal.Cursor = i
			break
		}
	}
	return m, nil
}

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
					want := make(map[string]string, len(m.orgs))
					for _, o := range m.orgs {
						want[targetArg(o)] = o.InstanceURL
					}
					sf.ReconcileRESTClients(want)
					// Free the in-memory orgData for orgs that vanished
					// from the LIVE list (logged out via the CLI in
					// another terminal). This prevents stale cursors and lists
					// from surviving a later re-auth. Live-only: a stale cache load
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
					if pinned == "" || pinnedFound || !msg.FromCache {
						m.pinnedDefaultRestored = true
					}
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
					if !found && len(m.orgs) > 0 {
						(&m).setSelectedOrg(0)
					}
				}
				if m.selected >= len(m.orgs) {
					(&m).setSelectedOrg(0)
				} else if m.selectedUsername == "" && len(m.orgs) > 0 {
					(&m).setSelectedOrg(m.selected)
				}
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
		d.OrgInfo.Apply(msg)
		if msg.FromCache {
			refresh = d.OrgInfo.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "networks":
		if d.Networks.Apply(msg) {
		}
		if msg.FromCache {
			refresh = d.Networks.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "permsets":
		landed := d.PermissionSets.Apply(msg)
		if msg.FromCache {
			refresh = d.PermissionSets.MaybeRefreshAfterCacheLoad(m.cache)
		}
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
			(&m).autocompleteInvalidate()
		}
		if msg.FromCache {
			refresh = d.SObjects.MaybeRefreshAfterCacheLoad(m.cache)
		}
	case "deploys_v2":
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
		if handled, r := m.routeListResource(d, msg); handled {
			refresh = r
		}
	}
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
