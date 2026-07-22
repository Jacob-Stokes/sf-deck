package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/theme"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/tablemodel"
)

func permSetColumnSchema() tablemodel.Schema[sf.PermissionSet] {
	return tablemodel.Schema[sf.PermissionSet]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "License", "Type", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.PermissionSet]{
			"Name":       textColumnDef[sf.PermissionSet]("NAME", tablemodel.Width{Min: 18, Ideal: 28}, func(ps sf.PermissionSet) string { return ps.Name }),
			"Label":      textColumnDef[sf.PermissionSet]("LABEL", tablemodel.Width{Min: 16, Ideal: 30}, func(ps sf.PermissionSet) string { return dashIfEmpty(ps.Label) }),
			"License":    textColumnDef[sf.PermissionSet]("LICENSE", tablemodel.Width{Min: 12, Ideal: 22}, func(ps sf.PermissionSet) string { return dashIfEmpty(ps.LicenseName) }),
			"Type":       textColumnDef[sf.PermissionSet]("TYPE", tablemodel.Width{Min: 8, Ideal: 14}, func(ps sf.PermissionSet) string { return dashIfEmpty(ps.Type) }),
			"Modified":   modifiedDateColumnDef[sf.PermissionSet](func(ps sf.PermissionSet) string { return ps.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.PermissionSet](func(ps sf.PermissionSet) string { return ps.LastModifiedByName }),
			"Marks":      marksColumnDef[sf.PermissionSet](18, 26),
		},
	}
}

func psgColumnSchema() tablemodel.Schema[sf.PermissionSetGroup] {
	return tablemodel.Schema[sf.PermissionSetGroup]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Label", "Status", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.PermissionSetGroup]{
			"Name":       textColumnDef[sf.PermissionSetGroup]("NAME", tablemodel.Width{Min: 18, Ideal: 28}, func(g sf.PermissionSetGroup) string { return g.DeveloperName }),
			"Label":      textColumnDef[sf.PermissionSetGroup]("LABEL", tablemodel.Width{Min: 16, Ideal: 30}, func(g sf.PermissionSetGroup) string { return dashIfEmpty(g.MasterLabel) }),
			"Status":     textColumnDef[sf.PermissionSetGroup]("STATUS", tablemodel.Width{Min: 8, Ideal: 12}, func(g sf.PermissionSetGroup) string { return dashIfEmpty(g.Status) }),
			"Modified":   modifiedDateColumnDef[sf.PermissionSetGroup](func(g sf.PermissionSetGroup) string { return g.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.PermissionSetGroup](func(g sf.PermissionSetGroup) string { return g.LastModifiedByName }),
			"Marks":      marksColumnDef[sf.PermissionSetGroup](16, 22),
		},
	}
}

func profileColumnSchema() tablemodel.Schema[sf.Profile] {
	return tablemodel.Schema[sf.Profile]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "License", "Type", "Modified", "ModifiedBy", "Marks"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.Profile]{
			"Name":       textColumnDef[sf.Profile]("NAME", tablemodel.Width{Min: 18, Ideal: 32}, func(p sf.Profile) string { return p.Name }),
			"License":    textColumnDef[sf.Profile]("LICENSE", tablemodel.Width{Min: 14, Ideal: 26}, func(p sf.Profile) string { return dashIfEmpty(p.UserLicenseName) }),
			"Type":       textColumnDef[sf.Profile]("TYPE", tablemodel.Width{Min: 10, Ideal: 16}, func(p sf.Profile) string { return dashIfEmpty(p.UserType) }),
			"Modified":   modifiedDateColumnDef[sf.Profile](func(p sf.Profile) string { return p.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.Profile](func(p sf.Profile) string { return p.LastModifiedByName }),
			"Marks":      marksColumnDef[sf.Profile](14, 18),
		},
	}
}

func queueColumnSchema() tablemodel.Schema[sf.QueueRow] {
	return tablemodel.Schema[sf.QueueRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "DeveloperName", "Email", "SObjects", "Modified", "ModifiedBy"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.QueueRow]{
			"Name":          textColumnDef[sf.QueueRow]("NAME", tablemodel.Width{Min: 18, Ideal: 28}, func(q sf.QueueRow) string { return q.Name }),
			"DeveloperName": textColumnDef[sf.QueueRow]("DEV NAME", tablemodel.Width{Min: 16, Ideal: 24}, func(q sf.QueueRow) string { return dashIfEmpty(q.DeveloperName) }),
			"Email":         textColumnDef[sf.QueueRow]("EMAIL", tablemodel.Width{Min: 18, Ideal: 28}, func(q sf.QueueRow) string { return dashIfEmpty(q.Email) }),
			"SObjects":      textColumnDef[sf.QueueRow]("SOBJECTS", tablemodel.Width{Min: 16, Ideal: 32}, queueSObjectsCell),
			"Modified":      modifiedDateColumnDef[sf.QueueRow](func(q sf.QueueRow) string { return q.LastModifiedDate }),
			"ModifiedBy":    modifiedByColumnDef[sf.QueueRow](func(q sf.QueueRow) string { return q.LastModifiedByName }),
		},
	}
}

func publicGroupColumnSchema() tablemodel.Schema[sf.PublicGroupRow] {
	return tablemodel.Schema[sf.PublicGroupRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "DeveloperName", "Bosses", "Modified", "ModifiedBy"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.PublicGroupRow]{
			"Name":          textColumnDef[sf.PublicGroupRow]("NAME", tablemodel.Width{Min: 18, Ideal: 32}, func(g sf.PublicGroupRow) string { return g.Name }),
			"DeveloperName": textColumnDef[sf.PublicGroupRow]("DEV NAME", tablemodel.Width{Min: 16, Ideal: 24}, func(g sf.PublicGroupRow) string { return dashIfEmpty(g.DeveloperName) }),
			"Bosses": textColumnDef[sf.PublicGroupRow]("INCLUDES BOSSES", tablemodel.Width{Min: 8, Ideal: 8}, func(g sf.PublicGroupRow) string {
				if g.DoesIncludeBosses {
					return "yes"
				}
				return "no"
			}),
			"Modified":   modifiedDateColumnDef[sf.PublicGroupRow](func(g sf.PublicGroupRow) string { return g.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.PublicGroupRow](func(g sf.PublicGroupRow) string { return g.LastModifiedByName }),
		},
	}
}

