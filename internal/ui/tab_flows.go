package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// bulkTagsForFlows pre-fetches tag bindings for every flow in the
// supplied slice. Memoised on *orgData via the gutter cache —
// successive renders within the same generation reuse the result.
// Returns nil when the gutter is hidden / store is unavailable / no
// current org / empty flows.
func (m Model) bulkTagsForFlows(flows []sf.Flow) map[string][]devproject.Tag {
	return bulkTagsForItems(m, flows, gutterDomainFlow, devproject.KindFlow,
		func(f sf.Flow) string { return f.DefinitionID })
}

// bulkProjectsForFlows is the project-membership equivalent. Same
// caching + early-return shape as bulkTagsForFlows.
func (m Model) bulkProjectsForFlows(flows []sf.Flow) map[string][]devproject.DevProject {
	return bulkProjectsForItems(m, flows, gutterDomainFlow, devproject.KindFlow,
		func(f sf.Flow) string { return f.DefinitionID })
}

// renderFlows draws the flows list as a tabulated NAME · TYPE · STATUS ·
// VERSION · LABEL grid. Status pops via per-row colour overrides on
// the STATUS column.
func (m Model) renderFlows(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	if !canUseOrg(o) {
		return theme.Subtle.Render("  org disconnected")
	}
	d := m.ensureOrgDataRef(o.Username)

	chips := m.stripRows(domainFlows, "*")
	if len(chips) == 0 {
		chips = []chipRow{{ID: "all", Label: "All", Count: -1}}
	}
	chipSel := m.flowsChipIdx()
	if chipSel < 0 || chipSel >= len(chips) {
		chipSel = 0
	}
	dash := m.renderDashboard("VIEWS", chips, chipSel, inner)

	var lines []string
	if dash != "" {
		lines = append(lines, dash)
	}

	// Tab-specific empty-state overrides — only the busy/loading and
	// project-mode hints diverge from the shared renderer's default
	// "no matches". The shared renderer handles the search-applied
	// case via the model's Empty fallback.
	if d.FlowList.Len() == 0 && d.Flows.Busy() {
		lines = append(lines, theme.Subtle.Render("  loading…"))
		return strings.Join(lines, "\n")
	}
	// Project-chip empty-state: gate on ExtraCount so a project with
	// no flows shows the collect hint, while an active search inside
	// a non-empty project still falls through to the shared renderer
	// (keeps the search box visible + shows "no matches" inline).
	if d.FlowList.ExtraCount() == 0 && m.projectChipActive() {
		lines = append(lines, theme.Subtle.Render(m.projectEmptyHint("flows")))
		return strings.Join(lines, "\n")
	}

	model, ok := flowsListSurface.BuildRenderModel(m, d)
	if !ok {
		lines = append(lines, theme.Subtle.Render("  loading…"))
		return strings.Join(lines, "\n")
	}
	usedAbove := usedLines(lines)
	budget := innerH - usedAbove
	lines = append(lines, renderListModel(m, model, m.focus, inner, budget)...)
	return strings.Join(lines, "\n")
}

// flowVersionCell formats the cell for the VERSION column. Normally
// just "v3" (the active version) or "—". When the newest version is a
// later DRAFT than the active one — i.e. someone edited the flow but
// hasn't activated the change — the cell shows "v3 (v4)": active
// first, latest-draft in brackets. A conditional footer hint explains
// the brackets when such a row is highlighted (see flowVersionMismatch).
//
// The bracketed form only appears when there's a distinct newer
// version; a flow whose active version IS its latest stays tight at
// "v3". Flows with no active version fall back to the latest number.
func flowVersionCell(f sf.Flow) string {
	switch {
	case f.ActiveVersionNum > 0:
		if f.LatestVersionNum > f.ActiveVersionNum {
			return fmt.Sprintf("v%d (v%d)", f.ActiveVersionNum, f.LatestVersionNum)
		}
		return fmt.Sprintf("v%d", f.ActiveVersionNum)
	case f.LatestVersionNum > 0:
		return fmt.Sprintf("v%d", f.LatestVersionNum)
	}
	return "—"
}

