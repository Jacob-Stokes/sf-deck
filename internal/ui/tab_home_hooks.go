package ui

// /home data lifecycle hooks — extracted from inline registry
// closures (2026-06-13 registry-purity pass; see
// tab_registry_purity_test.go for the ratchet that keeps logic out
// of the dispatch table).

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// ensureHomeData batches the always-needed Home payload with its
// co-travellers: Packages drives both the Home/Packages subtab AND
// the /packages top-level tab, so co-fetching means the user pays
// once regardless of which tab they land on first; Notifications is
// always-loaded so the bell-style header pill can show an unread
// count without visiting the subtab. RecentlyViewed lazy-loads only
// on the Recent subtab — the merged stream needs both sources, so
// the SF fetch kicks regardless of which chip is active (local data
// is already in-memory and paints instantly).
func (m *Model) ensureHomeData(d *orgData, _ sf.Org) tea.Cmd {
	var recentCmd tea.Cmd
	if m.currentSubtab() == SubtabHomeRecent {
		recentCmd = d.RecentlyViewed.Ensure(m.cache)
	}
	return tea.Batch(
		d.Home.Ensure(m.cache),
		d.OrgInfo.Ensure(m.cache),
		d.Packages.Ensure(m.cache),
		d.Notifications.Ensure(m.cache),
		recentCmd,
	)
}

// refreshHomeData scopes r to the active subtab — refreshing Limits
// shouldn't blast Notifications + Packages requests too. Limits and
// Licenses both render from d.Home, so one refresh covers both.
func (m Model) refreshHomeData(d *orgData) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabHomeRecent:
		return tea.Batch(
			d.Home.Refresh(m.cache),
			d.RecentlyViewed.Refresh(m.cache),
		)
	case SubtabHomeNotifications:
		return d.Notifications.Refresh(m.cache)
	case SubtabHomeLimits, SubtabHomeLicenses:
		return d.Home.Refresh(m.cache)
	}
	return tea.Batch(
		d.Home.Refresh(m.cache),
		d.OrgInfo.Refresh(m.cache),
		d.Packages.Refresh(m.cache),
		d.Notifications.Refresh(m.cache),
	)
}
