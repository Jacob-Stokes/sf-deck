package ui

// Misc list surfaces — /apex-logs, /deploys, /packages, /recent
// (mode-toggled), /home subtabs, and /system subtabs.
//
// The simple ones (apex logs, deploys, packages, home limits/
// licenses) use ListViewTableSpec[T]. Recent stays bespoke
// because the active ListView is picked dynamically by
// d.HomeRecentMode (sf-deck local log vs. Salesforce
// RecentlyViewed) — the spec builder assumes one ListView per
// surface. The home/system "shell" surfaces (no BuildRenderModel)
// stay as listSurface literals because they're driven by the
// renderer's bespoke composer, not the generic render-model path.

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// titleStater is the slice of a Resource[T] a list-surface title needs:
// when it was fetched, whether it's mid-fetch, and its last error. Every
// *resource.Resource[T] satisfies it structurally regardless of T, so
// standardListTitle can format any surface's header without generics.
type titleStater interface {
	FetchedAt() time.Time
	Busy() bool
	Err() error
}

// standardListTitle formats the canonical list-surface header line —
// "LABEL · N · <age><state-suffix>" — shared by ~20 ListViewTableSpec
// Title closures that all built this exact string by hand. count is the
// row count (usually d.SomeList.Len()); res carries the fetch/busy/err
// state.
func standardListTitle(label string, count int, res titleStater) string {
	return label + " · " + fmt.Sprintf("%d", count) + " · " +
		humanAge(res.FetchedAt()) + stateSuffix(res.Busy(), res.Err())
}

// singleColumnRecolor builds a ListViewTableSpec Recolor closure that
// tints exactly one column by a per-row colour. colorOf maps a row to
// its foreground colour (usually a small status→colour switch). Every
// other column, and any out-of-range row, renders with the base style
// untouched. Replaces the hand-written "if colName != X return base;
// return base.Foreground(f(items[row].Field))" closures.
func singleColumnRecolor[T any](targetCol string, colorOf func(T) color.Color) func([]T, int, int, string, lipgloss.Style) lipgloss.Style {
	return func(items []T, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		if colName != targetCol || row < 0 || row >= len(items) {
			return base
		}
		return base.Foreground(colorOf(items[row]))
	}
}

var apexLogsTableSpec = ListViewTableSpec[sf.ApexLogRow]{
	Schema:   apexLogColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.ApexLogRow] { return &d.ApexLogList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.ApexLogsTableState },
	Title: func(m Model, d *orgData, items []sf.ApexLogRow) string {
		return standardListTitle("APEX LOGS", d.ApexLogList.Len(), &d.ApexLogs)
	},
	ResErr: func(d *orgData) error { return d.ApexLogs.Err() },
	Empty:  "  no logs. run some apex or trace a user.",
}

var apexLogsListSurface = listSurfaceFromSpec(apexLogsTableSpec)

var setupAuditTableSpec = ListViewTableSpec[sf.SetupAuditRow]{
	Schema:   setupAuditColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.SetupAuditRow] { return &d.SetupAuditList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.SetupAuditTableState },
	Title: func(m Model, d *orgData, items []sf.SetupAuditRow) string {
		return standardListTitle("SETUP AUDIT TRAIL", d.SetupAuditList.Len(), &d.SetupAudit)
	},
	ResErr: func(d *orgData) error { return d.SetupAudit.Err() },
	Empty:  "  no recent Setup changes (or the log is empty).",
}

var setupAuditListSurface = listSurfaceFromSpec(setupAuditTableSpec)

var flowInterviewsTableSpec = ListViewTableSpec[sf.FlowInterviewRow]{
	Schema:   flowInterviewColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.FlowInterviewRow] { return &d.FlowInterviewList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.FlowInterviewTableState },
	Title: func(m Model, d *orgData, items []sf.FlowInterviewRow) string {
		return standardListTitle("FLOW INTERVIEWS", d.FlowInterviewList.Len(), &d.FlowInterviews)
	},
	ResErr: func(d *orgData) error { return d.FlowInterviews.Err() },
	Empty:  "  no paused or errored flow interviews — nothing stuck.",
}

