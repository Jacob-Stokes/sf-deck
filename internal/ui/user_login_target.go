package ui

// user_login_target.go — "Log in as user" ^O menu entry for any User
// row (in /users, in SOQL results, anywhere a User record appears).
//
// Mirrors the existing User-detail action menu entry (openLoginAs in
// tab_user_actions.go) but plumbed through the open menu so it works
// from any surface. The URL is /servlet/servlet.su with
// suorgadminid + targetuserid params — distinct from the community
// variant which uses sunetwork* params.

import (
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// userLoginAsTargets returns a single "Log in as user" OpenTarget
// when the row is a User and the admin's own User.Id is known and
// different from the target. Returns nil otherwise.
//
// Eligibility checks intentionally kept light: we don't query the
// target user's IsActive or the org's "admins can log in as any
// user" setting up-front. servlet.su returns SF's own error page
// when the impersonation fails — less surprising than the menu
// entry silently vanishing for some users but not others.
// enrichUserRowTargets wraps a UserRow with the same "Log in as
// user" target that contactCommunityLoginTargets adds for Contact
// rows on /objects/<X>/Records. /users tab surfaces hand bare
// UserRows to the open menu, which doesn't go through newRecordRef
// — so this is the equivalent injection point for typed rows.
//
// Skips the entry when the row is the admin themselves (looking
// at your own row's ^O and seeing "log in as user" would be silly),
// when IsActive is false (servlet.su returns Insufficient
// Privileges for inactive users), or when the org's Org Id isn't
// known yet (shouldn't happen; comes from sfdx auth).
func (m Model) enrichUserRowTargets(u sf.UserRow) sf.UserRow {
	o, ok := m.currentOrg()
	if !ok || o.OrgID == "" || u.ID == "" || !u.IsActive {
		return u
	}
	d := m.activeOrgData()
	if d != nil {
		adminID := d.Home.Value().UserID
		if adminID != "" && len(u.ID) >= 15 && len(adminID) >= 15 && u.ID[:15] == adminID[:15] {
			return u
		}
	}
	u.ExtraTargets = append(u.ExtraTargets, sf.OpenTarget{
		ID:       "login_as_user",
		Label:    "Log in as user",
		Shortcut: "l",
		Path:     sf.InternalLoginAsPath(o.OrgID, u.ID),
	})
	return u
}

func (m Model) userLoginAsTargets(rec map[string]any, o sf.Org) []sf.OpenTarget {
	sobj, id := sf.SObjectAndIDFromRecord(rec)
	if sobj != "User" || id == "" {
		return nil
	}
	if o.OrgID == "" {
		return nil
	}
	// Skip self-impersonation. Compare on the 15-char prefix so a
	// caller passing the 18-char form still matches.
	d := m.data[o.Username]
	if d != nil {
		adminID := d.Home.Value().UserID
		if adminID != "" {
			if id == adminID || (len(id) >= 15 && len(adminID) >= 15 && id[:15] == adminID[:15]) {
				return nil
			}
		}
	}
	return []sf.OpenTarget{{
		ID:       "login_as_user",
		Label:    "Log in as user",
		Shortcut: "l",
		Path:     sf.InternalLoginAsPath(o.OrgID, id),
	}}
}
