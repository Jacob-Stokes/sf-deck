package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/services/userops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// userActionRow describes one action shown in the sidebar list. ID
// drives dispatch in handleUserDetailAction; Run is invoked when the
// user hits Enter on the row.
type userActionRow struct {
	ID      string
	Label   string
	Hint    string
	Confirm string // empty → no confirmation; otherwise the question shown in the choice modal
	Allowed bool
	Reason  string
	Mutates bool
	Kind    settings.WriteKind
	Run     func(m *Model, alias, userID string) tea.Cmd
	// Separator marks the row as a visual divider rather than a real
	// action. Cursor movement skips it; Enter is a no-op.
	Separator bool
}

// userActionsFor returns the per-frame action menu for the cursored
// user. State-aware: Activate vs Deactivate flips on IsActive, Freeze
// vs Unfreeze on UserLogin.IsFrozen. Disabled actions render dimmed
// with a Reason.
//
// orgID is the org's 15-char Org Id (00D...) needed to build the
// servlet.su URL for the "Login as user" action. loginRowKnown
// signals whether we've already fetched the UserLogin row — when
// false, freeze toggles render disabled with a "loading" hint to
// avoid flicker on first paint.
func userActionsFor(u sf.UserRow, login sf.UserLoginRow, loginRowKnown bool, orgID string) []userActionRow {
	rows := []userActionRow{
		{
			ID:      "reset-password",
			Label:   "Reset password (email)",
			Hint:    "Email a temp password; user must change at next login.",
			Confirm: "Reset password for " + u.Username + "?",
			Allowed: u.IsActive,
			Reason:  "user is inactive",
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userResetPasswordCmd(m, alias, userID)
			},
		},
		{
			ID:      "reset-password-link",
			Label:   "Get password reset link",
			Hint:    "Generate a one-time URL and yank it to the clipboard.",
			Confirm: "Generate password reset link for " + u.Username + "?",
			Allowed: u.IsActive,
			Reason:  "user is inactive",
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userPasswordResetLinkCmd(m, alias, userID)
			},
		},
		{
			ID:      "reset-security-token",
			Label:   "Reset security token",
			Hint:    "Open the user's settings page (admin reset isn't supported by SF REST).",
			Allowed: true,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return openUserLightning(m, alias, userID, "security-token")
			},
		},
		{Separator: true},
	}

	switch {
	case !loginRowKnown:
		rows = append(rows, userActionRow{
			ID: "freeze-loading", Label: "Freeze user", Hint: "loading login state…", Allowed: false, Reason: "loading…",
		})
	case login.ID == "":
		rows = append(rows, userActionRow{
			ID: "freeze-na", Label: "Freeze user",
			Hint:    "User has never logged in — Salesforce hasn't created a UserLogin row yet.",
			Allowed: false,
			Reason:  "no UserLogin row yet",
		})
	case login.IsFrozen:
		rows = append(rows, userActionRow{
			ID:      "unfreeze",
			Label:   "Unfreeze user",
			Hint:    "Set UserLogin.IsFrozen=false. User can log in again.",
			Confirm: "Unfreeze " + u.Username + "?",
			Allowed: true,
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userSetFrozenCmd(m, alias, userID, false)
			},
		})
	default:
		rows = append(rows, userActionRow{
			ID:      "freeze",
			Label:   "Freeze user",
			Hint:    "Block login immediately without releasing the licence (reversible).",
			Confirm: "Freeze " + u.Username + "?",
			Allowed: true,
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userSetFrozenCmd(m, alias, userID, true)
			},
		})
	}

	if u.IsActive {
		rows = append(rows, userActionRow{
			ID:      "deactivate",
			Label:   "Deactivate user",
			Hint:    "Set IsActive=false. Frees up the licence; harder to reverse than freeze.",
			Confirm: "Deactivate " + u.Username + "?",
			Allowed: true,
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userSetActiveCmd(m, alias, userID, false)
			},
		})
	} else {
		rows = append(rows, userActionRow{
			ID:      "activate",
			Label:   "Activate user",
			Hint:    "Set IsActive=true. User can log in again (consumes a licence seat).",
			Confirm: "Re-activate " + u.Username + "?",
			Allowed: true,
			Mutates: true,
			Kind:    settings.WriteAnonymous,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return userSetActiveCmd(m, alias, userID, true)
			},
		})
	}

	rows = append(rows, userActionRow{Separator: true})
	rows = append(rows,
		userActionRow{
			ID:      "login-as",
			Label:   "Login as user",
			Hint:    "Open Salesforce as this user (su flow).",
			Allowed: u.IsActive && orgID != "",
			Reason: func() string {
				if !u.IsActive {
					return "user is inactive"
				}
				return "org id not loaded yet"
			}(),
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return openLoginAs(m, alias, userID, orgID)
			},
		},
		userActionRow{
			ID:      "open-detail",
			Label:   "Open in Salesforce",
			Hint:    "Lightning user detail page.",
			Allowed: true,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return openUserLightning(m, alias, userID, "view")
			},
		},
		userActionRow{
			ID:      "open-permsets",
			Label:   "View Permission Set Assignments",
			Hint:    "Lightning related-list of perm-sets on this user.",
			Allowed: true,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return openUserLightning(m, alias, userID, "permsets")
			},
		},
		userActionRow{
			ID:      "open-login-history",
			Label:   "View Login History",
			Hint:    "Open the user's recent LoginHistory in Salesforce.",
			Allowed: true,
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return openUserLightning(m, alias, userID, "login-history")
			},
		},
	)

	rows = append(rows, userActionRow{Separator: true})
	rows = append(rows,
		userActionRow{
			ID:      "yank-id",
			Label:   "Yank user Id",
			Hint:    "Copy the User.Id (15 / 18 char) to the clipboard.",
			Allowed: u.ID != "",
			Reason:  "no Id loaded yet",
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return yankToClipboardCmd(userID, "user Id")
			},
		},
		userActionRow{
			ID:      "yank-username",
			Label:   "Yank username",
			Hint:    "Copy the username (the @-domain login) to the clipboard.",
			Allowed: u.Username != "",
			Reason:  "no username loaded yet",
			Run: func(m *Model, alias, userID string) tea.Cmd {
				return yankToClipboardCmd(u.Username, "username")
			},
		},
	)
	return rows
}

