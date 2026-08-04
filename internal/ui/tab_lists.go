package ui

import (
	"strings"
)

// /apex-logs, /deploys, /packages — flat list surfaces backed by
// listSurface.BuildRenderModel + the shared renderListSurface helper.
// Each renderXxx is a thin orchestrator that picks the surface and
// hands off; everything visible lives on the listSurface declaration
// in list_surface.go.

// --- ApexLogs -----------------------------------------------------------

func (m Model) renderApexLogs(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &apexLogsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderSetupAudit(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &setupAuditListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderFlowInterviews(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &flowInterviewsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderAsyncJobs(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &asyncJobsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

func (m Model) renderScheduledJobs(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &scheduledJobsListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}

// renderActiveUsers draws the /users → Active subtab: a chip strip
// (All / No MFA / Recently active / API) over a single session-derived
// list that the chips filter client-side (deploys pattern — one
// resource, not per-chip re-query).
func (m Model) renderActiveUsers(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)

	chips := m.stripRows(domainActiveUsers, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.activeUsersChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.ActiveUsers.FetchedAt().IsZero() {
		if d.ActiveUsers.Busy() {
			lines = append(lines, dimLine("  loading active sessions…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load active sessions", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok2 := activeUsersListSurface.BuildRenderModel(m, d)
	if !ok2 {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	budget := innerH - usedLines(lines)
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// --- Deploys ------------------------------------------------------------

func (m Model) renderDeploys(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)

	chips := m.stripRows(domainDeploys, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.deploysChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	if d.Deploys.FetchedAt().IsZero() {
		if d.Deploys.Busy() {
			lines = append(lines, dimLine("  loading deploys…", inner))
		} else {
			lines = append(lines, dimLine("  press "+firstPretty(Keys.Refresh)+" to load deploys", inner))
		}
		return strings.Join(lines, "\n")
	}

	model, ok2 := deploysListSurface.BuildRenderModel(m, d)
	if !ok2 {
		lines = append(lines, dimLine("  loading…", inner))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// --- Packages -----------------------------------------------------------

func (m Model) renderPackages(w, innerH int) string {
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	return renderListSurface(m, &packagesListSurface, w, innerH,
		m.ensureOrgDataRef(o.Username))
}
