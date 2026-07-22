package sf

// community_login.go — "Log in to Experience as User" plumbing.
//
// Salesforce exposes no REST/Tooling/Apex API for community login-as;
// the Lightning button posts a request that ultimately resolves to
// /servlet/servlet.su with Experience-Cloud-specific params. Same
// servlet as the internal "Log in as User" flow (see openLoginAs in
// tab_user_actions.go) but with sunetwork* params instead of
// suorgadminid/targetuserid:
//
//   /servlet/servlet.su
//      ?oid=<OrgId>
//      &sunetworkid=<NetworkId>          ← the Experience site
//      &sunetworkuserid=<TargetUserId>   ← the contact's portal user
//      &retURL=%2F<recordId or path>
//
// Stable for years per community posts + the kevina-code/
// LogIntoExperienceAsUser Apex reference impl, but technically
// undocumented — Salesforce could change it. Caller should fall back
// gracefully if the response isn't a redirect.

import (
	"fmt"
)

// ContactCommunityUserID returns the Id of the active community User
// associated with the given Contact, or "" if no active community
// user exists for that contact.
//
// A "community user" is just any User whose ContactId is set — that's
// how Salesforce models external/portal users. Standard internal users
// have ContactId = null. We filter IsActive=true because servlet.su
// won't impersonate a frozen/inactive user.
func ContactCommunityUserID(orgTarget, contactID string) (string, error) {
	if contactID == "" {
		return "", nil
	}
	soql := fmt.Sprintf(
		"SELECT Id FROM User WHERE ContactId = '%s' AND IsActive = true LIMIT 1",
		sqlEscape(contactID),
	)
	q, err := Query(orgTarget, soql, false)
	if err != nil {
		return "", fmt.Errorf("ContactCommunityUserID: %w", err)
	}
	if len(q.Records) == 0 {
		return "", nil
	}
	return asString(q.Records[0]["Id"]), nil
}

// PersonContactID returns the implicit PersonContactId behind a Person
// Account — the hidden Contact that community Users link to via
// ContactId. Returns "" when the account isn't a Person Account (or has
// no person contact). Lets the community-login flow work from a Person
// Account row, not just a Contact: resolve the person contact, then feed
// it to ContactCommunityUserID like any other contact.
func PersonContactID(orgTarget, accountID string) (string, error) {
	if accountID == "" {
		return "", nil
	}
	soql := fmt.Sprintf(
		"SELECT PersonContactId FROM Account WHERE Id = '%s' AND IsPersonAccount = true LIMIT 1",
		sqlEscape(accountID),
	)
	q, err := Query(orgTarget, soql, false)
	if err != nil {
		return "", fmt.Errorf("PersonContactID: %w", err)
	}
	if len(q.Records) == 0 {
		return "", nil
	}
	return asString(q.Records[0]["PersonContactId"]), nil
}

// CommunityLoginAsPath returns the instance-relative servlet.su path
// (no host) that, when navigated to via an authenticated browser
// session, logs the admin in as the target community user inside the
// given Experience site.
//
// The path is instance-relative because sf-deck's openInBrowserCmd
// composes the full URL by prepending the org's instance URL — same
// pattern as the internal "Login as user" action. retURL drops the
// session on the contact's record page after the impersonation lands.
func CommunityLoginAsPath(orgID, networkID, targetUserID, retURLID string) string {
	ret := "%2F"
	if retURLID != "" {
		ret = "%2F" + retURLID
	}
	return fmt.Sprintf(
		"/servlet/servlet.su?oid=%s&sunetworkid=%s&sunetworkuserid=%s&retURL=%s",
		orgID, networkID, targetUserID, ret,
	)
}

// InternalLoginAsPath returns the instance-relative servlet.su path
// for impersonating an internal (non-community) user. Mirrors the
// URL Lightning's Setup → Users → "Login" link generates.
//
// CAVEAT — `suorgadminid` is misleadingly named: it carries the
// *target* user id (the user being impersonated), NOT the admin's
// own user id. The admin is inferred from the session cookie. This
// is confirmed by browser-captured URLs from working Lightning
// buttons; the misnomer is an undocumented Salesforce gotcha.
//
// orgID is the 15-char Org Id (00D...). retURL drops the session on
// the target user's detail page with noredirect=1 so SF doesn't
// rewrite the URL; isUserEntityOverride=1 bypasses the User entity's
// owner check needed for this flow.
func InternalLoginAsPath(orgID, targetUserID string) string {
	return fmt.Sprintf(
		"/servlet/servlet.su?oid=%s&suorgadminid=%s"+
			"&retURL=%%2F%s%%3Fnoredirect%%3D1"+
			"&isUserEntityOverride=1&targetURL=%%2Fhome%%2Fhome.jsp",
		orgID, targetUserID, targetUserID,
	)
}
