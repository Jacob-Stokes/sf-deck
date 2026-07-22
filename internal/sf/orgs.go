package sf

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type Org struct {
	Alias       string `json:"alias"`
	Username    string `json:"username"`
	InstanceURL string `json:"instanceUrl"`
	OrgID       string `json:"orgId"`
	IsSandbox   bool   `json:"isSandbox"`
	IsScratch   bool   `json:"isScratch"`
	IsDevHub    bool   `json:"isDevHub"`
	Status      string `json:"connectedStatus"`
	LastUsed    string `json:"lastUsed"`
	// ExpirationDate is set for scratch orgs only ("2006-01-02").
	ExpirationDate string `json:"expirationDate"`
	// IsDefault / IsDefaultDevHub mirror the sf CLI's global
	// target-org / target-dev-hub config — which org a bare `sf`
	// command outside the deck will hit.
	IsDefault       bool `json:"isDefaultUsername"`
	IsDefaultDevHub bool `json:"isDefaultDevHubUsername"`
}

// ScratchDaysLeft returns whole days until the scratch org expires
// (0 = expires today, negative = already expired). ok=false for
// non-scratch orgs or unparseable dates.
func (o Org) ScratchDaysLeft() (int, bool) {
	if !o.IsScratch || o.ExpirationDate == "" {
		return 0, false
	}
	exp, err := time.Parse("2006-01-02", o.ExpirationDate)
	if err != nil {
		return 0, false
	}
	// Expiry is date-granular; compare against the start of today so
	// "expires 2026-06-13" reads as 1 day left throughout 2026-06-12.
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return int(exp.Sub(today).Hours() / 24), true
}

// Kind returns a short edition-ish label for the sidebar.
func (o Org) Kind() string {
	switch {
	case o.IsScratch:
		return "Scratch"
	case o.IsSandbox:
		return "Sandbox"
	case o.IsDevHub:
		return "DevHub"
	default:
		return "Production"
	}
}

// Display returns the alias if set, otherwise a trimmed username.
func (o Org) Display() string {
	if o.Alias != "" {
		return o.Alias
	}
	return o.Username
}

type sfOrgListResult struct {
	Result struct {
		Other       []Org `json:"other"`
		Sandboxes   []Org `json:"sandboxes"`
		DevHubs     []Org `json:"devHubs"`
		ScratchOrgs []Org `json:"scratchOrgs"`
	} `json:"result"`
}

// ListOrgs shells out to `sf org list --json` and returns every known org.
// Stored as a package-level var so tests can stub it without faking
// the entire shell environment.
var ListOrgs = listOrgsViaSF

func listOrgsViaSF() ([]Org, error) {
	out, err := runSF("org", "list", "--json")
	if err != nil {
		return nil, err
	}
	var parsed sfOrgListResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var all []Org
	add := func(list []Org) {
		for _, o := range list {
			if seen[o.Username] {
				continue
			}
			seen[o.Username] = true
			all = append(all, o)
		}
	}
	add(parsed.Result.Other)
	add(parsed.Result.Sandboxes)
	add(parsed.Result.DevHubs)
	add(parsed.Result.ScratchOrgs)

	// Sort alphabetically by alias (falling back to username) so the
	// list shape stays stable across refetches. The previous
	// LastUsed-descending order shuffled the rail on every API call
	// because `sf` updates LastUsed on every command — visually noisy
	// and the cause of the "active org silently jumps" bug, because
	// m.selected was an int index into this slice. The rail itself
	// groups orgs at render time via OrgGroups so this ordering only
	// affects the ungrouped tail + the underlying index space.
	sort.SliceStable(all, func(i, j int) bool {
		return orgSortKey(all[i]) < orgSortKey(all[j])
	})
	return all, nil
}

// orgSortKey returns the display-name used for stable alphabetical
// sorting: alias if set (the friendly name shown in the rail), else
// username. Case-insensitive so "ACME-PROD" sorts next to "acme-test"
// rather than capital letters bunching to the top.
func orgSortKey(o Org) string {
	if o.Alias != "" {
		return strings.ToLower(o.Alias)
	}
	return strings.ToLower(o.Username)
}
