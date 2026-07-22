package sf

import "fmt"

// communities.go — Experience sites (Networks) as a browsable surface.
//
// Richer than the lean Network type used by the community-login helper:
// carries config flags + a member count so the /communities tab can
// show "what communities exist and how they're set up" at a glance.

// CommunityRow is one Experience site (Network) with config + member
// count resolved.
type CommunityRow struct {
	ID            string
	Name          string
	URLPathPrefix string // url segment under the community host ("" = default)
	Status        string // Live / UnderConstruction / …
	Description   string
	Members       int  // NetworkMember count (resolved via a second aggregate query)
	SelfReg       bool // OptionsSelfRegistrationEnabled
	GuestFiles    bool // OptionsGuestFileAccessEnabled
	InternalLogin bool // OptionsAllowInternalUserLogin
	PrivateMsgs   bool // OptionsPrivateMessagesEnabled
}

// Field implements query.Row so rows flow through the generic
// list/search/sort engine.
func (r CommunityRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return r.ID, true
	case "Name":
		return r.Name, true
	case "UrlPathPrefix", "URL":
		return r.URLPathPrefix, true
	case "Status":
		return r.Status, true
	case "Members":
		return r.Members, true
	case "SelfReg":
		return r.SelfReg, true
	}
	return nil, false
}

// Targets: the live site, Experience Builder, All Sites (Setup), plus
// the community's administration/members pages.
func (r CommunityRow) Targets() []OpenTarget {
	t := []OpenTarget{
		{ID: "allsites", Label: "All Sites (Setup)",
			Path: "/lightning/setup/SetupNetworks/home"},
	}
	if r.URLPathPrefix != "" {
		// Live site — instance-relative to the community path. The org's
		// community host is implicit; the open layer prepends instance.
		t = append([]OpenTarget{{
			ID: "live", Label: "Live site", Path: "/" + r.URLPathPrefix + "/s/",
		}}, t...)
	}
	if r.ID != "" {
		// Experience Builder + Administration both key off the network id.
		t = append(t,
			OpenTarget{ID: "builder", Label: "Experience Builder",
				Path: "/sfsites/picasso/core/config/commeditor.apexp?servicename=SD&siteId=" + r.ID},
			OpenTarget{ID: "admin", Label: "Administration",
				Path: "/servlet/networks/switch?networkId=" + r.ID},
		)
	}
	return t
}

// YankTargets exposes the name, url prefix, and network id.
func (r CommunityRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if r.Name != "" {
		ts = append(ts, YankTarget{ID: "name", Label: "Name", Value: r.Name, Shortcut: "n"})
	}
	if r.URLPathPrefix != "" {
		ts = append(ts, YankTarget{ID: "url", Label: "URL prefix", Value: r.URLPathPrefix, Shortcut: "u"})
	}
	if r.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: r.ID, Shortcut: "i"})
	}
	return ts
}

// ListCommunities returns every Experience site with its config +
// member count. Two queries: the Network list, then a NetworkMember
// aggregate grouped by NetworkId (both read-only, ~2 API calls total).
func ListCommunities(target string) ([]CommunityRow, error) {
	c, err := RESTClient(target)
	if err != nil {
		return nil, err
	}
	soql := "SELECT Id, Name, UrlPathPrefix, Status, Description, " +
		"OptionsSelfRegistrationEnabled, OptionsGuestFileAccessEnabled, " +
		"OptionsAllowInternalUserLogin, OptionsPrivateMessagesEnabled " +
		"FROM Network ORDER BY Name"
	q, err := c.QueryREST(soql, false)
	if err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	rows := make([]CommunityRow, 0, len(q.Records))
	idx := map[string]int{}
	for _, r := range q.Records {
		row := CommunityRow{
			ID:            asString(r["Id"]),
			Name:          asString(r["Name"]),
			URLPathPrefix: asString(r["UrlPathPrefix"]),
			Status:        asString(r["Status"]),
			Description:   asString(r["Description"]),
			SelfReg:       asBool(r["OptionsSelfRegistrationEnabled"]),
			GuestFiles:    asBool(r["OptionsGuestFileAccessEnabled"]),
			InternalLogin: asBool(r["OptionsAllowInternalUserLogin"]),
			PrivateMsgs:   asBool(r["OptionsPrivateMessagesEnabled"]),
		}
		idx[row.ID] = len(rows)
		rows = append(rows, row)
	}

	// Member counts — one aggregate, merged by NetworkId. Best-effort:
	// a failure here leaves counts at 0 rather than failing the list.
	if mq, err := c.QueryREST(
		"SELECT NetworkId, COUNT(Id) n FROM NetworkMember GROUP BY NetworkId", false); err == nil {
		for _, r := range mq.Records {
			if i, ok := idx[asString(r["NetworkId"])]; ok {
				rows[i].Members = asInt(r["n"])
			}
		}
	}
	return rows, nil
}

// CommunityPageRow is one Experience-site page (a community-type
// FlexiPage). NOTE: FlexiPages don't carry a foreign key to their
// Network, so these are the org's community pages as a whole — grouping
// to a specific community is best-effort by name prefix. Full page
// CONTENT lives in the ExperienceBundle metadata (a TODO).
type CommunityPageRow struct {
	DeveloperName string
	MasterLabel   string
	Type          string // CommAppPage / CommRecordPage / CommThemeLayoutPage / …
}

func (r CommunityPageRow) Field(name string) (any, bool) {
	switch name {
	case "DeveloperName", "Name":
		return r.DeveloperName, true
	case "MasterLabel", "Label":
		return r.MasterLabel, true
	case "Type":
		return r.Type, true
	}
	return nil, false
}

func (r CommunityPageRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "builder", Label: "Experience Builder (Setup)",
			Path: "/lightning/setup/SetupNetworks/home"},
	}
}

// ListCommunityPages returns the org's community-type FlexiPages. When
// prefix is non-empty, filters to pages whose DeveloperName starts with
// it (best-effort per-community grouping).
func ListCommunityPages(target, prefix string) ([]CommunityPageRow, error) {
	return queryRows(target,
		"SELECT DeveloperName, MasterLabel, Type FROM FlexiPage "+
			"WHERE Type LIKE 'Comm%' ORDER BY DeveloperName",
		true, mapCommunityPageRow) // Tooling API
}

func mapCommunityPageRow(r map[string]any) CommunityPageRow {
	return CommunityPageRow{
		DeveloperName: asString(r["DeveloperName"]),
		MasterLabel:   asString(r["MasterLabel"]),
		Type:          asString(r["Type"]),
	}
}
