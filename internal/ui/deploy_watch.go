package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type deployWatchTickMsg struct{}

func hasInFlightDeploys(d *orgData) bool {
	if d == nil {
		return false
	}
	for _, r := range d.Deploys.Value() {
		if r.InFlight() {
			return true
		}
	}
	return false
}

func (m *Model) deployWatchTickCmd() tea.Cmd {
	if m.deployWatchRunning {
		return nil
	}
	d := m.activeOrgData()
	if !hasInFlightDeploys(d) {
		return nil
	}
	m.deployWatchRunning = true
	interval := time.Duration(m.settings.APIDeployWatchSec()) * time.Second
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return deployWatchTickMsg{}
	})
}

func (m *Model) applyDeployWatchTick() tea.Cmd {
	m.deployWatchRunning = false
	d := m.activeOrgData()
	if !hasInFlightDeploys(d) {
		return nil
	}
	if Demo {
		before := map[string]bool{}
		for _, r := range d.Deploys.Value() {
			if r.InFlight() {
				before[r.ID] = true
			}
		}
		d.Deploys.Set(demoFlipInFlightDeploys(d.Deploys.Value()))
		d.SyncDeploysList()
		m.flashFinishedDeploys(before, d)
		return m.deployWatchTickCmd()
	}
	return d.Deploys.Refresh(m.cache)
}

func (m *Model) flashFinishedDeploys(before map[string]bool, d *orgData) {
	if len(before) == 0 {
		return
	}
	for _, r := range d.Deploys.Value() {
		if !before[r.ID] || r.InFlight() {
			continue
		}
		label := "deploy"
		if r.CheckOnly {
			label = "validation"
		}
		m.flash(label + " " + shortDeployStatus(r.Status) + " · " +
			r.CreatedByName + " · " + deployDurationLabel(r))
	}
}

func shortDeployStatus(s string) string {
	switch s {
	case "Succeeded":
		return "succeeded ✓"
	case "SucceededPartial":
		return "partially succeeded"
	case "Failed":
		return "FAILED ✗"
	case "Canceled":
		return "canceled"
	}
	return s
}

var _ = sf.DeployRow{}
