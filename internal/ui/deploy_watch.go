package ui

// /deploys live watch — while any row in the cached window is
// Pending / InProgress / Canceling, poll every few seconds so the
// status, counters, and duration tick over without the user mashing
// r. Self-stopping: the tick only re-arms while in-flight rows
// remain, mirroring the export activity tick's single-flight shape.

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type deployWatchTickMsg struct{}

// hasInFlightDeploys reports whether the org's cached deploy window
// holds any non-terminal rows.
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

// deployWatchTickCmd arms one watch tick (single-flight).
func (m *Model) deployWatchTickCmd() tea.Cmd {
	if m.deployWatchRunning {
		return nil
	}
	d := m.activeOrgData()
	if !hasInFlightDeploys(d) {
		return nil
	}
	m.deployWatchRunning = true
	// Settings-backed cadence ([ui.api] deploy_watch_sec, default 5s) —
	// read per arm so a settings change applies to the very next tick.
	interval := time.Duration(m.settings.APIDeployWatchSec()) * time.Second
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return deployWatchTickMsg{}
	})
}

// applyDeployWatchTick fires a forced Deploys refresh (the fetch
// path re-polls in-flight rows — see refreshInFlightDeploys) and
// the resulting deploys_v2 apply re-arms the next tick if needed.
func (m *Model) applyDeployWatchTick() tea.Cmd {
	m.deployWatchRunning = false
	d := m.activeOrgData()
	if !hasInFlightDeploys(d) {
		return nil
	}
	if Demo {
		// No network to poll — the timed flip IS the deploy
		// finishing. Runs through the same snapshot/flash/re-arm
		// shape as the live path so the tape records the real watch
		// behaviour.
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

// flashFinishedDeploys compares the pre-apply in-flight set with the
// post-apply rows and flashes a completion banner for any deploy
// that just reached a terminal status.
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
