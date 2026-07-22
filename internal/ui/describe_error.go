package ui

// Friendlier describe-error messaging.
//
// A describe can fail with a bare Salesforce error string that's
// technically accurate but misleading — most notably NOT_FOUND, which
// fires both for "object genuinely gone" AND "object exists in the
// catalog but the describe endpoint denied you" (common for protected
// managed-package objects). We can't tell those apart from the 404
// alone, so the hint states only what we CAN prove and hedges the rest
// honestly:
//
//   - namespaced object (Q9__X__c) → definitely managed; say so.
//   - in the sObject catalog but describe 404s → exists in the list,
//     so it's access-denied OR deleted-since-list-loaded (stale).
//   - not in the catalog → most likely stale / deleted.
//
// Non-NOT_FOUND errors pass through unchanged — they're usually
// self-explanatory (timeouts, auth, malformed).

import "strings"

// describeErrorLine renders the describe error for sobject as a red
// line plus, when useful, a dim explanatory hint on the next line.
// Returns the joined block. err must be non-nil.
func (m Model) describeErrorLine(sobject string, err error) string {
	msg := err.Error()
	line := redLine("  error: " + msg)
	hint := m.describeErrorHint(sobject, msg)
	if hint == "" {
		return line
	}
	return line + "\n" + dimLine("  "+hint, 9999)
}

// describeErrorHint returns the honest one-line explanation for a
// describe failure, or "" when the raw error already says enough.
func (m Model) describeErrorHint(sobject, msg string) string {
	// Only NOT_FOUND is ambiguous enough to warrant a hint.
	if !strings.Contains(msg, "NOT_FOUND") {
		return ""
	}
	if ns := managedNamespaceOf(sobject); ns != "" {
		// The namespace prefix is proof it's a managed-package object —
		// state that plainly. The 404 then means the package author
		// marked it protected, or your user wasn't granted access.
		return "managed-package object (namespace '" + ns + "') — describe denied; " +
			"it's likely protected by the package or not granted to your user"
	}
	if m.sobjectInCatalog(sobject) {
		// It's in the object list yet describe 404s. We can't tell
		// access-denied from deleted-since-the-list-loaded, so say both.
		return "it's in the object list but describe was denied — either you lack access, " +
			"or it was deleted since the list loaded (press " + firstPretty(Keys.Refresh) + " to refresh the object list)"
	}
	// Not in the catalog at all — most likely a stale reference.
	return "not in the current object list — most likely deleted (stale); press " + firstPretty(Keys.Refresh) + " to refresh"
}

// managedNamespaceOf returns the managed-package namespace prefix of an
// API name (the segment before the first "__" when it isn't the custom
// "__c"/"__e"/… suffix), or "" for an unmanaged object. E.g.
// "Q9__Active_DevOps_Partner__c" → "Q9"; "Account" / "Foo__c" → "".
func managedNamespaceOf(apiName string) string {
	i := strings.Index(apiName, "__")
	if i <= 0 {
		return ""
	}
	// A custom object with no namespace is "<Name>__c" — its first "__"
	// IS the suffix, so there's nothing before it that's a namespace.
	// Namespaced names have TWO "__": "<ns>__<Name>__c". Detect by
	// checking for a second "__" after the first.
	rest := apiName[i+2:]
	if !strings.Contains(rest, "__") {
		return ""
	}
	return apiName[:i]
}

// sobjectInCatalog reports whether sobject appears in the active org's
// loaded sObject list (EntityDefinition-backed). False when the list
// isn't loaded yet — caller treats that as "can't confirm", which is
// fine since the namespace check runs first.
func (m Model) sobjectInCatalog(sobject string) bool {
	o, ok := m.currentOrg()
	if !ok {
		return false
	}
	d := m.data[o.Username]
	if d == nil || d.SObjects.FetchedAt().IsZero() {
		return false
	}
	for _, s := range d.SObjects.Value() {
		if s.Name == sobject {
			return true
		}
	}
	return false
}
