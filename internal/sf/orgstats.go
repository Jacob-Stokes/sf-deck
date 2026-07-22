package sf

import "fmt"

// UserLicenseRow is one row of UserLicense. Read-only.
type UserLicenseRow struct {
	Name          string
	TotalLicenses int
	UsedLicenses  int
	Status        string
	MasterLabel   string
}

// Targets routes o on a license row to the Setup company-information
// page where assignments + remaining-seat counts live.
func (UserLicenseRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "company", Label: "Company Information",
			Path: "/lightning/setup/CompanyProfileInfo/home"},
	}
}

// PermSetLicenseRow is one row of PermissionSetLicense. Read-only.
type PermSetLicenseRow struct {
	MasterLabel   string
	DeveloperName string
	TotalLicenses int
	UsedLicenses  int
	Status        string
}

// Targets routes o on a perm-set-license row to the Setup page that
// lists permission set licenses. There's no per-license drill in
// Lightning — admins manage from the company info / PSL list.
func (PermSetLicenseRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "permsetlic", Label: "Permission Set Licenses",
			Path: "/lightning/setup/PermissionSetLicenses/home"},
	}
}

// UserSummary is a trimmed view of a User record plus totals. The totals
// come from separate aggregate queries so the caller doesn't have to
// scan the full set.
type UserSummary struct {
	TotalActive   int
	TotalInactive int
	RecentLogins  []UserRow
}

// UserRow is one row from the recent-logins user query.
type UserRow struct {
	ID            string
	Name          string
	Username      string
	ProfileName   string
	UserRoleName  string
	LastLoginDate string
	IsActive      bool
	// ExtraTargets are appended to Targets() after the standard
	// User-row destinations. Populated by the UI layer with
	// context-specific actions ("Log in as user") that need org
	// state the bare UserRow doesn't carry.
	ExtraTargets []OpenTarget
}

// Field implements query.Row for filter predicates. Both the bare
// "Profile" alias and the SF-shape "Profile.Name" resolve to the
// flattened ProfileName column we cache; same for UserRole.
func (u UserRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return u.ID, true
	case "Name":
		return u.Name, true
	case "Username":
		return u.Username, true
	case "Profile", "Profile.Name", "ProfileName":
		return u.ProfileName, true
	case "Role", "UserRole.Name", "UserRoleName":
		return u.UserRoleName, true
	case "LastLogin", "LastLoginDate":
		return u.LastLoginDate, true
	case "IsActive":
		return u.IsActive, true
	}
	return nil, false
}

// Targets opens the User record's Lightning detail page when an Id
// is present; falls back to the user-management list otherwise.
// ExtraTargets are appended after the standard set — the UI uses
// this slot to inject the "Log in as user" action (which needs the
// org's Org Id, not carried on the bare UserRow).
func (u UserRow) Targets() []OpenTarget {
	if u.ID == "" {
		return append([]OpenTarget{
			{ID: "users", Label: "Users (Setup)",
				Path: "/lightning/setup/ManageUsers/home"},
		}, u.ExtraTargets...)
	}
	return append([]OpenTarget{
		{ID: "view", Label: "User detail",
			Path: "/lightning/r/User/" + u.ID + "/view"},
		{ID: "users", Label: "Users (Setup)",
			Path: "/lightning/setup/ManageUsers/home"},
	}, u.ExtraTargets...)
}

// YankTargets exposes the values you actually copy for a user: the
// Username (the login / API identity), the display Name, and the Id.
func (u UserRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if u.Username != "" {
		ts = append(ts, YankTarget{ID: "username", Label: "Username", Value: u.Username, Shortcut: "u"})
	}
	if u.Name != "" {
		ts = append(ts, YankTarget{ID: "name", Label: "Name", Value: u.Name, Shortcut: "n"})
	}
	if u.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Id", Value: u.ID, Shortcut: "i"})
	}
	return ts
}

// AsyncJobRow is one row of AsyncApexJob (future/batch/queueable/etc.).
type AsyncJobRow struct {
	ID             string
	Status         string
	JobType        string
	ApexClassID    string // for drilling into the class body
	ApexClassName  string
	MethodName     string
	CreatedDate    string
	CompletedDate  string
	ExtendedStatus string // failure detail / progress note (the "why")
	JobItemsTotal  int
	JobItemsDone   int
	NumberOfErrors int
}

