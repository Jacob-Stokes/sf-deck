package sf

import "testing"

func TestValidateSalesforceID(t *testing.T) {
	for _, id := range []string{"005000000000001", "005000000000001AAA"} {
		if err := ValidateSalesforceID(id); err != nil {
			t.Fatalf("valid id %q rejected: %v", id, err)
		}
	}
	for _, id := range []string{"", "005", "00500000000000'", "005000000000001/.."} {
		if err := ValidateSalesforceID(id); err == nil {
			t.Fatalf("invalid id %q accepted", id)
		}
	}
}

func TestValidateSOQLIdentifier(t *testing.T) {
	for _, name := range []string{"Account", "Account.Name", "Thing__c", "PermissionsApiEnabled"} {
		if err := ValidateSOQLIdentifier(name); err != nil {
			t.Fatalf("valid identifier %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "Account WHERE Id != null", "Account..Name", "1Account", "Account/Name"} {
		if err := ValidateSOQLIdentifier(name); err == nil {
			t.Fatalf("invalid identifier %q accepted", name)
		}
	}
}