var flowInterviewsListSurface = listSurfaceFromSpec(flowInterviewsTableSpec)

var asyncJobsTableSpec = ListViewTableSpec[sf.AsyncJobRow]{
	Schema:   asyncJobColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.AsyncJobRow] { return &d.AsyncJobList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.AsyncJobTableState },
	Title: func(m Model, d *orgData, items []sf.AsyncJobRow) string {
		return standardListTitle("ASYNC JOBS", d.AsyncJobList.Len(), &d.AsyncJobs)
	},
	ResErr:  func(d *orgData) error { return d.AsyncJobs.Err() },
	Recolor: singleColumnRecolor("Status", func(r sf.AsyncJobRow) color.Color { return asyncJobStatusColor(r.Status) }),
	Empty:   "  no async jobs.",
}

var asyncJobsListSurface = listSurfaceFromSpec(asyncJobsTableSpec)

var scheduledJobsTableSpec = ListViewTableSpec[sf.CronTriggerRow]{
	Schema:   scheduledJobColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.CronTriggerRow] { return &d.ScheduledJobList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.ScheduledJobTableState },
	Title: func(m Model, d *orgData, items []sf.CronTriggerRow) string {
		return standardListTitle("SCHEDULED JOBS", d.ScheduledJobList.Len(), &d.ScheduledJobs)
	},
	ResErr:  func(d *orgData) error { return d.ScheduledJobs.Err() },
	Recolor: singleColumnRecolor("State", func(r sf.CronTriggerRow) color.Color { return cronStateColor(r.State) }),
	Empty:   "  no scheduled jobs.",
}

var scheduledJobsListSurface = listSurfaceFromSpec(scheduledJobsTableSpec)

// asyncJobStatusColor tints the AsyncApexJob Status column: green when
// done, yellow while queued/running, red on failure/abort.
func asyncJobStatusColor(status string) color.Color {
	switch status {
	case "Completed":
		return theme.Green
	case "Queued", "Preparing", "Processing", "Holding":
		return theme.Yellow
	case "Failed", "Aborted":
		return theme.Red
	}
	return theme.Fg
}

// cronStateColor tints the CronTrigger State column: green when
// waiting/executing normally, red for error/deleted states.
func cronStateColor(state string) color.Color {
	switch state {
	case "WAITING", "ACQUIRED", "EXECUTING", "COMPLETE":
		return theme.Green
	case "ERROR", "DELETED":
		return theme.Red
	case "PAUSED":
		return theme.Yellow
	}
	return theme.Fg
}

var activeUsersTableSpec = ListViewTableSpec[sf.ActiveUserRow]{
	Schema:   activeUsersColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.ActiveUserRow] { return &d.ActiveUserList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.ActiveUserTableState },
	Title: func(m Model, d *orgData, items []sf.ActiveUserRow) string {
		return standardListTitle("ACTIVE USERS", d.ActiveUserList.Len(), &d.ActiveUsers)
	},
	ResErr: func(d *orgData) error { return d.ActiveUsers.Err() },
	Empty:  "  no active sessions — nobody's logged in right now.",
}

var activeUsersListSurface = listSurfaceFromSpec(activeUsersTableSpec)

var communitiesTableSpec = ListViewTableSpec[sf.CommunityRow]{
	Schema:   communityColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.CommunityRow] { return &d.CommunityList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.CommunityTableState },
	Title: func(m Model, d *orgData, items []sf.CommunityRow) string {
		return standardListTitle("COMMUNITIES", d.CommunityList.Len(), &d.Community)
	},
	ResErr: func(d *orgData) error { return d.Community.Err() },
	Empty:  "  no Experience sites in this org.",
}

var communitiesListSurface = listSurfaceFromSpec(communitiesTableSpec)