// Field implements query.Row for filter predicates on the Home Jobs
// subtab.
func (j AsyncJobRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return j.ID, true
	case "Status":
		return j.Status, true
	case "Type", "JobType":
		return j.JobType, true
	case "Name", "ApexClass":
		return j.ApexClassName, true
	case "Method", "MethodName":
		return j.MethodName, true
	case "Created", "CreatedDate":
		return j.CreatedDate, true
	case "Errors":
		return j.NumberOfErrors, true
	}
	return nil, false
}

// Targets routes o on a job row to the Apex Jobs Setup page —
// per-job drill-in lives there. AsyncApexJob doesn't have a
// dedicated record page in Lightning.
func (AsyncJobRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "jobs", Label: "Apex Jobs (Setup)",
			Path: "/lightning/setup/AsyncApexJobs/home"},
	}
}

// YankTargets surfaces the copyable facts about an async job — the Id
// (for a SOQL lookup), the Apex class, the status, and (crucially) the
// failure detail, which is the thing you actually want to grab when a
// batch errored.
func (j AsyncJobRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if j.ApexClassName != "" {
		ts = append(ts, YankTarget{ID: "class", Label: "Apex class", Value: j.ApexClassName, Shortcut: "c"})
	}
	if j.ExtendedStatus != "" {
		ts = append(ts, YankTarget{ID: "error", Label: "Status detail / error", Value: j.ExtendedStatus, Shortcut: "e"})
	}
	if j.Status != "" {
		ts = append(ts, YankTarget{ID: "status", Label: "Status", Value: j.Status, Shortcut: "s"})
	}
	if j.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Job Id", Value: j.ID, Shortcut: "i"})
	}
	return ts
}

// AsyncJobs returns recent AsyncApexJob rows. Read-only SOQL.
func AsyncJobs(target string, limit int) ([]AsyncJobRow, error) {
	if limit <= 0 {
		limit = 10
	}
	soql := fmt.Sprintf(
		"SELECT Id, Status, JobType, ApexClassId, ApexClass.Name, MethodName, "+
			"CreatedDate, CompletedDate, ExtendedStatus, TotalJobItems, JobItemsProcessed, NumberOfErrors "+
			"FROM AsyncApexJob ORDER BY CreatedDate DESC LIMIT %d", limit)
	q, err := Query(target, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]AsyncJobRow, 0, len(q.Records))
	for _, r := range q.Records {
		row := AsyncJobRow{
			ID:             asString(r["Id"]),
			Status:         asString(r["Status"]),
			JobType:        asString(r["JobType"]),
			ApexClassID:    asString(r["ApexClassId"]),
			MethodName:     asString(r["MethodName"]),
			CreatedDate:    asString(r["CreatedDate"]),
			CompletedDate:  asString(r["CompletedDate"]),
			ExtendedStatus: asString(r["ExtendedStatus"]),
			JobItemsTotal:  asInt(r["TotalJobItems"]),
			JobItemsDone:   asInt(r["JobItemsProcessed"]),
			NumberOfErrors: asInt(r["NumberOfErrors"]),
		}
		if a, ok := r["ApexClass"].(map[string]any); ok {
			row.ApexClassName = asString(a["Name"])
		}
		out = append(out, row)
	}
	return out, nil
}

// CronTriggerRow is one scheduled job (CronTrigger) — the schedule
// itself, distinct from AsyncApexJob (the executions). Answers "what's
// scheduled to fire next".
type CronTriggerRow struct {
	ID             string
	Name           string // CronJobDetail.Name
	Type           string // CronJobDetail.JobType (scheduled apex / dashboard refresh / etc.)
	State          string // WAITING / ACQUIRED / EXECUTING / DELETED / COMPLETE / ERROR / PAUSED
	NextFireTime   string
	PreviousFire   string
	StartTime      string
	EndTime        string
	CronExpression string
	TimesTriggered int
}

// Field implements query.Row for filter predicates on the Scheduled chip.
func (c CronTriggerRow) Field(name string) (any, bool) {
	switch name {
	case "Id":
		return c.ID, true
	case "Name":
		return c.Name, true
	case "Type", "JobType":
		return c.Type, true
	case "State", "Status":
		return c.State, true
	case "NextFireTime", "Next":
		return c.NextFireTime, true
	case "CronExpression", "Expression":
		return c.CronExpression, true
	}
	return nil, false
}

