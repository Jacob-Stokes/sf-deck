package ui

// Fixture catalogs for the demo's operational surfaces — see
// demo_seed.go for the seeder and demo_seed_data.go for the core
// world (orgs, objects, flows, apex, records). This file adds the
// /system + /users + /components lists: setup audit trail, flow
// interviews, async + scheduled jobs, active users, recent logins,
// LWC/Aura bundle lists, and the flow-version definition maps. Same
// rules as demo_seed_data.go: everything fictional Northwind, nothing
// copied from a real org, timestamps in the formats the real
// fetchers return.
//
// The code-level drill-downs (apex/trigger bodies, bundle sources)
// live in demo_seed_data_code.go.

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// demoUserID is the stable fake User Id for the i-th member of the
// demo cast (demoPeople order, then the integration users appended by
// demoActiveUsers / demoRecentLogins).
func demoUserID(i int) string {
	return fmt.Sprintf("005DM00000DMU%02dAAA", i+1)
}

// demoUsername derives a cast member's login ("Mara Chen" ->
// mara.chen@northwind.example).
func demoUsername(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@northwind.example"
}

// demoIntegrationUsers extends the human cast with the API identities
// every real org accumulates. Indexes continue demoPeople's, so
// demoUserID stays collision-free across both lists.
var demoIntegrationUsers = []string{"EDI Gateway Integration", "Telemetry Ingest Service"}

// ---------------------------------------------------------------
// Setup audit trail
// ---------------------------------------------------------------

// demoSetupAudit backs /system's Audit subtab: a fortnight of the
// admin actions a working org actually logs, performed by the demo
// cast against the seeded metadata (flows, fields, permsets, classes).
func demoSetupAudit() []sf.SetupAuditRow {
	now := time.Now().UTC()
	rows := []struct {
		action, section, display string
		person                   int
		delegate                 string
		hoursAgo                 int
	}{
		{"activatedFlow", "Flows", "Activated flow version 11 of Carrier Onboarding", 0, "", 3},
		{"createdfield", "Custom Objects", "Created custom field Customs_Hold_Reason__c (Picklist) on Shipment", 1, "", 6},
		{"PermSetAssign", "Manage Users", "Assigned permission set Warehouse Operations to Tomas Silva", 2, "", 9},
		{"changedApexClass", "Apex Class", "Changed ShipmentService Apex Class code", 3, "", 14},
		{"changedValidationActive", "Validation Rules", "Changed active flag of validation rule Status Transitions on Shipment from Inactive to Active", 0, "", 21},
		{"createdpermset", "Manage Users", "Created permission set Customs Processing", 1, "", 27},
		{"loginas", "Manage Users", "Logged in as Tomas Silva", 4, "Mara Chen", 31},
		{"changedfield", "Custom Objects", "Changed custom field Credit_Limit__c on Account: decimal places from 0 to 2", 2, "", 45},
		{"frozeuser", "Manage Users", "Froze user Ingrid Larsen", 0, "", 52},
		{"profileFlsChanged", "Profiles", "Changed field-level security for Freight_Invoice__c.Total_Value__c on profile Logistics Operations", 1, "", 60},
		{"deactivatedFlow", "Flows", "Deactivated flow version 5 of Inventory Recount Request", 3, "", 74},
		{"createdApexTrigger", "Apex Trigger", "Created ColdChainAlertTrigger Apex Trigger code", 2, "", 90},
		{"changedPassword", "Manage Users", "Requested password reset for Priya Patel", 0, "", 110},
		{"createdgroup", "Groups", "Created public group Customs Escalations", 5, "", 130},
		{"queueMembership", "Queues", "Added Elif Demir to queue Claims Intake", 4, "", 150},
		{"remoteaccesscreate", "Connected Apps", "Created connected app Northwind EDI Gateway", 5, "", 170},
	}
	out := make([]sf.SetupAuditRow, 0, len(rows))
	for i, r := range rows {
		out = append(out, sf.SetupAuditRow{
			ID:          fmt.Sprintf("0YmDM00000DM%03dAAA", i+1),
			Action:      r.action,
			Section:     r.section,
			Display:     r.display,
			CreatedByID: demoUserID(r.person),
			CreatedBy:   demoPerson(r.person),
			Delegate:    r.delegate,
			CreatedDate: now.Add(-time.Duration(r.hoursAgo) * time.Hour),
		})
	}
	return out
}

// ---------------------------------------------------------------
// Flow interviews
// ---------------------------------------------------------------

