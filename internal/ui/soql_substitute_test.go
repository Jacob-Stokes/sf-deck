package ui

import (
	"strings"
	"testing"
)

func TestSubstituteSOQL_ReplacesKnownTokens(t *testing.T) {
	body := "SELECT Id FROM Opportunity WHERE OwnerId = '$ME' AND Org = '$ORG' AND CreatedDate >= $TODAY"
	got := substituteSOQL(body, soqlSubstitutions{
		UserID:   "005xx000001AAA",
		OrgAlias: "acme-test",
		Today:    "2026-05-01",
	})
	if !strings.Contains(got, "'005xx000001AAA'") {
		t.Errorf("$ME not substituted: %q", got)
	}
	if !strings.Contains(got, "'acme-test'") {
		t.Errorf("$ORG not substituted: %q", got)
	}
	if !strings.Contains(got, ">= 2026-05-01") {
		t.Errorf("$TODAY not substituted: %q", got)
	}
	if strings.Contains(got, "$ME") || strings.Contains(got, "$ORG") || strings.Contains(got, "$TODAY") {
		t.Errorf("token still present: %q", got)
	}
}

func TestSubstituteSOQL_LongTokenWinsOverShort(t *testing.T) {
	// $ME_USERNAME must replace before $ME, otherwise "$ME_USERNAME"
	// becomes "<userid>_USERNAME".
	body := "SELECT Id FROM User WHERE Username = '$ME_USERNAME' AND Id = '$ME'"
	got := substituteSOQL(body, soqlSubstitutions{
		UserID:   "005xx000001AAA",
		Username: "alice@org.test",
	})
	if !strings.Contains(got, "'alice@org.test'") {
		t.Errorf("$ME_USERNAME not substituted correctly: %q", got)
	}
	if !strings.Contains(got, "'005xx000001AAA'") {
		t.Errorf("$ME not substituted correctly: %q", got)
	}
	if strings.Contains(got, "AAA_USERNAME") {
		t.Errorf("ordering bug — $ME ate $ME_USERNAME's prefix: %q", got)
	}
}

func TestSubstituteSOQL_LeavesUnresolvedTokensInPlace(t *testing.T) {
	// When a token's value is empty (resource not loaded), the token
	// should stay in the body so a later re-load can complete the
	// substitution. We never silently substitute "''".
	body := "SELECT Id FROM User WHERE Id = '$ME'"
	got := substituteSOQL(body, soqlSubstitutions{}) // all zero
	if !strings.Contains(got, "$ME") {
		t.Errorf("empty UserID should preserve $ME, got: %q", got)
	}
}

func TestSubstituteSOQL_NoTokensNoChange(t *testing.T) {
	body := "SELECT Id, Name FROM Account ORDER BY CreatedDate DESC LIMIT 50"
	got := substituteSOQL(body, soqlSubstitutions{
		UserID:   "005xx",
		OrgAlias: "x",
		Today:    "y",
		NowISO:   "z",
		Username: "u",
	})
	if got != body {
		t.Errorf("body without tokens should be unchanged, got: %q", got)
	}
}

func TestSubstituteSOQL_MultipleOccurrences(t *testing.T) {
	body := "$ME or $ME or $ME"
	got := substituteSOQL(body, soqlSubstitutions{UserID: "X"})
	if got != "X or X or X" {
		t.Errorf("all occurrences should substitute: %q", got)
	}
}