var communityPagesTableSpec = ListViewTableSpec[sf.CommunityPageRow]{
	Schema:   communityPageColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.CommunityPageRow] { return &d.CommunityPageList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.CommunityPageTableState },
	ResErr: func(d *orgData) error {
		if r := d.CommunityPages[communityPageKey(d.CommunityCur)]; r != nil {
			return r.Err()
		}
		return nil
	},
	Title: func(m Model, d *orgData, items []sf.CommunityPageRow) string {
		return "PAGES · " + fmt.Sprintf("%d", d.CommunityPageList.Len()) + " (org-wide, best-effort)"
	},
	Empty: "  no community pages found.",
}

var communityPagesListSurface = listSurfaceFromSpec(communityPagesTableSpec)

var userSessionsTableSpec = ListViewTableSpec[sf.SessionRow]{
	Schema:   userSessionColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.SessionRow] { return &d.UserSessionList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.UserSessionTableState },
	ResErr: func(d *orgData) error {
		if r := d.UserSessions[d.SessionUserID]; r != nil {
			return r.Err()
		}
		return nil
	},
	Title: func(m Model, d *orgData, items []sf.SessionRow) string {
		who := d.SessionUserName
		if who == "" {
			who = d.SessionUserID
		}
		res := d.UserSessions[d.SessionUserID]
		age := ""
		if res != nil {
			age = " · " + humanAge(res.FetchedAt()) + stateSuffix(res.Busy(), res.Err())
		}
		return "SESSIONS · " + who + " · " + fmt.Sprintf("%d", d.UserSessionList.Len()) + age
	},
	Empty: "  no live sessions for this user.",
}

var userSessionsListSurface = listSurfaceFromSpec(userSessionsTableSpec)

var deploysTableSpec = ListViewTableSpec[sf.DeployRow]{
	Schema:   deployColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.DeployRow] { return &d.DeployList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.DeploysTableState },
	Title: func(m Model, d *orgData, items []sf.DeployRow) string {
		return standardListTitle("DEPLOYS", d.DeployList.Len(), &d.Deploys)
	},
	ResErr: func(d *orgData) error { return d.Deploys.Err() },
	Recolor: func(items []sf.DeployRow, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		if colName != "Status" {
			return base
		}
		switch items[row].Status {
		case "Succeeded":
			return base.Foreground(theme.Green)
		case "Failed", "Canceled":
			return base.Foreground(theme.Red)
		case "SucceededPartial":
			return base.Foreground(theme.Yellow)
		case "Pending", "InProgress", "Canceling", "Finalizing":
			return base.Foreground(theme.Yellow)
		}
		return base
	},
	Empty: "  no deploys",
}

var deploysListSurface = listSurfaceFromSpec(deploysTableSpec)

var packagesTableSpec = ListViewTableSpec[sf.InstalledPackage]{
	Schema:   packageColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[sf.InstalledPackage] { return &d.PackageList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.PackagesTableState },
	Title: func(m Model, d *orgData, items []sf.InstalledPackage) string {
		return standardListTitle("INSTALLED PACKAGES", d.PackageList.Len(), &d.Packages)
	},
	ResErr: func(d *orgData) error { return d.Packages.Err() },
	Empty:  "  no packages installed",
}

var packagesListSurface = listSurfaceFromSpec(packagesTableSpec)

