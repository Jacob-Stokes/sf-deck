package sf

import (
	"fmt"
	"sort"
	"time"
)

// active_users.go — "who's active right now", derived from AuthSession.
//
// Salesforce keeps a live row per session in AuthSession. It's queryable
// but almost nobody looks — the web UI has no "who's online" view. One
// person shows MANY sessions (every UI framework spins its own: Aura,
// Visualforce, OAuth, …), so a raw session list reads as a crowd when
// it's a handful of people. We therefore GROUP BY USER here: one row per
// person, represented by their most-recent session, with a session count
// and an "any session skipped MFA" rollup.
//
// Caveat baked into the model: a session lingers until timeout (up to
// ~2h after last activity), so "active" means "has a live session", not
// "clicking right now". LastActive (newest LastModifiedDate) is the best
// presence proxy; the "Recently active" chip narrows to a tight window.

// activeSessionTypes is the base scope: UI-family sessions plus API /
// OAuth integration sessions, so the API/integration chip has data.
var activeSessionTypes = []string{
	"UI", "Aura", "Visualforce", "UIFrontdoor", "TempUIFrontdoor",
	"API", "Oauth2", "SAML",
}

// ActiveUserRow is one user with at least one live session — their
// newest session as the representative, plus roll-ups across all of
// their sessions.
type ActiveUserRow struct {
	UserID        string
	UserName      string
	LoginType     string    // representative (newest) session's login type
	SessionType   string    // representative session type (UI / API / …)
	SecurityLevel string    // representative session: HIGH_ASSURANCE / LOW
	SourceIP      string    // representative session source IP
	Location      string    // representative session geo: "City, ISO" (best-effort)
	LastActive    time.Time // newest LastModifiedDate across the user's sessions
	Started       time.Time // representative session CreatedDate
	SessionCount  int       // how many live sessions this user has
	AnyLowMFA     bool      // true if ANY of the user's sessions is LOW security
	IsAPI         bool      // true if the representative session is an API/integration type

	// freshnessMinutes is minutes since LastActive, stamped at fetch
	// time so the "Recently active" chip can filter (Where RecentMinutes
	// <= 15) without needing the wall-clock at match time. Internal
	// filter aid, not a displayed column.
	freshnessMinutes int

	// repIsReal marks that the representative session is a real
	// (UI/browser) session rather than an API/internal one — so the
	// grouping only upgrades the representative once, on the first real
	// session it sees. Internal bookkeeping.
	repIsReal bool
}

// Field implements query.Row so chip predicates (client-side) and the
// generic sort/search engine work. The chip-filter fields — Security,
// Type, RecentMinutes, Count — are exposed here.
func (r ActiveUserRow) Field(name string) (any, bool) {
	switch name {
	case "UserId":
		return r.UserID, true
	case "User", "Users.Name", "UserName":
		return r.UserName, true
	case "LoginType":
		return r.LoginType, true
	case "SessionType", "Type":
		return r.SessionType, true
	case "SessionSecurityLevel", "Security":
		return r.SecurityLevel, true
	case "SourceIp", "IP":
		return r.SourceIP, true
	case "Location":
		return r.Location, true
	case "SessionCount", "Count":
		return r.SessionCount, true
	case "AnyLowMFA":
		return r.AnyLowMFA, true
	case "IsAPI":
		return r.IsAPI, true
	// RecentMinutes lets a chip say "active within the last N minutes"
	// (Where RecentMinutes <= 15), using the fetch-time-stamped value.
	case "RecentMinutes":
		return r.freshnessMinutes, true
	}
	return nil, false
}

// Targets: open the user's record. Active-user rows ARE users, so o
// drills straight to the person.
func (r ActiveUserRow) Targets() []OpenTarget {
	if r.UserID == "" {
		return []OpenTarget{{ID: "users", Label: "Users (Setup)",
			Path: "/lightning/setup/ManageUsers/home"}}
	}
	return []OpenTarget{
		{ID: "view", Label: "User detail",
			Path: "/lightning/r/User/" + r.UserID + "/view"},
		{ID: "users", Label: "Users (Setup)",
			Path: "/lightning/setup/ManageUsers/home"},
	}
}

// YankTargets: the name, the source IP (useful for a security note),
// and the user Id.
func (r ActiveUserRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if r.UserName != "" {
		ts = append(ts, YankTarget{ID: "name", Label: "Name", Value: r.UserName, Shortcut: "n"})
	}
	if r.SourceIP != "" {
		ts = append(ts, YankTarget{ID: "ip", Label: "Source IP", Value: r.SourceIP, Shortcut: "p"})
	}
	if r.UserID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: r.UserID, Shortcut: "i"})
	}
	return ts
}

// ActiveUsers returns one row per user with a live session, newest
// activity first. now is the reference time for the "minutes since
// active" freshness stamp — pass a real clock; callers in tests can
// pass a fixed time.
func ActiveUsers(target string, now time.Time) ([]ActiveUserRow, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	soql := "SELECT UsersId, Users.Name, LoginType, SessionType, " +
		"SessionSecurityLevel, SourceIp, CreatedDate, LastModifiedDate, " +
		"LoginGeo.City, LoginGeo.Subdivision, LoginGeo.CountryIso FROM AuthSession " +
		"WHERE SessionType IN (" + soqlIDList(activeSessionTypes) + ") " +
		"ORDER BY LastModifiedDate DESC"
	q, err := c.QueryREST(soql, false)
	if err != nil {
		return nil, fmt.Errorf("list active sessions: %w", err)
	}
	return groupSessionsByUser(q.Records, now), nil
}

