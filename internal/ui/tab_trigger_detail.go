package ui

// TabTriggerDetail — full-pane view of a single ApexTrigger. Drill
// from the Triggers subtab's list (enter on a row).
//
// Main pane: a navigable STATUS row (toggle) + the scrollable Apex
// BODY + a DANGER ZONE delete row. Tab swaps the row cursor ↔ the body
// scroll. The right sidebar is INFO-ONLY. The Body is rendered with
// light line numbers + chroma syntax highlighting (Java lexer — Apex
// is ~95% Java syntactically and chroma ships no dedicated Apex lexer).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/highlight"
)

// triggerDetailDrill is the canonical opener for TabTriggerDetail.
// The return tab is explicit because trigger detail is reused by
// Object Detail, Apex, and project surfaces.
func (m *Model) triggerDetailDrill(sobject, id string, returnTab Tab) tea.Cmd {
	if sobject == "" || id == "" || len(m.orgs) == 0 {
		return nil
	}
	o := m.orgs[m.selected]
	d := m.ensureOrgData(o.Username)
	d.DescribeCur = sobject
	d.Triggers.DrillID = id
	m.triggerActionCur = 0
	// Start with the row cursor active (not the body scroll) so the
	// status / edit / delete rows are immediately navigable. Tab swaps
	// to body-scroll for reading the Apex source.
	m.bodyFocus = false
	if returnTab != TabTriggerDetail {
		m.triggerDetailReturnTab = returnTab
	} else {
		m.triggerDetailReturnTab = TabObjectDetail
	}
	d.EnsureTriggerDetail(targetArg(o), id)
	m.setTab(TabTriggerDetail)
	return m.onTabChanged()
}

func (m Model) triggerDetailBackTab() Tab {
	if m.triggerDetailReturnTab != TabTriggerDetail {
		return m.triggerDetailReturnTab
	}
	return TabObjectDetail
}

// Trigger action indices — track triggerActionsFor's order.
const (
	trgActToggleStatus = 0
	trgActEditBody     = 1
	trgActDelete       = 2
)

// renderTriggerDetail is the main-pane renderer for TabTriggerDetail.
//
// Layout (top → bottom): a navigable STATUS row (toggle), a navigable
// "edit body" affordance, the scrollable Apex BODY, then a DANGER ZONE
// delete row. The three action rows take the cursor when bodyFocus is
// false; when bodyFocus is true the arrows scroll the BODY instead
// (Tab swaps). The right sidebar is INFO-ONLY.
func (m Model) renderTriggerDetail(w, innerH int) string {
	inner := w - 4
	o, ok := m.currentOrg()
	if !ok {
		return noOrgPlaceholder()
	}
	d := m.ensureOrgDataRef(o.Username)
	if d.DescribeCur == "" || d.Triggers.DrillID == "" {
		return theme.Subtle.Render("  press enter on a trigger in the Triggers subtab first")
	}

	r, ok := d.Triggers.Details[d.Triggers.DrillID]
	if !ok || r.FetchedAt().IsZero() {
		if r != nil && r.Busy() {
			return theme.Subtle.Render("  loading trigger body…")
		}
		if r != nil && r.Err() != nil {
			return redLine("  " + r.Err().Error())
		}
		return theme.Subtle.Render("  fetching trigger body…")
	}
	det := r.Value()

	// Navigable action rows live in a small detailRow set; the BODY +
	// header render between them. We track which navigable index the
	// cursor is on (only meaningful when !bodyFocus).
	curNav := m.triggerActionCur
	navCount := 3 // status, edit body, delete
	if curNav < 0 {
		curNav = 0
	}
	if curNav >= navCount {
		curNav = navCount - 1
	}
	rowActive := m.focus == focusMain && !m.bodyFocus

	var lines []string
	title := d.DescribeCur + "  /  " + det.Name
	lines = append(lines, sectionTitle(title))

	statusTxt := det.Status
	if !det.Valid && det.Status == "Active" {
		statusTxt += " (invalid)"
	}
	header := fmt.Sprintf("  api %.1f  ·  %d lines  ·  %s",
		det.ApiVer, lineCount(det.Body),
		humanAge(r.FetchedAt())+stateSuffix(r.Busy(), r.Err()))
	lines = append(lines, dimLine(header, inner))
	lines = append(lines, "")

	if det.Events != "" {
		lines = append(lines, sectionTitle("EVENTS"))
		lines = append(lines, dimLine("  "+det.Events, inner))
		lines = append(lines, "")
	}

	// STATUS row (navigable index 0).
	statusRow := detailRow{
		Text: kvLine("status", statusTxt, inner), Navigable: true,
		ActionIdx: trgActToggleStatus,
	}
	lines = append(lines, renderDetailLine(statusRow, rowActive && curNav == 0, rowActive, inner))
	lines = append(lines, "")

	// BODY header + the "edit body" affordance (navigable index 1).
	bodyHint := "  (tab to scroll · ↵ edit · / find)"
	if m.bodyFocus {
		bodyHint = "  (focused — tab to actions · ↵ edit · / find)"
	}
	editBodyRow := detailRow{
		Text:      sectionTitle("BODY") + lipgloss.NewStyle().Foreground(theme.FgDim).Render(bodyHint),
		Navigable: true, ActionIdx: trgActEditBody,
	}
	lines = append(lines, renderDetailLine(editBodyRow, rowActive && curNav == 1, rowActive, inner))

	// Reserve space for the danger row (blank + title + row = 3 lines).
	bodyHeight := innerH - len(lines) - 3
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	bodyView := m.renderCodeView(d, codeViewSpec{
		BodyID:  triggerBodyID(d.Triggers.DrillID),
		Body:    det.Body,
		Lang:    highlight.LangApex,
		Inner:   inner,
		Height:  bodyHeight,
		Focused: m.bodyFocus,
	})
	lines = append(lines, bodyView...)

	// DANGER ZONE delete (navigable index 2).
	lines = append(lines, "")
	lines = append(lines, redLine("DANGER ZONE"))
	deleteRow := detailRow{
		Text: "  " + redLine("delete trigger"), Navigable: true,
		ActionIdx: trgActDelete, Danger: true,
	}
	lines = append(lines, renderDetailLine(deleteRow, rowActive && curNav == 2, rowActive, inner))

	return strings.Join(lines, "\n")
}

