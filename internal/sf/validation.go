package sf

import (
	"errors"
	"fmt"
)

// ValidateSalesforceID accepts the two canonical Salesforce record-ID widths
// and rejects values that could alter a REST path or a SOQL literal.
func ValidateSalesforceID(id string) error {
	if id == "" {
		return errors.New("salesforce id is required")
	}
	if len(id) != 15 && len(id) != 18 {
		return fmt.Errorf("invalid Salesforce id %q (must be 15 or 18 characters)", id)
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !asciiAlpha(c) && (c < '0' || c > '9') {
			return fmt.Errorf("invalid Salesforce id %q (must be ASCII alphanumeric)", id)
		}
	}
	return nil
}

// ValidateSOQLIdentifier rejects syntax-bearing values before they are used as
// an sObject or field name. Relationship paths may contain dot-separated
// identifiers, while each segment must start with an ASCII letter.
func ValidateSOQLIdentifier(name string) error {
	if name == "" {
		return errors.New("SOQL identifier is required")
	}
	segmentStart := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '.' {
			if segmentStart || i == len(name)-1 {
				return fmt.Errorf("invalid SOQL identifier %q", name)
			}
			segmentStart = true
			continue
		}
		if segmentStart {
			if !asciiAlpha(c) {
				return fmt.Errorf("invalid SOQL identifier %q", name)
			}
			segmentStart = false
			continue
		}
		if !asciiAlpha(c) && (c < '0' || c > '9') && c != '_' {
			return fmt.Errorf("invalid SOQL identifier %q", name)
		}
	}
	return nil
}

func asciiAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
