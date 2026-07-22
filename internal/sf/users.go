package sf

// All-users query for the /users tab's "All Users" subtab. Distinct
// from Users() in orgstats.go which is the home-tab recent-logins
// summary (capped at ~10). This pulls a broader slice with a higher
// cap so chip predicates have material to filter against.
//
// Cap is intentional: large orgs can have tens of thousands of User
// records (community licences, retired accounts). 2000 is the sweet
// spot for chip-driven UX without dragging the network round-trip.
// Surfaces wanting genuinely-all-users go through SF list views via
// the records-mode chip toggle.

import (
	"encoding/json"
	"fmt"
)

// UsersList is the result of a chip-driven User fetch — rows plus
// the unbounded match count Salesforce reported. Mirrors
// RecordsList on the records side so renderers can surface a
// "showing X of Y · capped" hint when the cap kicked in.
type UsersList struct {
	Rows      []UserRow
	TotalSize int
	Query     string // SOQL we ran (handy for the dim status line)
}

// UsersForSOQL runs an arbitrary User-shaped SOQL and decodes the
// result into a UsersList. Used by the chip-keyed fetch path on
// /users · All users — each chip compiles its predicate to SOQL and
// hands it here, mirroring sf.RecordsForSOQL on the records side.
// cap caps the cursor follow client-side; cap <= 0 disables the cap.
//
// soql is expected to be a fully-formed SELECT against User; the
// caller is responsible for projecting at least Id, Name, Username,
// Profile.Name, UserRole.Name, LastLoginDate, IsActive (anything
// missing renders as a dash in the table).
func UsersForSOQL(target, soql string, cap int) (UsersList, error) {
	out := UsersList{Query: soql}
	q, err := QueryCapped(target, soql, false, cap)
	if err != nil {
		return out, err
	}
	out.TotalSize = q.TotalSize
	out.Rows = make([]UserRow, 0, len(q.Records))
	for _, r := range q.Records {
		row := UserRow{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			Username:      asString(r["Username"]),
			LastLoginDate: asString(r["LastLoginDate"]),
		}
		if b, ok := r["IsActive"].(bool); ok {
			row.IsActive = b
		}
		if p, ok := r["Profile"].(map[string]any); ok {
			row.ProfileName = asString(p["Name"])
		}
		if u, ok := r["UserRole"].(map[string]any); ok {
			row.UserRoleName = asString(u["Name"])
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// FetchUser returns a single User row by Id, with the same fields the
// list query projects. Used by the User detail surface to refresh the
// header card after an action mutates state.
func FetchUser(target, userID string) (UserRow, error) {
	soql := fmt.Sprintf(
		"SELECT Id, Name, Username, Profile.Name, UserRole.Name, LastLoginDate, IsActive "+
			"FROM User WHERE Id = '%s' LIMIT 1", userID)
	q, err := Query(target, soql, false)
	if err != nil {
		return UserRow{}, err
	}
	if len(q.Records) == 0 {
		return UserRow{}, fmt.Errorf("user %s not found", userID)
	}
	r := q.Records[0]
	row := UserRow{
		ID:            asString(r["Id"]),
		Name:          asString(r["Name"]),
		Username:      asString(r["Username"]),
		LastLoginDate: asString(r["LastLoginDate"]),
	}
	if b, ok := r["IsActive"].(bool); ok {
		row.IsActive = b
	}
	if p, ok := r["Profile"].(map[string]any); ok {
		row.ProfileName = asString(p["Name"])
	}
	if u, ok := r["UserRole"].(map[string]any); ok {
		row.UserRoleName = asString(u["Name"])
	}
	return row, nil
}

// ResetUserPassword fires a password reset on the given User. Uses
// the standard User password sub-resource — the platform sends the
// generated temp password to the user's email and forces a change
// on next login. Returns nil on success; the API returns 204.
func ResetUserPassword(target, userID string) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	path := c.APIPath("sobjects/User/" + userID + "/password")
	if _, err := c.delete(path); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	return nil
}

// SetUserActive flips IsActive on the given User. Used to deactivate
// (and reactivate) accounts from the User detail action menu.
func SetUserActive(target, userID string, active bool) error {
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"IsActive": active})
	path := c.APIPath("sobjects/User/" + userID)
	if _, err := c.patch(path, body); err != nil {
		return fmt.Errorf("set IsActive: %w", err)
	}
	return nil
}

// UserLoginRow is a thin slice of UserLogin — the freeze flag plus
// the row Id needed to PATCH it. Distinct from User.IsActive: freezing
// blocks login *immediately* without releasing the licence and is
// reversible without consuming user-management seats.
type UserLoginRow struct {
	ID       string
	UserID   string
	IsFrozen bool
}

// FetchUserLogin returns the UserLogin row keyed off UserId. Returns
// (zero, nil) if no row exists yet — Salesforce creates one on first
// login attempt, so brand-new users won't have one and freezing
// before first login isn't supported.
func FetchUserLogin(target, userID string) (UserLoginRow, error) {
	soql := fmt.Sprintf(
		"SELECT Id, UserId, IsFrozen FROM UserLogin WHERE UserId = '%s' LIMIT 1", userID)
	q, err := Query(target, soql, false)
	if err != nil {
		return UserLoginRow{}, err
	}
	if len(q.Records) == 0 {
		return UserLoginRow{}, nil
	}
	r := q.Records[0]
	row := UserLoginRow{
		ID:     asString(r["Id"]),
		UserID: asString(r["UserId"]),
	}
	if b, ok := r["IsFrozen"].(bool); ok {
		row.IsFrozen = b
	}
	return row, nil
}

