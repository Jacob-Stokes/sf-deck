package sf

// OrgInfo is the server-side metadata about the connected org —
// distinct from the local CLI's Org struct (which carries the user-
// chosen alias + auth details). Pulled via a single SOQL against
// the Organization sObject, which is a singleton row per org.
//
// The local Alias on Org is whatever the user typed when running
// `sf org login web --alias foo`; it has no presence server-side.
// OrgInfo.Name is the actual organisation display name set in
// Setup → Company Information ("Acme Sandbox", "Acme Production")
// — what makes orgs visually distinct beyond a CLI nickname.

// OrgInfo holds the per-org Organization metadata used by the Home
// tab's identity card.
type OrgInfo struct {
	// Name is the org's display name (Setup → Company Information →
	// Organization Name). Falls back to "" when the row is unloaded;
	// caller substitutes the local alias as the user-facing label.
	Name string

	// OrganizationType is the SF edition string, e.g. "Developer
	// Edition" / "Enterprise Edition" / "Performance Edition" /
	// "Trial" / "Base Edition".
	OrganizationType string

	// InstanceName is the cloud pod the org lives on ("EU17",
	// "NA45"). Useful for "is this org slow today?" checks against
	// status.salesforce.com.
	InstanceName string

	// NamespacePrefix is set on packaging / managed-package dev orgs
	// (e.g. "acme"). Empty on regular orgs — most users will see "".
	NamespacePrefix string

	// IsSandbox + TrialExpirationDate flag the org's lifecycle. The
	// CLI's Org.IsSandbox already covers the boolean; we re-fetch so
	// the home card has a single authoritative source.
	IsSandbox bool

	// PrimaryContact is the admin's name set in Company Information.
	// Useful when an admin opens an org they don't recognise — "oh
	// right, this is Sarah's org".
	PrimaryContact string
}

// FetchOrgInfo pulls Organization for the connected org. One row,
// one round-trip — Organization is a singleton.
func FetchOrgInfo(target string) (OrgInfo, error) {
	soql := "SELECT Name, OrganizationType, InstanceName, NamespacePrefix, " +
		"IsSandbox, PrimaryContact " +
		"FROM Organization LIMIT 1"
	q, err := Query(target, soql, false)
	if err != nil {
		return OrgInfo{}, err
	}
	if len(q.Records) == 0 {
		return OrgInfo{}, nil
	}
	r := q.Records[0]
	out := OrgInfo{
		Name:             asString(r["Name"]),
		OrganizationType: asString(r["OrganizationType"]),
		InstanceName:     asString(r["InstanceName"]),
		NamespacePrefix:  asString(r["NamespacePrefix"]),
		PrimaryContact:   asString(r["PrimaryContact"]),
	}
	if v, ok := r["IsSandbox"].(bool); ok {
		out.IsSandbox = v
	}
	return out, nil
}
