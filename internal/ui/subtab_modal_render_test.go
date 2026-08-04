package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestSubtabStripSurvivesInfoModal guards the regression where opening
// the inspect (i) info modal blanked the subtab strip: the main-hit
// layers used to draw opaque spaces over the strip when the compositor
// path ran (modal up). Home has subtabs, so its strip labels must
// still appear in the composited frame with the info modal open.
func TestSubtabStripSurvivesInfoModal(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	// A connected org so Home renders its subtab strip rather than the
	// no-orgs onboarding panel.
	m.orgs = []sf.Org{{
		Alias:       "smoke",
		Username:    "smoke@smoke.test",
		InstanceURL: "https://smoke.my.salesforce.com",
		Status:      "Connected",
		LastUsed:    time.Now().Format(time.RFC3339),
	}}
	_ = m.ensureOrgData("smoke@smoke.test")
	m.width, m.height = 200, 60
	m.focus = focusMain
	m.setTab(TabHome)

	// Baseline: strip visible without a modal.
	base := m.viewImpl().Content
	if !strings.Contains(base, "Landing") || !strings.Contains(base, "Notifications") {
		t.Fatalf("baseline home render missing subtab labels; got:\n%s", base)
	}

	// Open the inspect modal (forces the compositor overlay path).
	if modal, ok := m.inspectModalForCurrentView(); ok {
		m.showInfoModal(modal)
	} else {
		// Home has a sidebar, so this should always succeed; if it
		// doesn't the test can't exercise the path.
		t.Skip("home produced no inspect modal")
	}
	withModal := m.viewImpl().Content

	// The subtab strip must still be present under/around the modal.
	if !strings.Contains(withModal, "Landing") {
		t.Errorf("subtab strip label 'Landing' vanished with info modal open")
	}
	if !strings.Contains(withModal, "Notifications") {
		t.Errorf("subtab strip label 'Notifications' vanished with info modal open")
	}
}