func groupMemberColumnSchema() tablemodel.Schema[sf.GroupMemberRow] {
	return tablemodel.Schema[sf.GroupMemberRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Kind", "Name", "Email", "Id"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.GroupMemberRow]{
			"Kind": withColumnStyle(
				textColumnDef[sf.GroupMemberRow]("KIND", tablemodel.Width{Min: 6, Ideal: 8}, func(r sf.GroupMemberRow) string { return r.Kind }),
				lipgloss.NewStyle().Foreground(theme.Yellow),
			),
			"Name": withColumnStyle(
				textColumnDef[sf.GroupMemberRow]("NAME", tablemodel.Width{Min: 18, Ideal: 32}, func(r sf.GroupMemberRow) string {
					if r.Name == "" {
						return r.ID
					}
					return r.Name
				}),
				lipgloss.NewStyle().Foreground(theme.Fg),
			),
			"Email": withColumnStyle(
				textColumnDef[sf.GroupMemberRow]("EMAIL", tablemodel.Width{Min: 16, Ideal: 28}, func(r sf.GroupMemberRow) string { return dashIfEmpty(r.Email) }),
				lipgloss.NewStyle().Foreground(theme.FgDim),
			),
			"Id": withColumnStyle(
				textColumnDef[sf.GroupMemberRow]("ID", tablemodel.Width{Min: 18, Ideal: 20}, func(r sf.GroupMemberRow) string { return r.ID }),
				lipgloss.NewStyle().Foreground(theme.FgDim),
			),
		},
	}
}

func userColumnSchema() tablemodel.Schema[sf.UserRow] {
	return tablemodel.Schema[sf.UserRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Username", "Profile", "Role", "LastLogin"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.UserRow]{
			"Name": {
				Header: "NAME",
				Width:  tablemodel.Width{Min: 16, Ideal: 28},
				Style:  lipgloss.NewStyle().Foreground(theme.Fg),
				Render: func(u sf.UserRow) string {
					if u.Name != "" {
						return u.Name
					}
					return u.Username
				},
			},
			"Username": {
				Header: "USERNAME",
				Width:  tablemodel.Width{Min: 24, Ideal: 30},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(u sf.UserRow) string { return dashIfEmpty(u.Username) },
			},
			"Profile": {
				Header: "PROFILE",
				Width:  tablemodel.Width{Min: 18, Ideal: 22},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(u sf.UserRow) string { return dashIfEmpty(u.ProfileName) },
			},
			"Role": {
				Header: "ROLE",
				Width:  tablemodel.Width{Min: 14, Ideal: 18},
				Style:  lipgloss.NewStyle().Foreground(theme.Muted),
				Render: func(u sf.UserRow) string { return dashIfEmpty(u.UserRoleName) },
			},
			"LastLogin": {
				Header:  "LAST LOGIN",
				Width:   tablemodel.Width{Min: 14, Ideal: 16},
				Style:   lipgloss.NewStyle().Foreground(theme.FgDim),
				Render:  func(u sf.UserRow) string { return prettyDate(u.LastLoginDate) },
				SortKey: func(u sf.UserRow) string { return u.LastLoginDate },
			},
		},
	}
}

func apexLogColumnSchema() tablemodel.Schema[sf.ApexLogRow] {
	return tablemodel.Schema[sf.ApexLogRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "Status", "Operation", "Duration", "User"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.ApexLogRow]{
			"When": withSortKey(
				textColumnDef[sf.ApexLogRow]("WHEN", tablemodel.Width{Min: 14, Ideal: 16}, func(l sf.ApexLogRow) string { return prettyDate(l.StartTime) }),
				func(l sf.ApexLogRow) string { return l.StartTime }),
			"Status":    textColumnDef[sf.ApexLogRow]("STATUS", tablemodel.Width{Min: 8, Ideal: 10}, func(l sf.ApexLogRow) string { return l.Status }),
			"Operation": textColumnDef[sf.ApexLogRow]("OPERATION", tablemodel.Width{Min: 16, Ideal: 30}, func(l sf.ApexLogRow) string { return l.Operation }),
			"Duration":  textColumnDef[sf.ApexLogRow]("DUR", tablemodel.Width{Min: 8, Ideal: 10}, func(l sf.ApexLogRow) string { return fmt.Sprintf("%dms", l.DurationMs) }),
			"User":      textColumnDef[sf.ApexLogRow]("USER", tablemodel.Width{Min: 12, Ideal: 22}, func(l sf.ApexLogRow) string { return l.LogUser.Name }),
		},
	}
}

