package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestSetSelectedOrgKeepsFieldsInSync proves the helper used by every
// org-switch path mirrors the index and username into one consistent
// state. Without this the re-anchor logic in the orgs Apply branch
// can't find the org again after a reorder.
func TestSetSelectedOrgKeepsFieldsInSync(t *testing.T) {
	m := &Model{}
	m.orgs = []sf.Org{
		{Username: "u@a", Alias: "alpha"},
		{Username: "u@b", Alias: "bravo"},
		{Username: "u@c", Alias: "charlie"},
	}
	m.setSelectedOrg(1)
	if m.selected != 1 || m.selectedUsername != "u@b" {
		t.Errorf("setSelectedOrg(1) -> selected=%d username=%q, want 1 / u@b",
			m.selected, m.selectedUsername)
	}

	m.setSelectedOrg(2)
	if m.selected != 2 || m.selectedUsername != "u@c" {
		t.Errorf("setSelectedOrg(2) -> selected=%d username=%q, want 2 / u@c",
			m.selected, m.selectedUsername)
	}

	// Out-of-range is a safe no-op (doesn't desync the two fields).
	prev := m.selectedUsername
	m.setSelectedOrg(99)
	if m.selectedUsername != prev {
		t.Errorf("out-of-range setSelectedOrg should be a no-op; got username=%q want %q",
			m.selectedUsername, prev)
	}
	m.setSelectedOrg(-1)
	if m.selectedUsername != prev {
		t.Errorf("negative setSelectedOrg should be a no-op; got username=%q want %q",
			m.selectedUsername, prev)
	}
}

// TestSelectedUsernameSurvivesReorder simulates what the orgs Apply
// branch does: when m.orgs gets a new ordering between fetches, the
// integer index should re-point to the SAME org by username. The
// re-anchor logic is small and inline so we exercise it by hand here.
func TestSelectedUsernameSurvivesReorder(t *testing.T) {
	m := &Model{}
	m.orgs = []sf.Org{
		{Username: "u@a"},
		{Username: "u@b"}, // user is on this one
		{Username: "u@c"},
	}
	m.setSelectedOrg(1)
	if m.selectedUsername != "u@b" {
		t.Fatalf("setup: want u@b, got %q", m.selectedUsername)
	}

	// Simulate a refetch where 'sf org list' returned a different order
	// (e.g. LastUsed-driven shuffle in the old behaviour, or any future
	// stable-but-different sort).
	m.orgs = []sf.Org{
		{Username: "u@c"},
		{Username: "u@a"},
		{Username: "u@b"}, // u@b moved from index 1 -> 2
	}
	// Re-anchor logic mirrors update.go's orgs Apply branch.
	for i, o := range m.orgs {
		if o.Username == m.selectedUsername {
			m.selected = i
			break
		}
	}
	if m.selected != 2 {
		t.Errorf("after reorder, selected should follow u@b to its new index 2; got %d", m.selected)
	}
	if m.selectedUsername != "u@b" {
		t.Errorf("selectedUsername should NOT change on a reorder; got %q", m.selectedUsername)
	}
}