// triggerNavActionForCursor maps the trigger row cursor (0..2) to its
// action index. All three navigable rows map 1:1 to actions.
func triggerNavActionForCursor(cur int) (int, bool) {
	switch cur {
	case 0:
		return trgActToggleStatus, true
	case 1:
		return trgActEditBody, true
	case 2:
		return trgActDelete, true
	}
	return noAction, false
}

// triggerBodyID is the cache key for the trigger body's cursor +
// scroll. Prefixed so it can never collide with apex class /
// LWC body keys (those use their own prefixes).
func triggerBodyID(triggerID string) string {
	if triggerID == "" {
		return ""
	}
	return "trigger:" + triggerID
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// sidebarTriggerActions renders the TabTriggerDetail right-side panel
// — a context panel for the cursored row (or the body, when the body
// scroll is focused).
func (m Model) sidebarTriggerActions(inner int) string {
	ctx := m.triggerRowContext()
	ctx.Hints = append([]string{firstPretty(Keys.NextSubtab) + " body/rows"}, detailNavHints(true)...)
	return m.sidebarRowContext("TRIGGER · CONTEXT", inner, ctx)
}

func (m Model) triggerRowContext() rowContext {
	rows := RegistryRows(m, triggerRegistry)
	det, haveDet := m.currentTriggerDetail()

	// Body focused — describe scrolling + the edit affordance.
	if m.bodyFocus {
		ctx := rowContext{
			Heading: "Apex body",
			Help:    "j/k scroll · tab back to the action rows · ↵ to edit the source.",
			Routing: "Tooling API · ApexTrigger patch (compiles on save)",
		}
		if haveDet {
			ctx.Current = fmt.Sprintf("%d lines · api %.1f", lineCount(det.Body), det.ApiVer)
		}
		return ctx
	}

	idx, ok := triggerNavActionForCursor(m.triggerActionCur)
	if !ok || idx < 0 || idx >= len(rows) {
		return rowContext{}
	}
	a := rows[idx]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Routing: "Tooling API · ApexTrigger patch",
		Danger:  idx == trgActDelete,
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}
	switch idx {
	case trgActToggleStatus:
		ctx.Affects = "whether the trigger fires on matching DML."
		if haveDet {
			ctx.Current = det.Status
		}
	case trgActEditBody:
		ctx.Routing = "Tooling API · ApexTrigger patch (compiles on save)"
		ctx.Affects = "the Apex source; a compile error comes back as a red line."
		if haveDet {
			ctx.Current = fmt.Sprintf("%d lines", lineCount(det.Body))
		}
	case trgActDelete:
		ctx.Routing = "Tooling API · delete ApexTrigger"
		ctx.Affects = "permanently removes it; DML it guarded runs unchecked. No undo."
	}
	return ctx
}

func (m Model) currentTriggerDetail() (sf.TriggerDetail, bool) {
	o, ok := m.currentOrg()
	if !ok {
		return sf.TriggerDetail{}, false
	}
	d := m.data[o.Username]
	if d == nil || d.Triggers.DrillID == "" {
		return sf.TriggerDetail{}, false
	}
	r, ok := d.Triggers.Details[d.Triggers.DrillID]
	if !ok || r == nil || r.FetchedAt().IsZero() {
		return sf.TriggerDetail{}, false
	}
	return r.Value(), true
}
