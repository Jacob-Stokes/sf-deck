package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type soqlModalState struct {
	Title   string
	session soqlSession
}

func (m *Model) openSOQLModal(title, body string) {
	if title == "" {
		title = "SOQL"
	}
	session := newSOQLSession(body)
	session.soqlEditing = true
	session.soqlInput.Focus()
	m.soqlModal = &soqlModalState{
		Title:   title,
		session: session,
	}
	// Populate suggestions immediately so the popup isn't empty
	// on first paint of the modal.
	m.autocompleteRefresh(&m.soqlModal.session)
}

// soqlModalSelectedRecord returns the cursored record in the
// modal's result table, or false when there's nothing selectable.
// Mirrors soqlSelectedRecord but reads from the modal's session.
func (m Model) soqlModalSelectedRecord() (map[string]any, bool) {
	if m.soqlModal == nil {
		return nil, false
	}
	return m.soqlSessionSelectedRecord(&m.soqlModal.session)
}

func (m *Model) closeSOQLModal() {
	if m == nil || m.soqlModal == nil {
		return
	}
	if cancel := m.soqlModal.session.soqlCancel; cancel != nil {
		cancel()
	}
	m.soqlModal = nil
}

func (m Model) renderSOQLModal() string {
	if m.soqlModal == nil {
		return ""
	}
	w := modalWidth(m.width, 80, 140)
	if maxW := m.width - 4; maxW > 0 && w > maxW {
		w = maxW
	}
	h := modalHeight(m.height, 18, 32)
	if maxH := m.height - 4; maxH > 0 && h > maxH {
		h = maxH
	}
	body := m.renderSOQLSessionBody(&m.soqlModal.session, w, h-2, soqlSessionBodyOptions{
		title: m.soqlModal.Title,
		modal: true,
	})
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Cyan).
		Padding(0, 1).
		Width(w - 2).
		Render(body)
}

func (m Model) handleSOQLModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.soqlModal == nil {
		return m, nil
	}
	key := msg.String()
	s := &m.soqlModal.session
	switch {
	case key == "esc":
		(&m).closeSOQLModal()
		return m, nil
	case key == "ctrl+c" && s.soqlRunning && s.soqlCancel != nil:
		s.soqlCancel()
		s.soqlCancel = nil
		s.soqlRunning = false
		s.soqlRunGen++
		m.flash("query cancelled")
		return m, nil
	case key == "ctrl+p":
		m.promoteSOQLModal()
		return m, nil
	}
	if s.soqlEditing {
		return (&m).handleSOQLSessionEditKey(msg, s, soqlSessionModal)
	}
	if search := s.searchPtr(); search != nil && search.Active {
		return m.handleSOQLModalSearchInput(msg, search)
	}
	switch {
	case matches(key, Keys.SOQLEdit):
		s.soqlEditing = true
		s.soqlInput.Focus()
		m.autocompleteRefresh(s)
		return m, nil
	case matches(key, Keys.Drill):
		// Drill into the cursored result row when results are
		// showing. Pushes onto the recordDrillStack so esc on
		// the new record detail pops BACK to the parent record
		// the user was viewing — identical UX to Enter on a
		// reference field inside the record detail itself.
		//
		// The SOQL modal closes; the user can re-spawn it via
		// RELATED Enter on the parent if they want to browse
		// other children.
		if !s.soqlEditing && len(s.soqlResult.Records) > 0 {
			rec, ok := m.soqlModalSelectedRecord()
			if !ok {
				return m, nil
			}
			id, _ := rec["Id"].(string)
			if id == "" {
				m.flash("can't drill — row has no Id (add it to the SELECT)")
				return m, nil
			}
			sobject, _ := recordSObject(rec)
			if sobject == "" {
				m.flash("can't drill — row has no sObject type")
				return m, nil
			}
			name := recordDisplayName(rec)
			(&m).closeSOQLModal()
			mm := m
			cmd := (&mm).drillIntoRelatedRecord(relatedRecordHit{
				SObject: sobject,
				ID:      id,
				Label:   name,
			})
			return mm, cmd
		}
		// Otherwise (editor focused or no results yet) — Enter
		// re-runs the query.
		s.soqlEditing = true
		s.soqlInput.Focus()
		return (&m).handleSOQLSessionEditKey(tea.KeyPressMsg{Code: tea.KeyEnter}, s, soqlSessionModal)
	case matches(key, Keys.SOQLToggleTooling):
		s.soqlTooling = !s.soqlTooling
		if s.soqlTooling && s.soqlBulk {
			s.soqlBulk = false
			m.flash("tooling api: on (bulk off - bulk doesn't support tooling)")
		} else {
			m.flash("tooling api: " + onOff(s.soqlTooling))
		}
		return m, nil
	case matches(key, Keys.SOQLToggleBulk):
		s.soqlBulk = !s.soqlBulk
		if s.soqlBulk && s.soqlTooling {
			s.soqlTooling = false
			m.flash("bulk api: on (tooling off - bulk doesn't support tooling)")
		} else {
			m.flash("bulk api: " + onOff(s.soqlBulk))
		}
		return m, nil
	case matches(key, Keys.MoveDown):
		m.moveSOQLModalCursor(1)
		return m, nil
	case matches(key, Keys.MoveUp):
		m.moveSOQLModalCursor(-1)
		return m, nil
	case matches(key, Keys.JumpDown):
		m.moveSOQLModalCursor(m.jumpRows())
		return m, nil
	case matches(key, Keys.JumpUp):
		m.moveSOQLModalCursor(-m.jumpRows())
		return m, nil
	case matches(key, Keys.PageDown):
		m.moveSOQLModalCursor(pageJump(m.height))
		return m, nil
	case matches(key, Keys.PageUp):
		m.moveSOQLModalCursor(-pageJump(m.height))
		return m, nil
	case matches(key, Keys.GoBottom):
		m.moveSOQLModalCursor(1 << 20)
		return m, nil
	case matches(key, Keys.GoTop):
		m.moveSOQLModalCursor(-(1 << 20))
		return m, nil
	case matches(key, Keys.SearchStart):
		search := s.searchPtr()
		if search != nil {
			search.EnsureInit()
			search.Input.Focus()
			search.Active = true
			search.Committed = false
		}
		return m, nil
	case matches(key, Keys.SearchClear):
		if search := s.searchPtr(); search != nil {
			search.SetBuffer("")
			search.Active = false
			search.Committed = false
			m.resetSOQLModalCursor()
		}
		return m, nil
	case matches(key, Keys.ColScrollL):
		m.scrollSOQLModalColumn(-1)
		return m, nil
	case matches(key, Keys.ColScrollR):
		m.scrollSOQLModalColumn(1)
		return m, nil
	case matches(key, Keys.ColSort):
		m.sortSOQLModalColumn(false)
		return m, nil
	case matches(key, Keys.ColSortClear):
		m.sortSOQLModalColumn(true)
		return m, nil
	}
	return m, nil
}

