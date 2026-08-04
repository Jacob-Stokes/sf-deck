package ui

import (
	"fmt"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func aboutSettingsHint() string {
	info := buildinfo.Current()
	return info.DisplayVersion() + " · Apache-2.0 · Jacob Stokes"
}

func (m *Model) openUpdatesModal() tea.Cmd {
	if m.settings == nil {
		return nil
	}
	autoHint := "current: on · anonymous GitHub request at most daily"
	if !m.settings.AutomaticUpdateChecks() {
		autoHint = "current: off · manual checks still available"
	}
	if settings.UpdateChecksDisabledByEnv() {
		autoHint = "off for this process via SF_DECK_NO_UPDATE_CHECK"
	}
	checkHint := m.updateStatusLabel()
	if m.updateChecking {
		checkHint = "already checking…"
	}
	opts := []choiceOption{
		{Label: "Automatic stable-release checks", Hint: autoHint, Value: "automatic"},
		{Label: "Check now", Hint: checkHint + " · bypasses the daily cache", Value: "check_now"},
	}
	return m.settingsSubmenu("Updates", "updates", opts)
}

func (m *Model) openAboutModal() tea.Cmd {
	info := buildinfo.Current()
	updateStatus := m.updateStatusLabel()
	if m.settings != nil {
		auto := "on"
		if !m.settings.AutomaticUpdateChecks() {
			auto = "off"
		}
		updateStatus += " · automatic checks " + auto
	}
	rows := []infoRow{
		{Label: "Product", Body: "sf-deck"},
		{Label: "Version", Body: info.DisplayVersion()},
		{Label: "Commit", Body: info.ShortCommit()},
		{Label: "Built", Body: info.Date},
		{Label: "Platform", Body: runtime.GOOS + "/" + runtime.GOARCH},
		{Label: "Go", Body: runtime.Version()},
		{Label: "Updates", Body: updateStatus},
		{},
		{Label: "Author", Body: "Jacob Stokes"},
		{Label: "Copyright", Body: "© 2026 Jacob Stokes"},
		{Label: "License", Body: "Apache License 2.0"},
		{Label: "Source", Body: "https://github.com/Jacob-Stokes/sf-deck"},
		{Label: "Website", Body: "https://sfdeck.dev"},
		{Label: "Security", Body: "hello@jacobstokes.com"},
		{Label: "Privacy", Body: productlegal.PrivacyURL},
		{Label: "User terms", Body: productlegal.TermsURL},
		{},
		{Body: "Salesforce is a registered trademark of Salesforce, Inc."},
		{Body: "sf-deck is not affiliated with, endorsed by, or sponsored by Salesforce."},
		{},
		{Body: "No hosted backend, telemetry, or analytics. The developer does not receive Salesforce org data."},
		{Body: "Salesforce record payloads stay in memory and are not written to the persistent cache."},
		{Body: "Update checks are anonymous, cached, optional, and never install software."},
	}
	if m.updateResult.UpdateAvailable {
		rows = append(rows, infoRow{
			Label: "Available",
			Body: fmt.Sprintf("%s (%s) · %s",
				m.updateResult.LatestVersion, m.updateResult.Kind, m.updateResult.ReleaseURL),
		})
	}
	m.showInfoModal(infoModalState{
		Title: "About sf-deck",
		Rows:  rows,
		OnDismiss: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "__root__"} }
		},
	})
	return nil
}
