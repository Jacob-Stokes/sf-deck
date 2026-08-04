package ui

import (
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// The bulk of async updates flow through resourceUpdatedMsg in
// resource.go. Only two view-specific messages remain:

// soqlResultMsg delivers a SOQL query result. SOQL isn't a Resource —
// results are ephemeral and not cached.
//
// orgUser + tookMs travel with the result so the update-loop can
// log a soql_history row when the run lands. They're set by
// runSOQLCmd before dispatch.
type soqlResultMsg struct {
	session   soqlSessionTarget
	sessionID uint64
	soql      string
	data      sf.QueryResult
	err       error
	orgUser   string
	tookMs    int
	// gen is the soqlRunGen counter that was active when this query
	// was launched.  The reducer drops the message if m.soqlRunGen
	// has advanced — meaning the user cancelled or started a new
	// query while this one was in flight.  Without the gen check,
	// a cancelled-but-still-completing query could overwrite the
	// modal's idle state with stale results.
	gen uint64
}

type soqlSessionTarget string

const (
	soqlSessionTab   soqlSessionTarget = "tab"
	soqlSessionModal soqlSessionTarget = "modal"
)

// autocompleteValuesMsg carries the result of a Ctrl+Space live
// distinct-value fetch back to the SOQL autocomplete popup. Routes
// to whichever session (tab or modal) issued the fetch; stale
// messages (cancelled/superseded) are dropped via the gen check.
type autocompleteValuesMsg struct {
	session   soqlSessionTarget
	sessionID uint64
	gen       uint64
	field     string   // the field that was queried
	values    []string // distinct values returned (already de-quoted)
	err       error
}

// execResultMsg delivers an anonymous-Apex execution result. Same
// shape pattern as soqlResultMsg — orgUser travels with the message
// so the update-loop can log an apex_history row when the run lands.
type execResultMsg struct {
	body    string
	data    sf.ExecuteAnonymousResult
	err     error
	orgUser string
}

// HomeData is the aggregate cached payload for the Home view. Fetched by
// fetchHome and stored in Resource[HomeData].
type HomeData struct {
	APIVersion string
	Username   string
	UserID     string // Salesforce user Id (18-char), used by lens substitutions
	// UserName is the display name (FirstName + LastName, server-
	// rendered) — used by client-side chip predicates that filter on
	// rows whose only "by" identifier is a display string (Flow
	// CreatedBy/LastModifiedBy, etc.). Fetched alongside UserID in
	// one SOQL by fetchHome.
	UserName      string
	InstanceURL   string
	KeyLimits     []KeyLimit
	RecentDeploys []sf.DeployRow
	Packages      []sf.InstalledPackage

	// Home-subtab data. All optional — missing fields render as
	// "loading…" on their subtab.
	Users        sf.UserSummary
	UserLicenses []sf.UserLicenseRow
	PermSetLics  []sf.PermSetLicenseRow
	AsyncJobs    []sf.AsyncJobRow
	UsersErr     string
	LicensesErr  string
	JobsErr      string
}

// KeyLimit is a curated limit row shown on Home.
type KeyLimit struct {
	Name      string
	Max       int
	Remaining int
}

// Targets makes KeyLimit an sf.Openable so o on a limit row jumps to
// the Setup System Overview / Platform Usage history pages where the
// counters live in detail.
func (KeyLimit) Targets() []sf.OpenTarget {
	return []sf.OpenTarget{
		{ID: "system", Label: "System Overview (Setup)",
			Path: "/lightning/setup/SystemOverview/home"},
		{ID: "usage", Label: "Platform Usage History",
			Path: "/lightning/setup/PlatformEventUsage/home"},
		{ID: "company", Label: "Company Information",
			Path: "/lightning/setup/CompanyProfileInfo/home"},
	}
}

// Group classifies a limit into one of the natural buckets we render
// as section headers on the /home Limits subtab. Heuristic on the
// limit name — Salesforce's /limits payload uses a stable naming
// convention that maps cleanly to these categories. Unknown names
// land in "Other" so additions to the SF API don't make rows
// disappear.
func (k KeyLimit) Group() string {
	switch k.Name {
	case "DailyApiRequests", "HourlyODataCallout":
		return "API"
	case "DataStorageMB", "FileStorageMB":
		return "Storage"
	case "DailyBulkApiBatches", "DailyBulkV2QueryJobs", "DailyBulkV2QueryFileStorageMB":
		return "Bulk"
	case "DailyAsyncApexExecutions", "DailyAsyncApexTests", "ConcurrentAsyncGetReportInstances":
		return "Async"
	case "DailyStreamingApiEvents", "MonthlyPlatformEvents", "DailyDeliveredPlatformEvents", "StreamingApiConcurrentClients":
		return "Streaming"
	case "MassEmail", "SingleEmail":
		return "Email"
	case "DailyDurableGenericStreamingApiEvents", "DailyDurableStreamingApiEvents":
		return "Events"
	case "DailyGenericStreamingApiEvents":
		return "Streaming"
	case "DailyWorkflowEmails":
		return "Email"
	case "HourlyAsyncReportRuns", "HourlySyncReportRuns", "HourlyDashboardRefreshes", "HourlyDashboardResults", "HourlyDashboardStatuses", "HourlyTimeBasedWorkflow":
		return "Reports & Workflow"
	case "PermissionSets":
		return "Identity"
	}
	// Best-effort fallback on substrings before giving up.
	switch {
	case strings.HasPrefix(k.Name, "DailyApi"), strings.HasPrefix(k.Name, "HourlyOData"):
		return "API"
	case strings.Contains(k.Name, "Storage"):
		return "Storage"
	case strings.Contains(k.Name, "Bulk"):
		return "Bulk"
	case strings.Contains(k.Name, "Async"):
		return "Async"
	case strings.Contains(k.Name, "Streaming"), strings.Contains(k.Name, "PlatformEvent"):
		return "Streaming"
	case strings.Contains(k.Name, "Email"):
		return "Email"
	case strings.Contains(k.Name, "Report"), strings.Contains(k.Name, "Dashboard"):
		return "Reports & Workflow"
	}
	return "Other"
}

// sortLimitsByGroup orders limits by (group bucket, name) so the
// default rendering on /home → Limits clusters rows under section-
// like headers without needing a separate group-row injection.
// Bucket order is intentional: API first (the one people watch
// daily), then Storage, then everything else.
func sortLimitsByGroup(lims []KeyLimit) {
	rank := func(g string) int {
		switch g {
		case "API":
			return 0
		case "Storage":
			return 1
		case "Bulk":
			return 2
		case "Async":
			return 3
		case "Streaming":
			return 4
		case "Events":
			return 5
		case "Email":
			return 6
		case "Reports & Workflow":
			return 7
		case "Identity":
			return 8
		}
		return 99
	}
	sort.SliceStable(lims, func(i, j int) bool {
		gi, gj := rank(lims[i].Group()), rank(lims[j].Group())
		if gi != gj {
			return gi < gj
		}
		return lims[i].Name < lims[j].Name
	})
}

// Field implements query.Row so chip/filter predicates work on
// KeyLimit. Used by the Home Limits list-table search.
func (k KeyLimit) Field(name string) (any, bool) {
	switch name {
	case "Name":
		return k.Name, true
	case "Max":
		return k.Max, true
	case "Remaining":
		return k.Remaining, true
	case "Used":
		return k.Max - k.Remaining, true
	case "Group":
		return k.Group(), true
	}
	return nil, false
}

// homeLicenseRow merges UserLicense + PermSetLicense into a single
// row type so the Home Licenses subtab can present one unified table
// with a Kind discriminator. Loses some PSL-specific fields
// (DeveloperName) — they're surfaced in the existing renderer
// already.
type homeLicenseRow struct {
	Name   string
	Kind   string // "User" or "PermSet"
	Used   int
	Total  int
	Status string
}

// Field for filter / sort.
func (l homeLicenseRow) Field(name string) (any, bool) {
	switch name {
	case "Name":
		return l.Name, true
	case "Kind":
		return l.Kind, true
	case "Used":
		return l.Used, true
	case "Total":
		return l.Total, true
	case "Status":
		return l.Status, true
	case "Pct":
		if l.Total == 0 {
			return 0, true
		}
		return l.Used * 100 / l.Total, true
	}
	return nil, false
}

// Targets routes o on a license row to the right Setup page based on
// kind — UserLicense → company info, PermSetLicense → PSL list.
func (l homeLicenseRow) Targets() []sf.OpenTarget {
	if l.Kind == "PermSet" {
		return []sf.OpenTarget{{
			ID: "permsetlic", Label: "Permission Set Licenses",
			Path: "/lightning/setup/PermissionSetLicenses/home"}}
	}
	return []sf.OpenTarget{{
		ID: "company", Label: "Company Information",
		Path: "/lightning/setup/CompanyProfileInfo/home"}}
}
