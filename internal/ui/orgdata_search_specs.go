package ui

// orgData per-ListView search-spec installation.
//
// Extracted from newOrgData in model.go — these are the 20+ MatchSpec
// registrations that wire substring search + relevance scoring onto
// each ListView wrapper. The patterns are mechanically similar (Any /
// Field / Fields / Primary) and lifting them out keeps newOrgData
// readable. See internal/ui/uilayout.MakeMatcher for the field-scoped
// search syntax.

import (
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
	"github.com/Jacob-Stokes/sf-deck/internal/ui/uilayout"
)

// installOrgDataSearchSpecs wires substring + scoring on every
// ListView held by orgData. Called once per org from newOrgData.
// Per-chip ChipUsers lists are lazily created and share the
// d.userMatch / d.userScore wrappers set up here.
func installOrgDataSearchSpecs(d *orgData) {
	installReportsAnalyticsSearchSpecs(d)
	installMetaSearchSpecs(d)

	// ListView predicates — substring match across the searchable
	// fields, with `field:value` shorthand for column-scoped queries.
	// See uilayout.MakeMatcher for the syntax.
	installSearch(&d.SObjectList, uilayout.MatchSpec[sf.SObject]{
		Any: func(s sf.SObject) string {
			return strings.ToLower(s.Name + " " + s.Label)
		},
		Field: func(s sf.SObject, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(s.Name)
			case "Label":
				return strings.ToLower(s.Label)
			}
			return ""
		},
		Fields:  []string{"Name", "Label"},
		Primary: "Name",
	})
	d.SObjectList.SetExtra(func(s sf.SObject) bool {
		switch d.SObjectFilter {
		case FilterManageable:
			return s.IsCustomizable
		case FilterCustom:
			return sf.IsCustom(s.Name)
		}
		return true
	})
	installSearch(&d.ApexLogList, uilayout.MatchSpec[sf.ApexLogRow]{
		Any: func(r sf.ApexLogRow) string {
			return strings.ToLower(r.Operation + " " + r.Status + " " + r.LogUser.Name)
		},
		Field: func(r sf.ApexLogRow, field string) string {
			switch field {
			case "Operation":
				return strings.ToLower(r.Operation)
			case "Status":
				return strings.ToLower(r.Status)
			case "User":
				return strings.ToLower(r.LogUser.Name)
			}
			return ""
		},
		Fields:  []string{"Operation", "Status", "User"},
		Primary: "Operation",
	})
	installSearch(&d.DeployList, uilayout.MatchSpec[sf.DeployRow]{
		Any: func(r sf.DeployRow) string {
			return strings.ToLower(r.Status + " " + r.CreatedByName + " " + r.ID)
		},
		Field: func(r sf.DeployRow, field string) string {
			switch field {
			case "Status":
				return strings.ToLower(r.Status)
			case "By":
				return strings.ToLower(r.CreatedByName)
			case "Id":
				return strings.ToLower(r.ID)
			}
			return ""
		},
		Fields:  []string{"Status", "By", "Id"},
		Primary: "Status",
	})
	installSearch(&d.PackageList, uilayout.MatchSpec[sf.InstalledPackage]{
		Any: func(p sf.InstalledPackage) string {
			return strings.ToLower(p.SubscriberPackageName + " " + p.SubscriberPackageNamespace)
		},
		Field: func(p sf.InstalledPackage, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(p.SubscriberPackageName)
			case "Namespace":
				return strings.ToLower(p.SubscriberPackageNamespace)
			}
			return ""
		},
		Fields:  []string{"Name", "Namespace"},
		Primary: "Name",
	})
	installSearch(&d.FlowList, uilayout.MatchSpec[sf.Flow]{
		Any: func(f sf.Flow) string {
			return strings.ToLower(f.DeveloperName + " " + f.MasterLabel + " " +
				f.Description + " " + f.ProcessType + " " + f.Status)
		},
		Field: func(f sf.Flow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(f.DeveloperName)
			case "Label":
				return strings.ToLower(f.MasterLabel)
			case "Description":
				return strings.ToLower(f.Description)
			case "Type":
				return strings.ToLower(f.ProcessType)
			case "Status":
				return strings.ToLower(f.Status)
			}
			return ""
		},
		Fields:  []string{"Name", "Label", "Description", "Type", "Status"},
		Primary: "Name",
	})
	installSearch(&d.ReportList, uilayout.MatchSpec[sf.ReportSummary]{
		Any: func(r sf.ReportSummary) string {
			return strings.ToLower(r.Name + " " + r.FolderName + " " +
				r.Owner + " " + r.Description + " " + r.Format)
		},
		Field: func(r sf.ReportSummary, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(r.Name)
			case "Folder":
				return strings.ToLower(r.FolderName)
			case "Owner":
				return strings.ToLower(r.Owner)
			case "Description":
				return strings.ToLower(r.Description)
			case "Format":
				return strings.ToLower(r.Format)
			}
			return ""
		},
		Fields:  []string{"Name", "Folder", "Owner", "Description", "Format"},
		Primary: "Name",
	})
	installSearch(&d.ApexClassList, uilayout.MatchSpec[sf.ApexClassRow]{
		Any: func(a sf.ApexClassRow) string {
			return strings.ToLower(a.Name + " " + a.Status)
		},
		Field: func(a sf.ApexClassRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(a.Name)
			case "Status":
				return strings.ToLower(a.Status)
			}
			return ""
		},
		Fields:  []string{"Name", "Status"},
		Primary: "Name",
	})
	installSearch(&d.LWCBundleList, uilayout.MatchSpec[sf.LWCBundle]{
		Any: func(l sf.LWCBundle) string {
			return strings.ToLower(l.DeveloperName + " " + l.MasterLabel + " " + l.Description)
		},
		Field: func(l sf.LWCBundle, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(l.DeveloperName)
			case "Label":
				return strings.ToLower(l.MasterLabel)
			case "Description":
				return strings.ToLower(l.Description)
			}
			return ""
		},
		Fields:  []string{"Name", "Label", "Description"},
		Primary: "Name",
	})
	installSearch(&d.AuraBundleList, uilayout.MatchSpec[sf.AuraBundle]{
		Any: func(a sf.AuraBundle) string {
			return strings.ToLower(a.DeveloperName + " " + a.MasterLabel + " " + a.Description)
		},
		Field: func(a sf.AuraBundle, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(a.DeveloperName)
			case "Label":
				return strings.ToLower(a.MasterLabel)
			case "Description":
				return strings.ToLower(a.Description)
			}
			return ""
		},
		Fields:  []string{"Name", "Label", "Description"},
		Primary: "Name",
	})
	installSearch(&d.HomeNotifList, uilayout.MatchSpec[sf.Notification]{
		Any: func(n sf.Notification) string {
			return strings.ToLower(n.MessageTitle + " " + n.MessageBody + " " + n.Type)
		},
		Field: func(n sf.Notification, field string) string {
			switch field {
			case "Title":
				return strings.ToLower(n.MessageTitle)
			case "Body":
				return strings.ToLower(n.MessageBody)
			case "Type":
				return strings.ToLower(n.Type)
			}
			return ""
		},
		Fields:  []string{"Title", "Body", "Type"},
		Primary: "Title",
	})
	installSearch(&d.HomeLimitList, uilayout.MatchSpec[KeyLimit]{
		Any: func(k KeyLimit) string { return strings.ToLower(k.Name) },
		Field: func(k KeyLimit, field string) string {
			if field == "Name" {
				return strings.ToLower(k.Name)
			}
			return ""
		},
		Fields:  []string{"Name"},
		Primary: "Name",
	})
	userSpec := uilayout.MatchSpec[sf.UserRow]{
		Any: func(u sf.UserRow) string {
			return strings.ToLower(u.Name + " " + u.Username + " " + u.ProfileName + " " + u.UserRoleName)
		},
		Field: func(u sf.UserRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(u.Name)
			case "Username":
				return strings.ToLower(u.Username)
			case "Profile":
				return strings.ToLower(u.ProfileName)
			case "Role":
				return strings.ToLower(u.UserRoleName)
			}
			return ""
		},
		Fields:  []string{"Name", "Username", "Profile", "Role"},
		Primary: "Name",
	}
	userMatch := uilayout.MakeMatcher(userSpec)
	userScore := uilayout.MakeScorer(userSpec)
	d.HomeUserList.SetMatch(userMatch)
	d.HomeUserList.SetScorer(userScore)
	// AllUsersList was the legacy single-fetch view; per-chip
	// ListViews are lazily created by EnsureChipUsers, each wrapping
	// userMatch via the same MatchSpec[sf.UserRow] pattern.
	d.userMatch = userMatch
	d.userScore = userScore
	installSearch(&d.HomeLicenseList, uilayout.MatchSpec[homeLicenseRow]{
		Any: func(l homeLicenseRow) string {
			return strings.ToLower(l.Name + " " + l.Kind + " " + l.Status)
		},
		Field: func(l homeLicenseRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(l.Name)
			case "Kind":
				return strings.ToLower(l.Kind)
			case "Status":
				return strings.ToLower(l.Status)
			}
			return ""
		},
		Fields:  []string{"Name", "Kind", "Status"},
		Primary: "Name",
	})
	installSearch(&d.QueueList, uilayout.MatchSpec[sf.QueueRow]{
		Any: func(q sf.QueueRow) string {
			return strings.ToLower(q.Name + " " + q.DeveloperName + " " + q.Email + " " +
				strings.Join(q.SObjects, " "))
		},
		Field: func(q sf.QueueRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(q.Name)
			case "DeveloperName":
				return strings.ToLower(q.DeveloperName)
			case "Email":
				return strings.ToLower(q.Email)
			case "SObjects":
				return strings.ToLower(strings.Join(q.SObjects, " "))
			}
			return ""
		},
		Fields:  []string{"Name", "DeveloperName", "Email", "SObjects"},
		Primary: "Name",
	})
	installSearch(&d.PublicGroupList, uilayout.MatchSpec[sf.PublicGroupRow]{
		Any: func(g sf.PublicGroupRow) string {
			return strings.ToLower(g.Name + " " + g.DeveloperName)
		},
		Field: func(g sf.PublicGroupRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(g.Name)
			case "DeveloperName":
				return strings.ToLower(g.DeveloperName)
			}
			return ""
		},
		Fields:  []string{"Name", "DeveloperName"},
		Primary: "Name",
	})
	installSearch(&d.ApexTriggerList, uilayout.MatchSpec[sf.TriggerRow]{
		Any: func(t sf.TriggerRow) string {
			return strings.ToLower(t.Name + " " + t.Table + " " + t.Status + " " + t.Events)
		},
		Field: func(t sf.TriggerRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(t.Name)
			case "Table", "SObject":
				return strings.ToLower(t.Table)
			case "Status":
				return strings.ToLower(t.Status)
			case "Events":
				return strings.ToLower(t.Events)
			}
			return ""
		},
		Fields:  []string{"Name", "Table", "Status", "Events"},
		Primary: "Name",
	})
	recentSpec := uilayout.MatchSpec[RecentEntry]{
		Any: func(r RecentEntry) string {
			return strings.ToLower(r.Name + " " + r.Type + " " + r.ID + " " + r.Kind)
		},
		Field: func(r RecentEntry, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(r.Name)
			case "Type":
				return strings.ToLower(r.Type)
			case "Id":
				return strings.ToLower(r.ID)
			case "Kind":
				return strings.ToLower(r.Kind)
			}
			return ""
		},
		Fields:  []string{"Name", "Type", "Id", "Kind"},
		Primary: "Name",
	}
	installSearch(&d.RecentList, recentSpec)
	// SF list shares the same matcher + scorer — same row shape,
	// same search semantics — so toggling modes preserves typing-
	// to-filter behaviour.
	installSearch(&d.RecentSFList, recentSpec)
	installSearch(&d.RecentlyViewedList, uilayout.MatchSpec[sf.RecentlyViewedRow]{
		Any: func(r sf.RecentlyViewedRow) string {
			return strings.ToLower(r.Name + " " + r.SObjectType + " " + r.ID)
		},
		Field: func(r sf.RecentlyViewedRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(r.Name)
			case "Type":
				return strings.ToLower(r.SObjectType)
			case "Id":
				return strings.ToLower(r.ID)
			}
			return ""
		},
		Fields:  []string{"Name", "Type", "Id"},
		Primary: "Name",
	})
	installSearch(&d.PermSetList, uilayout.MatchSpec[sf.PermissionSet]{
		Any: func(p sf.PermissionSet) string {
			return strings.ToLower(p.Name + " " + p.Label + " " +
				p.Description + " " + p.LicenseName + " " + p.NamespacePrefix)
		},
		Field: func(p sf.PermissionSet, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(p.Name)
			case "Label":
				return strings.ToLower(p.Label)
			case "Description":
				return strings.ToLower(p.Description)
			case "License":
				return strings.ToLower(p.LicenseName)
			case "Namespace":
				return strings.ToLower(p.NamespacePrefix)
			}
			return ""
		},
		Fields:  []string{"Name", "Label", "Description", "License", "Namespace"},
		Primary: "Name",
	})
	installSearch(&d.PSGList, uilayout.MatchSpec[sf.PermissionSetGroup]{
		Any: func(g sf.PermissionSetGroup) string {
			return strings.ToLower(g.DeveloperName + " " + g.MasterLabel + " " +
				g.Description + " " + g.Status + " " + g.NamespacePrefix)
		},
		Field: func(g sf.PermissionSetGroup, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(g.DeveloperName)
			case "Label":
				return strings.ToLower(g.MasterLabel)
			case "Description":
				return strings.ToLower(g.Description)
			case "Status":
				return strings.ToLower(g.Status)
			case "Namespace":
				return strings.ToLower(g.NamespacePrefix)
			}
			return ""
		},
		Fields:  []string{"Name", "Label", "Description", "Status", "Namespace"},
		Primary: "Name",
	})
	installSearch(&d.ProfileList, uilayout.MatchSpec[sf.Profile]{
		Any: func(p sf.Profile) string {
			return strings.ToLower(p.Name + " " + p.Description + " " +
				p.UserType + " " + p.UserLicenseName)
		},
		Field: func(p sf.Profile, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(p.Name)
			case "Description":
				return strings.ToLower(p.Description)
			case "UserType":
				return strings.ToLower(p.UserType)
			case "License":
				return strings.ToLower(p.UserLicenseName)
			}
			return ""
		},
		Fields:  []string{"Name", "Description", "UserType", "License"},
		Primary: "Name",
	})
}

