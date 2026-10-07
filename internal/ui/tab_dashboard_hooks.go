package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func (m *Model) ensureApexData(d *orgData, _ sf.Org) tea.Cmd {
	if m.currentSubtab() == SubtabApexTriggers {
		return d.ApexTriggersFlat.Ensure(m.cache)
	}
	return d.ApexClasses.Ensure(m.cache)
}

func (m Model) refreshApexData(d *orgData) tea.Cmd {
	if m.currentSubtab() == SubtabApexTriggers {
		return d.ApexTriggersFlat.Refresh(m.cache)
	}
	return d.ApexClasses.Refresh(m.cache)
}

func (m *Model) ensureComponentsData(d *orgData, _ sf.Org) tea.Cmd {
	if m.currentSubtab() == SubtabComponentsAura {
		return d.AuraBundles.Ensure(m.cache)
	}
	return d.LWCBundles.Ensure(m.cache)
}

func (m Model) refreshComponentsData(d *orgData) tea.Cmd {
	if m.currentSubtab() == SubtabComponentsAura {
		return d.AuraBundles.Refresh(m.cache)
	}
	return d.LWCBundles.Refresh(m.cache)
}

func (m *Model) ensurePermsDashboardData(d *orgData, _ sf.Org) tea.Cmd {
	return tea.Batch(
		d.PermSets.Ensure(m.cache),
		d.PSGs.Ensure(m.cache),
		d.Profiles.Ensure(m.cache),
		d.Queues.Ensure(m.cache),
		d.PublicGroups.Ensure(m.cache),
	)
}

func (m Model) refreshPermsDashboardData(d *orgData) tea.Cmd {
	switch m.currentSubtab() {
	case SubtabPermSets:
		return d.PermSets.Refresh(m.cache)
	case SubtabPSGs:
		return d.PSGs.Refresh(m.cache)
	case SubtabProfiles:
		return d.Profiles.Refresh(m.cache)
	case SubtabPermsQueues:
		return d.Queues.Refresh(m.cache)
	case SubtabPermsPublicGroups:
		return d.PublicGroups.Refresh(m.cache)
	}
	return nil
}