func userSessionColumnSchema() tablemodel.Schema[sf.SessionRow] {
	tstamp := func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04")
	}
	return tablemodel.Schema[sf.SessionRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Type", "LastActive", "Location", "IP", "MFA", "Browser"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.SessionRow]{
			"Type":       textColumnDef[sf.SessionRow]("TYPE", tablemodel.Width{Min: 8, Ideal: 14}, func(r sf.SessionRow) string { return dashIfEmpty(r.SessionType) }),
			"LastActive": textColumnDef[sf.SessionRow]("LAST ACTIVE", tablemodel.Width{Min: 14, Ideal: 16}, func(r sf.SessionRow) string { return tstamp(r.LastActive) }),
			"Started":    textColumnDef[sf.SessionRow]("STARTED", tablemodel.Width{Min: 14, Ideal: 16}, func(r sf.SessionRow) string { return tstamp(r.Started) }),
			"Location":   textColumnDef[sf.SessionRow]("LOCATION", tablemodel.Width{Min: 12, Ideal: 18}, func(r sf.SessionRow) string { return dashIfEmpty(r.Location()) }),
			"IP":         textColumnDef[sf.SessionRow]("IP", tablemodel.Width{Min: 12, Ideal: 16}, func(r sf.SessionRow) string { return dashIfEmpty(r.SourceIP) }),
			"MFA": textColumnDef[sf.SessionRow]("MFA", tablemodel.Width{Min: 5, Ideal: 8}, func(r sf.SessionRow) string {
				if r.SecurityLevel == "HIGH_ASSURANCE" {
					return "high"
				}
				return dashIfEmpty(strings.ToLower(r.SecurityLevel))
			}),
			"Browser":  textColumnDef[sf.SessionRow]("BROWSER", tablemodel.Width{Min: 10, Ideal: 16}, func(r sf.SessionRow) string { return dashIfEmpty(r.Browser) }),
			"Platform": textColumnDef[sf.SessionRow]("PLATFORM", tablemodel.Width{Min: 8, Ideal: 12}, func(r sf.SessionRow) string { return dashIfEmpty(r.Platform) }),
			"App":      textColumnDef[sf.SessionRow]("APP", tablemodel.Width{Min: 8, Ideal: 14}, func(r sf.SessionRow) string { return dashIfEmpty(r.Application) }),
		},
	}
}

func communityColumnSchema() tablemodel.Schema[sf.CommunityRow] {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "—"
	}
	return tablemodel.Schema[sf.CommunityRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "URL", "Status", "Members", "SelfReg"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.CommunityRow]{
			"Name":    textColumnDef[sf.CommunityRow]("NAME", tablemodel.Width{Min: 16, Ideal: 28}, func(r sf.CommunityRow) string { return r.Name }),
			"URL":     textColumnDef[sf.CommunityRow]("URL", tablemodel.Width{Min: 10, Ideal: 18}, func(r sf.CommunityRow) string { return dashIfEmpty(r.URLPathPrefix) }),
			"Status":  textColumnDef[sf.CommunityRow]("STATUS", tablemodel.Width{Min: 8, Ideal: 14}, func(r sf.CommunityRow) string { return dashIfEmpty(r.Status) }),
			"Members": textColumnDef[sf.CommunityRow]("MEMBERS", tablemodel.Width{Min: 7, Ideal: 9}, func(r sf.CommunityRow) string { return fmt.Sprintf("%d", r.Members) }),
			"SelfReg": textColumnDef[sf.CommunityRow]("SELF-REG", tablemodel.Width{Min: 8, Ideal: 9}, func(r sf.CommunityRow) string { return yn(r.SelfReg) }),
			"Guest":   textColumnDef[sf.CommunityRow]("GUEST FILES", tablemodel.Width{Min: 8, Ideal: 11}, func(r sf.CommunityRow) string { return yn(r.GuestFiles) }),
		},
	}
}

func communityPageColumnSchema() tablemodel.Schema[sf.CommunityPageRow] {
	return tablemodel.Schema[sf.CommunityPageRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Label", "Name", "Type"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.CommunityPageRow]{
			"Label": textColumnDef[sf.CommunityPageRow]("LABEL", tablemodel.Width{Min: 16, Ideal: 28}, func(r sf.CommunityPageRow) string { return dashIfEmpty(r.MasterLabel) }),
			"Name":  textColumnDef[sf.CommunityPageRow]("DEV NAME", tablemodel.Width{Min: 20, Ideal: 40}, func(r sf.CommunityPageRow) string { return r.DeveloperName }),
			"Type":  textColumnDef[sf.CommunityPageRow]("TYPE", tablemodel.Width{Min: 12, Ideal: 20}, func(r sf.CommunityPageRow) string { return r.Type }),
		},
	}
}

func activeUsersColumnSchema() tablemodel.Schema[sf.ActiveUserRow] {
	tstamp := func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04")
	}
	return tablemodel.Schema[sf.ActiveUserRow]{
		// Presence-forward defaults: who, when last active, doing what,
		// since when, how many sessions. Security columns (IP, MFA) are
		// available and shown in the sidebar; the No-MFA chip surfaces
		// the security angle without cluttering the default view.
		DefaultColumns: func(scope string) []string {
			return []string{"User", "LastActive", "Location", "Type", "Sessions"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.ActiveUserRow]{
			"User":       textColumnDef[sf.ActiveUserRow]("USER", tablemodel.Width{Min: 16, Ideal: 26}, func(r sf.ActiveUserRow) string { return r.UserName }),
			"LastActive": textColumnDef[sf.ActiveUserRow]("LAST ACTIVE", tablemodel.Width{Min: 14, Ideal: 16}, func(r sf.ActiveUserRow) string { return tstamp(r.LastActive) }),
			"Location":   textColumnDef[sf.ActiveUserRow]("LOCATION", tablemodel.Width{Min: 12, Ideal: 18}, func(r sf.ActiveUserRow) string { return dashIfEmpty(r.Location) }),
			"Type":       textColumnDef[sf.ActiveUserRow]("TYPE", tablemodel.Width{Min: 8, Ideal: 14}, func(r sf.ActiveUserRow) string { return dashIfEmpty(r.SessionType) }),
			"Started":    textColumnDef[sf.ActiveUserRow]("STARTED", tablemodel.Width{Min: 14, Ideal: 16}, func(r sf.ActiveUserRow) string { return tstamp(r.Started) }),
			"Sessions":   textColumnDef[sf.ActiveUserRow]("#", tablemodel.Width{Min: 3, Ideal: 4}, func(r sf.ActiveUserRow) string { return fmt.Sprintf("%d", r.SessionCount) }),
			"IP":         textColumnDef[sf.ActiveUserRow]("IP", tablemodel.Width{Min: 12, Ideal: 16}, func(r sf.ActiveUserRow) string { return dashIfEmpty(r.SourceIP) }),
			"MFA": textColumnDef[sf.ActiveUserRow]("MFA", tablemodel.Width{Min: 5, Ideal: 6}, func(r sf.ActiveUserRow) string {
				if r.AnyLowMFA {
					return "low"
				}
				return "high"
			}),
			"Login": textColumnDef[sf.ActiveUserRow]("LOGIN", tablemodel.Width{Min: 8, Ideal: 14}, func(r sf.ActiveUserRow) string { return dashIfEmpty(r.LoginType) }),
		},
	}
}

