package sf

import (
	"encoding/json"
)

// OrgOpen launches the user's default browser to a Setup path in the
// named org, via `sf org open`. Read-only (it just opens a URL with
// the current session).
func OrgOpen(target, setupPath string) ([]byte, error) {
	return runSF("org", "open", "-p", setupPath, "-o", target)
}

// OrgDisplay is the shape of `sf org display --json`.
type OrgDisplay struct {
	ID              string `json:"id"`
	APIVersion      string `json:"apiVersion"`
	InstanceURL     string `json:"instanceUrl"`
	Username        string `json:"username"`
	Alias           string `json:"alias"`
	ConnectedStatus string `json:"connectedStatus"`
	ClientID        string `json:"clientId"`
}

type orgDisplayResult struct {
	Result OrgDisplay `json:"result"`
}

// DisplayOrg shells out to `sf org display -o <target> --json`. Read-only.
func DisplayOrg(target string) (OrgDisplay, error) {
	out, err := runSF("org", "display", "-o", target, "--json")
	if err != nil {
		return OrgDisplay{}, err
	}
	var parsed orgDisplayResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return OrgDisplay{}, err
	}
	return parsed.Result, nil
}

// Limit is one row from `sf limits api display`.
type Limit struct {
	Name      string `json:"name"`
	Max       int    `json:"max"`
	Remaining int    `json:"remaining"`
}

// Targets routes o on a limit row to the Setup page where most of
// these counters can be inspected — system overview shows a subset;
// platform usage history shows the rest.
func (Limit) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "system", Label: "System Overview (Setup)",
			Path: "/lightning/setup/SystemOverview/home"},
		{ID: "usage", Label: "Platform Usage History",
			Path: "/lightning/setup/PlatformEventUsage/home"},
	}
}

type limitsResult struct {
	Result []Limit `json:"result"`
}

// Limits returns API limits for the org. Read-only.
//
// Fast path: REST-direct. Falls back to CLI.
func Limits(target string) ([]Limit, error) {
	if c, err := RESTClient(target); err == nil {
		return c.LimitsREST()
	}
	out, err := runSF("limits", "api", "display", "-o", target, "--json")
	if err != nil {
		return nil, err
	}
	var parsed limitsResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	return parsed.Result, nil
}
