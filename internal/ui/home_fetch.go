package ui

import "github.com/Jacob-Stokes/sf-deck/internal/sf"

// fetchHome composes the Home view's data. Most of the SOQL is
// bundled into one composite REST call (FetchHomeStats); the
// non-SOQL bits — DisplayOrg / Limits (REST endpoints) and the
// installed-packages CLI shell-out — stay separate.
//
// loginLimit / asyncLimit / deployLimit are the row caps for the
// recent-logins, async-jobs, and recent-deploys widgets — threaded
// from settings by the caller.
func fetchHome(alias, username string, loginLimit, asyncLimit, deployLimit int) (HomeData, error) {
	data := HomeData{Username: username}

	if disp, err := sf.DisplayOrg(alias); err == nil {
		data.APIVersion = disp.APIVersion
		data.InstanceURL = disp.InstanceURL
		// disp.ID is the *org* id (00D…), not the user id (005…) the
		// CLI's JSON conflates the field name. Fetch the user id
		// separately via a 1-row SOQL so :userId substitutions in
		// lenses point at the actual current user.
	}
	// Resolve the User Id (005…) AND display name for the CLI session's
	// user. One cheap SOQL per home fetch — both are needed by chip
	// substitutions ($userId server-side, $userName client-side).
	if username != "" {
		if id, err := sf.CurrentUserIdentity(alias, username); err == nil {
			data.UserID = id.ID
			data.UserName = id.Name
		}
	}
	// All limits from /services/data/vNN/limits. The /home Limits
	// subtab renders the full set (grouped by KeyLimit.Group), and
	// the header-bar pill picks DailyApiRequests out of this same
	// slice — no need for a curated subset.
	if lims, err := sf.Limits(alias); err == nil {
		for _, l := range lims {
			data.KeyLimits = append(data.KeyLimits, KeyLimit{
				Name: l.Name, Max: l.Max, Remaining: l.Remaining,
			})
		}
	}
	// Packages are fetched separately by d.Packages (2h TTL, cached).
	// The home subtab reads from that resource directly — see
	// views_home.go renderHomePackages — so we don't fetch them
	// here. (Pre-refactor a second copy lived on HomeData; that
	// shelled out to `sf package installed list` every home fetch.)

	// One composite call covers the 7 home-stats queries (active /
	// inactive / recent-logins / user-licenses / permset-licenses /
	// async-jobs / recent-deploys).
	if stats, err := sf.FetchHomeStats(alias, loginLimit, asyncLimit, deployLimit); err == nil {
		data.Users = stats.Users
		data.UserLicenses = stats.UserLicenses
		data.PermSetLics = stats.PermSetLics
		data.AsyncJobs = stats.AsyncJobs
		data.RecentDeploys = stats.RecentDeploys
	} else {
		// On a hard composite failure, surface the error on every
		// subtab so the user knows it's the batch that broke (not
		// some bizarre per-widget issue).
		msg := err.Error()
		data.UsersErr = msg
		data.LicensesErr = msg
		data.JobsErr = msg
	}
	return data, nil
}
