package sf

// OrgInfo is the server-side metadata about the connected org —
// distinct from the local CLI's Org struct (which carries the user-
// chosen alias + auth details). Pulled via a single SOQL against
// the Organization sObject, which is a singleton row per org.

// OrgInfo holds the per-org Organization metadata used by the Home
// tab's identity card.
type OrgInfo struct {
	Name string

	OrganizationType string

	InstanceName string

	NamespacePrefix string

	// IsSandbox + TrialExpirationDate flag the org's lifecycle. The
	// CLI's Org.IsSandbox already covers the boolean; we re-fetch so
	// the home card has a single authoritative source.
	IsSandbox bool

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