// groupSessionsByUser collapses raw session rows into one row per user.
// Records arrive newest-first (ORDER BY LastModifiedDate DESC), so the
// FIRST session seen for a user is their representative (newest) one;
// later sessions only bump the count and the AnyLowMFA rollup.
func groupSessionsByUser(records []map[string]any, now time.Time) []ActiveUserRow {
	byUser := map[string]*ActiveUserRow{}
	order := []string{}
	for _, rec := range records {
		uid := asString(rec["UsersId"])
		if uid == "" {
			continue
		}
		sessType := asString(rec["SessionType"])
		sec := asString(rec["SessionSecurityLevel"])
		// A LOW-security session only counts toward the "no MFA" signal
		// when it's a REAL (browser) session. API / internal sessions
		// are commonly LOW by nature (server-to-server doesn't do MFA),
		// so folding them in would falsely flag users who actually
		// logged in with high assurance at their browser.
		low := sec != "" && sec != "HIGH_ASSURANCE" && isRepresentativeSession(sessType)
		last := parseSFDate(rec["LastModifiedDate"])

		if existing, ok := byUser[uid]; ok {
			existing.SessionCount++
			if low {
				existing.AnyLowMFA = true
			}
			// LastActive tracks the NEWEST activity across all of the
			// user's sessions, independent of which one represents them.
			if last.After(existing.LastActive) {
				existing.LastActive = last
				if !last.IsZero() {
					existing.freshnessMinutes = int(now.Sub(last).Minutes())
				}
			}
			// Upgrade the representative session when this one is a
			// better face for the user: a real (UI/browser) session
			// beats an API/internal one. Records arrive newest-first, so
			// the first real session we meet becomes representative and
			// later reals don't displace it. This keeps the card's IP /
			// MFA / security reflecting how the PERSON is logged in, not
			// a background platform call (which can be LOW security and
			// carry the literal "Salesforce.com IP").
			if isRepresentativeSession(sessType) && !existing.repIsReal {
				existing.LoginType = asString(rec["LoginType"])
				existing.SessionType = sessType
				existing.SecurityLevel = sec
				existing.SourceIP = cleanSourceIP(asString(rec["SourceIp"]))
				existing.Location = formatGeo(rec)
				existing.Started = parseSFDate(rec["CreatedDate"])
				existing.IsAPI = isAPISessionType(sessType)
				existing.repIsReal = true
			}
			continue
		}
		row := &ActiveUserRow{
			UserID:        uid,
			UserName:      relationName(rec, "Users"),
			LoginType:     asString(rec["LoginType"]),
			SessionType:   sessType,
			SecurityLevel: sec,
			SourceIP:      cleanSourceIP(asString(rec["SourceIp"])),
			Location:      formatGeo(rec),
			LastActive:    last,
			Started:       parseSFDate(rec["CreatedDate"]),
			SessionCount:  1,
			AnyLowMFA:     low,
			IsAPI:         isAPISessionType(sessType),
			repIsReal:     isRepresentativeSession(sessType),
		}
		if !last.IsZero() {
			row.freshnessMinutes = int(now.Sub(last).Minutes())
		}
		byUser[uid] = row
		order = append(order, uid)
	}
	out := make([]ActiveUserRow, 0, len(order))
	for _, uid := range order {
		out = append(out, *byUser[uid])
	}
	// order already reflects newest-first (records were sorted), but be
	// explicit so the contract survives a fetch-order change.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastActive.After(out[j].LastActive)
	})
	return out
}

// formatGeo composes a compact "City, ISO" location from a session's
// LoginGeo relationship. City is often null even when the country
// resolves, so we degrade gracefully: "Manchester, GB" → "GB" → "".
func formatGeo(rec map[string]any) string {
	geo, ok := rec["LoginGeo"].(map[string]any)
	if !ok {
		return ""
	}
	city := asString(geo["City"])
	sub := asString(geo["Subdivision"])
	iso := asString(geo["CountryIso"])
	// Prefer City; fall back to Subdivision (region/county) when the IP
	// only geolocates to that granularity — otherwise a session with a
	// known region but no city would read as a bare country ("GB"),
	// which mismatches the drill-in view (SessionRow.Location).
	place := city
	if place == "" {
		place = sub
	}
	switch {
	case place != "" && iso != "":
		return place + ", " + iso
	case place != "":
		return place
	default:
		return iso
	}
}

func isAPISessionType(t string) bool {
	switch t {
	case "API", "Oauth2":
		return true
	}
	return false
}

// isRepresentativeSession reports whether a session type reflects a
// real human at a browser — the kind that should represent the user in
// the grouped view. API / OAuth / internal-platform sessions are
// background activity and make poor representatives (they can be LOW
// security and carry "Salesforce.com IP").
func isRepresentativeSession(t string) bool {
	switch t {
	case "UI", "Aura", "Visualforce", "Setup", "Content",
		"UIFrontdoor", "TempUIFrontdoor":
		return true
	}
	return false
}

// cleanSourceIP normalises AuthSession.SourceIp for display. Salesforce
// substitutes the literal "Salesforce.com IP" for sessions that
// originate inside its own infrastructure (API / server-to-server /
// platform) rather than from an external client — it's not an address,
// so we relabel it honestly instead of showing it as if it were an IP.
func cleanSourceIP(ip string) string {
	if ip == "Salesforce.com IP" {
		return "internal (Salesforce)"
	}
	return ip
}
