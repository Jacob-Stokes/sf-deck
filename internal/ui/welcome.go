package ui

// First-launch welcome. On a user's very first TUI launch (tracked by
// the persistent settings flag ui.welcome_seen), a blocking choice modal
// introduces sf-deck and offers two opt-in paths: import a fully-seeded
// demo org, or start the guided walkthrough. The flag is set the moment
// the modal is shown, so it fires exactly once regardless of what the
// user picks (or if they skip).
//
// Phase 1 wires the trigger + modal + copy; the two action values
// (import demo / start walkthrough) are dispatched as messages so later
// phases can grow real behaviour behind them without touching this file.

import (
	tea "charm.land/bubbletea/v2"
)

// welcome action values — the choiceOption.Value carried through
// OnSuccessTyped when the user picks a row.
const (
	welcomeActionDemo        = "demo"
	welcomeActionWalkthrough = "walkthrough"
	welcomeActionSkip        = "skip"
)

// welcomeModalMsg is the self-message Init emits on first launch to open
// the welcome modal from Update (which has the pointer receiver
// openChoiceModal needs). Init runs on a value receiver and can't mutate
// m.choiceModal directly.
type welcomeModalMsg struct{}

// welcomeTriggerCmd returns a command that opens the welcome modal, or
// nil when it shouldn't fire. Called from Init. Fires only on a genuine
// first launch: the settings flag is unset AND we're not in --demo mode
// (demo already IS the tour-world; a welcome-to-demo modal is noise).
func (m Model) welcomeTriggerCmd() tea.Cmd {
	if Demo {
		return nil
	}
	// Debug override: [ui.debug] force_welcome shows the modal on every
	// launch so it can be tested without resetting welcome_seen.
	if m.settings.DebugForceWelcome() {
		return func() tea.Msg { return welcomeModalMsg{} }
	}
	if m.settings.WelcomeSeen() {
		return nil
	}
	return func() tea.Msg { return welcomeModalMsg{} }
}

// applyWelcomeModal opens the first-launch welcome modal and marks it
// seen. Marking seen here (on show, not on dismiss) guarantees the modal
// never re-appears even if the process dies before the user picks —
// showing it once is the contract, and a user who quits immediately
// shouldn't be nagged again.
func (m Model) applyWelcomeModal() (Model, tea.Cmd) {
	// Don't burn the once-only flag when the debug force toggle is on —
	// otherwise the first forced run would set welcome_seen and the
	// toggle would be the only thing keeping it appearing, which is
	// confusing. With force off, mark seen so it never reappears.
	if !m.settings.DebugForceWelcome() {
		m.settings.SetWelcomeSeen(true)
		_ = m.settings.Save()
	}

	demoLabel := "Import the demo org"
	demoHint := "Adds a fully-populated fictional org you can explore — no real org needed."
	if m.settings.DemoOrgImported() {
		demoLabel = "Demo org already imported"
		demoHint = "The demo org is in your org list. Switch to it to explore."
	}

	state := choiceModalState{
		Title: "Welcome to sf-deck",
		Hint:  "A keyboard-driven terminal UI for your Salesforce orgs · Enter to choose · Esc to skip",
		Options: []choiceOption{
			{
				Label: demoLabel,
				Hint:  demoHint,
				Value: welcomeActionDemo,
			},
			{
				Label: "Take the guided walkthrough",
				Hint:  "A short hands-on tour: sf-deck gives you tasks and follows along as you do them.",
				Value: welcomeActionWalkthrough,
			},
			{
				Label:  "Skip — explore on my own",
				Hint:   "Press ? on any screen for the full keymap. You can restart the tour later.",
				Value:  welcomeActionSkip,
				Cancel: true,
			},
		},
		OnSuccessTyped: func(val any) tea.Cmd {
			s, _ := val.(string)
			return func() tea.Msg { return welcomeActionMsg{action: s} }
		},
	}
	cmd := m.openChoiceModal(state)
	return m, cmd
}

// welcomeActionMsg carries the user's welcome-modal choice back into
// Update, where each action dispatches to its handler. Kept as a message
// (rather than calling handlers inline from the OnSuccessTyped closure)
// so the choice modal fully closes before the next surface opens.
type welcomeActionMsg struct{ action string }

// applyWelcomeAction dispatches the chosen welcome path. Phase 1 stubs
// the two real actions to flashes; phases 2 and 3 replace these arms
// with demo-org import and the walkthrough launch respectively.
func (m Model) applyWelcomeAction(msg welcomeActionMsg) (Model, tea.Cmd) {
	switch msg.action {
	case welcomeActionDemo:
		if m.settings.DemoOrgImported() {
			m.flash("Demo org already imported — switch to it in the org panel.")
			return m, nil
		}
		m.importDemoOrg()
		return m, nil
	case welcomeActionWalkthrough:
		m.startWalkthrough()
		return m, nil
	default:
		// Skip / esc: nothing to do; the flag is already set.
		return m, nil
	}
}
