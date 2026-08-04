package ui

// /users → Active → Enter: one user's live sessions in full.
//
// The Active subtab groups sessions by user (one representative row);
// this drill lists every session for the highlighted user, each with
// its location, browser/platform, IP, MFA, type, and TTL. Mirrors the
// queue-members parent→child pattern: activate sets a current-user id,
// the detail tab renders a keyed per-user list lazily fetched by
// EnsureUserSessions.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// activateActiveUser is Enter on an Active-subtab row: drill into that
// user's sessions.
func (m *Model) activateActiveUser() tea.Cmd {
	d := m.activeOrgData()
	if d == nil {
		return nil
	}
	r, ok := d.ActiveUserList.Selected()
	if !ok || r.UserID == "" {
		return nil
	}
	d.SessionUserID = r.UserID
	d.SessionUserName = r.UserName
	d.UserSessionList.ResetCursor()
	if s := d.ActiveUserList.SearchPtr(); s.Active {
		s.Active = false
		s.Committed = s.Buffer() != ""
	}
	m.setTab(TabUserSessions)
	return m.onTabChanged()
}

func (d *orgData) SyncUserSessionList() {
	if d.SessionUserID == "" {
		d.UserSessionList.Set(nil)
		return
	}
	if res := d.UserSessions[d.SessionUserID]; res != nil {
		d.UserSessionList.Set(res.Value())
		return
	}
	d.UserSessionList.Set(nil)
}

func (m Model) renderUserSessions(w, innerH int) string {
	inner := w - 4
	d := m.activeOrgData()
	if d == nil {
		return noOrgPlaceholder()
	}
	if d.SessionUserID == "" {
		return dimLine("  no user drilled in", inner)
	}
	res := d.UserSessions[d.SessionUserID]
	if res == nil || res.FetchedAt().IsZero() {
		if res != nil && res.Busy() {
			return dimLine("  loading sessions for "+d.SessionUserName+"…", inner)
		}
		return dimLine("  press "+firstPretty(Keys.Refresh)+" to load sessions", inner)
	}
	// Keep the shared list view pointed at THIS user's rows (guards
	// against a stale list after switching between drilled users).
	d.SyncUserSessionList()
	body := renderListSurface(m, &userSessionsListSurface, w, innerH, d)
	if body == "" {
		return dimLine("  loading…", inner)
	}
	return body
}

// ensureUserSessionsData / refreshUserSessionsData are the drill's
// registry data hooks.
func (m *Model) ensureUserSessionsData(d *orgData, o sf.Org) tea.Cmd {
	if d.SessionUserID == "" {
		return nil
	}
	return d.EnsureUserSessions(targetArg(o), d.SessionUserID).Ensure(m.cache)
}

func (m Model) refreshUserSessionsData(d *orgData) tea.Cmd {
	if d.SessionUserID == "" {
		return nil
	}
	return d.EnsureUserSessions(targetArg(m.orgs[m.selected]), d.SessionUserID).Refresh(m.cache)
}