// SetUserFrozen toggles UserLogin.IsFrozen for the given User.
// Returns an explanatory error if no UserLogin row exists yet (user
// has never tried to log in).
func SetUserFrozen(target, userID string, frozen bool) error {
	login, err := FetchUserLogin(target, userID)
	if err != nil {
		return err
	}
	if login.ID == "" {
		return fmt.Errorf("user has no UserLogin row yet — they must attempt login at least once before freeze can be set")
	}
	c, err := RESTClient(target)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"IsFrozen": frozen})
	path := c.APIPath("sobjects/UserLogin/" + login.ID)
	if _, err := c.patch(path, body); err != nil {
		return fmt.Errorf("set IsFrozen: %w", err)
	}
	return nil
}

// GenerateUserPasswordResetLink returns a one-time-use password reset
// URL for the given User. POST to the password sub-resource (with
// the field "NewPassword" omitted) returns a URL the admin can hand
// to the user directly — distinct from DELETE which emails a temp
// password.
func GenerateUserPasswordResetLink(target, userID string) (string, error) {
	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}
	path := c.APIPath("sobjects/User/" + userID + "/password")
	raw, err := c.post(path, []byte("{}"))
	if err != nil {
		return "", fmt.Errorf("generate reset link: %w", err)
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse reset link response: %w", err)
	}
	return resp.URL, nil
}

// LoginHistoryRow is one LoginHistory record — the per-user login
// audit trail (incl. FAILED attempts, which the User.LastLoginDate
// field never shows).
type LoginHistoryRow struct {
	LoginTime   string
	SourceIP    string
	LoginType   string
	Application string
	Browser     string
	Status      string
}

// UserLoginHistory returns the user's most recent logins, newest
// first. Read-only; verified on phd 2026-06-12.
func UserLoginHistory(target, userID string, limit int) ([]LoginHistoryRow, error) {
	if limit <= 0 {
		limit = 25
	}
	soql := fmt.Sprintf(
		"SELECT LoginTime, SourceIp, LoginType, Application, Browser, Status "+
			"FROM LoginHistory WHERE UserId = '%s' ORDER BY LoginTime DESC LIMIT %d",
		sqlEscape(userID), limit)
	q, err := Query(target, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]LoginHistoryRow, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, LoginHistoryRow{
			LoginTime:   asString(r["LoginTime"]),
			SourceIP:    asString(r["SourceIp"]),
			LoginType:   asString(r["LoginType"]),
			Application: asString(r["Application"]),
			Browser:     asString(r["Browser"]),
			Status:      asString(r["Status"]),
		})
	}
	return out, nil
}

// UserAccess is the "what does this user HAVE" summary: permission
// sets (excluding the profile-owned shadow permset) and group/queue
// memberships.
type UserAccess struct {
	PermSets []UserPermSetRow
	Groups   []UserGroupRow
}

type UserPermSetRow struct {
	Label    string
	ViaGroup string // PSG label when assigned through a group, else ""
}

type UserGroupRow struct {
	Name string
	Type string // "Queue" / "Regular" (public group) / role-ish types
}

// FetchUserAccess pulls both halves of the access summary. Two
// SOQLs; verified on phd 2026-06-12.
func FetchUserAccess(target, userID string) (UserAccess, error) {
	var out UserAccess
	psaSOQL := fmt.Sprintf(
		"SELECT PermissionSet.Label, PermissionSet.IsOwnedByProfile, "+
			"PermissionSetGroup.MasterLabel "+
			"FROM PermissionSetAssignment WHERE AssigneeId = '%s' "+
			"ORDER BY PermissionSet.Label", sqlEscape(userID))
	q, err := Query(target, psaSOQL, false)
	if err != nil {
		return out, err
	}
	for _, r := range q.Records {
		ps, _ := r["PermissionSet"].(map[string]any)
		if ps == nil {
			continue
		}
		if owned, ok := ps["IsOwnedByProfile"].(bool); ok && owned {
			// The profile's shadow permset — the card already shows
			// the profile by name; repeating it here is noise.
			continue
		}
		viaGroup := ""
		if psg, ok := r["PermissionSetGroup"].(map[string]any); ok {
			// PSG's display field is MasterLabel, not Name — the
			// generic relationName helper would come back empty.
			viaGroup = asString(psg["MasterLabel"])
		}
		out.PermSets = append(out.PermSets, UserPermSetRow{
			Label:    asString(ps["Label"]),
			ViaGroup: viaGroup,
		})
	}
	grpSOQL := fmt.Sprintf(
		"SELECT Group.Name, Group.Type FROM GroupMember "+
			"WHERE UserOrGroupId = '%s' ORDER BY Group.Name", sqlEscape(userID))
	q, err = Query(target, grpSOQL, false)
	if err != nil {
		return out, err
	}
	for _, r := range q.Records {
		g, _ := r["Group"].(map[string]any)
		if g == nil {
			continue
		}
		out.Groups = append(out.Groups, UserGroupRow{
			Name: asString(g["Name"]),
			Type: asString(g["Type"]),
		})
	}
	return out, nil
}
