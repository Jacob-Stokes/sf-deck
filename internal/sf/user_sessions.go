package sf

import (
	"fmt"
	"time"
)

// user_sessions.go — the per-user session drill behind /users → Active.
//
// The top-level Active list groups sessions BY USER (one representative
// row each). Drilling a user lands here: every one of THAT user's live
// sessions, each with its full detail — location (city/country),
// browser + platform, IP, MFA/security, type, TTL, timestamps. This is
// where all the AuthSession + LoginGeo + LoginHistory fields live
// without cluttering the summary.

// SessionRow is one AuthSession with its geo + device joins resolved.
type SessionRow struct {
	ID            string
	SessionType   string
	LoginType     string
	SecurityLevel string // HIGH_ASSURANCE / STANDARD / LOW
	SourceIP      string // cleaned (internal-sentinel relabelled)
	City          string
	Country       string // full name, e.g. "United Kingdom"
	CountryISO    string // "GB"
	Subdivision   string // region, e.g. "Manchester"
	Browser       string // from LoginHistory, e.g. "Chrome 148"
	Platform      string // "Mac OSX"
	Application   string // "Browser", "Salesforce CLI", …
	Started       time.Time
	LastActive    time.Time
	SecondsValid  int // NumSecondsValid — session TTL
}

func (r SessionRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "SessionType", "Type":
		return r.SessionType, true
	case "LoginType":
		return r.LoginType, true
	case "SessionSecurityLevel", "Security":
		return r.SecurityLevel, true
	case "SourceIp", "IP":
		return r.SourceIP, true
	case "City":
		return r.City, true
	case "Country":
		return r.Country, true
	case "Browser":
		return r.Browser, true
	case "Platform":
		return r.Platform, true
	case "Application":
		return r.Application, true
	case "LastActive":
		return r.LastActive, true
	}
	return nil, false
}

// Location is the compact "City, ISO" (degrading to region/country).
func (r SessionRow) Location() string {
	switch {
	case r.City != "" && r.CountryISO != "":
		return r.City + ", " + r.CountryISO
	case r.Subdivision != "" && r.CountryISO != "":
		return r.Subdivision + ", " + r.CountryISO
	case r.City != "":
		return r.City
	default:
		return r.CountryISO
	}
}

func (r SessionRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "sessions", Label: "Session management (Setup)",
			Path: "/lightning/setup/SessionManagement/home"},
	}
}

func (r SessionRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if r.SourceIP != "" {
		ts = append(ts, YankTarget{ID: "ip", Label: "Source IP", Value: r.SourceIP, Shortcut: "p"})
	}
	if loc := r.Location(); loc != "" {
		ts = append(ts, YankTarget{ID: "loc", Label: "Location", Value: loc, Shortcut: "l"})
	}
	if r.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: r.ID, Shortcut: "i"})
	}
	return ts
}

// UserSessions returns every live session for one user, newest first,
// with the geo + device joins resolved.
func UserSessions(target, userID string) ([]SessionRow, error) {
	if userID == "" {
		return nil, nil
	}
	return queryRows(target,
		fmt.Sprintf(
			"SELECT Id, SessionType, LoginType, SessionSecurityLevel, SourceIp, "+
				"NumSecondsValid, CreatedDate, LastModifiedDate, "+
				"LoginGeo.City, LoginGeo.Country, LoginGeo.CountryIso, LoginGeo.Subdivision, "+
				"LoginHistory.Browser, LoginHistory.Platform, LoginHistory.Application "+
				"FROM AuthSession WHERE UsersId = '%s' ORDER BY LastModifiedDate DESC",
			sqlEscape(userID)),
		false, mapSessionRow)
}

func mapSessionRow(rec map[string]any) SessionRow {
	row := SessionRow{
		ID:            asString(rec["Id"]),
		SessionType:   asString(rec["SessionType"]),
		LoginType:     asString(rec["LoginType"]),
		SecurityLevel: asString(rec["SessionSecurityLevel"]),
		SourceIP:      cleanSourceIP(asString(rec["SourceIp"])),
		SecondsValid:  asInt(rec["NumSecondsValid"]),
		Started:       parseSFDate(rec["CreatedDate"]),
		LastActive:    parseSFDate(rec["LastModifiedDate"]),
	}
	if geo, ok := rec["LoginGeo"].(map[string]any); ok {
		row.City = asString(geo["City"])
		row.Country = asString(geo["Country"])
		row.CountryISO = asString(geo["CountryIso"])
		row.Subdivision = asString(geo["Subdivision"])
	}
	if lh, ok := rec["LoginHistory"].(map[string]any); ok {
		row.Browser = asString(lh["Browser"])
		row.Platform = asString(lh["Platform"])
		row.Application = asString(lh["Application"])
	}
	return row
}