// Targets routes o on a scheduled-job row to the Scheduled Jobs Setup
// page — CronTrigger has no dedicated Lightning record page.
func (CronTriggerRow) Targets() []OpenTarget {
	return []OpenTarget{
		{ID: "sched", Label: "Scheduled Jobs (Setup)",
			Path: "/lightning/setup/ScheduledJobs/home"},
	}
}

// YankTargets surfaces the copyable facts about a scheduled job — the
// name, the cron expression (handy to reuse when scheduling), the Id,
// state, and next fire time.
func (c CronTriggerRow) YankTargets() []YankTarget {
	var ts []YankTarget
	if c.Name != "" {
		ts = append(ts, YankTarget{ID: "name", Label: "Name", Value: c.Name, Shortcut: "n"})
	}
	if c.CronExpression != "" {
		ts = append(ts, YankTarget{ID: "cron", Label: "Cron expression", Value: c.CronExpression, Shortcut: "c"})
	}
	if c.State != "" {
		ts = append(ts, YankTarget{ID: "state", Label: "State", Value: c.State, Shortcut: "s"})
	}
	if c.NextFireTime != "" {
		ts = append(ts, YankTarget{ID: "next", Label: "Next fire", Value: c.NextFireTime, Shortcut: "x"})
	}
	if c.ID != "" {
		ts = append(ts, YankTarget{ID: "id", Label: "Job Id", Value: c.ID, Shortcut: "i"})
	}
	return ts
}

// ScheduledJobApexClass resolves the Apex class Id behind a scheduled
// job (CronTrigger), for scheduled-Apex jobs. CronTrigger doesn't hold
// the class directly — the link is via an AsyncApexJob of JobType
// ScheduledApex whose CronTriggerId matches. Returns "" when the
// scheduled job isn't Apex-backed (dashboard refresh, report notify,
// etc.) or nothing links to it.
func ScheduledJobApexClass(target, cronTriggerID string) (classID, className string, err error) {
	if cronTriggerID == "" {
		return "", "", nil
	}
	soql := "SELECT ApexClassId, ApexClass.Name FROM AsyncApexJob " +
		"WHERE CronTriggerId = '" + sqlEscape(cronTriggerID) + "' AND ApexClassId != null " +
		"ORDER BY CreatedDate DESC LIMIT 1"
	q, err := Query(target, soql, false)
	if err != nil {
		return "", "", err
	}
	if len(q.Records) == 0 {
		return "", "", nil
	}
	r := q.Records[0]
	classID = asString(r["ApexClassId"])
	if a, ok := r["ApexClass"].(map[string]any); ok {
		className = asString(a["Name"])
	}
	return classID, className, nil
}

// ScheduledJobs returns CronTrigger rows (scheduled jobs). Read-only
// SOQL. Ordered by next fire time so the soonest-to-run lead.
func ScheduledJobs(target string, limit int) ([]CronTriggerRow, error) {
	if limit <= 0 {
		limit = 100
	}
	soql := fmt.Sprintf(
		"SELECT Id, CronJobDetail.Name, CronJobDetail.JobType, State, "+
			"NextFireTime, PreviousFireTime, StartTime, EndTime, "+
			"CronExpression, TimesTriggered "+
			"FROM CronTrigger ORDER BY NextFireTime ASC NULLS LAST LIMIT %d", limit)
	q, err := Query(target, soql, false)
	if err != nil {
		return nil, err
	}
	out := make([]CronTriggerRow, 0, len(q.Records))
	for _, r := range q.Records {
		row := CronTriggerRow{
			ID:             asString(r["Id"]),
			State:          asString(r["State"]),
			NextFireTime:   asString(r["NextFireTime"]),
			PreviousFire:   asString(r["PreviousFireTime"]),
			StartTime:      asString(r["StartTime"]),
			EndTime:        asString(r["EndTime"]),
			CronExpression: asString(r["CronExpression"]),
			TimesTriggered: asInt(r["TimesTriggered"]),
		}
		if d, ok := r["CronJobDetail"].(map[string]any); ok {
			row.Name = asString(d["Name"])
			row.Type = asString(d["JobType"])
		}
		out = append(out, row)
	}
	return out, nil
}
