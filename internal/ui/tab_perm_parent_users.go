package ui

// Assigned-users subtab of TabPermParentDetail.
//
// Read-only list of users assigned to this permission set.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// renderPermParentUsers is the main-pane renderer for the Users subtab.
func (m Model) renderPermParentUsers(w, inner, innerH int, o sf.Org) string {
	d := m.ensureOrgDataRef(o.Username)

	if d.PermParentPermSetID == "" {
		return psgNoDirectPermsNote(inner, "User assignment")
	}

	res := d.EnsureAssignedUsers(targetArg(o), d.PermParentPermSetID)
	if res.FetchedAt().IsZero() {
		var lines []string
		lines = append(lines, sectionTitle("ASSIGNED USERS"))
		if res.Busy() {
			lines = append(lines, theme.Subtle.Render("  loading assigned users…"))
		} else if err := res.Err(); err != nil {
			lines = append(lines, redLine("  "+err.Error()))
		} else {
			lines = append(lines, theme.Subtle.Render("  fetching…"))
		}
		return strings.Join(lines, "\n")
	}

	assignments := res.Value()
	cur := d.Cursors.Get(cursorKindAssignedUsers, len(assignments), d.PermParentPermSetID)

	age := humanAge(res.FetchedAt()) + stateSuffix(res.Busy(), res.Err())
	header := fmt.Sprintf("ASSIGNED USERS · %d · %s", len(assignments), age)

	var lines []string
	lines = append(lines, sectionTitle(header))
	if len(assignments) == 0 {
		lines = append(lines, theme.Subtle.Render("  no users assigned"))
		return strings.Join(lines, "\n")
	}

	// Column widths.
	nameW := inner * 30 / 100
	if nameW < 18 {
		nameW = 18
	}
	usernameW := inner * 40 / 100
	if usernameW < 20 {
		usernameW = 20
	}
	expW := inner - nameW - usernameW - 8
	if expW < 8 {
		expW = 8
	}

	lines = append(lines, renderUsersHeader(nameW, usernameW, expW, inner))
	lines = append(lines, renderRows(
		len(assignments), cur, innerH, len(lines), 0, inner,
		func(i int) string {
			return renderUserRow(assignments[i], i == cur, m.focus == focusMain, nameW, usernameW, expW, inner)
		},
	)...)
	return strings.Join(lines, "\n")
}

func renderUsersHeader(nameW, usernameW, expW, inner int) string {
	nameStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(nameW)
	usernameStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(usernameW)
	expStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(expW)
	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	line := "  " +
		nameStyle.Render("NAME") + sep +
		usernameStyle.Render("USERNAME") + sep +
		expStyle.Render("EXPIRES")
	return ansi.Truncate(line, inner, "…")
}

func renderUserRow(a sf.PermissionSetAssignment, selected, mainFocused bool, nameW, usernameW, expW, inner int) string {
	fg := theme.Fg
	if !a.AssigneeIsActive {
		fg = theme.FgDim
	}
	nameStyle := lipgloss.NewStyle().Foreground(fg).Width(nameW)
	usernameStyle := lipgloss.NewStyle().Foreground(theme.Muted).Width(usernameW)
	expStyle := lipgloss.NewStyle().Foreground(theme.FgDim).Width(expW)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	prefix := "  "
	if selected {
		barColor := theme.BorderHi
		if !mainFocused {
			barColor = theme.Muted
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}

	sep := lipgloss.NewStyle().Foreground(theme.Border).Render(" │ ")
	name := nameStyle.Render(ansi.Truncate(dashIfEmpty(a.AssigneeName), nameW-1, "…"))
	username := usernameStyle.Render(ansi.Truncate(dashIfEmpty(a.AssigneeUsername), usernameW-1, "…"))
	exp := expStyle.Render(ansi.Truncate(dashIfEmpty(prettyDate(a.ExpirationDate)), expW-1, "…"))
	return ansi.Truncate(prefix+name+sep+username+sep+exp, inner, "…")
}
