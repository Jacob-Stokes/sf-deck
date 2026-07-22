package ui

// SOQL token substitution.
//
// Saved queries can reference org-portable tokens that resolve to
// the active org's identity at load time:
//
//   $ME           → current user 18-char Id
//   $ME_USERNAME  → current user Username
//   $ORG          → org alias
//   $TODAY        → YYYY-MM-DD
//   $NOW          → ISO 8601 timestamp (current second, UTC)
//
// Future: $ORG_ID once we surface the organisation Id on Home or
// OrgInfo. Today neither resource carries it.
//
// Substitution happens when the user loads a saved query into the
// editor — the editor body shows the resolved values, so what the
// user runs is exactly what they see. The on-disk saved query
// keeps the original tokens, so the same row works across orgs.
//
// Tokens that can't be resolved (e.g. $ME before OrgInfo loads) are
// left in place so a later re-load can complete the substitution.
// We never silently substitute an empty string.

import (
	"strings"
	"time"
)

// soqlSubstitutions is the resolved-values bundle. Empty fields
// signal "couldn't resolve" — substituteSOQL leaves the matching
// tokens in the body untouched rather than producing a query
// fragment with empty quotes.
type soqlSubstitutions struct {
	UserID   string
	Username string
	OrgAlias string
	Today    string
	NowISO   string
}

// substituteSOQL applies the substitutions table to body, returning
// the rewritten string. Tokens are matched literally — no regex —
// so a token inside a quoted string literal still gets replaced,
// which is intentional: SOQL has no escape for these tokens
// otherwise, and the user-visible behaviour is "replace what looks
// like a token wherever it appears."
func substituteSOQL(body string, s soqlSubstitutions) string {
	pairs := []struct {
		token string
		value string
	}{
		{"$ME_USERNAME", s.Username}, // longer tokens first so $ME doesn't shadow $ME_USERNAME
		{"$ME", s.UserID},
		{"$ORG", s.OrgAlias},
		{"$TODAY", s.Today},
		{"$NOW", s.NowISO},
	}
	for _, p := range pairs {
		if p.value == "" {
			continue
		}
		body = strings.ReplaceAll(body, p.token, p.value)
	}
	return body
}

// substitutionsFor builds the active substitutions table from the
// current org context. Falls back to empty fields where the
// underlying resource hasn't loaded — substituteSOQL preserves the
// token in that case so a subsequent reload completes the swap.
func (m Model) substitutionsFor(d *orgData) soqlSubstitutions {
	s := soqlSubstitutions{
		Today:  time.Now().UTC().Format("2006-01-02"),
		NowISO: time.Now().UTC().Format(time.RFC3339),
	}
	if len(m.orgs) > 0 {
		o := m.orgs[m.selected]
		s.OrgAlias = o.Alias
		s.Username = o.Username
	}
	if d != nil {
		// Home resource carries UserID once fetched. OrgID isn't
		// surfaced on any resource yet; add when Home or OrgInfo
		// grows the field.
		s.UserID = d.Home.Value().UserID
	}
	return s
}