// demoFlowInterviews backs /system's Interviews subtab: paused runs of
// the seeded screen flows plus the one errored interview that makes
// the surface worth having. Flow labels match demoFlows().
func demoFlowInterviews() []sf.FlowInterviewRow {
	now := time.Now().UTC()
	rows := []struct {
		flow, status, element, pause string
		person                       int
		hoursAgo                     int
	}{
		{"Customer Onboarding", "Started", "Collect_Company_Details", "", 4, 1},
		{"Dock Booking Wizard", "Error", "Create_Dock_Booking_Record", "", 2, 5},
		{"Carrier Onboarding", "Paused", "Wait_For_Compliance_Docs", "Waiting for compliance documents", 0, 26},
		{"Claim Submission Wizard", "Paused", "Pause_For_Assessor_Review", "Assessor review", 3, 49},
		{"Rate Card Approval", "Paused", "Wait_For_Finance_Signoff", "Finance sign-off", 1, 72},
		{"Quote Request Intake", "Paused", "Wait_For_Customer_Reply", "Customer reply window", 5, 96},
	}
	out := make([]sf.FlowInterviewRow, 0, len(rows))
	for i, r := range rows {
		started := now.Add(-time.Duration(r.hoursAgo) * time.Hour)
		out = append(out, sf.FlowInterviewRow{
			ID:          fmt.Sprintf("0FoDM00000DM%03dAAA", i+1),
			Label:       r.flow + " " + started.Format("2006-01-02, 15:04"),
			Status:      r.status,
			Element:     r.element,
			PauseLabel:  r.pause,
			CreatedByID: demoUserID(r.person),
			CreatedBy:   demoPerson(r.person),
			CreatedDate: started,
		})
	}
	return out
}

// ---------------------------------------------------------------
// Async + scheduled jobs
// ---------------------------------------------------------------

// demoApexClassIDs maps seeded class name -> Id so cross-referencing
// fixtures (async jobs, drill-down bodies) can't drift from the list.
func demoApexClassIDs() map[string]string {
	classes := demoApexClasses()
	out := make(map[string]string, len(classes))
	for _, c := range classes {
		out[c.Name] = c.ID
	}
	return out
}

// demoAsyncJobs backs the Jobs subtab: recent AsyncApexJob rows whose
// class Ids resolve to seeded apex classes, so o / Enter on a job row
// lands on a real body.
func demoAsyncJobs() []sf.AsyncJobRow {
	ids := demoApexClassIDs()
	now := time.Now().UTC()
	ts := func(minsAgo int) string {
		if minsAgo < 0 {
			return "" // still running / not finished
		}
		return now.Add(-time.Duration(minsAgo) * time.Minute).Format("2006-01-02T15:04:05.000+0000")
	}
	rows := []struct {
		status, jobType, class, method, extended string
		created, completed, total, done, errs    int
	}{
		{"Queued", "Queueable", "ColdChainAlertQueueable", "", "", 4, -1, 0, 0, 0},
		{"Processing", "BatchApex", "ShipmentEtaRecalcBatch", "", "", 12, -1, 118, 41, 0},
		{"Completed", "Queueable", "RateCardSyncQueueable", "", "", 65, 64, 1, 1, 0},
		{"Completed", "Future", "ShipmentService", "recalcRoutes", "", 130, 129, 1, 1, 0},
		{"Failed", "BatchApex", "DemurrageAccrualBatch", "",
			"First error: System.LimitException: Too many SOQL queries: 101", 210, 195, 12, 9, 1},
		{"Completed", "BatchApex", "SupplierScorecardBatch", "", "", 480, 452, 42, 42, 0},
		{"Completed", "ScheduledApex", "NightlyKpiSnapshotBatch", "", "", 620, 619, 1, 1, 0},
		{"Completed", "BatchApex", "NightlyKpiSnapshotBatch", "", "", 619, 588, 9, 9, 0},
		{"Aborted", "BatchApex", "SupplierScorecardBatch", "",
			"Aborted by Mara Chen before first batch", 1500, 1499, 42, 0, 0},
		{"Completed", "Future", "ClaimService", "notifyAssessors", "", 1620, 1619, 1, 1, 0},
		{"Completed", "Queueable", "RateCardSyncQueueable", "", "", 1730, 1729, 1, 1, 0},
		{"Completed", "BatchApex", "ShipmentEtaRecalcBatch", "", "", 2880, 2856, 116, 116, 0},
	}
	out := make([]sf.AsyncJobRow, 0, len(rows))
	for i, r := range rows {
		out = append(out, sf.AsyncJobRow{
			ID:             fmt.Sprintf("707DM00000DM%03dAAA", i+1),
			Status:         r.status,
			JobType:        r.jobType,
			ApexClassID:    ids[r.class],
			ApexClassName:  r.class,
			MethodName:     r.method,
			CreatedDate:    ts(r.created),
			CompletedDate:  ts(r.completed),
			ExtendedStatus: r.extended,
			JobItemsTotal:  r.total,
			JobItemsDone:   r.done,
			NumberOfErrors: r.errs,
		})
	}
	return out
}

