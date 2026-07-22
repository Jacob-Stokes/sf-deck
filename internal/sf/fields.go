package sf

// Tooling-API helpers scoped to CustomField: ID lookup (naming is
// quirky across namespaces + custom/standard/CMT entities, so we
// centralise the strip-and-retry logic here) and the metadata update
// shim (delegates to the generic UpdateToolingMetadata).
//
// Everything else Tooling-write that operates on CustomField.Metadata
// lives in field_actions.go in the ui package — this file is purely
// sobject plumbing.

import (
	"fmt"
	"strings"
)

// CustomFieldID looks up the Tooling-API CustomField record for a
// given (sobject, field-API-name) pair. Returns the 18-char Tooling
// Id — different from the 15/18-char data-record Id.
//
// Naming quirks we handle:
//   - TableEnumOrId wants the bare object name (no namespace, no
//     __c for custom objects — it's essentially the CustomObject's
//     DeveloperName, or the literal name for standard objects).
//   - CustomField.DeveloperName is the bare field name (no namespace,
//     no __c).
//
// We receive the full API names ("chatfeedauto__Test_Object__c.
// chatfeedauto__MyField__c"), strip namespace + __c here, and cache
// the result in the UI so the lookup only happens once per field.
func CustomFieldID(target, sobject, fieldAPIName string) (string, error) {
	fieldDev := developerNameFromAPIName(fieldAPIName)

	c, err := RESTClient(target)
	if err != nil {
		return "", err
	}

	// TableEnumOrId behaves inconsistently across org types for
	// namespaced / custom objects. We try the most likely candidates
	// in order: bare developer name, full API name, and finally look
	// up the CustomObject row by DeveloperName and use its Id.
	//
	// Once one returns a row, we take it.
	candidates := []string{developerNameFromAPIName(sobject)}
	if developerNameFromAPIName(sobject) != sobject {
		candidates = append(candidates, sobject)
	}

	for _, tbl := range candidates {
		soql := fmt.Sprintf(
			"SELECT Id FROM CustomField "+
				"WHERE TableEnumOrId = '%s' AND DeveloperName = '%s'",
			sqlEscape(tbl), sqlEscape(fieldDev))
		q, err := c.QueryREST(soql, true)
		if err != nil {
			return "", fmt.Errorf("lookup CustomField id: %w", err)
		}
		if len(q.Records) > 0 {
			id := asString(q.Records[0]["Id"])
			if id != "" {
				return id, nil
			}
		}
	}

	// Last resort: resolve TableEnumOrId → CustomObject.Id, then
	// query CustomField by that Id.
	objDev := developerNameFromAPIName(sobject)
	objSOQL := fmt.Sprintf(
		"SELECT Id FROM CustomObject WHERE DeveloperName = '%s'",
		sqlEscape(objDev))
	if q, err := c.QueryREST(objSOQL, true); err == nil && len(q.Records) > 0 {
		objID := asString(q.Records[0]["Id"])
		if objID != "" {
			fieldSOQL := fmt.Sprintf(
				"SELECT Id FROM CustomField "+
					"WHERE TableEnumOrId = '%s' AND DeveloperName = '%s'",
				sqlEscape(objID), sqlEscape(fieldDev))
			if q2, err := c.QueryREST(fieldSOQL, true); err == nil && len(q2.Records) > 0 {
				if id := asString(q2.Records[0]["Id"]); id != "" {
					return id, nil
				}
			}
		}
	}

	return "", fmt.Errorf(
		"no CustomField row for %s.%s (tried DeveloperName=%q against TableEnumOrId variants)",
		sobject, fieldAPIName, fieldDev)
}

// developerNameFromAPIName strips a namespace prefix ("ns__") and a
// "__c" suffix from a Salesforce API name. Standard objects/fields
// (no namespace, no __c) are returned unchanged.
//
// Examples:
//
//	chatfeedauto__Test_Object__c → Test_Object
//	MyField__c                   → MyField
//	Account                      → Account
//	chatfeedauto__MyThing        → MyThing  (managed-package standard)
func developerNameFromAPIName(name string) string {
	// __c suffix (or __mdt for custom metadata types — treat the
	// same way since CustomField.DeveloperName drops both).
	for _, suf := range []string{"__c", "__mdt", "__e", "__b"} {
		if strings.HasSuffix(name, suf) {
			name = name[:len(name)-len(suf)]
			break
		}
	}
	// Namespace prefix: the first "__" split leaves <ns, rest>. The
	// "rest" is what the Tooling API keys on.
	if i := strings.Index(name, "__"); i > 0 {
		name = name[i+2:]
	}
	return name
}
