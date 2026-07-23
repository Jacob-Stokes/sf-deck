package settings

import (
	"testing"
	"time"
)

func TestLegalAcceptanceIsVersioned(t *testing.T) {
	var nilSettings *Settings
	if nilSettings.LegalAccepted("2026-07-23") {
		t.Fatal("nil settings must not bypass legal acknowledgement")
	}
	s := &Settings{}
	if s.LegalAccepted("2026-07-23") {
		t.Fatal("fresh settings unexpectedly accepted")
	}
	at := time.Date(2026, 7, 23, 12, 30, 0, 0, time.FixedZone("BST", 3600))
	s.AcceptLegal("2026-07-23", at)
	if !s.LegalAccepted("2026-07-23") {
		t.Fatal("current revision not accepted")
	}
	if s.LegalAccepted("2026-08-01") {
		t.Fatal("acceptance must not carry across policy revisions")
	}
	version, acceptedAt := s.LegalAcceptance()
	if version != "2026-07-23" || acceptedAt != "2026-07-23T11:30:00Z" {
		t.Fatalf("acceptance = %q, %q", version, acceptedAt)
	}
}