func flowInterviewColumnSchema() tablemodel.Schema[sf.FlowInterviewRow] {
	when := func(r sf.FlowInterviewRow) string {
		if r.CreatedDate.IsZero() {
			return "—"
		}
		return r.CreatedDate.Format("2006-01-02 15:04")
	}
	return tablemodel.Schema[sf.FlowInterviewRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Status", "Flow", "Element", "When", "By"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.FlowInterviewRow]{
			"Status":  textColumnDef[sf.FlowInterviewRow]("STATUS", tablemodel.Width{Min: 8, Ideal: 10}, func(r sf.FlowInterviewRow) string { return r.Status }),
			"Flow":    textColumnDef[sf.FlowInterviewRow]("FLOW", tablemodel.Width{Min: 20, Ideal: 44}, func(r sf.FlowInterviewRow) string { return r.Label }),
			"Element": textColumnDef[sf.FlowInterviewRow]("ELEMENT", tablemodel.Width{Min: 12, Ideal: 22}, func(r sf.FlowInterviewRow) string { return dashIfEmpty(r.Element) }),
			"When":    textColumnDef[sf.FlowInterviewRow]("WHEN", tablemodel.Width{Min: 14, Ideal: 16}, when),
			"By":      textColumnDef[sf.FlowInterviewRow]("BY", tablemodel.Width{Min: 12, Ideal: 22}, func(r sf.FlowInterviewRow) string { return r.CreatedBy }),
			"Pause":   textColumnDef[sf.FlowInterviewRow]("PAUSE", tablemodel.Width{Min: 10, Ideal: 18}, func(r sf.FlowInterviewRow) string { return dashIfEmpty(r.PauseLabel) }),
		},
	}
}

func asyncJobColumnSchema() tablemodel.Schema[sf.AsyncJobRow] {
	return tablemodel.Schema[sf.AsyncJobRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Status", "Type", "ApexClass", "Progress", "Errors", "Created", "Completed"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.AsyncJobRow]{
			"Status":    textColumnDef[sf.AsyncJobRow]("STATUS", tablemodel.Width{Min: 8, Ideal: 12}, func(j sf.AsyncJobRow) string { return j.Status }),
			"Type":      textColumnDef[sf.AsyncJobRow]("TYPE", tablemodel.Width{Min: 8, Ideal: 16}, func(j sf.AsyncJobRow) string { return dashIfEmpty(j.JobType) }),
			"ApexClass": textColumnDef[sf.AsyncJobRow]("APEX CLASS", tablemodel.Width{Min: 16, Ideal: 30}, func(j sf.AsyncJobRow) string { return dashIfEmpty(j.ApexClassName) }),
			"Progress":  textColumnDef[sf.AsyncJobRow]("PROGRESS", tablemodel.Width{Min: 8, Ideal: 10}, asyncJobProgressCell),
			"Errors":    textColumnDef[sf.AsyncJobRow]("ERRORS", tablemodel.Width{Min: 6, Ideal: 8}, func(j sf.AsyncJobRow) string { return fmt.Sprintf("%d", j.NumberOfErrors) }),
			"Created": withSortKey(
				textColumnDef[sf.AsyncJobRow]("CREATED", tablemodel.Width{Min: 14, Ideal: 16}, func(j sf.AsyncJobRow) string { return prettyDate(j.CreatedDate) }),
				func(j sf.AsyncJobRow) string { return j.CreatedDate }),
			"Completed": withSortKey(
				textColumnDef[sf.AsyncJobRow]("COMPLETED", tablemodel.Width{Min: 14, Ideal: 16}, func(j sf.AsyncJobRow) string { return prettyDate(j.CompletedDate) }),
				func(j sf.AsyncJobRow) string { return j.CompletedDate }),
			"Method": textColumnDef[sf.AsyncJobRow]("METHOD", tablemodel.Width{Min: 10, Ideal: 20}, func(j sf.AsyncJobRow) string { return dashIfEmpty(j.MethodName) }),
		},
	}
}

// asyncJobProgressCell renders JobItemsDone/JobItemsTotal ("3/10");
// jobs with no batch items (future/queueable) show a dash.
func asyncJobProgressCell(j sf.AsyncJobRow) string {
	if j.JobItemsTotal == 0 {
		return "—"
	}
	return fmt.Sprintf("%d/%d", j.JobItemsDone, j.JobItemsTotal)
}

func scheduledJobColumnSchema() tablemodel.Schema[sf.CronTriggerRow] {
	return tablemodel.Schema[sf.CronTriggerRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "State", "NextFire", "LastFire", "Expression", "Fired"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.CronTriggerRow]{
			"Name":  textColumnDef[sf.CronTriggerRow]("NAME", tablemodel.Width{Min: 18, Ideal: 34}, func(c sf.CronTriggerRow) string { return dashIfEmpty(c.Name) }),
			"State": textColumnDef[sf.CronTriggerRow]("STATE", tablemodel.Width{Min: 8, Ideal: 12}, func(c sf.CronTriggerRow) string { return dashIfEmpty(c.State) }),
			"NextFire": withSortKey(
				textColumnDef[sf.CronTriggerRow]("NEXT FIRE", tablemodel.Width{Min: 14, Ideal: 16}, func(c sf.CronTriggerRow) string { return prettyDate(c.NextFireTime) }),
				func(c sf.CronTriggerRow) string { return c.NextFireTime }),
			"LastFire": withSortKey(
				textColumnDef[sf.CronTriggerRow]("LAST FIRE", tablemodel.Width{Min: 14, Ideal: 16}, func(c sf.CronTriggerRow) string { return prettyDate(c.PreviousFire) }),
				func(c sf.CronTriggerRow) string { return c.PreviousFire }),
			"Expression": textColumnDef[sf.CronTriggerRow]("EXPRESSION", tablemodel.Width{Min: 12, Ideal: 22}, func(c sf.CronTriggerRow) string { return dashIfEmpty(c.CronExpression) }),
			"Fired":      textColumnDef[sf.CronTriggerRow]("FIRED", tablemodel.Width{Min: 6, Ideal: 8}, func(c sf.CronTriggerRow) string { return fmt.Sprintf("%d", c.TimesTriggered) }),
			"Type":       textColumnDef[sf.CronTriggerRow]("TYPE", tablemodel.Width{Min: 10, Ideal: 20}, func(c sf.CronTriggerRow) string { return dashIfEmpty(c.Type) }),
		},
	}
}

