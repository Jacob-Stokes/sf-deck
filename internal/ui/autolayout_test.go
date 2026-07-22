package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

// TestApplyStartupAutoLayout: sidebar placement is now driven by the
// SidebarPosition setting (applied at construction), not by a
// width-based decision here. applyStartupAutoLayout is a dormant
// one-shot hook — it latches but never moves the sidebar ("auto" is
// reserved / coming soon). These tests pin that it doesn't touch
// sidebarStacked and still latches / defers correctly.
func TestApplyStartupAutoLayout(t *testing.T) {
	mk := func(width int, stacked, open bool) *Model {
		m := &Model{}
		m.settings = &settings.Settings{}
		m.width = width
		m.sidebarStacked = stacked
		m.sidebarOpen = open
		return m
	}

	t.Run("does not move a beside sidebar", func(t *testing.T) {
		m := mk(100, false, true) // narrow, but no width-based restack anymore
		m.applyStartupAutoLayout()
		if m.sidebarStacked {
			t.Fatal("applyStartupAutoLayout must not stack the sidebar (position-driven now)")
		}
	})
	t.Run("does not move a stacked sidebar", func(t *testing.T) {
		m := mk(200, true, true) // wide, but a bottom preference must stand
		m.applyStartupAutoLayout()
		if !m.sidebarStacked {
			t.Fatal("applyStartupAutoLayout must not un-stack the sidebar")
		}
	})
	t.Run("one-shot latch", func(t *testing.T) {
		m := mk(120, false, true)
		m.applyStartupAutoLayout()
		if !m.startupLayoutDone {
			t.Fatal("first pass with a real width should latch")
		}
	})
	t.Run("zero width = defer", func(t *testing.T) {
		m := mk(0, false, true)
		m.applyStartupAutoLayout()
		if m.startupLayoutDone {
			t.Fatal("zero width should defer the latch, not consume it")
		}
	})
}

// TestSidebarStartsStacked pins the position → boot-time stacked flag
// mapping: only "bottom" stacks; rhs and auto start beside (auto is a
// no-op today).
func TestSidebarStartsStacked(t *testing.T) {
	cases := []struct {
		pos  string
		want bool
	}{
		{settings.SidebarPositionRHS, false},
		{settings.SidebarPositionBottom, true},
		{settings.SidebarPositionAuto, false}, // coming soon → defaults to beside
		{"", false},                           // unset → RHS
	}
	for _, c := range cases {
		s := &settings.Settings{}
		s.SetSidebarPosition(c.pos)
		if got := s.SidebarStartsStacked(); got != c.want {
			t.Errorf("position %q: SidebarStartsStacked = %v, want %v", c.pos, got, c.want)
		}
	}
}