// --- /reports Dashboards + Report Types (added with the /meta build;
// these two shipped 2026-06-11 without matchers, so / matched nothing) ---

func installReportsAnalyticsSearchSpecs(d *orgData) {
	installSearch(&d.DashboardList, uilayout.MatchSpec[sf.DashboardRow]{
		Any: func(r sf.DashboardRow) string {
			return strings.ToLower(r.Title + " " + r.DeveloperName + " " + r.FolderName)
		},
		Field: func(r sf.DashboardRow, field string) string {
			switch field {
			case "Title":
				return strings.ToLower(r.Title)
			case "DeveloperName":
				return strings.ToLower(r.DeveloperName)
			case "Folder":
				return strings.ToLower(r.FolderName)
			}
			return ""
		},
		Fields:  []string{"Title", "DeveloperName", "Folder"},
		Primary: "Title",
	})
	installSearch(&d.ReportTypeList, uilayout.MatchSpec[sf.ReportTypeRow]{
		Any: func(r sf.ReportTypeRow) string {
			return strings.ToLower(r.Label + " " + r.Type + " " + r.Category)
		},
		Field: func(r sf.ReportTypeRow, field string) string {
			switch field {
			case "Label":
				return strings.ToLower(r.Label)
			case "Type":
				return strings.ToLower(r.Type)
			case "Category":
				return strings.ToLower(r.Category)
			}
			return ""
		},
		Fields:  []string{"Label", "Type", "Category"},
		Primary: "Label",
	})
}

