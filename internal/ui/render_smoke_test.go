package ui

// Render smoke tests.
//
// Iterates every registered Tab + subtab and asserts that calling
// View() on a minimal Model doesn't panic. The actual rendered
// bytes aren't inspected — that would either need golden files
// (brittle on layout tweaks) or a real terminal harness.
// "Doesn't panic" is the floor: any change that breaks rendering
// for any tab is caught at unit-test time, not by a user noticing
// the TTY freeze on switching tabs.
//
// This is the safety net that should have existed before any of
// the recent listSurface refactors. Add new tabs / subtabs to the
// registry and they're automatically covered here — no per-tab
// test boilerplate needed.

import (
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/cache"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// TestRenderEveryTab walks the TabSpec registry, sets the model
// to each tab in turn, and calls View(). A panic anywhere in the
// render tree fails the test naming the offending tab + subtab.
//
// Two model states per tab so we cover the common branches:
//
//  1. No org loaded — exercises "no org selected" guards.
//  2. One org loaded with empty orgData — exercises "list empty,
//     data not yet fetched" branches.
//
// We don't fake fetched-data states (would need to hand-build full
// Resource[T] bodies for each tab); the empty-state coverage
// catches the bulk of nil-dereferencing risk.
func TestRenderEveryTab(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		setup func(*Model)
	}{
		{name: "no_org", setup: func(m *Model) {}},
		{
			name: "one_org_empty_data",
			setup: func(m *Model) {
				m.orgs = []sf.Org{{
					Alias:       "smoke",
					Username:    "smoke@smoke.test",
					InstanceURL: "https://smoke.my.salesforce.com",
					Status:      "Connected",
					LastUsed:    time.Now().Format(time.RFC3339),
				}}
				_ = m.ensureOrgData("smoke@smoke.test")
			},
		},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			specs := tabSpecs()
			for tab, spec := range specs {
				tab := tab
				spec := spec
				t.Run(tab.String(), func(t *testing.T) {
					if len(spec.Subtabs) == 0 {
						runRenderForTab(t, scenario.setup, tab, -1)
						return
					}
					for i, sub := range spec.Subtabs {
						i := i
						sub := sub
						t.Run(string(sub.ID), func(t *testing.T) {
							runRenderForTab(t, scenario.setup, tab, i)
						})
					}
				})
			}
		})
	}
}

func TestRenderStashesCompositorHitTargets(t *testing.T) {
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	m.width, m.height = 160, 50
	m.focus = focusMain
	_ = m.viewImpl()

	for _, id := range []string{zoneTabID(TabHome), zoneNavTags, zoneNavDevProjects} {
		if !compositorHitsID(m, id) {
			t.Fatalf("expected compositor hit target %q after render", id)
		}
	}
}

// runRenderForTab constructs a fresh Model, applies the scenario
// setup, sets the tab + subtab, and calls View(). subIdx < 0 means
// "leave subtab at default."
//
// Captures any panic via the View()-level recover (which logs but
// doesn't propagate); we additionally recover here so the test
// runtime sees the panic + names the tab. View() will return the
// panic-fallback frame; we don't assert on its contents — just
// that we got *a* View back.
func runRenderForTab(t *testing.T, setup func(*Model), tab Tab, subIdx int) {
	t.Helper()
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	defer c.Close()

	m := New(c)
	setup(&m)
	m.width, m.height = 200, 60
	m.setTab(tab)

	if subIdx >= 0 && len(m.orgs) > 0 {
		spec := lookupTabSpec(tab)
		if spec != nil && spec.SetSubtabIdx != nil && subIdx < len(spec.Subtabs) {
			spec.SetSubtabIdx(&m, subIdx)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic rendering tab=%v subIdx=%d: %v", tab, subIdx, r)
		}
	}()

	// Bypass View()'s defer-recover wrapper so a render panic
	// propagates here and fails the test loudly. The wrapper exists
	// to give end users a safe fallback frame at runtime; tests want
	// the opposite guarantee — surface every panic.
	for _, focusVal := range []focus{focusOrgs, focusMain} {
		m.focus = focusVal
		_ = m.viewImpl()
	}
}

func compositorHitsID(m Model, id string) bool {
	if m.lastCompositor == nil {
		return false
	}
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			if m.lastCompositor.Hit(x, y).ID() == id {
				return true
			}
		}
	}
	return false
}