// activateUserDetail runs the action row currently under the
// main-pane cursor. Wired as TabUserDetail's Activate closure.
func (m *Model) activateUserDetail() tea.Cmd {
	d := m.activeOrgData()
	if d == nil || d.UserCur == "" {
		return nil
	}
	actions := m.cursoredUserActions(d)
	sel := clampSelectableUserIdx(actions, d.UserActionCur)
	if sel < 0 || sel >= len(actions) {
		return nil
	}
	a := actions[sel]
	if a.Separator {
		return nil
	}
	if !a.Allowed {
		m.flash(a.Reason)
		return nil
	}
	if len(m.orgs) == 0 {
		return nil
	}
	alias := targetArg(m.orgs[m.selected])
	if a.Mutates {
		if ok, reason := m.canWriteCurrent(a.Kind); !ok {
			m.flash(reason)
			return nil
		}
	}
	userID := d.UserCur
	if a.Confirm != "" {
		runFn := a.Run
		return m.openChoiceModal(choiceModalState{
			Title: a.Confirm,
			Hint:  a.Hint,
			Options: []choiceOption{
				{Label: "Yes, do it", Hint: a.Hint, Value: true},
				{Label: "Cancel", Hint: "do nothing", Value: false, Cancel: true},
			},
			Save: func(val any) error { return nil },
			OnSuccessTyped: func(val any) tea.Cmd {
				if v, _ := val.(bool); !v {
					return nil
				}
				return runFn(m, alias, userID)
			},
		})
	}
	return a.Run(m, alias, userID)
}

// cursoredUserActions resolves the action menu for the cursored
// user, threading in the cached UserLogin row + the current admin
// User Id so freeze + login-as can render correctly.
func (m Model) cursoredUserActions(d *orgData) []userActionRow {
	if d == nil || d.UserCur == "" {
		return nil
	}
	row := m.cursoredUserRow(d, d.UserCur)
	login, known := sf.UserLoginRow{}, false
	if d.UserLoginRows != nil {
		login, known = d.UserLoginRows[d.UserCur]
	}
	orgID := ""
	if o, ok := m.currentOrg(); ok {
		orgID = o.OrgID
	}
	actions := userActionsFor(row, login, known, orgID)
	for i := range actions {
		if actions[i].Separator || !actions[i].Mutates || !actions[i].Allowed {
			continue
		}
		if ok, reason := m.canWriteCurrent(actions[i].Kind); !ok {
			actions[i].Allowed = false
			actions[i].Reason = reason
		}
	}
	return actions
}

// userActionDoneMsg is dispatched after a destructive user action
// (reset / deactivate) returns. Caller handles the flash + refetch.
type userActionDoneMsg struct {
	UserID string
	Action string
	Err    error
}

func userResetPasswordCmd(m *Model, alias, userID string) tea.Cmd {
	service := userWriteService(m)
	return func() tea.Msg {
		_, err := service.ResetPassword(context.Background(), userops.Input{Target: alias, UserID: userID})
		return userActionDoneMsg{UserID: userID, Action: "reset-password", Err: err}
	}
}

func userSetActiveCmd(m *Model, alias, userID string, active bool) tea.Cmd {
	service := userWriteService(m)
	return func() tea.Msg {
		_, err := service.SetActive(context.Background(), userops.Input{Target: alias, UserID: userID}, active)
		action := "deactivate"
		if active {
			action = "activate"
		}
		return userActionDoneMsg{UserID: userID, Action: action, Err: err}
	}
}

