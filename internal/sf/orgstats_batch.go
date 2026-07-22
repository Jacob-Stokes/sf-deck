package sf

// Batched home-stats fetch.
//
// /home renders five independent data widgets (Users/Licenses/PermSet
// licenses/Jobs/Recent deploys) plus two counts (active/inactive
// users). Calling each as a separate SOQL is 8 round-trips per Home
// fetch — and Home fetches per-org. Bundling into one /composite call
// brings that down to 1.

import (
	"encoding/json"
	"fmt"
)

// HomeStats is the payload of the batched home fetch.
type HomeStats struct {
	Users         UserSummary
	UserLicenses  []UserLicenseRow
	PermSetLics   []PermSetLicenseRow
	AsyncJobs     []AsyncJobRow
	RecentDeploys []DeployRow
}

// FetchHomeStats issues all the home-page SOQL queries in one
// composite REST call. Per-subrequest errors don't fail the whole
// fetch — the caller still gets whatever subrequests succeeded
// (matches the old serial behaviour where each `if err == nil`
// guarded a separate field).
func FetchHomeStats(target string, recentLoginLimit, asyncJobLimit, deployLimit int) (HomeStats, error) {
	if recentLoginLimit <= 0 {
		recentLoginLimit = 10
	}
	if asyncJobLimit <= 0 {
		asyncJobLimit = 10
	}
	if deployLimit <= 0 {
		deployLimit = 5
	}
	c, err := RESTClient(target)
	if err != nil {
		return HomeStats{}, err
	}

	soqlActive := "SELECT COUNT() FROM User WHERE IsActive = true"
	soqlInactive := "SELECT COUNT() FROM User WHERE IsActive = false"
	soqlRecentLogins := fmt.Sprintf(
		"SELECT Id, Name, Username, Profile.Name, UserRole.Name, LastLoginDate, IsActive "+
			"FROM User WHERE IsActive = true AND LastLoginDate != null "+
			"ORDER BY LastLoginDate DESC NULLS LAST LIMIT %d", recentLoginLimit)
	soqlUserLics := "SELECT Name, TotalLicenses, UsedLicenses, Status, MasterLabel " +
		"FROM UserLicense ORDER BY Name"
	soqlPSLics := "SELECT MasterLabel, DeveloperName, TotalLicenses, UsedLicenses, Status " +
		"FROM PermissionSetLicense ORDER BY MasterLabel"
	soqlJobs := fmt.Sprintf(
		"SELECT Id, Status, JobType, ApexClass.Name, MethodName, "+
			"CreatedDate, CompletedDate, TotalJobItems, JobItemsProcessed, NumberOfErrors "+
			"FROM AsyncApexJob ORDER BY CreatedDate DESC LIMIT %d", asyncJobLimit)
	soqlDeploys := fmt.Sprintf(
		"SELECT Id, Status, CreatedBy.Name, CreatedDate, NumberComponentsDeployed, NumberComponentsTotal "+
			"FROM DeployRequest ORDER BY CreatedDate DESC LIMIT %d", deployLimit)

	requests := []CompositeRequest{
		{Method: "GET", URL: c.QueryURL(soqlActive, false), ReferenceID: "active"},
		{Method: "GET", URL: c.QueryURL(soqlInactive, false), ReferenceID: "inactive"},
		{Method: "GET", URL: c.QueryURL(soqlRecentLogins, false), ReferenceID: "logins"},
		{Method: "GET", URL: c.QueryURL(soqlUserLics, false), ReferenceID: "userlics"},
		{Method: "GET", URL: c.QueryURL(soqlPSLics, false), ReferenceID: "pslics"},
		// AsyncApexJob and DeployRequest are Tooling sobjects.
		{Method: "GET", URL: c.QueryURL(soqlJobs, true), ReferenceID: "jobs"},
		{Method: "GET", URL: c.QueryURL(soqlDeploys, true), ReferenceID: "deploys"},
	}
	responses, err := c.Composite(requests, false)
	if err != nil {
		return HomeStats{}, err
	}
	results, _ := CompositeQueryResults(responses)

	var stats HomeStats
	if q, ok := results["active"]; ok {
		stats.Users.TotalActive = q.TotalSize
	}
	if q, ok := results["inactive"]; ok {
		stats.Users.TotalInactive = q.TotalSize
	}
	if q, ok := results["logins"]; ok {
		for _, r := range q.Records {
			row := UserRow{
				ID:            asString(r["Id"]),
				Name:          asString(r["Name"]),
				Username:      asString(r["Username"]),
				LastLoginDate: asString(r["LastLoginDate"]),
			}
			if b, ok := r["IsActive"].(bool); ok {
				row.IsActive = b
			}
			if p, ok := r["Profile"].(map[string]any); ok {
				row.ProfileName = asString(p["Name"])
			}
			if u, ok := r["UserRole"].(map[string]any); ok {
				row.UserRoleName = asString(u["Name"])
			}
			stats.Users.RecentLogins = append(stats.Users.RecentLogins, row)
		}
	}
	if q, ok := results["userlics"]; ok {
		for _, r := range q.Records {
			stats.UserLicenses = append(stats.UserLicenses, UserLicenseRow{
				Name:          asString(r["Name"]),
				MasterLabel:   asString(r["MasterLabel"]),
				Status:        asString(r["Status"]),
				TotalLicenses: asInt(r["TotalLicenses"]),
				UsedLicenses:  asInt(r["UsedLicenses"]),
			})
		}
	}
	if q, ok := results["pslics"]; ok {
		for _, r := range q.Records {
			stats.PermSetLics = append(stats.PermSetLics, PermSetLicenseRow{
				MasterLabel:   asString(r["MasterLabel"]),
				DeveloperName: asString(r["DeveloperName"]),
				Status:        asString(r["Status"]),
				TotalLicenses: asInt(r["TotalLicenses"]),
				UsedLicenses:  asInt(r["UsedLicenses"]),
			})
		}
	}
	if q, ok := results["jobs"]; ok {
		for _, r := range q.Records {
			row := AsyncJobRow{
				ID:             asString(r["Id"]),
				Status:         asString(r["Status"]),
				JobType:        asString(r["JobType"]),
				MethodName:     asString(r["MethodName"]),
				CreatedDate:    asString(r["CreatedDate"]),
				CompletedDate:  asString(r["CompletedDate"]),
				JobItemsTotal:  asInt(r["TotalJobItems"]),
				JobItemsDone:   asInt(r["JobItemsProcessed"]),
				NumberOfErrors: asInt(r["NumberOfErrors"]),
			}
			if a, ok := r["ApexClass"].(map[string]any); ok {
				row.ApexClassName = asString(a["Name"])
			}
			stats.AsyncJobs = append(stats.AsyncJobs, row)
		}
	}
	if q, ok := results["deploys"]; ok {
		for _, r := range q.Records {
			row := DeployRow{
				ID:          asString(r["Id"]),
				Status:      asString(r["Status"]),
				CreatedDate: asString(r["CreatedDate"]),
			}
			if u, ok := r["CreatedBy"].(map[string]any); ok {
				row.CreatedByName = asString(u["Name"])
			}
			row.ComponentsDeployed = asInt(r["NumberComponentsDeployed"])
			row.ComponentsTotal = asInt(r["NumberComponentsTotal"])
			stats.RecentDeploys = append(stats.RecentDeploys, row)
		}
	}
	return stats, nil
}

// _ keeps json imported even when nothing in this file currently
// uses it directly — composite responses arrive as json.RawMessage
// and we may extend the helpers to peek at error bodies in future.
var _ = json.Marshal
