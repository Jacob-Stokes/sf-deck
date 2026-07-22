package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// rec builds a records map with the SObject type baked into attributes,
// matching what the record surfaces feed to the community-login path.
func recWith(sobj, id string, extra map[string]any) map[string]any {
	m := map[string]any{
		"attributes": map[string]any{"type": sobj, "url": "/x/" + id},
		"Id":         id,
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// TestCommunityLogin_RecordShapeResolution verifies the contact-id
// resolution that gates the "Log in to community" target — the part
// that makes it work for Person Accounts, not just Contacts. It uses
// the in-record PersonContactId path so no live SOQL is needed.
func TestCommunityLogin_RecordShapeResolution(t *testing.T) {
	// A Person Account carrying its PersonContactId on the record.
	acct := recWith("Account", "001AAA", map[string]any{"PersonContactId": "003PERSON"})
	sobj, id := sf.SObjectAndIDFromRecord(acct)
	if sobj != "Account" || id != "001AAA" {
		t.Fatalf("record shape = %q/%q, want Account/001AAA", sobj, id)
	}
	if pc, _ := acct["PersonContactId"].(string); pc != "003PERSON" {
		t.Errorf("PersonContactId not readable from record: %q", pc)
	}

	// A plain Contact: the row id IS the contact id.
	con := recWith("Contact", "003CONTACT", nil)
	if s, cid := sf.SObjectAndIDFromRecord(con); s != "Contact" || cid != "003CONTACT" {
		t.Errorf("contact shape = %q/%q, want Contact/003CONTACT", s, cid)
	}

	// A non-person Account (no PersonContactId) → nothing to resolve
	// from the record; the code would fall through to a lookup (which
	// returns "" for a business account).
	biz := recWith("Account", "001BIZ", nil)
	if pc, ok := biz["PersonContactId"].(string); ok && pc != "" {
		t.Errorf("business account should have no PersonContactId, got %q", pc)
	}

	// An unrelated object never offers community login.
	other := recWith("Opportunity", "006XYZ", nil)
	if s, _ := sf.SObjectAndIDFromRecord(other); s != "Opportunity" {
		t.Errorf("shape = %q, want Opportunity", s)
	}
}
