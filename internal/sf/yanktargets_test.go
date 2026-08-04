package sf

import "testing"

// TestComponentTypesAreYankable locks in the fix for "yank offers only
// URLs, no API name / label / Id" on component resources. Each type
// below must implement Yankable and produce at least an API-name value
// target (not just its URL Targets()).
func TestComponentTypesAreYankable(t *testing.T) {
	cases := []struct {
		name    string
		y       Yankable
		wantAPI string // value expected on the "api" (or username) row
	}{
		{"ApexClassRow", ApexClassRow{Name: "AccountService", ID: "01p000000000001"}, "AccountService"},
		{"LWCBundle", LWCBundle{DeveloperName: "myCmp", MasterLabel: "My Cmp", ID: "0Rb0000000001"}, "myCmp"},
		{"AuraBundle", AuraBundle{DeveloperName: "myAura", ID: "0Rb0000000002"}, "myAura"},
		{"PermissionSet", PermissionSet{Name: "Admin_PS", Label: "Admin PS", ID: "0PS000000000001"}, "Admin_PS"},
		{"PermissionSetGroup", PermissionSetGroup{DeveloperName: "PSG1", MasterLabel: "PSG One", ID: "0PG00000000001"}, "PSG1"},
		{"Profile", Profile{Name: "System Administrator", ID: "00e000000000001"}, "System Administrator"},
		{"QueueRow", QueueRow{DeveloperName: "Support_Q", Name: "Support Queue", ID: "00G000000000001"}, "Support_Q"},
		{"PublicGroupRow", PublicGroupRow{DeveloperName: "All_Sales", Name: "All Sales", ID: "00G000000000002"}, "All_Sales"},
		{"CustomLabelRow", CustomLabelRow{Name: "Greeting", MasterLabel: "Greeting", Value: "Hi", ID: "1010000000001"}, "Greeting"},
		{"MetaEntityRow", MetaEntityRow{QualifiedApiName: "Country__mdt", Label: "Country", KeyPrefix: "m00"}, "Country__mdt"},
		{"StaticResourceRow", StaticResourceRow{Name: "logo", ID: "081000000000001"}, "logo"},
		{"NamedCredentialRow", NamedCredentialRow{DeveloperName: "MyAPI", Endpoint: "https://x", ID: "0XA00000000001"}, "MyAPI"},
	}
	for _, c := range cases {
		ts := c.y.YankTargets()
		if len(ts) == 0 {
			t.Errorf("%s: YankTargets() empty — value-yanks missing", c.name)
			continue
		}
		// The first value target carries the primary identity string.
		if ts[0].Value != c.wantAPI {
			t.Errorf("%s: first yank value = %q, want %q", c.name, ts[0].Value, c.wantAPI)
		}
		if ts[0].Shortcut == "" {
			t.Errorf("%s: first yank target has no accelerator", c.name)
		}
	}
}

// TestUserRowYankTargets: username is the primary copy for a user.
func TestUserRowYankTargets(t *testing.T) {
	ts := UserRow{Username: "a@b.com", Name: "Ada B", ID: "005000000000001"}.YankTargets()
	if len(ts) != 3 || ts[0].Value != "a@b.com" {
		t.Fatalf("UserRow yank targets = %+v, want username-first (3 rows)", ts)
	}
}

// TestYankLabelDroppedWhenEqualsAPI: no redundant "Label" row when the
// label just repeats the API name.
func TestYankLabelDroppedWhenEqualsAPI(t *testing.T) {
	// MasterLabel == DeveloperName → only api + id, no label row.
	ts := LWCBundle{DeveloperName: "same", MasterLabel: "same", ID: "0Rb1"}.YankTargets()
	for _, y := range ts {
		if y.ID == "label" {
			t.Errorf("label row should be dropped when it equals API name: %+v", ts)
		}
	}
}
