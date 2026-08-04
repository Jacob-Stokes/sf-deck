package ui

// /objects drill · Flows subtab — the flows whose TRIGGER OBJECT is
// the drilled sObject (record-triggered before/after save + delete,
// scheduled-on-object). Enter drills into the existing /flow detail
// tab — FlowDefinitionView's DurableId is a FlowDefinition id, the
// same key the /flows list drills on. Screen flows aren't listed:
// they have no trigger object, so "scoped to this object" has no
// queryable meaning for them.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m Model) renderObjectFlows(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	sobj := d.DescribeCur
	if sobj == "" {
		return theme.Subtle.Render("  press enter on an object in /objects first")
	}

	r, ok2 := d.ObjectFlows.Lists[sobj]
	if !ok2 || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading flows…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching flows…")
	}
	rows := r.Value()

	var lines []string
	lines = append(lines, sectionTitle(fmt.Sprintf("RECORD-TRIGGERED FLOWS · %d · %s",
		len(rows),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))))
	lines = append(lines, "")

	if len(rows) == 0 {
		lines = append(lines, theme.Subtle.Render("  no flows trigger on this object"))
		return strings.Join(lines, "\n")
	}

	sel := d.ObjectFlows.Cursors[sobj]
	if sel < 0 || sel >= len(rows) {
		sel = 0
	}

	labelW := inner / 2
	if labelW < 24 {
		labelW = 24
	}
	// Rows arrive phase-sorted then TriggerOrder-sorted (see
	// sf.ListObjectFlows), so we walk them once and drop a numbered phase
	// header whenever the phase RANK changes — Salesforce's Flow Trigger
	// Explorer grouping ("what runs when"). The cursor still indexes the
	// flat rows, so Enter / j / k are unaffected.
	lastRank := -1
	phaseNum := 0
	for i, row := range rows {
		rank := sf.FlowPhaseRank(row)
		if rank != lastRank {
			lastRank = rank
			phaseNum++
			if i > 0 {
				lines = append(lines, "")
			}
			header := fmt.Sprintf("%d. %s", phaseNum, flowPhaseHeading(rank))
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).Render("  "+header))
		}
		prefix := "    "
		nameStyle := lipgloss.NewStyle().Foreground(theme.Fg)
		if i == sel && m.focus == focusMain {
			prefix = "  ▌ "
			nameStyle = nameStyle.Bold(true)
		}
		dot := lipgloss.NewStyle().Foreground(theme.Green).Render("●")
		if !row.IsActive {
			dot = lipgloss.NewStyle().Foreground(theme.Muted).Render("○")
		}
		// order prefix: the real TriggerOrder (right-aligned) when set,
		// so the run sequence within a phase reads top-to-bottom.
		order := "    "
		if row.HasTriggerOrder {
			order = lipgloss.NewStyle().Foreground(theme.Muted).Render(fmt.Sprintf("%3d ", row.TriggerOrder))
		}
		label := padRight(truncate(row.Label, labelW), labelW)
		lines = append(lines, prefix+order+dot+" "+nameStyle.Render(label)+" "+objectFlowMeta(row))
	}
	lines = append(lines, "",
		dimLine("  ● active · ○ inactive · #=trigger order · ↵ flow detail · "+firstPretty(Keys.OpenDefault)+" flow builder", inner))
	return strings.Join(lines, "\n")
}

// objectFlowMeta renders the dim per-row detail cluster shown after the
// flow name: active version, record-trigger DML, and an out-of-date
// flag — the same info Salesforce's Trigger Explorer surfaces per row.
func objectFlowMeta(row sf.ObjectFlowRow) string {
	var parts []string
	if row.VersionNumber > 0 {
		parts = append(parts, fmt.Sprintf("V%d", row.VersionNumber))
	}
	if dml := prettyRecordTriggerType(row.RecordTriggerType); dml != "" {
		parts = append(parts, dml)
	}
	dim := lipgloss.NewStyle().Foreground(theme.FgDim).Render(strings.Join(parts, " · "))
	if row.IsOutOfDate {
		dim += " " + lipgloss.NewStyle().Foreground(theme.Yellow).Render("⚠ out of date")
	}
	return dim
}

// prettyRecordTriggerType humanizes FlowDefinitionView.RecordTriggerType
// (which DML fires the flow). Empty for non-record triggers.
func prettyRecordTriggerType(t string) string {
	switch t {
	case "Create":
		return "on create"
	case "Update":
		return "on update"
	case "CreateAndUpdate":
		return "create + update"
	case "Delete":
		return "on delete"
	}
	return ""
}

// flowPhaseHeading names a phase (by its FlowPhaseRank) using Salesforce's
// Flow Trigger Explorer wording so the grouped view reads like Setup.
func flowPhaseHeading(rank int) string {
	switch rank {
	case 0:
		return "Fast Field Updates (before save)"
	case 1:
		return "Actions and Related Records (after save)"
	case 2:
		return "Run Asynchronously (after save, async path)"
	case 3:
		return "Before Delete"
	case 4:
		return "Scheduled"
	case 5:
		return "Platform Event"
	}
	return "Other"
}

// activateObjectFlow is Enter on a Flows-subtab row: drill into the
// shared /flow detail tab keyed by the definition id.
func (m *Model) activateObjectFlow() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || d.DescribeCur == "" {
		return nil
	}
	r := d.ObjectFlows.Lists[d.DescribeCur]
	if r == nil {
		return nil
	}
	rows := r.Value()
	sel := d.ObjectFlows.Cursors[d.DescribeCur]
	if sel < 0 || sel >= len(rows) {
		return nil
	}
	d.FlowCur = rows[sel].DefinitionID
	m.setTab(TabFlowDetail)
	return m.onTabChanged()
}
