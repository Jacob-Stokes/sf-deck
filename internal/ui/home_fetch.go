package ui

import "github.com/Jacob-Stokes/sf-deck/internal/sf"

func fetchHome(alias, username string, loginLimit, asyncLimit, deployLimit int) (HomeData, error) {
	data := HomeData{Username: username}

	if disp, err := sf.DisplayOrg(alias); err == nil {
		data.APIVersion = disp.APIVersion
		data.InstanceURL = disp.InstanceURL
	}
	if username != "" {
		if id, err := sf.CurrentUserIdentity(alias, username); err == nil {
			data.UserID = id.ID
			data.UserName = id.Name
		}
	}
	if lims, err := sf.Limits(alias); err == nil {
		for _, l := range lims {
			data.KeyLimits = append(data.KeyLimits, KeyLimit{
				Name: l.Name, Max: l.Max, Remaining: l.Remaining,
			})
		}
	}

	if stats, err := sf.FetchHomeStats(alias, loginLimit, asyncLimit, deployLimit); err == nil {
		data.Users = stats.Users
		data.UserLicenses = stats.UserLicenses
		data.PermSetLics = stats.PermSetLics
		data.AsyncJobs = stats.AsyncJobs
		data.RecentDeploys = stats.RecentDeploys
	} else {
		msg := err.Error()
		data.UsersErr = msg
		data.LicensesErr = msg
		data.JobsErr = msg
	}
	return data, nil
}
