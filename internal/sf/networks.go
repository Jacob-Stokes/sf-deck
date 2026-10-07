package sf

import (
	"fmt"
)

// Network is one Experience Cloud site. Status filters to "Live"
// (DownForMaintenance / UnderConstruction are skipped) since you can
// only log into a Live site.
type Network struct {
	ID            string
	Name          string
	UrlPathPrefix string // url segment under the org's community host; empty = default
	Status        string
}

// ListNetworks returns the org's Live Experience Cloud sites in name
// order. One round-trip; callers should cache (Network metadata
// rarely changes).
func ListNetworks(orgTarget string) ([]Network, error) {
	soql := "SELECT Id, Name, UrlPathPrefix, Status FROM Network " +
		"WHERE Status = 'Live' ORDER BY Name"
	q, err := Query(orgTarget, soql, false)
	if err != nil {
		return nil, fmt.Errorf("ListNetworks: %w", err)
	}
	out := make([]Network, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, Network{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			UrlPathPrefix: asString(r["UrlPathPrefix"]),
			Status:        asString(r["Status"]),
		})
	}
	return out, nil
}
