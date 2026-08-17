package ui

// /user-detail — drill-in for one User.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

func (m *Model) triggerOpenUser(userID string) tea.Cmd {
	if userID == "" {
		return nil
	}
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	d.UserCur = userID
	d.UserActionCur = 0
	m.setTab(TabUserDetail)
	o, ok := m.currentOrg()
	if !ok {
		return m.onTabChanged()
	}
	return tea.Batch(m.onTabChanged(), userFetchCmd(targetArg(o), userID))
}

func (m Model) renderUserDetail(w, innerH int) string {
	d := m.activeOrgData()
	if d == nil {
		return noOrgPlaceholder()
	}
	if d.UserCur == "" {
		return theme.Subtle.Render("  no user — back to /users")
	}
	inner := w - 4

	row := m.cursoredUserRow(d, d.UserCur)
	if row.ID == "" {
		return dimLine("  loading user…", inner)
	}

	var lines []string
	lines = append(lines, userDetailHeader("USER · "+row.Username, inner))
	lines = append(lines, "")
	lines = append(lines, kvLine("Name", dashIfEmpty(row.Name), inner))
	lines = append(lines, kvLine("Username", dashIfEmpty(row.Username), inner))
	lines = append(lines, kvLine("Profile", dashIfEmpty(row.ProfileName), inner))
	lines = append(lines, kvLine("Role", dashIfEmpty(row.UserRoleName), inner))
	lines = append(lines, kvLine("Last login", prettyDate(row.LastLoginDate), inner))
	status := "active"
	statusStyle := lipgloss.NewStyle().Foreground(theme.Green)
	if !row.IsActive {
		status = "inactive"
		statusStyle = lipgloss.NewStyle().Foreground(theme.Red)
	}
	lines = append(lines, kvLine("Status", statusStyle.Render(status), inner))
	lines = append(lines, "")

	actions := m.cursoredUserActions(d)
	sel := clampSelectableUserIdx(actions, d.UserActionCur)
	active := m.focus == focusMain
	lines = append(lines, sectionTitle("ACTIONS"))
	actionsStart := len(lines) // absolute index of the first action row
	for i, a := range actions {
		lines = append(lines, renderUserActionLine(a, i == sel && active, active, inner))
	}
	lines = append(lines, m.renderUserAuditSections(d, inner)...)

	cursorAbs := actionsStart + sel
	return scrollLinesToCursor(lines, cursorAbs, innerH)
}

// renderUserAuditSections renders LOGIN HISTORY (latest attempts,
// failures tinted red) and ACCESS (permission sets + group/queue
// memberships) for the drilled user. Both are fetched alongside the
// user card on drill — see userFetchCmd.
func (m Model) renderUserAuditSections(d *orgData, inner int) []string {
	var lines []string
	if hist := d.UserLoginHist[d.UserCur]; len(hist) > 0 {
		lines = append(lines, "", sectionTitle("LOGIN HISTORY"))
		okStyle := lipgloss.NewStyle().Foreground(theme.Green)
		failStyle := lipgloss.NewStyle().Foreground(theme.Red)
		dim := lipgloss.NewStyle().Foreground(theme.FgDim)
		for _, h := range hist {
			status := okStyle.Render("ok")
			if h.Status != "Success" {
				status = failStyle.Render(h.Status)
			}
			app := h.Application
			if app == "" {
				app = h.LoginType
			}
			line := "  " + prettyDate(h.LoginTime) + "  " + status + "  " +
				dim.Render(h.SourceIP+" · "+app)
			lines = append(lines, ansi.Truncate(line, inner, "…"))
		}
	}
	access, ok := d.UserAccessMap[d.UserCur]
	if !ok || (len(access.PermSets) == 0 && len(access.Groups) == 0) {
		return lines
	}
	lines = append(lines, "", sectionTitle("ACCESS"))
	dim := lipgloss.NewStyle().Foreground(theme.FgDim)
	if len(access.PermSets) > 0 {
		lines = append(lines, dim.Render("  permission sets"))
		for _, ps := range access.PermSets {
			label := "    " + ps.Label
			if ps.ViaGroup != "" {
				label += dim.Render(" (via " + ps.ViaGroup + ")")
			}
			lines = append(lines, ansi.Truncate(label, inner, "…"))
		}
	}
	if len(access.Groups) > 0 {
		lines = append(lines, dim.Render("  groups & queues"))
		for _, g := range access.Groups {
			kind := "group"
			if g.Type == "Queue" {
				kind = "queue"
			}
			lines = append(lines, ansi.Truncate(
				"    "+g.Name+dim.Render(" · "+kind), inner, "…"))
		}
	}
	return lines
}

func renderUserActionLine(a userActionRow, cursored, active bool, inner int) string {
	if a.Separator {
		return "  " + dimLine(strings.Repeat("─", clamp(inner-2, 1, 24)), inner)
	}
	prefix := "  "
	barColor := theme.Muted
	if cursored {
		if active {
			barColor = theme.BorderHi
		}
		prefix = lipgloss.NewStyle().Foreground(barColor).Render("▌") + " "
	}
	labelStyle := lipgloss.NewStyle().Foreground(theme.Fg)
	if !a.Allowed {
		labelStyle = lipgloss.NewStyle().Foreground(theme.FgDim)
	} else if cursored {
		labelStyle = labelStyle.Bold(true)
	}
	line := prefix + labelStyle.Render(a.Label)
	if cursored {
		tail := "  ↵ run"
		if !a.Allowed {
			tail = "  " + a.Reason
		} else if a.Hint != "" {
			tail = "  " + a.Hint
		}
		tailStyle := lipgloss.NewStyle().Foreground(theme.FgDim)
		if ansi.StringWidth(line)+ansi.StringWidth(tail) <= inner {
			line += tailStyle.Render(tail)
		}
	}
	return ansi.Truncate(line, inner, "…")
}