// demoScheduledJobs backs the Scheduled subtab: the crons behind the
// seeded batch classes. CronJobDetail.JobType is Salesforce's raw
// code ("7" = scheduled Apex, "3" = dashboard refresh), matching what
// the live fetcher returns.
func demoScheduledJobs() []sf.CronTriggerRow {
	now := time.Now().UTC()
	ts := func(minsFromNow int) string {
		return now.Add(time.Duration(minsFromNow) * time.Minute).Format("2006-01-02T15:04:05.000+0000")
	}
	return []sf.CronTriggerRow{
		{ID: "08eDM00000DM001AAA", Name: "Rate Card Sync", Type: "7", State: "WAITING",
			NextFireTime: ts(37), PreviousFire: ts(-23), StartTime: ts(-60 * 24 * 190),
			CronExpression: "0 0 * * * ?", TimesTriggered: 1832},
		{ID: "08eDM00000DM002AAA", Name: "Nightly KPI Snapshot", Type: "7", State: "WAITING",
			NextFireTime: ts(540), PreviousFire: ts(-900), StartTime: ts(-60 * 24 * 214),
			CronExpression: "0 0 2 * * ?", TimesTriggered: 214},
		{ID: "08eDM00000DM003AAA", Name: "Supplier Scorecard Refresh", Type: "7", State: "WAITING",
			NextFireTime: ts(600), PreviousFire: ts(-840), StartTime: ts(-60 * 24 * 130),
			CronExpression: "0 0 3 ? * MON-FRI", TimesTriggered: 96},
		{ID: "08eDM00000DM004AAA", Name: "Demurrage Accrual", Type: "7", State: "ERROR",
			NextFireTime: "", PreviousFire: ts(-1290), StartTime: ts(-60 * 24 * 62),
			CronExpression: "0 30 1 * * ?", TimesTriggered: 62},
		{ID: "08eDM00000DM005AAA", Name: "Weekly Ops Dashboard Refresh", Type: "3", State: "WAITING",
			NextFireTime: ts(60 * 24 * 3), PreviousFire: ts(-60 * 24 * 4), StartTime: ts(-60 * 24 * 287),
			CronExpression: "0 0 6 ? * MON", TimesTriggered: 41},
	}
}

// ---------------------------------------------------------------
// Users: active sessions + recent logins
// ---------------------------------------------------------------

// demoActiveUsers backs /users' Active subtab: one row per cast member
// with a live session, plus the two integration identities (API
// sessions, LOW security — the rows the MFA chip exists to catch).
// Source IPs come from the RFC 5737 documentation ranges.
func demoActiveUsers() []sf.ActiveUserRow {
	now := time.Now().UTC()
	rows := []struct {
		person                              int // demoPeople index; >= len(demoPeople) = integration user
		sessionType, loginType, security    string
		ip, location                        string
		lastMins, startedMins, sessionCount int
		lowMFA, isAPI                       bool
	}{
		{0, "UI", "Application", "HIGH_ASSURANCE", "198.51.100.14", "Rotterdam, NL", 2, 190, 4, false, false},
		{1, "Aura", "Application", "HIGH_ASSURANCE", "198.51.100.23", "Leeds, GB", 7, 260, 3, false, false},
		{2, "UI", "Application", "LOW", "203.0.113.40", "Hamburg, DE", 11, 95, 2, true, false},
		{3, "UI", "SAML Idp Initiated SSO", "HIGH_ASSURANCE", "198.51.100.87", "Antwerp, BE", 25, 330, 2, false, false},
		{4, "Visualforce", "Application", "HIGH_ASSURANCE", "203.0.113.9", "Lyon, FR", 48, 140, 5, false, false},
		{5, "UI", "Application", "LOW", "198.51.100.61", "Aarhus, DK", 83, 100, 1, true, false},
		{6, "API", "Remote Access 2.0", "LOW", "192.0.2.201", "Dublin, IE", 1, 55, 6, true, true},
		{7, "Oauth2", "Remote Access 2.0", "LOW", "192.0.2.202", "Dublin, IE", 4, 470, 2, true, true},
	}
	name := func(i int) string {
		if i < len(demoPeople) {
			return demoPerson(i)
		}
		return demoIntegrationUsers[i-len(demoPeople)]
	}
	out := make([]sf.ActiveUserRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, sf.ActiveUserRow{
			UserID:        demoUserID(r.person),
			UserName:      name(r.person),
			LoginType:     r.loginType,
			SessionType:   r.sessionType,
			SecurityLevel: r.security,
			SourceIP:      r.ip,
			Location:      r.location,
			LastActive:    now.Add(-time.Duration(r.lastMins) * time.Minute),
			Started:       now.Add(-time.Duration(r.startedMins) * time.Minute),
			SessionCount:  r.sessionCount,
			AnyLowMFA:     r.lowMFA,
			IsAPI:         r.isAPI,
		})
	}
	return out
}

