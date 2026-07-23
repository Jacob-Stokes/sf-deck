package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/buildinfo"
	"github.com/Jacob-Stokes/sf-deck/internal/updatecheck"
)

type updateCheckMsg struct {
	result updatecheck.Result
	err    error
	manual bool
}

// updateCheckCmd starts release discovery without blocking first paint. Auto
// checks are skipped in demo/development builds and when disabled; a manual
// check is allowed from a development build but demo remains strictly
// no-network.
func (m *Model) updateCheckCmd(manual bool) tea.Cmd {
	if m == nil || m.updates == nil {
		return nil
	}
	info := buildinfo.Current()
	if Demo {
		if manual {
			return func() tea.Msg {
				return updateCheckMsg{
					err:    errors.New("update checks are disabled in demo mode (demo makes no network calls)"),
					manual: true,
				}
			}
		}
		return nil
	}
	if !manual {
		if info.IsDevelopment() || m.settings == nil || !m.settings.AutomaticUpdateChecks() {
			return nil
		}
	}
	if manual {
		m.updateChecking = true
	}
	checker := m.updates
	return func() tea.Msg {
		result, err := checker.Check(context.Background(), info.Version,
			updatecheck.Options{Force: manual})
		return updateCheckMsg{result: result, err: err, manual: manual}
	}
}

func (m *Model) applyUpdateCheck(msg updateCheckMsg) tea.Cmd {
	m.updateChecking = false
	m.updateChecked = true
	m.updateErr = ""
	if msg.err != nil {
		m.updateErr = msg.err.Error()
	} else {
		m.updateResult = msg.result
	}
	if !msg.manual {
		return nil // automatic failures are intentionally silent
	}
	state := m.updateInfoModal(msg.result, msg.err)
	m.showInfoModal(state)
	return nil
}

func (m Model) updateInfoModal(result updatecheck.Result, err error) infoModalState {
	rows := []infoRow{}
	if err != nil {
		rows = append(rows,
			infoRow{Label: "Status", Body: "Check failed"},
			infoRow{Label: "Reason", Body: err.Error()},
			infoRow{},
			infoRow{Body: "Nothing was downloaded or installed."},
		)
	} else {
		rows = append(rows,
			infoRow{Label: "Current", Body: displayUpdateVersion(result.CurrentVersion)},
		)
		if result.LatestVersion != "" {
			rows = append(rows, infoRow{Label: "Latest stable", Body: result.LatestVersion})
		}
		switch {
		case result.DevelopmentBuild:
			rows = append(rows, infoRow{Label: "Status", Body: "Development build — comparison is informational"})
		case result.NoStableRelease:
			rows = append(rows, infoRow{Label: "Status", Body: "No stable release has been published"})
		case result.UpdateAvailable:
			kind := result.Kind
			if kind == "" {
				kind = "stable"
			}
			rows = append(rows,
				infoRow{Label: "Status", Body: strings.ToUpper(kind[:1]) + kind[1:] + " update available"},
				infoRow{Label: "Homebrew", Body: "brew upgrade --cask sf-deck"},
			)
		default:
			rows = append(rows, infoRow{Label: "Status", Body: "You are up to date"})
		}
		if !result.PublishedAt.IsZero() {
			rows = append(rows, infoRow{Label: "Published", Body: result.PublishedAt.Local().Format("2 Jan 2006, 15:04 MST")})
		}
		if result.ReleaseURL != "" {
			rows = append(rows, infoRow{Label: "Release page", Body: result.ReleaseURL})
		}
		if !result.CheckedAt.IsZero() {
			source := "GitHub"
			if result.FromCache {
				source = "24-hour cache"
			}
			rows = append(rows, infoRow{Label: "Checked", Body: result.CheckedAt.Local().Format("2 Jan 2006, 15:04 MST") + " · " + source})
		}
		rows = append(rows, infoRow{}, infoRow{Body: "sf-deck only notifies; it never downloads or installs updates."})
	}
	return infoModalState{
		Title: "sf-deck update",
		Rows:  rows,
		OnDismiss: func() tea.Cmd {
			return func() tea.Msg { return openSettingsSubmenuMsg{pick: "updates"} }
		},
	}
}

func displayUpdateVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return buildinfo.Current().DisplayVersion()
	}
	return v
}

func (m Model) updateStatusLabel() string {
	switch {
	case m.updateChecking:
		return "checking…"
	case m.updateErr != "":
		return "last check failed"
	case m.updateResult.UpdateAvailable:
		return fmt.Sprintf("%s available (%s)", m.updateResult.LatestVersion, m.updateResult.Kind)
	case m.updateChecked && m.updateResult.DevelopmentBuild:
		return "development build"
	case m.updateChecked && m.updateResult.NoStableRelease:
		return "no stable release published"
	case m.updateChecked:
		return "up to date"
	case m.settings != nil && !m.settings.AutomaticUpdateChecks():
		return "automatic checks off"
	default:
		return "not checked yet"
	}
}

func (m Model) updateNoticeText() string {
	if !m.updateResult.UpdateAvailable {
		return ""
	}
	return fmt.Sprintf("↑ %s sf-deck update available: %s · Settings → Updates",
		m.updateResult.Kind, m.updateResult.LatestVersion)
}
