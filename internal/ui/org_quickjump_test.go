package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestOrgQuickJumpQSelectsFirstOrgBeforeChordLeader(t *testing.T) {
	orgs := []sf.Org{
		{Alias: "dev", Username: "dev@example.com", IsSandbox: true, Status: "Connected"},
		{Alias: "uat", Username: "uat@example.com", IsSandbox: true, Status: "Connected"},
	}
	m := Model{
		modelServices: modelServices{settings: &settings.Settings{}},
		modelOrgs: modelOrgs{
			orgs:     orgs,
			selected: 1,
			data:     make(map[string]*orgData),
		},
		modelRuntime: modelRuntime{
			focus:              focusOrgs,
			orgQuickJumpActive: true,
		},
	}

	nextModel, _ := m.handleKey(kp("q"))
	next := nextModel.(Model)
	if next.selected != 0 {
		t.Fatalf("selected = %d, want first org", next.selected)
	}
	if next.chordActive {
		t.Fatal("q while org quick-jump is armed must not enter chord mode")
	}
	if next.orgQuickJumpActive {
		t.Fatal("org quick-jump should close after a selection")
	}
}