// openUserLightning opens a Lightning page tied to userID. Slots:
//
//	view           — user detail page
//	permsets       — Permission Set Assignments related list
//	login-history  — LoginHistory related list
//	security-token — the user's personal "Reset My Security Token" page
func openUserLightning(m *Model, alias, userID, slot string) tea.Cmd {
	if m == nil || userID == "" {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	path := "/lightning/r/User/" + userID + "/view"
	label := "User detail"
	switch slot {
	case "permsets":
		path = "/lightning/r/User/" + userID + "/related/PermissionSetAssignments/view"
		label = "Permission Set Assignments"
	case "login-history":
		path = "/lightning/r/User/" + userID + "/related/UserLogins/view"
		label = "Login History"
	case "security-token":
		path = "/_ui/system/security/ResetApiTokenEdit"
		label = "Reset Security Token"
	}
	m.flash("opening " + label + "…")
	return m.openInBrowserCmd(o, sf.OpenTarget{ID: slot, Label: label, Path: path})
}

// applyUserActionDone folds a destructive-action result into
// orgData. Refreshes the user row + the AllUsers resource so the
// list reflects the new state.
func (m *Model) applyUserActionDone(msg userActionDoneMsg) tea.Cmd {
	if msg.Err != nil {
		m.flash(msg.Action + " failed: " + msg.Err.Error())
		return nil
	}
	switch msg.Action {
	case "reset-password":
		m.flash("password reset — temp password emailed to user")
	case "reset-password-link":
		m.flash("password reset link copied to clipboard")
	case "deactivate":
		m.flash("user deactivated")
	case "activate":
		m.flash("user activated")
	case "freeze":
		m.flash("user frozen")
	case "unfreeze":
		m.flash("user unfrozen")
	case "yank-user Id":
		m.flash("yanked user Id")
	case "yank-username":
		m.flash("yanked username")
	}
	if msg.UserID == "" {
		return nil
	}
	d := m.activeOrgData()
	if d == nil || len(m.orgs) == 0 {
		return nil
	}
	target := targetArg(m.orgs[m.selected])
	return tea.Batch(
		userFetchCmd(target, msg.UserID),
		refreshActiveUsersChip(*m, d),
		d.Home.Refresh(m.cache),
	)
}

// userPasswordResetLinkCmd asks SF for a one-time password-reset URL
// and yanks it to the clipboard. The URL is short-lived and tied to
// this user; the admin pastes it directly to the user.
func userPasswordResetLinkCmd(m *Model, alias, userID string) tea.Cmd {
	service := userWriteService(m)
	return func() tea.Msg {
		result, err := service.GenerateResetLink(context.Background(), userops.Input{Target: alias, UserID: userID})
		if err != nil {
			return userActionDoneMsg{UserID: userID, Action: "reset-password-link", Err: err}
		}
		if err := writeClipboard(result.URL); err != nil {
			return userActionDoneMsg{UserID: userID, Action: "reset-password-link", Err: fmt.Errorf("copy reset link: %w", err)}
		}
		return userActionDoneMsg{UserID: userID, Action: "reset-password-link"}
	}
}

// userSetFrozenCmd flips UserLogin.IsFrozen via SF REST.
func userSetFrozenCmd(m *Model, alias, userID string, frozen bool) tea.Cmd {
	service := userWriteService(m)
	return func() tea.Msg {
		_, err := service.SetFrozen(context.Background(), userops.Input{Target: alias, UserID: userID}, frozen)
		action := "unfreeze"
		if frozen {
			action = "freeze"
		}
		return userActionDoneMsg{UserID: userID, Action: action, Err: err}
	}
}

// yankToClipboardCmd writes value to the system clipboard and flashes
// "yanked <label>". label appears in the success flash so the user
// knows what landed in their clipboard.
func yankToClipboardCmd(value, label string) tea.Cmd {
	return func() tea.Msg {
		err := writeClipboard(value)
		return userActionDoneMsg{Action: "yank-" + label, Err: err}
	}
}

// openLoginAs opens Salesforce as the target user via the su flow.
// orgID is the 15-char Org Id; the admin doing the impersonation is
// inferred server-side from the session cookie.
func openLoginAs(m *Model, alias, targetUserID, orgID string) tea.Cmd {
	if m == nil || targetUserID == "" || orgID == "" {
		return nil
	}
	o, ok := m.currentOrg()
	if !ok {
		return nil
	}
	path := sf.InternalLoginAsPath(orgID, targetUserID)
	m.flash("opening su flow…")
	return m.openInBrowserCmd(o, sf.OpenTarget{ID: "su", Label: "Login as user", Path: path})
}

// stepThroughSelectable moves the cursor by delta, skipping
// separator rows. delta sign drives direction; positive deltas
// advance, negatives retreat. Cursor clamps to the first / last
// selectable row at the boundaries.
func stepThroughSelectable(rows []userActionRow, cur, delta int) int {
	if len(rows) == 0 {
		return 0
	}
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	for delta > 0 {
		next := cur + step
		if next < 0 || next >= len(rows) {
			break
		}
		cur = next
		if !rows[cur].Separator {
			delta--
		}
	}
	for cur >= 0 && cur < len(rows) && rows[cur].Separator {
		cur += step
	}
	if cur < 0 {
		cur = 0
	}
	if cur >= len(rows) {
		cur = len(rows) - 1
	}
	return cur
}
