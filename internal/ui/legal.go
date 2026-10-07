package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
)

type legalModalMsg struct{}
type legalAcceptedMsg struct{}

func (m Model) legalAccepted() bool {
	return Demo || (m.settings != nil && m.settings.LegalAccepted(productlegal.PolicyVersion))
}

func (m Model) legalTriggerCmd() tea.Cmd {
	if m.legalAccepted() {
		return nil
	}
	return func() tea.Msg { return legalModalMsg{} }
}

// applyLegalModal blocks real-org discovery until the user acknowledges the
// current privacy notice and user agreement. The copy is deliberately factual:
// sf-deck runs locally, never receives org data centrally, and keeps record
// payloads in memory rather than in its persistent cache.
func (m Model) applyLegalModal() (Model, tea.Cmd) {
	state := choiceModalState{
		Title: "Before sf-deck connects",
		Hint: "sf-deck has no hosted backend or telemetry; the developer does not receive your org data.\n" +
			"Record payloads stay in memory and are not written to the persistent cache.\n" +
			"Metadata and user-authored working state may be stored locally under ~/.sf-deck.\n" +
			"Continue only for orgs you are authorized to access.\n" +
			"Privacy: " + productlegal.PrivacyURL + "\n" +
			"Terms: " + productlegal.TermsURL,
		Wide: true,
		Options: []choiceOption{
			{
				Label: "Accept and continue",
				Hint:  "I agree to the user agreement and acknowledge the privacy notice.",
				Value: "accept",
			},
			{
				Label: "Quit without connecting",
				Hint:  "No Salesforce org will be discovered or contacted.",
				Value: "quit",
			},
		},
		Save: func(val any) error {
			if val != "accept" || m.settings == nil {
				return nil
			}
			m.settings.AcceptLegal(productlegal.PolicyVersion, time.Now())
			return m.settings.Save()
		},
		OnSuccessTyped: func(val any) tea.Cmd {
			if val == "accept" {
				return func() tea.Msg { return legalAcceptedMsg{} }
			}
			return tea.Quit
		},
		OnCancel: func() tea.Cmd { return tea.Quit },
	}
	return m, m.openChoiceModal(state)
}

func (m Model) applyLegalAccepted() (Model, tea.Cmd) {
	return m, m.runtimeStartupCmd()
}