func (m Model) handleSOQLModalSearchInput(msg tea.KeyMsg, search *searchState) (Model, tea.Cmd) {
	if search == nil {
		return m, nil
	}
	search.EnsureInit()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "enter", "tab":
		search.Active = false
		search.Committed = search.Buffer() != ""
		return m, nil
	case "ctrl+u":
		search.Active = false
		search.Committed = false
		search.SetBuffer("")
		m.resetSOQLModalCursor()
		return m, nil
	}
	before := search.Buffer()
	newInput, cmd := search.Input.Update(msg)
	search.Input = newInput
	if search.Buffer() != before {
		m.resetSOQLModalCursor()
	}
	return m, cmd
}

func (m *Model) scrollSOQLModalColumn(delta int) {
	if m == nil || m.soqlModal == nil {
		return
	}
	s := &m.soqlModal.session
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, s.soqlResult.Records, s.searchPtr(), theme.Current.ID, s.soqlInput.Value())
	if entry == nil || len(entry.listCols) == 0 {
		return
	}
	state := &s.soqlTable
	cur := effectiveColCursor(state, entry.listCols)
	next := cur + delta
	if next < 0 {
		next = 0
	}
	if next >= len(entry.listCols) {
		next = len(entry.listCols) - 1
	}
	state.ColCursor = next
	width := modalWidth(m.width, 80, 140)
	if maxW := m.width - 4; maxW > 0 && width > maxW {
		width = maxW
	}
	ensureColCursorVisible(state, entry.listCols, width-4)
}

func (m *Model) moveSOQLModalCursor(delta int) {
	if m == nil || m.soqlModal == nil {
		return
	}
	s := &m.soqlModal.session
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, s.soqlResult.Records, s.searchPtr(), theme.Current.ID, s.soqlInput.Value())
	if entry == nil || len(entry.filtered) == 0 {
		s.soqlRowCur = 0
		return
	}
	soqlSessionTableAdapter(s, entry).MoveDisplay(delta)
}

func (m *Model) sortSOQLModalColumn(clear bool) {
	if m == nil || m.soqlModal == nil {
		return
	}
	s := &m.soqlModal.session
	if clear {
		s.soqlTable.SortColumn = ""
		s.soqlTable.SortDesc = false
		m.resetSOQLModalCursor()
		return
	}
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, s.soqlResult.Records, s.searchPtr(), theme.Current.ID, s.soqlInput.Value())
	if entry == nil || len(entry.listCols) == 0 {
		return
	}
	col := s.soqlTable.ColCursor
	if col < 0 {
		col = 0
	}
	if col >= len(entry.listCols) {
		col = len(entry.listCols) - 1
	}
	name := entry.listCols[col].Name
	if s.soqlTable.SortColumn == name {
		s.soqlTable.SortDesc = !s.soqlTable.SortDesc
	} else {
		s.soqlTable.SortColumn = name
		s.soqlTable.SortDesc = false
	}
	m.resetSOQLModalCursor()
}

func (m *Model) resetSOQLModalCursor() {
	if m == nil || m.soqlModal == nil {
		return
	}
	s := &m.soqlModal.session
	d, _ := m.activeOrgState()
	entry := soqlProjectionFor(d, s.soqlResult.Records, s.searchPtr(), theme.Current.ID, s.soqlInput.Value())
	if entry == nil || len(entry.filtered) == 0 {
		s.soqlRowCur = 0
		return
	}
	soqlSessionTableAdapter(s, entry).ResetDisplayTop()
}

func (m *Model) promoteSOQLModal() {
	if m == nil || m.soqlModal == nil {
		return
	}
	if m.soqlModal.session.soqlRunning {
		m.flash("wait for query to finish before promoting")
		return
	}
	m.soqlSession = m.soqlModal.session
	m.soqlModal = nil
	m.soqlSubtabIdx = 0
	m.focus = focusMain
	m.setTab(TabSOQL)
}
