package ui

// Friendlier describe-error messaging.

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

func (m Model) describeErrorHint(sobject, msg string) string {
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
	return "not in the current object list — most likely deleted (stale); press " + firstPretty(Keys.Refresh) + " to refresh"
}

func managedNamespaceOf(apiName string) string {
	i := strings.Index(apiName, "__")
	if i <= 0 {
		return ""
	}
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
