package ui

// /perms top-level tab. Dispatches to one of three list sub-views
// (Permission Sets, Permission Set Groups, Profiles) based on the
// currently-selected dashboard subtab. The subtab strip itself is
// rendered by the generic render_header machinery.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderPermsDashboard(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}

	subs := permsDashboardSubtabs()
	sel := m.permsDashboardSubtab()
	if sel < 0 || sel >= len(subs) {
		sel = 0
	}
	strip := renderSubtabStrip(subs, sel, w-4)
	body := innerH - subtabReserve(strip)

	// Chip dashboard for the active subtab (PermSets/PSGs/Profiles).
	// Built from the subtab's chip registry + the synthetic project
	// chip when an org-project is loaded with members of that kind.
	var chipDash string
	switch subs[sel].ID {
	case SubtabPermSets:
		chipDash = m.renderPermsChipDashboard(domainPermSets, m.permsetsChipIdx(), inner)
	case SubtabPSGs:
		chipDash = m.renderPermsChipDashboard(domainPSGs, m.psgsChipIdx(), inner)
	case SubtabProfiles:
		chipDash = m.renderPermsChipDashboard(domainProfiles, m.profilesChipIdx(), inner)
	case SubtabPermsQueues:
		chipDash = m.renderPermsChipDashboard(domainQueues, m.queuesChipIdx(), inner)
	case SubtabPermsPublicGroups:
		chipDash = m.renderPermsChipDashboard(domainPublicGroup, m.publicGroupsChipIdx(), inner)
	}
	body -= subtabReserve(chipDash)

	var content string
	switch subs[sel].ID {
	case SubtabPermSets:
		content = m.renderPermSetsList(w, inner, body, o)
	case SubtabPSGs:
		content = m.renderPSGList(w, inner, body, o)
	case SubtabProfiles:
		content = m.renderProfilesList(w, inner, body, o)
	case SubtabPermsQueues:
		content = m.renderQueueList(w, inner, body, o)
	case SubtabPermsPublicGroups:
		content = m.renderPublicGroupList(w, inner, body, o)
	default:
		content = theme.Subtle.Render("  (no subtab)")
	}
	parts := make([]string, 0, 3)
	if strip != "" {
		parts = append(parts, strip)
	}
	if chipDash != "" {
		parts = append(parts, chipDash)
	}
	parts = append(parts, content)
	return strings.Join(parts, "\n")
}

// renderPermsChipDashboard wraps the standard chip dashboard for the
// /perms subtabs. Returns "" when the registry has no rows yet (empty
// strip wouldn't add information).
func (m Model) renderPermsChipDashboard(domain chipDomain, sel, inner int) string {
	chips := m.stripRows(domain, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	if sel < 0 || sel >= len(chips) {
		sel = 0
	}
	return m.renderDashboard("VIEWS", chips, sel, inner)
}

func (m Model) renderPermSetsList(w, inner, innerH int, o sf.Org) string {
	return renderListSurface(m, &permsetsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderPSGList(w, inner, innerH int, o sf.Org) string {
	return renderListSurface(m, &psgsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderProfilesList(w, inner, innerH int, o sf.Org) string {
	return renderListSurface(m, &profilesListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderQueueList(w, inner, innerH int, o sf.Org) string {
	return renderListSurface(m, &queuesListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderPublicGroupList(w, inner, innerH int, o sf.Org) string {
	return renderListSurface(m, &publicGroupsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}