// recentListSurface — /recent + /home → Recent shared surface.
//
// One data source at a time, picked by d.HomeRecentMode:
//   - ChipModeLocal      → d.RecentList   (sf-deck local visit log)
//   - ChipModeSalesforce → d.RecentSFList (Salesforce RecentlyViewed)
//
// L (Keys.LensModeToggle) flips between them. Each mode has its own
// cursor / search state on its ListView, so toggling preserves state
// per source. Spec builder doesn't fit because the active list is
// chosen at render time — bespoke closure stays.
var recentListSurface = listSurface{
	State: func(d *orgData) *uilayout.ListTableState { return &d.RecentTableState },
	Cols:  recentCols,
	SearchPtr: func(d *orgData) *searchState {
		if lv := activeRecentListPtr(d); lv != nil {
			return lv.SearchPtr()
		}
		return nil
	},
	MoveCursor: func(d *orgData, n int) {
		if lv := activeRecentListPtr(d); lv != nil {
			lv.MoveBy(n)
		}
	},
	ResetCursor: func(d *orgData) {
		if lv := activeRecentListPtr(d); lv != nil {
			lv.ResetCursor()
		}
	},
	BuildRenderModel: func(m Model, d *orgData) (listRenderModel, bool) {
		if d == nil {
			return listRenderModel{}, false
		}
		// Lazy-sync the SF list before render so toggling into
		// Salesforce mode doesn't render an empty list while the
		// converter catches up.
		if d.HomeRecentMode == ChipModeSalesforce {
			m.syncRecentSFList(d.username)
		}
		lv := activeRecentListPtr(d)
		if lv == nil {
			return listRenderModel{}, false
		}
		resolved := mustResolveColumns(recentColumnSchema())
		cols := resolved.ListColumns()
		installListViewOrderRows(lv, &d.RecentTableState, cols,
			func(items []RecentEntry, row, col int) string {
				return resolvedSortCellForListColumn(resolved, items, cols, row, col)
			})
		items := lv.Filtered()

		modeLabel := "sf-deck"
		if d.HomeRecentMode == ChipModeSalesforce {
			modeLabel = "Salesforce"
		}
		title := fmt.Sprintf("RECENTLY VIEWED · %s · %d", modeLabel, lv.Len())
		if d.HomeRecentMode == ChipModeSalesforce {
			if d.RecentlyViewed.Busy() && d.RecentlyViewed.FetchedAt().IsZero() {
				title += " · loading from Salesforce…"
			} else if err := d.RecentlyViewed.Err(); err != nil {
				title += " · sf fetch failed"
			}
		}

		left, right := m.kindRefGutters(devproject.KindRecord, len(items),
			func(row int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				r := items[row]
				if r.Type == "" || r.ID == "" {
					return ""
				}
				return r.Type + ":" + r.ID
			})
		return listRenderModel{
			Title:  title,
			State:  &d.RecentTableState,
			Search: lv.SearchPtr(),
			Cols:   cols,
			N:      len(items),
			Cursor: lv.Cursor(),
			Cell: func(row, col int) string {
				if row < 0 || row >= len(items) {
					return ""
				}
				return resolvedCellForListColumn(resolved, items, cols, row, col)
			},
			Gutters:      left,
			RightGutters: right,
			Empty:        emptyRecentMessage(d.HomeRecentMode),
			FooterExtras: firstPretty(Keys.LensModeToggle) + " toggle source",
			DataVersion:  listVersionWithStore(lv.Version(), m),
		}, true
	},
}

// emptyRecentMessage returns the empty-state copy for /home Recent
// scoped to the active mode — the action the user should take
// differs by source.
func emptyRecentMessage(mode ChipMode) string {
	if mode == ChipModeSalesforce {
		return "  no recent activity from Salesforce — open something in Lightning to populate this list"
	}
	return "  nothing visited yet in sf-deck — drill into anything to start tracking"
}

// ---- /home subtabs --------------------------------------------------

// homeRecentListSurface — the /home → Recent subtab's listSurface.
// Same shape as recentListSurface; falls back to the shared
// BuildRenderModel when the registry needs one.
var homeRecentListSurface = listSurface{
	State: func(d *orgData) *uilayout.ListTableState { return &d.RecentTableState },
	Cols:  recentCols,
	SearchPtr: func(d *orgData) *searchState {
		if lv := activeRecentListPtr(d); lv != nil {
			return lv.SearchPtr()
		}
		return nil
	},
	MoveCursor: func(d *orgData, n int) {
		if lv := activeRecentListPtr(d); lv != nil {
			lv.MoveBy(n)
		}
	},
	ResetCursor: func(d *orgData) {
		if lv := activeRecentListPtr(d); lv != nil {
			lv.ResetCursor()
		}
	},
}

var homeNotificationsListSurface = listSurface{
	State:       func(d *orgData) *uilayout.ListTableState { return &d.HomeNotifTableState },
	Cols:        homeNotifCols,
	SearchPtr:   func(d *orgData) *searchState { return d.HomeNotifList.SearchPtr() },
	MoveCursor:  func(d *orgData, n int) { d.HomeNotifList.MoveBy(n) },
	ResetCursor: func(d *orgData) { d.HomeNotifList.ResetCursor() },
}

