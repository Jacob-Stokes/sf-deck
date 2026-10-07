package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

type utilityID string

const (
	utilityOrgs utilityID = "orgs"
	// utilityBookmarks is preserved as a constant for back-compat with
	// keymap toml + downstream code that still references the symbol.
	// Dev Projects is no longer a left-rail panel — it's reached via
	// the right-rail "- Dev Projects" nav pill instead. Anything that
	// tests `currentUtility().ID == utilityBookmarks` now reads as
	// false because the only utility in the rail is Orgs.
	utilityBookmarks utilityID = "bookmarks"
)

type leftrailUtility struct {
	ID    utilityID
	Label string
}

func leftrailUtilities() []leftrailUtility {
	return []leftrailUtility{
		{ID: utilityOrgs, Label: "Orgs"},
	}
}

func (m Model) currentUtility() leftrailUtility {
	utils := leftrailUtilities()
	i := m.leftUtilityIdx
	if i < 0 || i >= len(utils) {
		i = 0
	}
	return utils[i]
}

func orgsUtilityIdx() int {
	return utilityIdx(utilityOrgs)
}

func utilityIdx(id utilityID) int {
	for i, u := range leftrailUtilities() {
		if u.ID == id {
			return i
		}
	}
	return 0
}

func (m Model) renderLeftWidget(w, h, innerH int) string {
	inner := w - 4
	if inner < 4 {
		inner = 4
	}
	body := m.renderOrgsWidget(inner)

	style := theme.Panelled
	if m.focus == focusOrgs {
		style = theme.PanelledFocus
	}
	return style.Width(w).Height(h).MaxHeight(h).Render(clipLines(body, innerH))
}

func (m Model) renderOrgsWidget(inner int) string {
	if m.orgsRes.Busy() && len(m.orgs) == 0 {
		return theme.Subtle.Render("  loading…")
	}
	if len(m.orgs) == 0 {
		return theme.Subtle.Render("  no orgs found")
	}

	groups := m.settings.OrgGroups()
	rows := buildRailRows(m.orgs, groups)

	quickJump := m.orgQuickJumpActive
	cursor := m.orgRailCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}

	var b strings.Builder
	headersSeen := 0
	for ri, row := range rows {
		onCursor := ri == cursor && m.focus == focusOrgs
		switch row.Kind {
		case railRowGroupHeader:
			if headersSeen > 0 {
				b.WriteByte('\n')
			}
			headersSeen++
			b.WriteString(m.renderRailGroupHeader(row, onCursor, groups, inner))
			b.WriteByte('\n')
		case railRowOrg:
			b.WriteString(m.renderRailOrgRow(row, onCursor, quickJump, inner))
			b.WriteByte('\n')
		}
	}

	if m.focus == focusOrgs && inner >= 16 {
		b.WriteString("\n")
		b.WriteString(m.renderOrgsRailHints(rows, cursor, inner))
	}

	return b.String()
}

func (m Model) renderOrgsRailHints(_ []orgRailRow, _ int, inner int) string {
	hints := []string{
		firstPretty(Keys.OrgGroupToggle) + ":fold",
		firstPretty(Keys.OrgManageOpen) + ":manage",
	}
	line := strings.Join(hints, " · ")
	line = ansi.Truncate(line, inner, "…")
	return lipgloss.NewStyle().Foreground(theme.FgDim).Render(line)
}

// renderRailGroupHeader draws one group header row: arrow indicator
// (▌ expanded / ▷ collapsed), name, member count on the right.
// Synthetic Ungrouped renders the same shape — the user just can't
// rename or delete it.
func (m Model) renderRailGroupHeader(row orgRailRow, onCursor bool, groups []settings.OrgGroupConfig, inner int) string {
	collapsed := groupHeaderCollapsed(groups, row.GroupID)
	count := groupMemberCount(m.orgs, groups, row.GroupID)
	name := groupHeaderLabel(groups, row.GroupID)

	arrow := "▌"
	if collapsed {
		arrow = "▷"
	}
	arrowColor := theme.Muted
	if onCursor {
		arrowColor = theme.BorderHi
	}
	nameStyle := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	if row.GroupID == ungroupedID {
		nameStyle = lipgloss.NewStyle().Foreground(theme.FgDim)
	}
	if onCursor {
		nameStyle = nameStyle.Underline(true)
	}

	countStr := fmt.Sprintf("%d", count)
	countStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	jumpSlot := "  "
	left := jumpSlot + lipgloss.NewStyle().Foreground(arrowColor).Render(arrow) + " "
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(countStr)
	nameMax := inner - leftW - rightW - 1
	if nameMax < 2 {
		nameMax = 2
	}
	name = ansi.Truncate(name, nameMax, "…")
	rendered := nameStyle.Render(name)
	pad := inner - leftW - lipgloss.Width(rendered) - rightW
	if pad < 1 {
		pad = 1
	}
	return left + rendered + strings.Repeat(" ", pad) + countStyle.Render(countStr)
}