func setupAuditColumnSchema() tablemodel.Schema[sf.SetupAuditRow] {
	when := func(r sf.SetupAuditRow) string {
		if r.CreatedDate.IsZero() {
			return "—"
		}
		return r.CreatedDate.Format("2006-01-02 15:04")
	}
	by := func(r sf.SetupAuditRow) string {
		// Surface delegate ("acted as") when present — the actor of
		// record is CreatedBy, but a login-as session shows who was
		// impersonating.
		if r.Delegate != "" {
			return r.CreatedBy + " (as " + r.Delegate + ")"
		}
		return r.CreatedBy
	}
	return tablemodel.Schema[sf.SetupAuditRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "Section", "Change", "By", "Action"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.SetupAuditRow]{
			"When":    textColumnDef[sf.SetupAuditRow]("WHEN", tablemodel.Width{Min: 14, Ideal: 16}, when),
			"Section": textColumnDef[sf.SetupAuditRow]("SECTION", tablemodel.Width{Min: 12, Ideal: 20}, func(r sf.SetupAuditRow) string { return r.Section }),
			// The Display sentence is the payload — give it the room.
			"Change": textColumnDef[sf.SetupAuditRow]("CHANGE", tablemodel.Width{Min: 24, Ideal: 60}, func(r sf.SetupAuditRow) string { return r.Display }),
			"By":     textColumnDef[sf.SetupAuditRow]("BY", tablemodel.Width{Min: 12, Ideal: 22}, by),
			"Action": textColumnDef[sf.SetupAuditRow]("ACTION", tablemodel.Width{Min: 12, Ideal: 24}, func(r sf.SetupAuditRow) string { return r.Action }),
		},
	}
}

func deployColumnSchema() tablemodel.Schema[sf.DeployRow] {
	return tablemodel.Schema[sf.DeployRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Status", "Kind", "Components", "Tests", "Duration", "When", "By"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.DeployRow]{
			"Status": textColumnDef[sf.DeployRow]("STATUS", tablemodel.Width{Min: 10, Ideal: 14}, func(r sf.DeployRow) string { return r.Status }),
			"Kind": textColumnDef[sf.DeployRow]("KIND", tablemodel.Width{Min: 8, Ideal: 10}, func(r sf.DeployRow) string {
				if r.CheckOnly {
					return "validate"
				}
				return "deploy"
			}),
			"Components": textColumnDef[sf.DeployRow]("COMPONENTS", tablemodel.Width{Min: 10, Ideal: 12}, func(r sf.DeployRow) string {
				out := fmt.Sprintf("%d/%d", r.ComponentsDeployed, r.ComponentsTotal)
				if r.ComponentErrors > 0 {
					out += fmt.Sprintf(" ·%d✗", r.ComponentErrors)
				}
				return out
			}),
			"Tests": textColumnDef[sf.DeployRow]("TESTS", tablemodel.Width{Min: 8, Ideal: 10}, func(r sf.DeployRow) string {
				if r.TestsTotal == 0 {
					return "—"
				}
				out := fmt.Sprintf("%d/%d", r.TestsCompleted, r.TestsTotal)
				if r.TestErrors > 0 {
					out += fmt.Sprintf(" ·%d✗", r.TestErrors)
				}
				return out
			}),
			"Duration": textColumnDef[sf.DeployRow]("TOOK", tablemodel.Width{Min: 6, Ideal: 8}, func(r sf.DeployRow) string {
				return deployDurationLabel(r)
			}),
			"ChangeSet": textColumnDef[sf.DeployRow]("CHANGE SET", tablemodel.Width{Min: 12, Ideal: 20}, func(r sf.DeployRow) string { return r.ChangeSetName }),
			"TestLevel": textColumnDef[sf.DeployRow]("TEST LEVEL", tablemodel.Width{Min: 10, Ideal: 16}, func(r sf.DeployRow) string { return r.TestLevel }),
			"When": withSortKey(
				textColumnDef[sf.DeployRow]("WHEN", tablemodel.Width{Min: 14, Ideal: 16}, func(r sf.DeployRow) string { return prettyDate(r.CreatedDate) }),
				func(r sf.DeployRow) string { return r.CreatedDate }),
			"By": textColumnDef[sf.DeployRow]("BY", tablemodel.Width{Min: 12, Ideal: 22}, func(r sf.DeployRow) string { return r.CreatedByName }),
			"Id": textColumnDef[sf.DeployRow]("ID", tablemodel.Width{Min: 18, Ideal: 22}, func(r sf.DeployRow) string { return r.ID }),
		},
	}
}

// deployDurationLabel renders Start→Completed compactly ("16s",
// "4m12s"); in-flight rows show an ellipsis instead of a span.
func deployDurationLabel(r sf.DeployRow) string {
	if r.InFlight() {
		return "…"
	}
	d := r.Duration()
	if d == 0 {
		return "—"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func packageColumnSchema() tablemodel.Schema[sf.InstalledPackage] {
	return tablemodel.Schema[sf.InstalledPackage]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Namespace", "Version"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.InstalledPackage]{
			"Name":      textColumnDef[sf.InstalledPackage]("NAME", tablemodel.Width{Min: 20, Ideal: 32}, func(p sf.InstalledPackage) string { return p.SubscriberPackageName }),
			"Namespace": textColumnDef[sf.InstalledPackage]("NAMESPACE", tablemodel.Width{Min: 12, Ideal: 22}, func(p sf.InstalledPackage) string { return p.SubscriberPackageNamespace }),
			"Version":   textColumnDef[sf.InstalledPackage]("VERSION", tablemodel.Width{Min: 8, Ideal: 12}, func(p sf.InstalledPackage) string { return p.SubscriberPackageVersionNumber }),
		},
	}
}