// demoRecentLogins fills HomeData.Users so /users' Recent subtab (and
// the Home user summary) isn't empty in demo mode. Same cast + Ids as
// demoActiveUsers, so the two views of "who's around" agree.
func demoRecentLogins() []sf.UserRow {
	now := time.Now().UTC()
	rows := []struct {
		person  int
		profile string
		role    string
		minsAgo int
	}{
		{6, "Integration User", "", 1},
		{0, "System Administrator", "Ops Leadership", 2},
		{7, "Integration User", "", 4},
		{1, "System Administrator", "Ops Leadership", 7},
		{2, "Logistics Operations", "EU Logistics", 11},
		{3, "Logistics Operations", "EU Logistics", 25},
		{4, "Standard User", "Warehouse - Rotterdam", 48},
		{5, "Standard User", "Finance", 83},
	}
	name := func(i int) string {
		if i < len(demoPeople) {
			return demoPerson(i)
		}
		return demoIntegrationUsers[i-len(demoPeople)]
	}
	out := make([]sf.UserRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, sf.UserRow{
			ID:            demoUserID(r.person),
			Name:          name(r.person),
			Username:      demoUsername(name(r.person)),
			ProfileName:   r.profile,
			UserRoleName:  r.role,
			LastLoginDate: now.Add(-time.Duration(r.minsAgo) * time.Minute).Format("2006-01-02T15:04:05.000+0000"),
			IsActive:      true,
		})
	}
	return out
}

// ---------------------------------------------------------------
// LWC + Aura bundle lists
// ---------------------------------------------------------------

// demoLWCSpec carries one component's identity plus the hooks the
// source generator (demo_seed_data_code.go) needs: which seeded apex
// class serves it, what it iterates over, what its empty state says.
type demoLWCSpec struct {
	dev, label, desc      string
	icon                  string // lightning-card icon-name
	apexClass, apexMethod string
	object                string // meta.xml record-page target
	emptyMsg              string
	exposed               bool
}

func demoLWCSpecs() []demoLWCSpec {
	return []demoLWCSpec{
		{dev: "shipmentTracker", label: "Shipment Tracker",
			desc: "Milestone timeline for a shipment record page.", icon: "standard:orders",
			apexClass: "ShipmentService", apexMethod: "recentForDisplay", object: "Shipment__c",
			emptyMsg: "No milestones recorded yet.", exposed: true},
		{dev: "orderTile", label: "Order Tile",
			desc: "Compact open-order summary for account pages.", icon: "standard:orders",
			apexClass: "InventoryService", apexMethod: "recentForDisplay", object: "Account",
			emptyMsg: "No open orders for this account.", exposed: true},
		{dev: "shipmentMap", label: "Shipment Map",
			desc: "Live position list for in-transit shipments.", icon: "standard:address",
			apexClass: "CarrierService", apexMethod: "recentForDisplay", object: "Shipment__c",
			emptyMsg: "No shipments currently in transit.", exposed: true},
		{dev: "invoicePanel", label: "Invoice Panel",
			desc: "Freight invoice lines with dispute flags.", icon: "standard:contract",
			apexClass: "FreightInvoiceService", apexMethod: "recentForDisplay", object: "Freight_Invoice__c",
			emptyMsg: "No invoice lines to review.", exposed: true},
		{dev: "carrierPicker", label: "Carrier Picker",
			desc: "Searchable carrier lookup with service levels.", icon: "standard:partners",
			apexClass: "CarrierService", apexMethod: "recentForDisplay", object: "Quote_Request__c",
			emptyMsg: "No carriers match the current search.", exposed: true},
		{dev: "dockScheduler", label: "Dock Scheduler",
			desc: "Dock slot bookings for one warehouse day.", icon: "standard:event",
			apexClass: "DockBookingService", apexMethod: "recentForDisplay", object: "Warehouse__c",
			emptyMsg: "No dock bookings for this day.", exposed: true},
		{dev: "warehouseHeatmap", label: "Warehouse Heatmap",
			desc: "Bin utilisation by zone for a warehouse.", icon: "standard:location",
			apexClass: "WarehouseService", apexMethod: "recentForDisplay", object: "Warehouse__c",
			emptyMsg: "No bin utilisation data yet.", exposed: false},
		{dev: "coldChainMonitor", label: "Cold Chain Monitor",
			desc: "Reefer temperature readings with excursion alerts.", icon: "standard:metrics",
			apexClass: "ContainerService", apexMethod: "recentForDisplay", object: "Container__c",
			emptyMsg: "No temperature readings in range.", exposed: true},
	}
}

