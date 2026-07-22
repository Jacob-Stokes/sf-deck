package sf

import (
	"testing"
	"time"
)

func TestGroupSessionsByUser(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	rec := func(uid, name, stype, sec, when string) map[string]any {
		return map[string]any{
			"UsersId":              uid,
			"Users":                map[string]any{"Name": name},
			"SessionType":          stype,
			"SessionSecurityLevel": sec,
			"LastModifiedDate":     when,
			"CreatedDate":          when,
			"SourceIp":             "1.2.3.4",
			"LoginType":            "Application",
		}
	}
	// Newest-first, as the SOQL ORDER BY delivers.
	records := []map[string]any{
		rec("005A", "Ada", "UI", "HIGH_ASSURANCE", "2026-07-08T14:58:00.000+0000"),     // Ada newest (2m ago)
		rec("005A", "Ada", "Aura", "LOW", "2026-07-08T14:40:00.000+0000"),              // Ada 2nd session, LOW
		rec("005B", "Bob", "Oauth2", "HIGH_ASSURANCE", "2026-07-08T13:00:00.000+0000"), // Bob, API, 2h ago
	}

	rows := groupSessionsByUser(records, now)

	if len(rows) != 2 {
		t.Fatalf("expected 2 users, got %d", len(rows))
	}
	// Newest activity first → Ada then Bob.
	ada, bob := rows[0], rows[1]
	if ada.UserName != "Ada" || bob.UserName != "Bob" {
		t.Fatalf("order/name wrong: %q, %q", ada.UserName, bob.UserName)
	}
	// Ada: represented by her NEWEST session (UI, HIGH), 2 sessions,
	// AnyLowMFA true because her Aura session was LOW.
	if ada.SessionCount != 2 {
		t.Errorf("Ada session count = %d, want 2", ada.SessionCount)
	}
	if ada.SecurityLevel != "HIGH_ASSURANCE" {
		t.Errorf("Ada representative security = %q, want HIGH_ASSURANCE (newest)", ada.SecurityLevel)
	}
	if !ada.AnyLowMFA {
		t.Error("Ada AnyLowMFA should be true (one session was LOW)")
	}
	if ada.freshnessMinutes != 2 {
		t.Errorf("Ada freshnessMinutes = %d, want 2", ada.freshnessMinutes)
	}
	if ada.IsAPI {
		t.Error("Ada IsAPI should be false (UI session)")
	}
	// Bob: single API session, not low, ~120 min stale.
	if !bob.IsAPI {
		t.Error("Bob IsAPI should be true (Oauth2)")
	}
	if bob.AnyLowMFA {
		t.Error("Bob AnyLowMFA should be false")
	}
	if bob.freshnessMinutes != 120 {
		t.Errorf("Bob freshnessMinutes = %d, want 120", bob.freshnessMinutes)
	}
}

// TestGroupPrefersRealSessionOverAPI reproduces the Praveena case: a
// user's NEWEST session is a background API session (LOW security,
// "Salesforce.com IP"), but they also have a real browser session. The
// representative row must reflect the BROWSER session — real IP, high
// assurance — and must NOT be flagged as no-MFA for the API session.
func TestGroupPrefersRealSessionOverAPI(t *testing.T) {
	now := time.Date(2026, 7, 8, 15, 40, 0, 0, time.UTC)
	full := func(uid, name, stype, sec, ip, when string) map[string]any {
		return map[string]any{
			"UsersId": uid, "Users": map[string]any{"Name": name},
			"SessionType": stype, "SessionSecurityLevel": sec,
			"SourceIp": ip, "LastModifiedDate": when, "CreatedDate": when,
			"LoginType": "Application",
		}
	}
	records := []map[string]any{
		// Newest = API, LOW, Salesforce-internal IP.
		full("005P", "Praveena", "API", "LOW", "Salesforce.com IP", "2026-07-08T15:36:00.000+0000"),
		// Older = real browser session, HIGH, real IP.
		full("005P", "Praveena", "UI", "HIGH_ASSURANCE", "84.64.180.58", "2026-07-08T15:30:00.000+0000"),
	}
	rows := groupSessionsByUser(records, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 user, got %d", len(rows))
	}
	p := rows[0]
	if p.SourceIP != "84.64.180.58" {
		t.Errorf("representative IP = %q, want the browser IP 84.64.180.58 (not the API session)", p.SourceIP)
	}
	if p.SecurityLevel != "HIGH_ASSURANCE" {
		t.Errorf("representative security = %q, want HIGH_ASSURANCE", p.SecurityLevel)
	}
	if p.SessionType != "UI" {
		t.Errorf("representative session type = %q, want UI", p.SessionType)
	}
	if p.AnyLowMFA {
		t.Error("AnyLowMFA should be false — the only LOW session was a background API call")
	}
	if p.IsAPI {
		t.Error("IsAPI should be false — the representative is a browser session")
	}
	// LastActive still tracks the newest activity (the API session).
	if got := p.LastActive.Format("15:04"); got != "15:36" {
		t.Errorf("LastActive = %s, want 15:36 (newest across all sessions)", got)
	}
}