// --- /meta surfaces ------------------------------------------------------

func installMetaSearchSpecs(d *orgData) {
	installSearch(&d.MetaTypesList, uilayout.MatchSpec[sf.MetadataTypeInfo]{
		Any: func(t sf.MetadataTypeInfo) string {
			return strings.ToLower(t.XMLName + " " + t.DirectoryName)
		},
	})
	installSearch(&d.MetaTypeItemList, uilayout.MatchSpec[sf.MetadataItem]{
		Any: func(i sf.MetadataItem) string {
			return strings.ToLower(i.FullName + " " + i.NamespacePrefix)
		},
	})
	installSearch(&d.CustomLabelList, uilayout.MatchSpec[sf.CustomLabelRow]{
		Any: func(r sf.CustomLabelRow) string {
			return strings.ToLower(r.Name + " " + r.Value + " " + r.Category)
		},
		Field: func(r sf.CustomLabelRow, field string) string {
			switch field {
			case "Name":
				return strings.ToLower(r.Name)
			case "Value":
				return strings.ToLower(r.Value)
			case "Category":
				return strings.ToLower(r.Category)
			}
			return ""
		},
		Fields:  []string{"Name", "Value", "Category"},
		Primary: "Name",
	})
	entitySpec := uilayout.MatchSpec[sf.MetaEntityRow]{
		Any: func(r sf.MetaEntityRow) string {
			return strings.ToLower(r.QualifiedApiName + " " + r.Label)
		},
	}
	installSearch(&d.CMTList, entitySpec)
	installSearch(&d.CustomSettingList, entitySpec)
	installSearch(&d.StaticResourceList, uilayout.MatchSpec[sf.StaticResourceRow]{
		Any: func(r sf.StaticResourceRow) string {
			return strings.ToLower(r.Name + " " + r.ContentType)
		},
	})
	installSearch(&d.NamedCredList, uilayout.MatchSpec[sf.NamedCredentialRow]{
		Any: func(r sf.NamedCredentialRow) string {
			return strings.ToLower(r.DeveloperName + " " + r.MasterLabel + " " + r.Endpoint)
		},
	})
	installSearch(&d.RemoteSiteList, uilayout.MatchSpec[sf.RemoteSiteRow]{
		Any: func(r sf.RemoteSiteRow) string {
			return strings.ToLower(r.SiteName + " " + r.EndpointUrl)
		},
	})
}
