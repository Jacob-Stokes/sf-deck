package ui

// Left rail layout.
//
// The left pane is a single toggleable widget pane that hosts utilities
// (Orgs, Bookmarks, …). Each utility is one subtab at the top of the
// pane; the active subtab's content fills the rest of the pane.
//
// Layout:
//
//   ┌─────────────────────────┐
//   │ Orgs · Bookmarks        │  ← subtab strip
//   │ ─────────────────────── │
//   │                         │
//   │  (selected utility)     │
//   │                         │
//   └─────────────────────────┘
//
// Adding a new utility = add a utilityID const, a row in
// leftrailUtilities() with label + glyph, and a render case in
// renderLeftWidget(). Nothing else in the codebase needs to know the
// list. Toggling which utility is active uses the same `[` / `]`
// subtab keys (or `tab` / `shift+tab`) as elsewhere — see
// cycleLeftUtility in update_nav.go.

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

// utilityID identifies one left-rail utility. Stable across renames
// (used in persistence + keymap config).
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

// leftrailUtility is the display metadata for one utility.
type leftrailUtility struct {
	ID    utilityID
	Label string
}

// leftrailUtilities lists every utility in display order. Currently
// just Orgs — the rail is org-focused only. Dev Projects moved to a
// right-rail navigation pill (see render_tabs.go).
func leftrailUtilities() []leftrailUtility {
	return []leftrailUtility{
		{ID: utilityOrgs, Label: "Orgs"},
	}
}

// currentUtility returns the leftrailUtility currently shown in the
// widget pane. Falls back to the first one if the stored index is out
// of range (defends against config drift).
func (m Model) currentUtility() leftrailUtility {
	utils := leftrailUtilities()
	i := m.leftUtilityIdx
	if i < 0 || i >= len(utils) {
		i = 0
	}
	return utils[i]
}

// orgsUtilityIdx returns the index of the Orgs utility in the utility
// list. Always 0 now that Orgs is the only utility, but callers still
// use this rather than a literal so the indirection survives if the
// rail ever grows extra utilities again.
func orgsUtilityIdx() int {
	return utilityIdx(utilityOrgs)
}

// utilityIdx finds a utility by ID. Returns 0 when not found; callers
// defend against drift with the same guard as currentUtility.
func utilityIdx(id utilityID) int {
	for i, u := range leftrailUtilities() {
		if u.ID == id {
			return i
		}
	}
	return 0
}

// renderLeftWidget draws the left rail body. With Dev Projects moved
// to the right-rail nav, the rail just shows the Orgs panel.
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

// renderOrgsWidget is the content of the Orgs utility. Walks the
// flattened row list (group headers + indented org rows) produced by
// buildRailRows. Users with no groups configured see the same flat
// list as before — buildRailRows omits the synthetic Ungrouped header
// when there are zero user groups.
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
			// Blank divider above every header except the first one,
			// so groups visually separate. Pure render concern — the
			// row list itself is unchanged so cursor indices line up.
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

	// Context-aware key hints — only when the rail itself is focused.
	// Skip in narrow widths (would just truncate to noise).
	if m.focus == focusOrgs && inner >= 16 {
		b.WriteString("\n")
		b.WriteString(m.renderOrgsRailHints(rows, cursor, inner))
	}

	return b.String()
}

// renderOrgsRailHints produces a one-line dim hint pointing the
// user at the org-manage modal for any edit action. The rail itself
// only owns navigation aids (j/k, space to fold/expand) — keeping
// it tight in the narrow rail and putting the full key list in the
// modal which has room to breathe.
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

	// 2-col jump slot kept for vertical alignment with org rows below.
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

// renderRailOrgRow draws one indented org row under a group header
// (or under the synthetic Ungrouped section). Member rows are
// indented by 2 cols relative to the existing flat layout to keep
// the parent group visually distinct.
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

// scratchExpiryTag renders the scratch-org countdown ("3d left"),
// amber within a week, red within two days / expired. Empty for
// non-scratch orgs. Date-granular — see sf.Org.ScratchDaysLeft.
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