func recentColumnSchema() tablemodel.Schema[RecentEntry] {
	return tablemodel.Schema[RecentEntry]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "Kind", "Name", "Detail", "Id"}
		},
		Columns: map[string]tablemodel.ColumnDef[RecentEntry]{
			"When": withColumnStyle(
				textColumnDef[RecentEntry]("WHEN", tablemodel.Width{Min: 12, Ideal: 14}, func(r RecentEntry) string { return humanTimeAgo(r.VisitedAt) }),
				lipgloss.NewStyle().Foreground(theme.Muted),
			),
			"Kind": withColumnStyle(
				textColumnDef[RecentEntry]("KIND", tablemodel.Width{Min: 8, Ideal: 10}, func(r RecentEntry) string { return entryKindLabel(r.Kind) }),
				lipgloss.NewStyle().Foreground(theme.Yellow),
			),
			"Name": withColumnStyle(
				textColumnDef[RecentEntry]("NAME", tablemodel.Width{Min: 16, Ideal: 32}, func(r RecentEntry) string { return dashIfEmpty(recentNameForRow(r)) }),
				lipgloss.NewStyle().Foreground(theme.Fg),
			),
			"Detail": withColumnStyle(
				textColumnDef[RecentEntry]("DETAIL", tablemodel.Width{Min: 12, Ideal: 22}, func(r RecentEntry) string { return dashIfEmpty(recentDetailForRow(r)) }),
				lipgloss.NewStyle().Foreground(theme.Cyan),
			),
			"Id": withColumnStyle(
				textColumnDef[RecentEntry]("ID", tablemodel.Width{Min: 18, Ideal: 20}, func(r RecentEntry) string { return r.ID }),
				lipgloss.NewStyle().Foreground(theme.FgDim),
			),
		},
	}
}

func soqlSavedColumnSchema() tablemodel.Schema[devproject.SavedQuery] {
	return tablemodel.Schema[devproject.SavedQuery]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Tags", "Body", "Description", "Updated"}
		},
		Columns: map[string]tablemodel.ColumnDef[devproject.SavedQuery]{
			"Name":        textColumnDef[devproject.SavedQuery]("NAME", tablemodel.Width{Min: 16, Ideal: 28}, func(q devproject.SavedQuery) string { return q.Name }),
			"Tags":        textColumnDef[devproject.SavedQuery]("TAGS", tablemodel.Width{Min: 8, Ideal: 18}, func(q devproject.SavedQuery) string { return "" }),
			"Body":        textColumnDef[devproject.SavedQuery]("QUERY", tablemodel.Width{Min: 24, Ideal: 60}, func(q devproject.SavedQuery) string { return collapseSOQL(q.Body) }),
			"Description": textColumnDef[devproject.SavedQuery]("DESCRIPTION", tablemodel.Width{Min: 16, Ideal: 30}, func(q devproject.SavedQuery) string { return q.Description }),
			"Updated":     textColumnDef[devproject.SavedQuery]("UPDATED", tablemodel.Width{Min: 10, Ideal: 14}, func(q devproject.SavedQuery) string { return humanTimeAgo(q.UpdatedAt) }),
		},
	}
}

func soqlHistoryColumnSchema() tablemodel.Schema[devproject.SOQLHistoryEntry] {
	return tablemodel.Schema[devproject.SOQLHistoryEntry]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "Body", "Rows", "Took", "Status"}
		},
		Columns: map[string]tablemodel.ColumnDef[devproject.SOQLHistoryEntry]{
			"When": textColumnDef[devproject.SOQLHistoryEntry]("WHEN", tablemodel.Width{Min: 10, Ideal: 14}, func(e devproject.SOQLHistoryEntry) string { return humanTimeAgo(e.ExecutedAt) }),
			"Body": textColumnDef[devproject.SOQLHistoryEntry]("QUERY", tablemodel.Width{Min: 24, Ideal: 60}, func(e devproject.SOQLHistoryEntry) string { return collapseSOQL(e.Body) }),
			"Rows": textColumnDef[devproject.SOQLHistoryEntry]("ROWS", tablemodel.Width{Min: 6, Ideal: 8}, func(e devproject.SOQLHistoryEntry) string {
				if e.Error != "" {
					return "—"
				}
				return fmt.Sprintf("%d", e.RowCount)
			}),
			"Took":   textColumnDef[devproject.SOQLHistoryEntry]("TOOK", tablemodel.Width{Min: 6, Ideal: 8}, func(e devproject.SOQLHistoryEntry) string { return fmt.Sprintf("%dms", e.DurationMs) }),
			"Status": textColumnDef[devproject.SOQLHistoryEntry]("STATUS", tablemodel.Width{Min: 8, Ideal: 14}, soqlHistoryStatusCell),
		},
	}
}

func execSavedColumnSchema() tablemodel.Schema[devproject.SavedApex] {
	return tablemodel.Schema[devproject.SavedApex]{
		DefaultColumns: func(scope string) []string {
			return []string{"Name", "Body", "Description", "Updated"}
		},
		Columns: map[string]tablemodel.ColumnDef[devproject.SavedApex]{
			"Name":        textColumnDef[devproject.SavedApex]("NAME", tablemodel.Width{Min: 16, Ideal: 28}, func(a devproject.SavedApex) string { return a.Name }),
			"Body":        textColumnDef[devproject.SavedApex]("BODY", tablemodel.Width{Min: 24, Ideal: 60}, func(a devproject.SavedApex) string { return collapseApex(a.Body) }),
			"Description": textColumnDef[devproject.SavedApex]("DESCRIPTION", tablemodel.Width{Min: 16, Ideal: 30}, func(a devproject.SavedApex) string { return a.Description }),
			"Updated":     textColumnDef[devproject.SavedApex]("UPDATED", tablemodel.Width{Min: 10, Ideal: 14}, func(a devproject.SavedApex) string { return humanTimeAgo(a.UpdatedAt) }),
		},
	}
}