// flowSortVersion returns the version number to sort a flow row by:
// the active version when one is live, otherwise the latest. 0 for a
// flow with neither (sorts to the bottom ascending). Mirrors the
// number flowVersionCell shows as the primary (unbracketed) "v%d".
func flowSortVersion(f sf.Flow) int {
	if f.ActiveVersionNum > 0 {
		return f.ActiveVersionNum
	}
	return f.LatestVersionNum
}

// flowVersionMismatch reports whether a flow's newest version is a
// later version than its active one — the case flowVersionCell
// renders as "v3 (v4)". Drives the conditional footer hint.
func flowVersionMismatch(f sf.Flow) bool {
	return f.ActiveVersionNum > 0 && f.LatestVersionNum > f.ActiveVersionNum
}

// flowLatestStatusWord gives a status-accurate lowercase word for the
// LATEST version, used in the "(v4) = newer <word> version" footer
// hint. Almost always "draft" in practice, but a newer version can
// also be Obsolete or an InvalidDraft, so we report what it actually
// is rather than assuming. Falls back to a neutral word when the
// latest version's status wasn't captured.
func flowLatestStatusWord(f sf.Flow) string {
	switch f.LatestVersionStatus {
	case "Draft":
		return "draft"
	case "Obsolete":
		return "obsolete"
	case "InvalidDraft":
		return "invalid draft"
	case "Active":
		// A newer version that is itself Active is unusual (only one
		// version is active at a time) but describe it plainly if seen.
		return "active"
	case "":
		return "unactivated" // status not fetched — neutral fallback
	default:
		return strings.ToLower(f.LatestVersionStatus)
	}
}

// flowStatusColor returns the tint for STATUS cells — green active,
// yellow draft, red invalid, muted everything else.
func flowStatusColor(status string) color.Color {
	switch status {
	case "Active":
		return theme.Green
	case "Draft":
		return theme.Yellow
	case "Obsolete", "Inactive":
		return theme.Muted
	case "InvalidDraft":
		return theme.Red
	}
	return theme.Muted
}

// flowNameColor tints flow names by status so at-a-glance state is
// readable without peeking at the dot.
func flowNameColor(f sf.Flow) color.Color {
	switch f.Status {
	case "Active":
		return theme.Fg
	case "Obsolete", "Inactive":
		return theme.FgDim
	case "InvalidDraft":
		return theme.Red
	case "Draft":
		return theme.Yellow
	}
	return theme.Fg
}

func shortenProcessType(p string) string {
	switch p {
	case "AutoLaunchedFlow":
		return "auto"
	case "Flow":
		return "screen"
	case "Workflow":
		return "wf"
	case "InvocableProcess":
		return "pb"
	case "CustomEvent":
		return "event"
	case "CheckoutFlow":
		return "checkout"
	case "FieldServiceMobile", "FieldServiceWeb":
		return "field-svc"
	case "ContactRequestFlow":
		return "contact"
	case "ActionCadenceAutolaunchedFlow":
		return "cadence"
	}
	return strings.ToLower(p)
}

