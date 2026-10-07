package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

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
