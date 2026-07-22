package ui

// /user-detail — drill-in for one User.
//
// Main pane shows the read-only user card (Name / Username / Profile /
// Role / Last login / Status) followed by a navigable ACTIONS list.
// Most user actions are operations / launchers rather than edits of a
// displayed property, so — unlike the metadata-edit detail surfaces
// where each editable property is its own row — the whole action list
// lives in the main pane:
//
//   - Reset password / get reset link (SF emails or yanks a temp URL)
//   - Freeze / Unfreeze (UserLogin.IsFrozen)
//   - Activate / Deactivate (PATCH IsActive)
//   - Login as user (su flow) · Open in Setup · perm-sets · login history
//   - Yank Id / username to the clipboard
//
// Arrows walk the list (skipping separators); Enter runs the cursored
// action; Esc returns to /users. Destructive / mutating actions gate
// behind a confirm modal + the org safety level. The right sidebar is
// INFO-ONLY — safe to hide.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
)

// triggerOpenUser drills into the User detail tab. Sets UserCur,
// switches tabs, kicks off a fresh User fetch so the card shows the
// latest values (post any chip-driven mutations earlier in the
// session). The ACTIONS list in the main pane is the cursor target —
// no body/sidebar focus split anymore.
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

// renderUserDetail draws the body of TabUserDetail: the read-only
// user card followed by the navigable ACTIONS list. Most user actions
// are operations / launchers (reset password, login-as, open in
// Setup, yank Id…) rather than edits of a displayed property, so the
// whole action list lives here in the main pane. Arrow keys walk it
// (skipping separators), Enter runs the cursored action. The sidebar
// is INFO-ONLY.
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

	// ACTIONS — the navigable list. Cursor (d.UserActionCur) indexes
	// the full slice (separators included); stepThroughSelectable keeps
	// it off divider rows.
	actions := m.cursoredUserActions(d)
	sel := clampSelectableUserIdx(actions, d.UserActionCur)
	active := m.focus == focusMain
	lines = append(lines, sectionTitle("ACTIONS"))
	actionsStart := len(lines) // absolute index of the first action row
	for i, a := range actions {
		lines = append(lines, renderUserActionLine(a, i == sel && active, active, inner))
	}
	// Audit sections — read-only, below the actions. Soft-absent
	// when the org denies LoginHistory / PSA reads.
	lines = append(lines, m.renderUserAuditSections(d, inner)...)

	// Keep the cursored action visible when the list overflows.
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

// renderUserActionLine renders one action row in the main-pane list.
// Separators become a thin divider; the cursored row gets a bar +
// trailing "↵ run" affordance (or the disabled reason when blocked).
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

// sidebarUserDetail renders the right-pane panel — a context panel for
// the cursored action in the main-pane list.
func (m Model) sidebarUserDetail(inner int) string {
	ctx, ok := m.userRowContext()
	if !ok {
		return sideEmpty("no user")
	}
	return m.sidebarRowContext("USER · CONTEXT", inner, ctx)
}

// userRowContext builds the context for the cursored user action.
// (false when there's no user loaded.)
func (m Model) userRowContext() (rowContext, bool) {
	d := m.activeOrgData()
	if d == nil || d.UserCur == "" {
		return rowContext{}, false
	}
	actions := m.cursoredUserActions(d)
	sel := clampSelectableUserIdx(actions, d.UserActionCur)
	// User launchers live in the main-pane list, so the hint bar is
	// just navigation.
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

// moveUserDetailCursor walks the main-pane ACTIONS list, skipping
// separator rows.
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

// cursoredUserRow returns the freshest UserRow we have for userID.
// Prefers d.UserDetailRows (post-fetch snapshot); falls back to the
// list-row data so the card renders during the initial load.
func (m Model) cursoredUserRow(d *orgData, userID string) sf.UserRow {
	if d == nil || userID == "" {
		return sf.UserRow{}
	}
	if d.UserDetailRows != nil {
		if r, ok := d.UserDetailRows[userID]; ok && r.ID != "" {
			return r
		}
	}
	// Walk every cached per-chip All Users ListView for the row —
	// the user may have drilled in from any chip.
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

// userFetchedMsg is dispatched after FetchUser returns. Caches the
// fresh row so the detail card updates without a full list refresh.
// UserLogin sub-row is fetched alongside so the freeze toggle can
// render state-aware on the first frame.
type userFetchedMsg struct {
	UserID   string
	Row      sf.UserRow
	Login    sf.UserLoginRow
	LoginErr error // non-nil and ignored when no UserLogin row exists yet
	Err      error
	// Audit sections — soft-fail: nil on query error so the card +
	// actions still render when LoginHistory/PSA aren't readable
	// (field-level perms vary by org).
	History []sf.LoginHistoryRow
	Access  sf.UserAccess
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

// applyUserFetched folds a userFetchedMsg into orgData.
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
	// Cache even when LoginErr is set — empty Id signals "no row";
	// the action menu renders the freeze toggle as disabled.
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

// userDetailHeader renders the bold title row at the top of the
// User detail body. Local helper so we don't collide with the
// shared kvLine helper in format.go.
func userDetailHeader(text string, inner int) string {
	style := lipgloss.NewStyle().Foreground(theme.Fg).Bold(true)
	return lipgloss.NewStyle().Width(inner).Render("  " + style.Render(text))
}