func execHistoryColumnSchema() tablemodel.Schema[devproject.ApexHistoryEntry] {
	return tablemodel.Schema[devproject.ApexHistoryEntry]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "Body", "Took", "Status"}
		},
		Columns: map[string]tablemodel.ColumnDef[devproject.ApexHistoryEntry]{
			"When":   textColumnDef[devproject.ApexHistoryEntry]("WHEN", tablemodel.Width{Min: 10, Ideal: 14}, func(e devproject.ApexHistoryEntry) string { return humanTimeAgo(e.ExecutedAt) }),
			"Body":   textColumnDef[devproject.ApexHistoryEntry]("SNIPPET", tablemodel.Width{Min: 24, Ideal: 60}, func(e devproject.ApexHistoryEntry) string { return collapseApex(e.Body) }),
			"Took":   textColumnDef[devproject.ApexHistoryEntry]("TOOK", tablemodel.Width{Min: 6, Ideal: 8}, func(e devproject.ApexHistoryEntry) string { return fmt.Sprintf("%dms", e.DurationMs) }),
			"Status": textColumnDef[devproject.ApexHistoryEntry]("STATUS", tablemodel.Width{Min: 10, Ideal: 14}, execHistoryStatusCell),
		},
	}
}

func homeNotifColumnSchema() tablemodel.Schema[sf.Notification] {
	return tablemodel.Schema[sf.Notification]{
		DefaultColumns: func(scope string) []string {
			return []string{"When", "State", "Type", "Title", "Body"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.Notification]{
			"When": withSortKey(
				textColumnDef[sf.Notification]("WHEN", tablemodel.Width{Min: 14, Ideal: 14}, func(n sf.Notification) string { return prettyDate(n.LastModified) }),
				func(n sf.Notification) string { return n.LastModified }),
			"State": textColumnDef[sf.Notification]("STATE", tablemodel.Width{Min: 6, Ideal: 6}, notifStateCell),
			"Type":  textColumnDef[sf.Notification]("TYPE", tablemodel.Width{Min: 12, Ideal: 12}, func(n sf.Notification) string { return notifTypeLabel(n.Type) }),
			"Title": textColumnDef[sf.Notification]("TITLE", tablemodel.Width{Min: 36, Ideal: 36}, notifTitleCell),
			"Body":  textColumnDef[sf.Notification]("BODY", tablemodel.Width{Min: 16, Ideal: 28}, notifBodyCell),
		},
	}
}

func homeLimitColumnSchema() tablemodel.Schema[KeyLimit] {
	return tablemodel.Schema[KeyLimit]{
		DefaultColumns: func(scope string) []string {
			return []string{"Group", "Limit", "Used", "Max", "Pct", "Usage"}
		},
		Columns: map[string]tablemodel.ColumnDef[KeyLimit]{
			"Group": textColumnDef[KeyLimit]("GROUP", tablemodel.Width{Min: 8, Ideal: 12}, func(l KeyLimit) string { return l.Group() }),
			"Limit": textColumnDef[KeyLimit]("LIMIT", tablemodel.Width{Min: 16, Ideal: 32}, func(l KeyLimit) string { return l.Name }),
			"Used":  textColumnDef[KeyLimit]("USED", tablemodel.Width{Min: 10, Ideal: 12}, func(l KeyLimit) string { return fmtThousands(l.Max - l.Remaining) }),
			"Max":   textColumnDef[KeyLimit]("MAX", tablemodel.Width{Min: 10, Ideal: 12}, func(l KeyLimit) string { return fmtThousands(l.Max) }),
			"Pct":   textColumnDef[KeyLimit]("%", tablemodel.Width{Min: 6, Ideal: 6}, limitPctCell),
			"Usage": textColumnDef[KeyLimit]("USAGE", tablemodel.Width{Min: 22, Ideal: 22}, func(l KeyLimit) string { return asciiBar(l.Max-l.Remaining, l.Max, 20) }),
		},
	}
}

func homeLicenseColumnSchema() tablemodel.Schema[homeLicenseRow] {
	return tablemodel.Schema[homeLicenseRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"License", "Kind", "Used", "Total", "Pct", "Status", "Usage"}
		},
		Columns: map[string]tablemodel.ColumnDef[homeLicenseRow]{
			"License": textColumnDef[homeLicenseRow]("LICENSE", tablemodel.Width{Min: 16, Ideal: 28}, func(l homeLicenseRow) string { return l.Name }),
			"Kind":    textColumnDef[homeLicenseRow]("KIND", tablemodel.Width{Min: 10, Ideal: 10}, func(l homeLicenseRow) string { return l.Kind }),
			"Used":    textColumnDef[homeLicenseRow]("USED", tablemodel.Width{Min: 8, Ideal: 8}, func(l homeLicenseRow) string { return fmtThousands(l.Used) }),
			"Total":   textColumnDef[homeLicenseRow]("TOTAL", tablemodel.Width{Min: 8, Ideal: 8}, func(l homeLicenseRow) string { return fmtThousands(l.Total) }),
			"Pct":     textColumnDef[homeLicenseRow]("%", tablemodel.Width{Min: 6, Ideal: 6}, licensePctCell),
			"Status":  textColumnDef[homeLicenseRow]("STATUS", tablemodel.Width{Min: 10, Ideal: 10}, func(l homeLicenseRow) string { return dashIfEmpty(l.Status) }),
			"Usage":   textColumnDef[homeLicenseRow]("USAGE", tablemodel.Width{Min: 22, Ideal: 22}, func(l homeLicenseRow) string { return asciiBar(l.Used, l.Total, 20) }),
		},
	}
}

