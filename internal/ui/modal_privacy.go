package ui

import (
	tea "charm.land/bubbletea/v2"

	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
)

func (m *Model) openPrivacyModal() tea.Cmd {
	version, acceptedAt := "", ""
	if m.settings != nil {
		version, acceptedAt = m.settings.LegalAcceptance()
	}
	status := "not accepted"
	if m.settings != nil && m.settings.LegalAccepted(productlegal.PolicyVersion) {
		status = "accepted " + acceptedAt
	} else if version != "" {
		status = "previous revision " + version + " accepted"
	}
	rows := []infoRow{
		{Label: "Policy", Body: productlegal.PolicyVersion + " · " + status},
		{Label: "Privacy", Body: productlegal.PrivacyURL},
		{Label: "Terms", Body: productlegal.TermsURL},
		{},
		{Body: "sf-deck has no hosted backend, account system, analytics, or application telemetry."},
		{Body: "The developer does not receive your Salesforce org data or credentials."},
		{Body: "Record lists, record details, SOQL/report results, and list-view rows stay in process memory and are not persistently cached."},
		{Body: "Metadata caches, settings, saved query text/history, saved Apex/history, projects, tags, and logs may be stored locally under ~/.sf-deck."},
		{},
		{Label: "Inspect", Body: "sf-deck data inspect"},
		{Label: "Erase", Body: "Close sf-deck, then run: sf-deck data erase --yes"},
		{Label: "Bundles", Body: "Add --include-bundles to remove the default ~/sf-deck-bundles directory."},
		{Label: "Disconnect", Body: "sf-deck org logout --org <alias> --yes (or sf org logout --target-org <alias>)"},
		{Body: "Exports and bundles at custom paths remain at the paths you selected."},
	}
	m.showInfoModal(infoModalState{
		Title: "Privacy & local data",
		Rows:  rows,
		OnDismiss: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "__root__"} }
		},
	})
	return nil
}