var homeLimitsTableSpec = ListViewTableSpec[KeyLimit]{
	Schema:   homeLimitColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[KeyLimit] { return &d.HomeLimitList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.HomeLimitTableState },
	ResErr:   func(d *orgData) error { return d.Home.Err() },
	ColStyles: map[string]lipgloss.Style{
		"Group": lipgloss.NewStyle().Foreground(theme.Cyan),
		"Limit": lipgloss.NewStyle().Foreground(theme.Fg),
		"Used":  lipgloss.NewStyle().Foreground(theme.Muted),
		"Max":   lipgloss.NewStyle().Foreground(theme.Muted),
		"Pct":   lipgloss.NewStyle().Foreground(theme.Muted),
		"Usage": lipgloss.NewStyle().Foreground(theme.FgDim),
	},
	Title: func(m Model, d *orgData, items []KeyLimit) string {
		return "LIMITS · " + humanAge(d.Home.FetchedAt()) +
			stateSuffix(d.Home.Busy(), d.Home.Err())
	},
	Recolor: func(items []KeyLimit, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		if colName != "Pct" || items[row].Max == 0 {
			return base
		}
		l := items[row]
		pct := (l.Max - l.Remaining) * 100 / l.Max
		switch {
		case pct >= 90:
			return base.Foreground(theme.Red)
		case pct >= 70:
			return base.Foreground(theme.Yellow)
		case pct >= 40:
			return base.Foreground(theme.Cyan)
		}
		return base
	},
	Empty: "  no limits returned by the org",
}

var homeLimitsListSurface = listSurfaceFromSpec(homeLimitsTableSpec)

var homeLicensesTableSpec = ListViewTableSpec[homeLicenseRow]{
	Schema:   homeLicenseColumnSchema(),
	ListPtr:  func(d *orgData) *ListView[homeLicenseRow] { return &d.HomeLicenseList },
	StatePtr: func(d *orgData) *uilayout.ListTableState { return &d.HomeLicenseTableState },
	ResErr:   func(d *orgData) error { return d.Home.Err() },
	ColStyles: map[string]lipgloss.Style{
		"License": lipgloss.NewStyle().Foreground(theme.Fg),
		"Kind":    lipgloss.NewStyle().Foreground(theme.Cyan),
		"Used":    lipgloss.NewStyle().Foreground(theme.Muted),
		"Total":   lipgloss.NewStyle().Foreground(theme.Muted),
		"Pct":     lipgloss.NewStyle().Foreground(theme.Muted),
		"Status":  lipgloss.NewStyle().Foreground(theme.FgDim),
		"Usage":   lipgloss.NewStyle().Foreground(theme.FgDim),
	},
	Title: func(m Model, d *orgData, items []homeLicenseRow) string {
		return "LICENSES · " + humanAge(d.Home.FetchedAt()) +
			stateSuffix(d.Home.Busy(), d.Home.Err())
	},
	Recolor: func(items []homeLicenseRow, row, col int, colName string, base lipgloss.Style) lipgloss.Style {
		l := items[row]
		switch colName {
		case "Pct":
			if l.Total == 0 {
				return base
			}
			pct := l.Used * 100 / l.Total
			switch {
			case pct >= 90:
				return base.Foreground(theme.Red)
			case pct >= 70:
				return base.Foreground(theme.Yellow)
			}
		case "Status":
			if l.Status != "" && l.Status != "Active" {
				return base.Foreground(theme.Yellow)
			}
		}
		return base
	},
	EmptyFn: func(m Model, d *orgData) string {
		if e := d.Home.Value().LicensesErr; e != "" {
			return "  licenses query failed: " + e
		}
		return "  no licenses"
	},
}

var homeLicensesListSurface = listSurfaceFromSpec(homeLicensesTableSpec)

// ---- /system subtabs (Logs / Deploys; API has no list-table) -------