// TestFormatGeo composes the compact top-level location, degrading
// City → Subdivision → country ISO. The Subdivision fallback keeps the
// list view in sync with the drill-in (SessionRow.Location): a session
// whose IP resolves to a region but not a city ("Southwark, GB") must
// not read as a bare country in the list.
func TestFormatGeo(t *testing.T) {
	geo := func(city, sub, iso string) map[string]any {
		return map[string]any{"LoginGeo": map[string]any{"City": city, "Subdivision": sub, "CountryIso": iso}}
	}
	cases := []struct {
		rec  map[string]any
		want string
	}{
		{geo("Manchester", "", "GB"), "Manchester, GB"},
		{geo("Manchester", "Greater Manchester", "GB"), "Manchester, GB"}, // City wins over subdivision
		{geo("", "Southwark", "GB"), "Southwark, GB"},                     // subdivision fallback (the bug)
		{geo("", "", "GB"), "GB"},                                         // neither → bare country
		{geo("Manchester", "", ""), "Manchester"},
		{geo("", "Southwark", ""), "Southwark"},
		{geo("", "", ""), ""},
		{map[string]any{}, ""}, // no LoginGeo at all (API/internal session)
	}
	for _, c := range cases {
		if got := formatGeo(c.rec); got != c.want {
			t.Errorf("formatGeo(%v) = %q, want %q", c.rec["LoginGeo"], got, c.want)
		}
	}
}

// TestSessionRowLocation covers the per-session location degradation:
// city → subdivision → country ISO.
func TestSessionRowLocation(t *testing.T) {
	cases := []struct {
		r    SessionRow
		want string
	}{
		{SessionRow{City: "Manchester", CountryISO: "GB"}, "Manchester, GB"},
		{SessionRow{Subdivision: "Newham", CountryISO: "GB"}, "Newham, GB"},
		{SessionRow{CountryISO: "GB"}, "GB"},
		{SessionRow{}, ""},
	}
	for _, c := range cases {
		if got := c.r.Location(); got != c.want {
			t.Errorf("Location(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

// TestCleanSourceIP relabels Salesforce's internal-IP sentinel.
func TestCleanSourceIP(t *testing.T) {
	if got := cleanSourceIP("Salesforce.com IP"); got != "internal (Salesforce)" {
		t.Errorf("cleanSourceIP sentinel = %q", got)
	}
	if got := cleanSourceIP("84.64.180.58"); got != "84.64.180.58" {
		t.Errorf("cleanSourceIP passthrough = %q", got)
	}
}

// TestActiveUserRowFieldExposesChipFilters: the fields the built-in
// chips filter on must be resolvable + the right type for query.Eval.
func TestActiveUserRowFieldExposesChipFilters(t *testing.T) {
	r := ActiveUserRow{AnyLowMFA: true, IsAPI: true, freshnessMinutes: 5}
	if v, ok := r.Field("AnyLowMFA"); !ok || v != true {
		t.Errorf("AnyLowMFA field = %v,%v", v, ok)
	}
	if v, ok := r.Field("IsAPI"); !ok || v != true {
		t.Errorf("IsAPI field = %v,%v", v, ok)
	}
	if v, ok := r.Field("RecentMinutes"); !ok || v != 5 {
		t.Errorf("RecentMinutes field = %v,%v", v, ok)
	}
}
