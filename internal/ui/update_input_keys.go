package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/soqlfmt"
)

// handleSOQLKey is the key handler while the SOQL editor is active.
// Mode-internal keys (enter/esc/ctrl+c) stay hardcoded; everything
// else forwards to the bubbles/textinput widget so the user gets
// cursor nav, word jumps, home/end, etc.
func (m Model) handleSOQLKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	mm := m
	return (&mm).handleSOQLSessionEditKey(msg, &mm.soqlSession, soqlSessionTab)
}

func (m *Model) handleSOQLSessionEditKey(msg tea.KeyMsg, s *soqlSession, target soqlSessionTarget) (Model, tea.Cmd) {
	if s == nil {
		return *m, nil
	}
	key := msg.String()

	// Autocomplete popup gets first crack at navigation keys.
	// Tab accepts the highlighted suggestion; up/down cycle; esc
	// dismisses the popup (without exiting edit mode). When the
	// popup consumes the key we return early so the textinput
	// doesn't also process it.
	if consumed, cmd := m.autocompleteKey(s, key); consumed {
		// Tab inserted text — refresh suggestions for the new
		// cursor position. up/down/esc don't change the buffer
		// but refresh is cheap (memo-skipped on no change).
		m.autocompleteRefresh(s)
		return *m, tea.Batch(cmd, m.drainAutocompleteCmds())
	}

	switch key {
	case "esc":
		s.soqlEditing = false
		s.soqlInput.Blur()
		if s.autocomplete != nil {
			s.autocomplete.Items = nil
		}
		return *m, nil
	case "ctrl+l":
		// Pretty-print: reflow the buffer into clause-per-line
		// shape. Idempotent — running twice gives the same result.
		// Cursor lands at the end of the formatted query because
		// preserving the original byte offset after a wholesale
		// reflow is fragile and the user almost always wants to
		// keep editing at the bottom anyway.
		formatted := soqlfmt.Format(s.soqlInput.Value())
		s.soqlInput.SetValue(formatted)
		s.soqlInput.CursorEnd()
		m.autocompleteRefresh(s)
		return *m, m.drainAutocompleteCmds()
	case "ctrl+c":
		return *m, tea.Quit
	case "enter":
		if len(m.orgs) == 0 {
			return *m, nil
		}
		s.soqlEditing = false
		s.soqlRunning = true
		s.soqlErr = nil
		s.soqlInput.Blur()
		if s.autocomplete != nil {
			s.autocomplete.Items = nil
		}
		o := m.orgs[m.selected]
		// Build the cancellable context now so the closure has
		// access AND so the dispatcher can call m.soqlCancel from
		// the ctrl+c handler.  Bump the run generation so any
		// late-arriving result from a previous run gets dropped.
		ctx, cancel := context.WithCancel(context.Background())
		s.soqlCancel = cancel
		s.soqlRunGen++
		return *m, m.runSOQLCmd(o, s.soqlInput.Value(), s.soqlTooling, s.soqlBulk, ctx, s.soqlRunGen, target, s.id)
	}
	newInput, cmd := s.soqlInput.Update(msg)
	s.soqlInput = newInput
	// Recompute suggestions for the post-keystroke buffer + drain
	// any describe-ensure cmds the engine queued (relationship
	// hop touched an uncached sObject).
	m.autocompleteRefresh(s)
	return *m, tea.Batch(cmd, m.drainAutocompleteCmds())
}

// handleRecordEditKey routes keystrokes to the active field
// editor while the user is in /record inline-edit mode. esc /
// enter / tab / ctrl+s have global meaning here (cancel the
// edit, commit, advance to next field, save batch); everything
// else passes through to the editor's HandleKey.
func (m Model) handleRecordEditKey(msg tea.KeyMsg, session *recordEditSession) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		mm := m
		(&mm).cancelCurrentEdit()
		return mm, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "tab":
		mm := m
		(&mm).commitCurrentEdit()
		return mm, nil
	}
	editor := resolveFieldEditor(session.Editing.Field)
	if editor == nil {
		return m, nil
	}
	_, cmd := editor.HandleKey(session.Editing, msg)
	return m, cmd
}

// handleExecKey is the key handler while the /exec textarea is
// active. Mode-internal keys stay hardcoded; everything else
// forwards to the bubbles/textarea widget so the user gets
// multi-line nav (arrow keys move across rows), word jumps,
// home/end, etc. Enter inserts a newline inside the textarea;
// running the snippet requires the user to exit edit mode first
// (esc) and press Enter again — same gesture as /soql but doubled
// because Apex bodies need newlines.
func (m Model) handleExecKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.execEditing = false
		m.execInput.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	newInput, cmd := m.execInput.Update(msg)
	m.execInput = newInput
	return m, cmd
}