func textColumnDef[T any](header string, width tablemodel.Width, render func(T) string) tablemodel.ColumnDef[T] {
	return tablemodel.ColumnDef[T]{
		Header:     header,
		Width:      width,
		Render:     render,
		Searchable: true,
		Exportable: true,
	}
}

func modifiedDateColumnDef[T any](render func(T) string) tablemodel.ColumnDef[T] {
	def := textColumnDef("MODIFIED", tablemodel.Width{Min: 14, Ideal: 16}, func(row T) string {
		return prettyDate(render(row))
	})
	def.SortKey = render
	return def
}

func modifiedByColumnDef[T any](render func(T) string) tablemodel.ColumnDef[T] {
	return textColumnDef("MODIFIED BY", tablemodel.Width{Min: 12, Ideal: 22}, func(row T) string {
		return dashIfEmpty(render(row))
	})
}

func withColumnStyle[T any](def tablemodel.ColumnDef[T], style lipgloss.Style) tablemodel.ColumnDef[T] {
	def.Style = style
	return def
}

// withSortKey attaches a SortKey to an existing column def so a
// column whose rendered cell is a human label (e.g. "1.5 KB") still
// orders by its raw underlying value.
func withSortKey[T any](def tablemodel.ColumnDef[T], key func(T) string) tablemodel.ColumnDef[T] {
	def.SortKey = key
	return def
}

func marksColumnDef[T any](ideal, max int) tablemodel.ColumnDef[T] {
	return tablemodel.ColumnDef[T]{
		Header: "FLAGS",
		Width:  tablemodel.Width{Min: 8, Ideal: ideal, Max: max},
	}
}

func queueSObjectsCell(q sf.QueueRow) string {
	if len(q.SObjects) == 0 {
		return "—"
	}
	s := q.SObjects[0]
	if len(q.SObjects) > 1 {
		s += ", " + q.SObjects[1]
	}
	if len(q.SObjects) > 2 {
		s += fmt.Sprintf(" +%d", len(q.SObjects)-2)
	}
	return s
}

func soqlHistoryStatusCell(e devproject.SOQLHistoryEntry) string {
	if e.Error != "" {
		return "error"
	}
	return "ok"
}

func execHistoryStatusCell(e devproject.ApexHistoryEntry) string {
	switch {
	case !e.Compiled:
		return "compile error"
	case !e.Success:
		return "runtime error"
	}
	return "ok"
}

func notifStateCell(n sf.Notification) string {
	if n.Read {
		return "read"
	}
	return "NEW"
}

func notifTitleCell(n sf.Notification) string {
	if n.MessageTitle != "" {
		return n.MessageTitle
	}
	return n.Type
}

func notifBodyCell(n sf.Notification) string {
	body := strings.ReplaceAll(n.MessageBody, "\n", " ")
	return strings.ReplaceAll(body, "\r", " ")
}

func limitPctCell(l KeyLimit) string {
	if l.Max == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", (l.Max-l.Remaining)*100/l.Max)
}

func licensePctCell(l homeLicenseRow) string {
	if l.Total == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", l.Used*100/l.Total)
}

func dashboardColumnSchema() tablemodel.Schema[sf.DashboardRow] {
	return tablemodel.Schema[sf.DashboardRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Title", "Folder", "RunAs", "Modified", "ModifiedBy"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.DashboardRow]{
			"Title":  textColumnDef[sf.DashboardRow]("TITLE", tablemodel.Width{Min: 22, Ideal: 40}, func(d sf.DashboardRow) string { return d.Title }),
			"Folder": textColumnDef[sf.DashboardRow]("FOLDER", tablemodel.Width{Min: 16, Ideal: 30}, func(d sf.DashboardRow) string { return dashIfEmpty(d.FolderName) }),
			"RunAs": textColumnDef[sf.DashboardRow]("RUN AS", tablemodel.Width{Min: 10, Ideal: 14}, func(d sf.DashboardRow) string {
				// Type is the running-user mode; LoggedInUser = dynamic.
				switch d.Type {
				case "LoggedInUser":
					return "viewer"
				case "SpecifiedUser":
					return "fixed user"
				}
				return dashIfEmpty(d.Type)
			}),
			"Modified":   modifiedDateColumnDef[sf.DashboardRow](func(d sf.DashboardRow) string { return d.LastModifiedDate }),
			"ModifiedBy": modifiedByColumnDef[sf.DashboardRow](func(d sf.DashboardRow) string { return d.LastModifiedByName }),
		},
	}
}

func reportTypeColumnSchema() tablemodel.Schema[sf.ReportTypeRow] {
	return tablemodel.Schema[sf.ReportTypeRow]{
		DefaultColumns: func(scope string) []string {
			return []string{"Label", "Type", "Category", "Custom", "Joined"}
		},
		Columns: map[string]tablemodel.ColumnDef[sf.ReportTypeRow]{
			"Label":    textColumnDef[sf.ReportTypeRow]("LABEL", tablemodel.Width{Min: 22, Ideal: 38}, func(r sf.ReportTypeRow) string { return r.Label }),
			"Type":     textColumnDef[sf.ReportTypeRow]("API NAME", tablemodel.Width{Min: 18, Ideal: 32}, func(r sf.ReportTypeRow) string { return r.Type }),
			"Category": textColumnDef[sf.ReportTypeRow]("CATEGORY", tablemodel.Width{Min: 14, Ideal: 24}, func(r sf.ReportTypeRow) string { return r.Category }),
			"Custom": textColumnDef[sf.ReportTypeRow]("CUSTOM", tablemodel.Width{Min: 7, Ideal: 8}, func(r sf.ReportTypeRow) string {
				if r.Custom {
					return "yes"
				}
				return "no"
			}),
			"Joined": textColumnDef[sf.ReportTypeRow]("JOINED OK", tablemodel.Width{Min: 8, Ideal: 10}, func(r sf.ReportTypeRow) string {
				if r.SupportsJoined {
					return "yes"
				}
				return "no"
			}),
		},
	}
}