func (m Model) renderRailOrgRow(row orgRailRow, onCursor bool, quickJump bool, inner int) string {
	o := row.Org
	i := row.OrgIdx
	// "Selected" for visual purposes is whichever row owns the rail
	// cursor when the rail is focused; otherwise fall back to
	// m.selected so the active org still shows a bar when focus is
	// elsewhere.
	selected := onCursor || (m.focus != focusOrgs && i == m.selected)

	jumpSlot := "  "
	if quickJump {
		if ltr := orgQuickJumpLetterFor(i); ltr != "" {
			jumpSlot = lipgloss.NewStyle().
				Foreground(theme.Yellow).
				Bold(true).
				Render(ltr) + " "
		}
	}
	prefix := jumpSlot + "  "
	if selected {
		barColor := theme.BorderHi
		if m.focus != focusOrgs {
			barColor = theme.Muted
		}
		prefix = jumpSlot + lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}

	dot := statusDot(o.Status)

	label := o.Display()
	if label == "" {
		label = "(no alias)"
	}
	labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	subStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
	if selected {
		labelStyle = labelStyle.Bold(true)
		subStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	}

	safetyTag := safetyTagInline(m.safetyFor(o))
	safetyW := lipgloss.Width(safetyTag)
	defaults := cliDefaultMarkers(o)
	defaultsW := lipgloss.Width(defaults)
	labelMax := inner - 6 - safetyW - defaultsW - 1
	if labelMax < 4 {
		labelMax = 4
	}
	label = ansi.Truncate(label, labelMax, "…")
	main := prefix + dot + " " + labelStyle.Render(label) + defaults + " " + safetyTag
	main = ansi.Truncate(main, inner, "…")

	sub := "      " + o.Kind() + " · " + o.Username
	sub = subStyle.Render(ansi.Truncate(sub, inner, "…"))
	if tag := scratchExpiryTag(o); tag != "" {
		sub += " " + tag
	}

	return main + "\n" + sub
}

// cliDefaultMarkers renders the sf CLI default-org markers for one
// row: cyan * when the org is the global target-org, cyan ^ when
// it's the target-dev-hub. Glyphs deliberately match the Org
// Manager keybinds that SET them (* and ^) so the marker doubles as
// a reminder of the key. Empty for ordinary orgs.
func cliDefaultMarkers(o sf.Org) string {
	out := ""
	if o.IsDefault {
		out += "*"
	}
	if o.IsDefaultDevHub {
		out += "^"
	}
	if out == "" {
		return ""
	}
	return " " + lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true).Render(out)
}

func scratchExpiryTag(o sf.Org) string {
	days, ok := o.ScratchDaysLeft()
	if !ok {
		return ""
	}
	var label string
	var c color.Color
	switch {
	case days < 0:
		label, c = "expired", theme.Red
	case days == 0:
		label, c = "expires today", theme.Red
	case days <= 2:
		label, c = fmt.Sprintf("%dd left", days), theme.Red
	case days <= 7:
		label, c = fmt.Sprintf("%dd left", days), theme.Yellow
	default:
		label, c = fmt.Sprintf("%dd left", days), theme.Muted
	}
	return lipgloss.NewStyle().Foreground(c).Render(label)
}

// safetyTagInline renders a small colored safety tag suitable for an
// in-line row (orgs list). Not padded like the header pill — just
// colored letters so it compacts well at narrow widths.
func safetyTagInline(lvl settings.SafetyLevel) string {
	var c color.Color
	switch lvl {
	case settings.SafetyReadOnly:
		c = theme.Green
	case settings.SafetyRecords:
		c = theme.Yellow
	case settings.SafetyMetadata:
		c = lipgloss.Color("208")
	case settings.SafetyFull:
		c = theme.Red
	default:
		c = theme.Muted
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(lvl.Label())
}