func demoLWCBundleID(i int) string {
	return fmt.Sprintf("0RbDM00000DMC%02dAAA", i+1)
}

func demoLWCBundles() []sf.LWCBundle {
	specs := demoLWCSpecs()
	out := make([]sf.LWCBundle, 0, len(specs))
	for i, s := range specs {
		mod := demoStamp(i * 7)
		out = append(out, sf.LWCBundle{
			ID: demoLWCBundleID(i), DeveloperName: s.dev, MasterLabel: s.label,
			Description: s.desc, ApiVersion: 62, IsExposed: s.exposed,
			CreatedDate: demoCreatedFromModified(mod), CreatedByName: demoPerson(i + 1),
			LastModifiedDate: mod, LastModifiedByName: demoPerson(i),
		})
	}
	return out
}

// demoAuraSpec mirrors demoLWCSpec for the legacy bundles.
type demoAuraSpec struct {
	dev, label, desc      string
	icon                  string
	apexClass, apexMethod string
}

func demoAuraSpecs() []demoAuraSpec {
	return []demoAuraSpec{
		{dev: "freightAuditViewer", label: "Freight Audit Viewer",
			desc: "Read-only viewer for freight invoice audit history.", icon: "standard:contract",
			apexClass: "FreightInvoiceService", apexMethod: "recentForDisplay"},
		{dev: "legacyRateCalculator", label: "Legacy Rate Calculator",
			desc: "Pre-LWC rate calculator kept for the classic quote console.", icon: "standard:pricebook",
			apexClass: "TariffService", apexMethod: "recentForDisplay"},
		{dev: "quickQuoteAction", label: "Quick Quote Action",
			desc: "Global action that raises a quote request from anywhere.", icon: "standard:quotes",
			apexClass: "RouteService", apexMethod: "recentForDisplay"},
		{dev: "shipmentConsoleActions", label: "Shipment Console Actions",
			desc: "Quick status actions for the service console footer.", icon: "standard:orders",
			apexClass: "ShipmentService", apexMethod: "recentForDisplay"},
	}
}

func demoAuraBundleID(i int) string {
	return fmt.Sprintf("0AbDM00000DMA%02dAAA", i+1)
}

func demoAuraBundles() []sf.AuraBundle {
	specs := demoAuraSpecs()
	out := make([]sf.AuraBundle, 0, len(specs))
	for i, s := range specs {
		mod := demoStamp(i*9 + 5)
		out = append(out, sf.AuraBundle{
			ID: demoAuraBundleID(i), DeveloperName: s.dev, MasterLabel: s.label,
			Description: s.desc, ApiVersion: 62,
			CreatedDate: demoCreatedFromModified(mod), CreatedByName: demoPerson(i + 2),
			LastModifiedDate: mod, LastModifiedByName: demoPerson(i + 1),
		})
	}
	return out
}

// ---------------------------------------------------------------
// Flow version definitions
// ---------------------------------------------------------------

// demoFlowVersionDefs builds the per-version Flow metadata maps the
// in-terminal definition viewer renders (cache key
// "flowversiondef:<versionID>") — one small, plausible definition per
// seeded flow version.
func demoFlowVersionDefs() map[string]map[string]any {
	flows := demoFlows()
	versionsByDef := demoFlowVersionsByDef()
	out := make(map[string]map[string]any, len(flows)*4)
	for _, f := range flows {
		for _, v := range versionsByDef[f.DefinitionID] {
			out[v.ID] = demoFlowVersionDef(f, v)
		}
	}
	return out
}