// renderFlowDetail draws the version history for a specific flow
// definition. Active version is marked with a ★. Enter on a version is
// a no-op; o/ctrl+o open the version in the Flow Builder.
func (m Model) renderFlowDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.FlowCur == "" {
		return theme.Subtle.Render("  press enter on a flow in /flows first")
	}

	// Header: flow definition info from the list row.
	var header sf.Flow
	for _, f := range d.Flows.Value() {
		if f.DefinitionID == d.FlowCur {
			header = f
			break
		}
	}

	var lines []string
	if header.DeveloperName != "" {
		title := header.DeveloperName
		if header.MasterLabel != "" && header.MasterLabel != header.DeveloperName {
			title += " — " + header.MasterLabel
		}
		lines = append(lines, sectionTitle(title))
		meta := []string{}
		if header.ProcessType != "" {
			meta = append(meta, "type "+header.ProcessType)
		}
		if header.Namespace != "" {
			meta = append(meta, "ns "+header.Namespace)
		}
		if header.APIVersion > 0 {
			meta = append(meta, fmt.Sprintf("api v%d", header.APIVersion))
		}
		if header.ActiveVersionNum > 0 {
			meta = append(meta, fmt.Sprintf("active v%d", header.ActiveVersionNum))
		}
		if header.LatestVersionNum > 0 && header.LatestVersionNum != header.ActiveVersionNum {
			meta = append(meta, fmt.Sprintf("latest v%d", header.LatestVersionNum))
		}
		lines = append(lines, dimLine("  "+strings.Join(meta, " · "), inner))
		if header.Description != "" {
			lines = append(lines, dimLine("  "+header.Description, inner))
		}
		lines = append(lines, "")
	}

	// Version list.
	r, ok := d.FlowVersions[d.FlowCur]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading versions…"))
		} else if r != nil && r.Err() != nil {
			lines = append(lines, redLine("  "+r.Err().Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching versions…"))
		}
		return strings.Join(lines, "\n")
	}
	versions := r.Value()
	lines = append(lines, sectionTitle(fmt.Sprintf("VERSIONS · %d", len(versions))))
	dimSuffix := stateSuffix(r.Busy(), r.Err())
	if dimSuffix != "" {
		lines[len(lines)-1] = lines[len(lines)-1] + dimSuffix
	}
	if len(versions) == 0 {
		lines = append(lines, theme.Subtle.Render("  none"))
		return strings.Join(lines, "\n")
	}
	sel := d.Cursors.Get(cursorKindFlowVersion, len(versions), d.FlowCur)
	cols := []tableColumn{
		{Header: "VERSION", Width: 10, Style: lipgloss.NewStyle().Foreground(theme.Fg)},
		{Header: "STATUS", Width: 12, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "API", Width: 8, Style: lipgloss.NewStyle().Foreground(theme.Muted)},
		{Header: "MODIFIED", Width: 16, Style: lipgloss.NewStyle().Foreground(theme.FgDim)},
		{Header: "BY", Width: -1, Style: lipgloss.NewStyle().Foreground(theme.FgDim)},
	}
	lines = append(lines, renderTableHeader(cols, inner))
	// trailing=2 reserves the blank + help line appended below.
	lines = append(lines, renderRows(
		len(versions), sel, innerH, len(lines), 2, inner,
		func(i int) string {
			v := versions[i]
			rowCols := make([]tableColumn, len(cols))
			copy(rowCols, cols)
			rowCols[1].Style = lipgloss.NewStyle().Foreground(flowStatusColor(v.Status))

			vlabel := fmt.Sprintf("v%d", v.VersionNumber)
			if v.ID == header.ActiveVersionID {
				vlabel = "★ " + vlabel
			}
			return renderInteractiveTableRow(rowCols, []string{
				vlabel,
				v.Status,
				fmt.Sprintf("v%d", v.APIVersion),
				prettyDate(v.LastModifiedDate),
				v.LastModifiedBy,
			}, i == sel, m.focus == focusMain, inner)
		},
	)...)
	// Enter behaviour is configurable (Settings → Navigation & input):
	// open Flow Builder (default) or view the definition in-terminal.
	// The hint follows it so ↵ always reads true.
	var enterHint string
	if m.settings.FlowVersionEnterOpens() {
		// ↵ and o both open Flow Builder — collapse them.
		enterHint = "↵/" + firstPretty(Keys.OpenDefault) + " → Flow Builder"
	} else {
		enterHint = "↵ view definition · " + firstPretty(Keys.OpenDefault) + " → Flow Builder"
	}
	lines = append(lines, "", m.footerHint(
		"  "+enterHint+" · "+
			firstPretty(Keys.YankDefault)+" yank url · "+
			firstPretty(Keys.FlowRename)+" rename · "+
			firstPretty(Keys.FlowVersionDelete)+" delete version", inner))
	return strings.Join(lines, "\n")
}