func (m Model) sidebarUserDetail(inner int) string {
	ctx, ok := m.userRowContext()
	if !ok {
		return sideEmpty("no user")
	}
	return m.sidebarRowContext("USER · CONTEXT", inner, ctx)
}

func (m Model) userRowContext() (rowContext, bool) {
	d := m.activeOrgData()
	if d == nil || d.UserCur == "" {
		return rowContext{}, false
	}
	actions := m.cursoredUserActions(d)
	sel := clampSelectableUserIdx(actions, d.UserActionCur)
	navHints := detailNavHints(false)
	if sel < 0 || sel >= len(actions) {
		return rowContext{Hints: navHints}, true
	}
	a := actions[sel]
	ctx := rowContext{
		Heading: a.Label,
		Help:    a.Hint,
		Hints:   navHints,
	}
	if !a.Allowed {
		ctx.Blocked = a.Reason
	}
	switch {
	case a.Mutates && a.Confirm != "":
		ctx.Routing = "writes to Salesforce · confirms first"
	case a.Mutates:
		ctx.Routing = "writes to Salesforce"
	default:
		ctx.Routing = "opens a browser / yanks to clipboard — no write"
	}
	// Destructive-ish user ops flagged so the heading reads red.
	switch a.ID {
	case "deactivate", "freeze", "reset-password":
		ctx.Danger = true
	}
	return ctx, true
}

func (m *Model) moveUserDetailCursor(delta int) {
	d := m.activeOrgData()
	if d == nil || d.UserCur == "" {
		return
	}
	actions := m.cursoredUserActions(d)
	if len(actions) == 0 {
		return
	}
	d.UserActionCur = stepThroughSelectable(actions, d.UserActionCur, delta)
}

// clampSelectableUserIdx is clampSelectableIdx for the userActionRow
// slice (the sidebar variant works on []actionRow).
func clampSelectableUserIdx(rows []userActionRow, sel int) int {
	if len(rows) == 0 {
		return 0
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= len(rows) {
		sel = len(rows) - 1
	}
	if !rows[sel].Separator {
		return sel
	}
	for i := sel; i < len(rows); i++ {
		if !rows[i].Separator {
			return i
		}
	}
	for i := sel; i >= 0; i-- {
		if !rows[i].Separator {
			return i
		}
	}
	return sel
}

func (m Model) cursoredUserRow(d *orgData, userID string) sf.UserRow {
	if d == nil || userID == "" {
		return sf.UserRow{}
	}
	if d.UserDetailRows != nil {
		if r, ok := d.UserDetailRows[userID]; ok && r.ID != "" {
			return r
		}
	}
	for _, lv := range d.ChipUsersList {
		if lv == nil {
			continue
		}
		for _, u := range lv.Items() {
			if u.ID == userID {
				return u
			}
		}
	}
	for _, u := range d.HomeUserList.Items() {
		if u.ID == userID {
			return u
		}
	}
	return sf.UserRow{ID: userID}
}

type userFetchedMsg struct {
	UserID   string
	Row      sf.UserRow
	Login    sf.UserLoginRow
	LoginErr error // non-nil and ignored when no UserLogin row exists yet
	Err      error
	History  []sf.LoginHistoryRow
	Access   sf.UserAccess
}

func userFetchCmd(target, userID string) tea.Cmd {
	return func() tea.Msg {
		row, err := sf.FetchUser(target, userID)
		login, loginErr := sf.FetchUserLogin(target, userID)
		hist, _ := sf.UserLoginHistory(target, userID, 15)
		access, _ := sf.FetchUserAccess(target, userID)
		return userFetchedMsg{
			UserID:   userID,
			Row:      row,
			Login:    login,
			LoginErr: loginErr,
			Err:      err,
			History:  hist,
			Access:   access,
		}
	}
}

func (m *Model) applyUserFetched(msg userFetchedMsg) tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	if msg.Err != nil {
		m.flash("user fetch failed: " + msg.Err.Error())
		return nil
	}
	if d.UserDetailRows == nil {
		d.UserDetailRows = map[string]sf.UserRow{}
	}
	d.UserDetailRows[msg.UserID] = msg.Row
	if d.UserLoginRows == nil {
		d.UserLoginRows = map[string]sf.UserLoginRow{}
	}
	d.UserLoginRows[msg.UserID] = msg.Login
	if d.UserLoginHist == nil {
		d.UserLoginHist = map[string][]sf.LoginHistoryRow{}
	}
	d.UserLoginHist[msg.UserID] = msg.History
	if d.UserAccessMap == nil {
		d.UserAccessMap = map[string]sf.UserAccess{}
	}
	d.UserAccessMap[msg.UserID] = msg.Access
	return nil
}

func userDetailHeader(text string, inner int) string {
	style := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	return lipgloss.NewStyle().Width(inner).Render("  " + style.Render(text))
}