// demoFlowAutoSuffixes mirrors the suffix set demoFlows composes its
// autolaunched dev names from, so the definition builder can recover
// the domain object from a DeveloperName.
var demoFlowAutoSuffixes = []string{"_After_Save", "_Status_Sync", "_Escalation", "_Approval"}

// demoFlowObjectFor recovers the triggering object from an
// autolaunched flow's dev name ("Cold_Chain_Alert_Status_Sync" ->
// "Cold_Chain_Alert__c"). Every autolaunched demo flow is built on a
// custom-object domain, so __c always applies.
func demoFlowObjectFor(devName string) string {
	for _, sfx := range demoFlowAutoSuffixes {
		if strings.HasSuffix(devName, sfx) {
			return strings.TrimSuffix(devName, sfx) + "__c"
		}
	}
	return ""
}

func demoFlowVersionDef(f sf.Flow, v sf.FlowVersion) map[string]any {
	def := map[string]any{
		"label":          v.MasterLabel,
		"apiVersion":     float64(v.APIVersion),
		"processType":    v.ProcessType,
		"status":         v.Status,
		"interviewLabel": v.MasterLabel + " {!$Flow.CurrentDateTime}",
		"description":    "Northwind " + v.MasterLabel + " automation.",
	}
	if obj := demoFlowObjectFor(f.DeveloperName); obj != "" {
		// Record-triggered shape: start on the object, one decision,
		// one same-record update.
		def["start"] = map[string]any{
			"locationX": 50, "locationY": 50,
			"object":            obj,
			"recordTriggerType": "CreateAndUpdate",
			"triggerType":       "RecordAfterSave",
			"connector":         map[string]any{"targetReference": "Check_Status_Changed"},
		}
		def["decisions"] = []any{map[string]any{
			"name": "Check_Status_Changed", "label": "Status changed?",
			"locationX": 176, "locationY": 50,
			"defaultConnectorLabel": "No change",
			"rules": []any{map[string]any{
				"name": "Status_Changed", "label": "Status changed",
				"conditionLogic": "and",
				"conditions": []any{map[string]any{
					"leftValueReference": "$Record.Status__c",
					"operator":           "IsChanged",
					"rightValue":         map[string]any{"booleanValue": true},
				}},
				"connector": map[string]any{"targetReference": "Stamp_External_Ref"},
			}},
		}}
		def["recordUpdates"] = []any{map[string]any{
			"name": "Stamp_External_Ref", "label": "Stamp external ref",
			"locationX": 308, "locationY": 50,
			"inputReference": "$Record",
			"inputAssignments": []any{map[string]any{
				"field": "External_Ref__c",
				"value": map[string]any{"elementReference": "$Record.Id"},
			}},
		}}
		return def
	}
	// Screen-flow shape: two screens and a record create.
	def["start"] = map[string]any{
		"locationX": 50, "locationY": 50,
		"connector": map[string]any{"targetReference": "Intro"},
	}
	def["screens"] = []any{
		map[string]any{
			"name": "Intro", "label": v.MasterLabel + " - Intro",
			"locationX": 176, "locationY": 50,
			"allowBack": false, "allowFinish": true, "allowPause": true,
			"fields": []any{map[string]any{
				"name": "Intro_Text", "fieldType": "DisplayText",
				"fieldText": "<p>Welcome to " + v.MasterLabel + ". Complete the steps to continue.</p>",
			}},
			"connector": map[string]any{"targetReference": "Details"},
		},
		map[string]any{
			"name": "Details", "label": v.MasterLabel + " - Details",
			"locationX": 308, "locationY": 50,
			"allowBack": true, "allowFinish": true, "allowPause": true,
			"fields": []any{
				map[string]any{"name": "Reference", "fieldType": "InputField", "isRequired": true},
				map[string]any{"name": "Notes", "fieldType": "LargeTextArea", "isRequired": false},
			},
			"connector": map[string]any{"targetReference": "Create_Request"},
		},
	}
	def["recordCreates"] = []any{map[string]any{
		"name": "Create_Request", "label": "Create request",
		"locationX": 440, "locationY": 50,
		"object": "Quote_Request__c",
		"inputAssignments": []any{map[string]any{
			"field": "Notes__c",
			"value": map[string]any{"elementReference": "Notes"},
		}},
	}}
	return def
}
