package ui

// Fixture catalogs for `sf-deck --demo` — see demo_seed.go for the
// seeder. Everything here is fictional and index-generated: name
// pools crossed with small arithmetic so lists fill full pages and
// scroll, without a single value copied from a real org.

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// demoPeople is the admin/dev cast that shows up as LastModifiedBy /
// CreatedBy across every surface.
var demoPeople = []string{
	"Mara Chen", "Priya Patel", "Jonah Okafor",
	"Elif Demir", "Tomas Silva", "Ingrid Larsen",
}

func demoPerson(i int) string { return demoPeople[i%len(demoPeople)] }

// demoStamp returns a plausible LastModifiedDate, spread over the
// ~90 days before launch so date columns look lived-in.
func demoStamp(i int) string {
	return time.Now().UTC().
		Add(-time.Duration(3+(i*37)%2160) * time.Hour).
		Format("2006-01-02T15:04:05.000+0000")
}

// keyPrefixFor synthesizes custom-object key prefixes a0A, a0B …
// a0Z, a1A … the way real orgs hand them out in creation order.
func keyPrefixFor(i int) string {
	return fmt.Sprintf("a%d%c", i/26, 'A'+rune(i%26))
}

// ---------------------------------------------------------------
// sObject catalog
// ---------------------------------------------------------------

var demoStdObjects = []struct{ n, l, kp string }{
	{"Account", "Account", "001"},
	{"AccountContactRelation", "Account Contact Relationship", "07k"},
	{"Asset", "Asset", "02i"},
	{"Attachment", "Attachment", "00P"},
	{"BusinessHours", "Business Hours", "01m"},
	{"Campaign", "Campaign", "701"},
	{"CampaignMember", "Campaign Member", "00v"},
	{"Case", "Case", "500"},
	{"CaseComment", "Case Comment", "00a"},
	{"Contact", "Contact", "003"},
	{"ContentDocument", "Content Document", "069"},
	{"ContentVersion", "Content Version", "068"},
	{"Contract", "Contract", "800"},
	{"Dashboard", "Dashboard", "01Z"},
	{"EmailMessage", "Email Message", "02s"},
	{"EmailTemplate", "Email Template", "00X"},
	{"Event", "Event", "00U"},
	{"FeedItem", "Feed Item", "0D5"},
	{"Folder", "Folder", "00l"},
	{"Group", "Group", "00G"},
	{"Holiday", "Holiday", "00m"},
	{"Lead", "Lead", "00Q"},
	{"Note", "Note", "002"},
	{"Opportunity", "Opportunity", "006"},
	{"OpportunityContactRole", "Opportunity Contact Role", "00K"},
	{"OpportunityLineItem", "Opportunity Product", "00k"},
	{"Order", "Order", "801"},
	{"OrderItem", "Order Product", "802"},
	{"Organization", "Organization", "00D"},
	{"PermissionSet", "Permission Set", "0PS"},
	{"PermissionSetAssignment", "Permission Set Assignment", "0Pa"},
	{"Pricebook2", "Price Book", "01s"},
	{"PricebookEntry", "Price Book Entry", "01u"},
	{"Product2", "Product", "01t"},
	{"Profile", "Profile", "00e"},
	{"Quote", "Quote", "0Q0"},
	{"QuoteLineItem", "Quote Line Item", "0QL"},
	{"RecordType", "Record Type", "012"},
	{"Report", "Report", "00O"},
	{"Task", "Task", "00T"},
	{"Topic", "Topic", "0TO"},
	{"User", "User", "005"},
	{"UserRole", "Role", "00E"},
}

var demoCustObjects = []struct{ n, l string }{
	{"Shipment__c", "Shipment"},
	{"Shipment_Line__c", "Shipment Line"},
	{"Warehouse__c", "Warehouse"},
	{"Carrier__c", "Carrier"},
	{"Route__c", "Route"},
	{"Customs_Declaration__c", "Customs Declaration"},
	{"Inventory_Adjustment__c", "Inventory Adjustment"},
	{"Supplier_Scorecard__c", "Supplier Scorecard"},
	{"Freight_Invoice__c", "Freight Invoice"},
	{"Dock_Booking__c", "Dock Booking"},
	{"Container__c", "Container"},
	{"Container_Lease__c", "Container Lease"},
	{"Pallet__c", "Pallet"},
	{"Bin_Location__c", "Bin Location"},
	{"Stock_Item__c", "Stock Item"},
	{"Goods_Receipt__c", "Goods Receipt"},
	{"Pick_List__c", "Pick List"},
	{"Packing_Slip__c", "Packing Slip"},
	{"Delivery_Window__c", "Delivery Window"},
	{"Fleet_Vehicle__c", "Fleet Vehicle"},
	{"Driver__c", "Driver"},
	{"Maintenance_Log__c", "Maintenance Log"},
	{"Fuel_Card__c", "Fuel Card"},
	{"Toll_Charge__c", "Toll Charge"},
	{"Border_Crossing__c", "Border Crossing"},
	{"Tariff_Code__c", "Tariff Code"},
	{"Duty_Rate__c", "Duty Rate"},
	{"Insurance_Policy__c", "Insurance Policy"},
	{"Claim__c", "Claim"},
	{"Damage_Report__c", "Damage Report"},
	{"Temperature_Log__c", "Temperature Log"},
	{"Cold_Chain_Alert__c", "Cold Chain Alert"},
	{"Port_Call__c", "Port Call"},
	{"Vessel__c", "Vessel"},
	{"Voyage__c", "Voyage"},
	{"Demurrage_Charge__c", "Demurrage Charge"},
	{"Rate_Card__c", "Rate Card"},
	{"Quote_Request__c", "Quote Request"},
	{"Service_Level__c", "Service Level"},
	{"KPI_Snapshot__c", "KPI Snapshot"},
}

func demoSObjects() []sf.SObject {
	out := make([]sf.SObject, 0, len(demoStdObjects)+len(demoCustObjects))
	for i, s := range demoStdObjects {
		out = append(out, sf.SObject{Name: s.n, Label: s.l, KeyPrefix: s.kp,
			IsCustomizable: true, ApexTriggerable: true,
			LastModifiedDate: demoStamp(i), LastModifiedByName: demoPerson(i)})
	}
	for i, s := range demoCustObjects {
		out = append(out, sf.SObject{Name: s.n, Label: s.l, KeyPrefix: keyPrefixFor(i),
			IsCustomizable: true, ApexTriggerable: true, WorkflowEnabled: true,
			DeploymentStatus: "Deployed",
			LastModifiedDate: demoStamp(i + 7), LastModifiedByName: demoPerson(i + 1)})
	}
	return out
}

// ---------------------------------------------------------------
// Describes
// ---------------------------------------------------------------

func demoField(name, label, typ string, length int, custom bool) sf.Field {
	return sf.Field{Name: name, Label: label, Type: typ, Length: length,
		Custom: custom, Nillable: true, Permissionable: custom}
}

// demoCuratedFields carries hand-written field sets for the objects
// people actually know — a Salesforce admin pausing a tape on the
// Contact or Opportunity fields list must see the real shape, not a
// generic filler set.
var demoCuratedFields = map[string][]sf.Field{
	"Contact": {
		demoField("FirstName", "First Name", "string", 40, false),
		demoField("LastName", "Last Name", "string", 80, false),
		demoField("AccountId", "Account ID", "reference", 18, false),
		demoField("Email", "Email", "email", 80, false),
		demoField("Phone", "Business Phone", "phone", 40, false),
		demoField("Title", "Title", "string", 128, false),
		demoField("Department", "Department", "string", 80, false),
		demoField("MailingCity", "Mailing City", "string", 40, false),
		demoField("MailingCountry", "Mailing Country", "string", 80, false),
		demoField("Birthdate", "Birthdate", "date", 0, false),
		demoField("Preferred_Contact_Method__c", "Preferred Contact Method", "picklist", 0, true),
		demoField("Booking_Portal_User__c", "Booking Portal User", "boolean", 0, true),
	},
	"Opportunity": {
		demoField("AccountId", "Account ID", "reference", 18, false),
		demoField("Amount", "Amount", "currency", 0, false),
		demoField("CloseDate", "Close Date", "date", 0, false),
		demoField("StageName", "Stage", "picklist", 0, false),
		demoField("Probability", "Probability (%)", "percent", 0, false),
		demoField("Type", "Opportunity Type", "picklist", 0, false),
		demoField("LeadSource", "Lead Source", "picklist", 0, false),
		demoField("NextStep", "Next Step", "string", 255, false),
		demoField("IsClosed", "Closed", "boolean", 0, false),
		demoField("IsWon", "Won", "boolean", 0, false),
		demoField("Annual_Volume_Pallets__c", "Annual Volume (Pallets)", "double", 0, true),
		demoField("Contract_Lane__c", "Contract Lane", "reference", 18, true),
	},
	"Case": {
		demoField("CaseNumber", "Case Number", "string", 30, false),
		demoField("AccountId", "Account ID", "reference", 18, false),
		demoField("ContactId", "Contact ID", "reference", 18, false),
		demoField("Subject", "Subject", "string", 255, false),
		demoField("Status", "Status", "picklist", 0, false),
		demoField("Priority", "Priority", "picklist", 0, false),
		demoField("Origin", "Case Origin", "picklist", 0, false),
		demoField("Reason", "Case Reason", "picklist", 0, false),
		demoField("IsClosed", "Closed", "boolean", 0, false),
		demoField("Related_Shipment__c", "Related Shipment", "reference", 18, true),
	},
	"Lead": {
		demoField("FirstName", "First Name", "string", 40, false),
		demoField("LastName", "Last Name", "string", 80, false),
		demoField("Company", "Company", "string", 255, false),
		demoField("Email", "Email", "email", 80, false),
		demoField("Phone", "Phone", "phone", 40, false),
		demoField("Status", "Lead Status", "picklist", 0, false),
		demoField("LeadSource", "Lead Source", "picklist", 0, false),
		demoField("Industry", "Industry", "picklist", 0, false),
		demoField("IsConverted", "Converted", "boolean", 0, false),
	},
	"Task": {
		demoField("Subject", "Subject", "combobox", 255, false),
		demoField("Status", "Status", "picklist", 0, false),
		demoField("Priority", "Priority", "picklist", 0, false),
		demoField("ActivityDate", "Due Date", "date", 0, false),
		demoField("WhoId", "Name ID", "reference", 18, false),
		demoField("WhatId", "Related To ID", "reference", 18, false),
		demoField("Description", "Comments", "textarea", 32000, false),
	},
	"User": {
		demoField("FirstName", "First Name", "string", 40, false),
		demoField("LastName", "Last Name", "string", 80, false),
		demoField("Email", "Email", "email", 128, false),
		demoField("Username", "Username", "string", 80, false),
		demoField("Alias", "Alias", "string", 8, false),
		demoField("IsActive", "Active", "boolean", 0, false),
		demoField("ProfileId", "Profile ID", "reference", 18, false),
		demoField("TimeZoneSidKey", "Time Zone", "picklist", 0, false),
	},
	"Product2": {
		demoField("ProductCode", "Product Code", "string", 255, false),
		demoField("Description", "Product Description", "textarea", 4000, false),
		demoField("Family", "Product Family", "picklist", 0, false),
		demoField("IsActive", "Active", "boolean", 0, false),
	},
	"Order": {
		demoField("AccountId", "Account ID", "reference", 18, false),
		demoField("OrderNumber", "Order Number", "string", 30, false),
		demoField("EffectiveDate", "Order Start Date", "date", 0, false),
		demoField("Status", "Status", "picklist", 0, false),
		demoField("TotalAmount", "Order Amount", "currency", 0, false),
	},
	"Account": {
		demoField("AccountNumber", "Account Number", "string", 40, false),
		demoField("Type", "Account Type", "picklist", 0, false),
		demoField("Industry", "Industry", "picklist", 0, false),
		demoField("AnnualRevenue", "Annual Revenue", "currency", 0, false),
		demoField("NumberOfEmployees", "Employees", "int", 0, false),
		demoField("Phone", "Account Phone", "phone", 40, false),
		demoField("Website", "Website", "url", 255, false),
		demoField("BillingCity", "Billing City", "string", 40, false),
		demoField("BillingCountry", "Billing Country", "string", 80, false),
		demoField("Description", "Account Description", "textarea", 32000, false),
		demoField("Preferred_Carrier__c", "Preferred Carrier", "reference", 18, true),
		demoField("Shipping_Terms__c", "Shipping Terms", "picklist", 0, true),
		demoField("Credit_Limit__c", "Credit Limit", "currency", 0, true),
		demoField("On_Stop__c", "On Stop", "boolean", 0, true),
		demoField("Account_Tier__c", "Account Tier", "picklist", 0, true),
		demoField("Last_Shipment_Date__c", "Last Shipment Date", "date", 0, true),
		demoField("Customs_Broker__c", "Customs Broker", "string", 120, true),
		demoField("Pallet_Capacity__c", "Pallet Capacity", "double", 0, true),
		demoField("Strategic_Account__c", "Strategic Account", "boolean", 0, true),
		demoField("Delivery_Window_Notes__c", "Delivery Window Notes", "textarea", 4000, true),
	},
	"Shipment__c": {
		demoField("Status__c", "Status", "picklist", 0, true),
		demoField("Origin_Warehouse__c", "Origin Warehouse", "reference", 18, true),
		demoField("Destination__c", "Destination", "string", 255, true),
		demoField("Carrier__c", "Carrier", "reference", 18, true),
		demoField("Route__c", "Route", "reference", 18, true),
		demoField("Ship_Date__c", "Ship Date", "date", 0, true),
		demoField("Expected_Delivery__c", "Expected Delivery", "date", 0, true),
		demoField("Actual_Delivery__c", "Actual Delivery", "datetime", 0, true),
		demoField("Total_Weight_Kg__c", "Total Weight (kg)", "double", 0, true),
		demoField("Pallet_Count__c", "Pallet Count", "double", 0, true),
		demoField("Customs_Status__c", "Customs Status", "picklist", 0, true),
		demoField("Temperature_Controlled__c", "Temperature Controlled", "boolean", 0, true),
		demoField("Incoterms__c", "Incoterms", "picklist", 0, true),
	},
}

// demoCustomFieldPool feeds generated describes for the custom
// objects without a curated set; demoStdFieldPool does the same for
// the long tail of standard objects.
var demoCustomFieldPool = []struct {
	name, label, typ string
}{
	{"Status__c", "Status", "picklist"},
	{"Active__c", "Active", "boolean"},
	{"External_Ref__c", "External Reference", "string"},
	{"Notes__c", "Notes", "textarea"},
	{"Region__c", "Region", "picklist"},
	{"Start_Date__c", "Start Date", "date"},
	{"End_Date__c", "End Date", "date"},
	{"Quantity__c", "Quantity", "double"},
	{"Unit_Cost__c", "Unit Cost", "currency"},
	{"Approved__c", "Approved", "boolean"},
	{"Priority__c", "Priority", "picklist"},
	{"Owner_Team__c", "Owner Team", "string"},
	{"Risk_Score__c", "Risk Score", "double"},
	{"Last_Audit_Date__c", "Last Audit Date", "date"},
	{"Reference_Code__c", "Reference Code", "string"},
	{"Total_Value__c", "Total Value", "currency"},
}

var demoStdFieldPool = []struct {
	name, label, typ string
}{
	{"Description", "Description", "textarea"},
	{"Type", "Type", "picklist"},
	{"Status", "Status", "picklist"},
	{"IsActive", "Active", "boolean"},
	{"Title", "Title", "string"},
	{"Body", "Body", "textarea"},
}

// demoDescribeFor builds a describe for any catalog object: system
// fields first (the part every object shares), then a curated set
// for the flagship objects or a deterministic slice of the filler
// pool for the rest.
func demoDescribeFor(o sf.SObject) sf.SObjectDescribe {
	custom := o.DeploymentStatus != "" // only custom objects carry it in the catalog
	fields := []sf.Field{
		demoField("Id", o.Label+" ID", "id", 18, false),
		demoField("Name", o.Label+" Name", "string", 80, false),
		demoField("OwnerId", "Owner ID", "reference", 18, false),
		demoField("CreatedDate", "Created Date", "datetime", 0, false),
		demoField("CreatedById", "Created By ID", "reference", 18, false),
		demoField("LastModifiedDate", "Last Modified Date", "datetime", 0, false),
		demoField("LastModifiedById", "Last Modified By ID", "reference", 18, false),
		demoField("SystemModstamp", "System Modstamp", "datetime", 0, false),
	}
	if curated, ok := demoCuratedFields[o.Name]; ok {
		fields = append(fields, curated...)
	} else {
		// Deterministic per-object slice of the filler pool: the
		// name hash picks where to start and how many to take, so
		// every object's fields list looks different but re-seeds
		// identically.
		h := 0
		for _, c := range o.Name {
			h += int(c)
		}
		pool := demoStdFieldPool
		if custom {
			pool = demoCustomFieldPool
		}
		n := 4 + h%(len(pool)-3)
		for i := 0; i < n; i++ {
			p := pool[(h+i)%len(pool)]
			length := 0
			if p.typ == "string" {
				length = 120
			}
			if p.typ == "textarea" {
				length = 4000
			}
			fields = append(fields, demoField(p.name, p.label, p.typ, length, custom))
		}
	}
	return sf.SObjectDescribe{
		Name: o.Name, Label: o.Label, LabelPlural: o.Label + "s",
		Custom: custom, KeyPrefix: o.KeyPrefix,
		Queryable: true, Creatable: true, Updatable: true, Deletable: true,
		MruEnabled: true,
		Fields:     fields,
	}
}

// demoBaselineFor backs the object-action sidebar's feature-toggle
// state (object_baseline:<obj>) so a drill doesn't flash a demo-mode
// fetch error while "loading current toggle state".
func demoBaselineFor(o sf.SObject) *sf.CustomObjectBaseline {
	yes, no := true, false
	b := &sf.CustomObjectBaseline{
		Label: o.Label, PluralLabel: o.Label + "s",
		NameFieldLabel: o.Label + " Name", NameFieldType: "Text",
		SharingModel:  "ReadWrite",
		EnableReports: &yes, EnableActivities: &yes,
		EnableHistory: &no, EnableFeeds: &no, EnableSearch: &yes,
	}
	if o.DeploymentStatus != "" { // custom objects in the catalog
		b.Description = "Northwind " + o.Label + " tracking."
	}
	return b
}

// ---------------------------------------------------------------
// Permission sets + FLS
// ---------------------------------------------------------------

// demoPermsetPicker backs the FLS subtab's parent strip: three
// profiles and five standalone permission sets.
func demoPermsetPicker() []sf.FLSPickerEntry {
	return []sf.FLSPickerEntry{
		{ID: "0PSDM00000DMPF1AAA", Name: "X00eAdminDemo", Label: "System Administrator", ProfileID: "00eDM00000DMPR1AAA"},
		{ID: "0PSDM00000DMPF2AAA", Name: "X00eStandardDemo", Label: "Standard User", ProfileID: "00eDM00000DMPR2AAA"},
		{ID: "0PSDM00000DMPF3AAA", Name: "X00eLogisticsOps", Label: "Logistics Operations", ProfileID: "00eDM00000DMPR3AAA"},
		{ID: "0PSDM00000DMPS1AAA", Name: "Shipment_Management", Label: "Shipment Management", IsPermSet: true},
		{ID: "0PSDM00000DMPS2AAA", Name: "Warehouse_Operations", Label: "Warehouse Operations", IsPermSet: true},
		{ID: "0PSDM00000DMPS3AAA", Name: "Customs_Processing", Label: "Customs Processing", IsPermSet: true},
		{ID: "0PSDM00000DMPS4AAA", Name: "Finance_Freight_Invoices", Label: "Finance - Freight Invoices", IsPermSet: true},
		{ID: "0PSDM00000DMPS5AAA", Name: "Read_Only_Reporting", Label: "Read Only Reporting", IsPermSet: true},
	}
}

// demoFLSRows builds one (sobject, parent) FLS payload from the
// object's describe. Admin sees everything R+E, the read-only
// reporting set sees R only, and the middle of the ladder gets a
// deterministic mix — a row is omitted entirely when the parent has
// no access (matching the live API, where no FieldPermissions row
// means no access).
func demoFLSRows(sobject string, fields []sf.Field, parentIdx, parentCount int, parentID string) []sf.FieldPermissionRow {
	var out []sf.FieldPermissionRow
	n := 0
	for _, f := range fields {
		if !f.Permissionable {
			continue
		}
		n++
		read, edit := true, true
		switch {
		case parentIdx == 0: // System Administrator
		case parentIdx == parentCount-1: // Read Only Reporting
			edit = false
		default:
			read = (n+parentIdx)%4 != 0
			edit = read && (n+parentIdx*3)%3 != 0
		}
		if !read {
			continue
		}
		out = append(out, sf.FieldPermissionRow{
			ID:       fmt.Sprintf("01kDM%02d%02d%03dAAA", parentIdx, n%100, len(out)+1),
			Field:    sobject + "." + f.Name,
			ParentID: parentID,
			Read:     read,
			Edit:     edit,
		})
	}
	return out
}

// ---------------------------------------------------------------
// Flows
// ---------------------------------------------------------------

func demoFlows() []sf.Flow {
	type row struct {
		dev, label, ptype, status string
		ver                       int
	}
	rows := []row{
		{"Carrier_Onboarding", "Carrier Onboarding", "Flow", "Active", 11},
		{"Dock_Booking_Screen", "Dock Booking Wizard", "Flow", "Active", 5},
		{"Supplier_Review_Cycle", "Supplier Review Cycle", "Flow", "Active", 3},
		{"Inventory_Recount", "Inventory Recount Request", "Flow", "Obsolete", 6},
		{"Quote_Request_Intake", "Quote Request Intake", "Flow", "Active", 8},
		{"Claim_Submission", "Claim Submission Wizard", "Flow", "Active", 4},
		{"Driver_Checklist", "Driver Daily Checklist", "Flow", "Active", 2},
		{"Warehouse_Audit", "Warehouse Audit Walkthrough", "Flow", "Draft", 1},
		{"Customer_Onboarding", "Customer Onboarding", "Flow", "Active", 9},
		{"Rate_Card_Approval", "Rate Card Approval", "Flow", "Active", 6},
	}
	// Two record-triggered/autolaunched flows per domain — the bulk
	// that makes the flows tab scroll like a real org's.
	domains := []struct{ dev, label string }{
		{"Shipment", "Shipment"}, {"Shipment_Line", "Shipment Line"},
		{"Warehouse", "Warehouse"}, {"Carrier", "Carrier"},
		{"Route", "Route"}, {"Customs_Declaration", "Customs Declaration"},
		{"Inventory_Adjustment", "Inventory Adjustment"}, {"Supplier_Scorecard", "Supplier Scorecard"},
		{"Freight_Invoice", "Freight Invoice"}, {"Dock_Booking", "Dock Booking"},
		{"Container", "Container"}, {"Fleet_Vehicle", "Fleet Vehicle"},
		{"Driver", "Driver"}, {"Claim", "Claim"},
		{"Vessel", "Vessel"}, {"Port_Call", "Port Call"},
		{"Goods_Receipt", "Goods Receipt"}, {"Cold_Chain_Alert", "Cold Chain Alert"},
	}
	suffixes := []struct{ dev, label string }{
		{"After_Save", "Stamp Defaults on Save"},
		{"Status_Sync", "Status Sync"},
		{"Escalation", "Escalation Routing"},
		{"Approval", "Approval Routing"},
	}
	for i, d := range domains {
		for j := 0; j < 2; j++ {
			sfx := suffixes[(i+j)%len(suffixes)]
			status := "Active"
			if (i*2+j)%9 == 8 {
				status = "Draft"
			}
			rows = append(rows, row{
				dev:    d.dev + "_" + sfx.dev,
				label:  d.label + " - " + sfx.label,
				ptype:  "AutoLaunchedFlow",
				status: status,
				ver:    1 + (i*3+j)%11,
			})
		}
	}
	out := make([]sf.Flow, 0, len(rows))
	for i, r := range rows {
		ver := r.ver
		active := ver
		if r.status != "Active" {
			active = 0
		}
		defID := demoFlowDefID(i)
		out = append(out, sf.Flow{
			DefinitionID: defID, DeveloperName: r.dev,
			MasterLabel: r.label, ProcessType: r.ptype, Status: r.status,
			ActiveVersionNum: active, LatestVersionNum: ver, APIVersion: 62,
			// Version-record IDs so `o` can offer the "Flow Builder
			// (active version)" target, not just the Setup page. Latest
			// is always the top version; active is 0 for draft flows.
			ActiveVersionID:  demoFlowVersionID(i, active),
			LatestVersionID:  demoFlowVersionID(i, ver),
			LastModifiedDate: demoStamp(i * 2), LastModifiedBy: demoPerson(i),
		})
	}
	return out
}

// demoFlowDefID is the stable fake FlowDefinition Id for the i-th
// demo flow (0-based). Shared by demoFlows + demoFlowVersionsByDef so
// the flow struct and its version list agree on the key.
func demoFlowDefID(i int) string {
	return fmt.Sprintf("300DM00000DM%03dAAA", i+1)
}

// demoFlowVersionID is the fake FlowVersion (301 prefix) Id for
// version n of the i-th demo flow. Returns "" for n==0 (a draft flow
// with no active version), so an empty ActiveVersionID reads as "no
// active version" the same way a real org would.
func demoFlowVersionID(i, n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("301DM%03d%05dAAA", i+1, n)
}

// demoFlowVersionsByDef builds the per-FlowDefinition version list the
// flow-detail drill reads (cache key "flowversions:<defID>"). Each
// flow gets its full 1..latest version history so drilling into a
// flow shows a populated versions table instead of a demo-mode
// network error. Keyed by DefinitionID to match EnsureFlowVersions.
func demoFlowVersionsByDef() map[string][]sf.FlowVersion {
	flows := demoFlows()
	out := make(map[string][]sf.FlowVersion, len(flows))
	for i, f := range flows {
		latest := f.LatestVersionNum
		versions := make([]sf.FlowVersion, 0, latest)
		// Newest first, matching how the real versions list renders.
		for n := latest; n >= 1; n-- {
			status := "Obsolete"
			switch {
			case n == f.ActiveVersionNum:
				status = "Active"
			case n == latest && f.ActiveVersionNum != latest:
				// Top version that isn't active = the working draft.
				status = "Draft"
			}
			versions = append(versions, sf.FlowVersion{
				ID:               demoFlowVersionID(i, n),
				DefinitionID:     f.DefinitionID,
				VersionNumber:    n,
				MasterLabel:      f.MasterLabel,
				ProcessType:      f.ProcessType,
				APIVersion:       f.APIVersion,
				Status:           status,
				CreatedDate:      demoStamp(i*2 + (latest - n)),
				CreatedBy:        demoPerson(i),
				LastModifiedDate: demoStamp(i*2 + (latest - n)),
				LastModifiedBy:   demoPerson(i),
			})
		}
		out[f.DefinitionID] = versions
	}
	return out
}

// ---------------------------------------------------------------
// Apex classes
// ---------------------------------------------------------------

func demoApexClasses() []sf.ApexClassRow {
	domains := []string{
		"Shipment", "Carrier", "Route", "Customs", "FreightInvoice",
		"Warehouse", "DockBooking", "Inventory", "Supplier", "Container",
		"Vessel", "Tariff", "Claim", "Driver", "Fleet",
	}
	roles := []string{"TriggerHandler", "Service", "Selector", "ServiceTest"}
	out := make([]sf.ApexClassRow, 0, len(domains)*len(roles)+len(demoAsyncApexNames))
	i := 0
	for _, d := range domains {
		for _, r := range roles {
			i++
			out = append(out, sf.ApexClassRow{
				ID: fmt.Sprintf("01pDM00000DM%03dAAA", i), Name: d + r, Status: "Active",
				IsValid: true, ApiVersion: 62, LengthNoComments: 600 + (i*257)%4200,
				LastModifiedDate: demoStamp(i * 3), LastModifiedByName: demoPerson(i),
			})
		}
	}
	// Async workers (batch + queueable), appended AFTER the domain grid
	// so the grid's index-derived Ids stay stable. The async-job and
	// scheduled-job fixtures reference these by Id, so the Jobs surfaces
	// drill onto real seeded classes.
	for _, n := range demoAsyncApexNames {
		i++
		out = append(out, sf.ApexClassRow{
			ID: fmt.Sprintf("01pDM00000DM%03dAAA", i), Name: n, Status: "Active",
			IsValid: true, ApiVersion: 62, LengthNoComments: 600 + (i*257)%4200,
			LastModifiedDate: demoStamp(i * 3), LastModifiedByName: demoPerson(i),
		})
	}
	return out
}

// demoAsyncApexNames is the batch/queueable roster shared by
// demoApexClasses (the rows), demoAsyncJobs (the executions), and
// demoScheduledJobs (the crons).
var demoAsyncApexNames = []string{
	"SupplierScorecardBatch", "ShipmentEtaRecalcBatch", "NightlyKpiSnapshotBatch",
	"DemurrageAccrualBatch", "RateCardSyncQueueable", "ColdChainAlertQueueable",
}

// ---------------------------------------------------------------
// Deploys
// ---------------------------------------------------------------

func demoDeploys() []sf.DeployRow {
	now := time.Now()
	ts := func(minsAgo int) string {
		return now.Add(-time.Duration(minsAgo) * time.Minute).UTC().Format("2006-01-02T15:04:05.000+0000")
	}
	rows := []sf.DeployRow{
		// StartDate = seed time (not minutes ago) — the watch flip
		// fires 12s after StartDate, and the tape needs to catch the
		// InProgress state on screen before it completes.
		{ID: "0AfDM00000DEMO01", Status: "InProgress", CreatedByName: "Mara Chen", CreatedDate: ts(0),
			StartDate: ts(0), Type: "Api", TestLevel: "RunLocalTests",
			ComponentsDeployed: 14, ComponentsTotal: 23, TestsTotal: 41, TestsCompleted: 12},
		{ID: "0AfDM00000DEMO02", Status: "Succeeded", CreatedByName: "Priya Patel", CreatedDate: ts(95),
			StartDate: ts(95), CompletedDate: ts(93), Type: "Api",
			ComponentsDeployed: 8, ComponentsTotal: 8, TestsTotal: 17, TestsCompleted: 17},
		{ID: "0AfDM00000DEMO03", Status: "Failed", CreatedByName: "Jonah Okafor", CreatedDate: ts(240),
			StartDate: ts(240), CompletedDate: ts(238), Type: "Api", ErrorMessage: "ShipmentServiceTest.testBulkInsert assertion failed",
			ComponentsDeployed: 11, ComponentsTotal: 12, ComponentErrors: 1, TestsTotal: 41, TestsCompleted: 30, TestErrors: 1},
		{ID: "0AfDM00000DEMO04", Status: "Succeeded", CheckOnly: true, CreatedByName: "Mara Chen", CreatedDate: ts(300),
			StartDate: ts(300), CompletedDate: ts(297), Type: "Api",
			ComponentsDeployed: 23, ComponentsTotal: 23, TestsTotal: 41, TestsCompleted: 41},
		{ID: "0AfDM00000DEMO05", Status: "Succeeded", CreatedByName: "Jonah Okafor", CreatedDate: ts(1440),
			StartDate: ts(1440), CompletedDate: ts(1437), Type: "Api",
			ComponentsDeployed: 3, ComponentsTotal: 3},
		{ID: "0AfDM00000DEMO06", Status: "Canceled", CreatedByName: "Priya Patel", CreatedDate: ts(2880),
			StartDate: ts(2880), CompletedDate: ts(2879), CanceledByName: "Priya Patel", Type: "Api",
			ComponentsDeployed: 0, ComponentsTotal: 19},
	}
	// Fill the rest of the 25-row deploys window with history spread
	// over the prior two weeks: mostly green, the occasional failure
	// and validation-only run.
	for i := 0; i < 19; i++ {
		minsAgo := 3000 + i*960 + (i*131)%240
		comps := 2 + (i*11)%34
		tests := (i * 13) % 60
		r := sf.DeployRow{
			ID:        fmt.Sprintf("0AfDM00000DM%03dAAA", i+1),
			Status:    "Succeeded",
			CheckOnly: i%4 == 3,
			Type:      "Api", CreatedByName: demoPerson(i),
			CreatedDate: ts(minsAgo), StartDate: ts(minsAgo), CompletedDate: ts(minsAgo - 2 - i%5),
			ComponentsDeployed: comps, ComponentsTotal: comps,
			TestsTotal: tests, TestsCompleted: tests,
		}
		if i%7 == 5 {
			r.Status = "Failed"
			r.ComponentErrors = 1
			r.ComponentsDeployed = comps - 1
			r.ErrorMessage = "Required field missing on " + []string{"Shipment__c", "Carrier__c", "Route__c"}[i%3]
		}
		rows = append(rows, r)
	}
	return rows
}

// ---------------------------------------------------------------
// Records
// ---------------------------------------------------------------

// demoRecordObjects fixes the iteration order for demoRecordHits
// (maps don't have one).
var demoRecordObjects = []string{
	"Account", "Contact", "Opportunity", "Case",
	"Shipment__c", "Carrier__c", "Warehouse__c",
}

// demoRecordLists builds every seeded records payload, memoized —
// the seeder reads it once per org and demoRecordHits scans it per
// ctrl+k keystroke.
var demoRecordLists = sync.OnceValue(func() map[string]sf.RecordsList {
	lists := map[string]sf.RecordsList{
		"Account":      demoAccountRecords(),
		"Contact":      demoContactRecords(),
		"Opportunity":  demoOpportunityRecords(),
		"Case":         demoCaseRecords(),
		"Shipment__c":  demoShipmentRecords(),
		"Carrier__c":   demoCarrierRecords(),
		"Warehouse__c": demoWarehouseRecords(),
	}
	// Backfill CreatedDate on every row + column set so the "Recently
	// created" chip has a populated, sortable date column in demo mode.
	// Created is derived to sit a little BEFORE LastModifiedDate (a
	// record is modified after it's created), keeping the two columns
	// visually distinct.
	for name, rl := range lists {
		for i, rec := range rl.Records {
			if _, has := rec["CreatedDate"]; !has {
				rec["CreatedDate"] = demoCreatedFromModified(rec["LastModifiedDate"])
			}
			// Audit person columns render from nested JSON
			// (rec["CreatedBy"]["Name"]); seed both so the columns
			// aren't em-dashes in demo mode.
			if _, has := rec["CreatedBy"]; !has {
				rec["CreatedBy"] = map[string]any{"Name": demoPerson(i)}
			}
			if _, has := rec["LastModifiedBy"]; !has {
				rec["LastModifiedBy"] = map[string]any{"Name": demoPerson(i + 2)}
			}
		}
		rl.Columns = withAuditColumns(rl.Columns)
		lists[name] = rl
	}
	return lists
})

// demoCreatedFromModified returns a CreatedDate string a few days
// before the given LastModifiedDate value. Falls back to a generic
// old stamp when the input isn't a parseable timestamp.
func demoCreatedFromModified(modified any) string {
	s, _ := modified.(string)
	if t, err := time.Parse("2006-01-02T15:04:05.000-0700", s); err == nil {
		return t.Add(-time.Duration(5+len(s)%20) * 24 * time.Hour).
			Format("2006-01-02T15:04:05.000-0700")
	}
	return demoStamp(60)
}

// withAuditColumns ensures the demo record column set carries the
// standard audit columns in the same order the live path projects
// them: CreatedDate · CreatedBy.Name before LastModifiedDate, and
// LastModifiedBy.Name after it. Existing non-audit columns keep their
// positions; already-present audit columns aren't duplicated.
func withAuditColumns(cols []string) []string {
	have := map[string]bool{}
	for _, c := range cols {
		have[c] = true
	}
	out := make([]string, 0, len(cols)+4)
	for _, c := range cols {
		if c == "LastModifiedDate" {
			if !have["CreatedDate"] {
				out = append(out, "CreatedDate")
				have["CreatedDate"] = true
			}
			if !have["CreatedBy.Name"] {
				out = append(out, "CreatedBy.Name")
				have["CreatedBy.Name"] = true
			}
			out = append(out, c)
			if !have["LastModifiedBy.Name"] {
				out = append(out, "LastModifiedBy.Name")
				have["LastModifiedBy.Name"] = true
			}
			continue
		}
		out = append(out, c)
	}
	// Object with no LastModifiedDate column: append the created pair
	// at the end so "Recently created" still has its sort axis.
	if !have["CreatedDate"] {
		out = append(out, "CreatedDate")
	}
	if !have["CreatedBy.Name"] {
		out = append(out, "CreatedBy.Name")
	}
	return out
}

// ---------------------------------------------------------------
// Visit log + recently-viewed
// ---------------------------------------------------------------

// demoVisitedAccountIdx / demoVisitedShipmentIdx pin which seeded
// records the demo user has "visited". The visit log, the visited
// chip-records seeds, and the SF recently-viewed payload all derive
// from these so the three views of recency agree.
var (
	demoVisitedAccountIdx  = []int{1, 4, 7}
	demoVisitedShipmentIdx = []int{0, 3}
)

// demoRecentVisits is the local visit log persisted into the
// (ephemeral) demo settings. Every "Recently viewed" chip — objects,
// flows, apex, per-sObject records — reads from this, so without it
// each surface lands on an empty default chip and the demo looks
// dead on arrival.
func demoRecentVisits() []settings.RecentConfig {
	now := time.Now()
	at := func(minsAgo int) time.Time { return now.Add(-time.Duration(minsAgo) * time.Minute) }
	str := func(rec map[string]any, k string) string { s, _ := rec[k].(string); return s }
	acct := demoRecordLists()["Account"].Records
	ship := demoRecordLists()["Shipment__c"].Records
	out := []settings.RecentConfig{
		{Kind: "record", ID: str(acct[demoVisitedAccountIdx[0]], "Id"), Name: str(acct[demoVisitedAccountIdx[0]], "Name"), Type: "Account", VisitedAt: at(6)},
		{Kind: "sobject", ID: "Shipment__c", Name: "Shipment", VisitedAt: at(12)},
		{Kind: "record", ID: str(ship[demoVisitedShipmentIdx[0]], "Id"), Name: str(ship[demoVisitedShipmentIdx[0]], "Name"), Type: "Shipment__c", VisitedAt: at(19)},
		{Kind: "flow", ID: "300DM00000DM001AAA", Name: "Carrier Onboarding", VisitedAt: at(27)},
		{Kind: "sobject", ID: "Account", Name: "Account", VisitedAt: at(34)},
		{Kind: "apex_class", ID: "01pDM00000DM001AAA", Name: "ShipmentTriggerHandler", VisitedAt: at(48)},
		{Kind: "record", ID: str(acct[demoVisitedAccountIdx[1]], "Id"), Name: str(acct[demoVisitedAccountIdx[1]], "Name"), Type: "Account", VisitedAt: at(65)},
		{Kind: "sobject", ID: "Carrier__c", Name: "Carrier", VisitedAt: at(80)},
		{Kind: "record", ID: str(ship[demoVisitedShipmentIdx[1]], "Id"), Name: str(ship[demoVisitedShipmentIdx[1]], "Name"), Type: "Shipment__c", VisitedAt: at(95)},
		{Kind: "flow", ID: "300DM00000DM002AAA", Name: "Dock Booking Wizard", VisitedAt: at(110)},
		{Kind: "sobject", ID: "Freight_Invoice__c", Name: "Freight Invoice", VisitedAt: at(130)},
		{Kind: "apex_class", ID: "01pDM00000DM002AAA", Name: "ShipmentService", VisitedAt: at(150)},
		{Kind: "record", ID: str(acct[demoVisitedAccountIdx[2]], "Id"), Name: str(acct[demoVisitedAccountIdx[2]], "Name"), Type: "Account", VisitedAt: at(170)},
		{Kind: "sobject", ID: "Contact", Name: "Contact", VisitedAt: at(190)},
		{Kind: "sobject", ID: "Warehouse__c", Name: "Warehouse", VisitedAt: at(220)},
	}
	return out
}

// demoVisitedChipRecords builds the per-sObject Visited chip
// payloads (chiprecords:<obj>:__visited__) — the records the visit
// log says were opened, served through the same chip-records
// resource the live path uses (SOQL `Id IN (visited)`).
func demoVisitedChipRecords() map[string]sf.RecordsList {
	pick := func(list sf.RecordsList, idx []int) sf.RecordsList {
		out := list
		out.Records = nil
		for _, i := range idx {
			out.Records = append(out.Records, list.Records[i])
		}
		out.TotalSize = len(out.Records)
		return out
	}
	return map[string]sf.RecordsList{
		"Account":     pick(demoRecordLists()["Account"], demoVisitedAccountIdx),
		"Shipment__c": pick(demoRecordLists()["Shipment__c"], demoVisitedShipmentIdx),
	}
}

// demoRecentlyViewed mirrors the record entries of the visit log as
// the org-side RecentlyViewed payload, so the SF-mode recent chips
// agree with the local log.
func demoRecentlyViewed() []sf.RecentlyViewedRow {
	var out []sf.RecentlyViewedRow
	for _, v := range demoRecentVisits() {
		if v.Kind != "record" {
			continue
		}
		out = append(out, sf.RecentlyViewedRow{
			ID: v.ID, Name: v.Name, SObjectType: v.Type, LastViewedDate: v.VisitedAt,
		})
	}
	return out
}

// demoListViews backs listviews:<obj> for the record objects so the
// records strip's Salesforce-mode catalog loads without a fetch
// error. IDs are per-object so org-hopping never collides.
func demoListViews(sobject string, ord int) []sf.ListView {
	label := strings.TrimSuffix(sobject, "__c")
	label = strings.ReplaceAll(label, "_", " ")
	id := func(n int) string { return fmt.Sprintf("00BDM00000DM%02d%dAAA", ord, n) }
	return []sf.ListView{
		{ID: id(1), Name: "All " + label + "s", DeveloperName: "All" + strings.ReplaceAll(label, " ", ""), SobjectType: sobject, IsSoqlCompatible: true},
		{ID: id(2), Name: "My " + label + "s", DeveloperName: "My" + strings.ReplaceAll(label, " ", ""), SobjectType: sobject, IsSoqlCompatible: true},
		{ID: id(3), Name: "Recently Modified", DeveloperName: "RecentlyModified" + strings.ReplaceAll(label, " ", ""), SobjectType: sobject, IsSoqlCompatible: true},
	}
}

// ---------------------------------------------------------------
// Triggers + notifications
// ---------------------------------------------------------------

func demoTriggers() []sf.TriggerRow {
	rows := []struct{ name, table, events string }{
		{"ShipmentTrigger", "Shipment__c", "before insert, after update"},
		{"ShipmentLineTrigger", "Shipment_Line__c", "before insert, before update"},
		{"CarrierTrigger", "Carrier__c", "after insert, after update"},
		{"RouteTrigger", "Route__c", "before update"},
		{"CustomsDeclarationTrigger", "Customs_Declaration__c", "after insert"},
		{"FreightInvoiceTrigger", "Freight_Invoice__c", "before insert, after update"},
		{"DockBookingTrigger", "Dock_Booking__c", "before insert"},
		{"ClaimTrigger", "Claim__c", "after insert, after update"},
		{"ColdChainAlertTrigger", "Cold_Chain_Alert__c", "after insert"},
		{"GoodsReceiptTrigger", "Goods_Receipt__c", "before insert, before update"},
		{"AccountTrigger", "Account", "before update"},
		{"ContactTrigger", "Contact", "before insert, before update"},
		{"CaseTrigger", "Case", "after insert"},
		{"OpportunityTrigger", "Opportunity", "before update, after update"},
		{"LeadTrigger", "Lead", "before insert"},
	}
	out := make([]sf.TriggerRow, 0, len(rows))
	for i, r := range rows {
		status := "Active"
		if i == 8 {
			status = "Inactive"
		}
		out = append(out, sf.TriggerRow{
			ID: fmt.Sprintf("01qDM00000DM%03dAAA", i+1), Name: r.name, Table: r.table,
			Status: status, Events: r.events, Valid: true,
			Len: 200 + (i*173)%1800, ApiVer: 62,
			LastModifiedDate: demoStamp(i * 5), LastModifiedByName: demoPerson(i + 2),
		})
	}
	return out
}

// demoApexLogs fills the System tab's Logs subtab (its default
// landing) so arriving on `7` doesn't flash a demo-mode fetch error.
func demoApexLogs() []sf.ApexLogRow {
	ops := []struct{ op, status string }{
		{"/services/data/v62.0/sobjects/Shipment__c", "Success"},
		{"ShipmentTriggerHandler", "Success"},
		{"@future ShipmentService.recalcRoutes", "Success"},
		{"FreightInvoiceMatcher", "CompileFail"},
		{"/apexrest/carrier/rates", "Success"},
		{"Batch SupplierScorecardBatch", "Success"},
		{"VF: DockBookingController", "Success"},
		{"ShipmentServiceTest", "Success"},
		{"/services/data/v62.0/query", "Success"},
		{"Carrier rate sync (queueable)", "Success"},
		{"ColdChainAlertTrigger", "Success"},
		{"Anonymous Apex", "Success"},
	}
	now := time.Now().UTC()
	out := make([]sf.ApexLogRow, 0, len(ops))
	for i, o := range ops {
		r := sf.ApexLogRow{
			ID: fmt.Sprintf("07LDM00000DM%03dAAA", i+1), Application: "Unknown",
			DurationMs: 40 + (i*271)%4200, LogLength: 800 + (i*913)%52000,
			Operation: o.op, Status: o.status,
			StartTime: now.Add(-time.Duration(4+i*23) * time.Minute).Format("2006-01-02T15:04:05.000+0000"),
		}
		r.LogUser.Name = demoPerson(i)
		out = append(out, r)
	}
	return out
}

func demoNotifications() sf.NotificationsList {
	now := time.Now().UTC()
	at := func(hoursAgo int) string {
		return now.Add(-time.Duration(hoursAgo) * time.Hour).Format("2006-01-02T15:04:05.000Z")
	}
	n := []sf.Notification{
		{ID: "demo-notif-1", Type: "approval_request", MessageTitle: "Priya Patel requested approval",
			MessageBody: "Rate Card 2026 - Baltic lanes awaits your approval.", Read: false, Seen: false, LastModified: at(2)},
		{ID: "demo-notif-2", Type: "task_mention", MessageTitle: "Mara Chen mentioned you",
			MessageBody: "@you can you check the customs hold on SHP-1177?", Read: false, Seen: true, LastModified: at(7)},
		{ID: "demo-notif-3", Type: "task_assigned", MessageTitle: "New task assigned",
			MessageBody: "Review Q3 carrier scorecards before Friday.", Read: true, Seen: true, LastModified: at(26)},
		{ID: "demo-notif-4", Type: "share", MessageTitle: "Jonah Okafor shared a report",
			MessageBody: "On-time delivery by region - May", Read: true, Seen: true, LastModified: at(50)},
	}
	return sf.NotificationsList{Notifications: n, UnreadCount: 2}
}

// demoChipSlices carves a record fixture into per-chip subsets for
// the built-in records chips ("recent" is the Changed chip — full
// list; the rest are plausible filters of it). IDs must match
// qchip.RecordBuiltins.
func demoChipSlices(list sf.RecordsList) map[string]sf.RecordsList {
	slice := func(keep func(i int) bool) sf.RecordsList {
		out := list
		out.Records = nil
		for i, r := range list.Records {
			if keep(i) {
				out.Records = append(out.Records, r)
			}
		}
		out.TotalSize = len(out.Records)
		return out
	}
	return map[string]sf.RecordsList{
		"recent":      list,
		"today":       slice(func(i int) bool { return i < 6 }),
		"this-week":   slice(func(i int) bool { return i < 20 }),
		"mine":        slice(func(i int) bool { return i%3 == 0 }),
		"mine-recent": slice(func(i int) bool { return i%3 == 0 && i < 24 }),
	}
}

var demoAccountNames = func() []string {
	curated := []string{
		"Acme Freight Co", "Globex Logistics", "Initech Supply", "Umbrella Imports",
		"Stark Distribution", "Wayne Shipping Group", "Tyrell Cargo", "Wonka Confections",
		"Soylent Foods", "Cyberdyne Components", "Oscorp Materials", "Gringotts Vaults Ltd",
		"Duff Beverages", "Sterling Cooper Goods", "Dunder Mifflin Paper", "Pied Piper Parcel",
		"Hooli Hardware", "Aviato Air Cargo", "Bluth Produce", "Vandelay Industries",
		"Kruger Industrial", "Prestige Worldwide", "Genco Olive Oil", "Sabre Office Systems",
		"Octan Fuels", "Monarch Textiles", "Zenith Electronics", "Apex Fasteners",
		"Meridian Foods", "Pinnacle Plastics", "Cascade Timber", "Summit Steelworks",
		"Harbor Marine Supply", "Beacon Lighting Co", "Atlas Machinery", "Orion Optics",
		"Vega Instruments", "Polaris Outdoor", "Crescent Bakery Supply", "Solstice Apparel",
		"Equinox Fitness Gear", "Aurora Chemicals", "Borealis Packaging", "Zephyr Cooling",
		"Mistral Fans Ltd", "Sirocco Heating", "Tempest Tools", "Cyclone Cleaning Co",
		"Monsoon Irrigation", "Tundra Refrigeration", "Savanna Seeds", "Prairie Grain Co",
		"Delta Valves", "Gamma Imaging", "Omega Bearings", "Sigma Sensors",
		"Lambda Lighting", "Kappa Ceramics", "Theta Therapeutics", "Epsilon Foods",
	}
	prefixes := []string{
		"Northgate", "Bluewater", "Ironwood", "Stonebridge", "Fairhaven",
		"Westbrook", "Silverline", "Oakfield", "Redmoor", "Greystone",
		"Eastvale", "Longford", "Hartwell", "Brackenridge", "Millbrook",
	}
	suffixes := []string{"Logistics", "Freight", "Holdings", "Trading"}
	out := curated
	for i := 0; len(out) < 120; i++ {
		out = append(out, prefixes[i%len(prefixes)]+" "+suffixes[(i/len(prefixes))%len(suffixes)])
	}
	return out
}()

func demoAccountRecords() sf.RecordsList {
	industries := []string{"Transportation", "Manufacturing", "Retail", "Food & Beverage", "Technology", "Energy"}
	cities := []string{"Rotterdam", "Hamburg", "Leeds", "Antwerp", "Lyon", "Gdansk", "Bilbao", "Aarhus"}
	recs := make([]map[string]any, 0, len(demoAccountNames))
	for i, n := range demoAccountNames {
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("001DM00000DM%04dAAA", i+1),
			"Name":             n,
			"Industry":         industries[i%len(industries)],
			"BillingCity":      cities[i%len(cities)],
			"LastModifiedDate": demoStamp(i),
		})
	}
	return sf.RecordsList{
		SObject: "Account", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, Industry, BillingCity, LastModifiedDate FROM Account ORDER BY LastModifiedDate DESC",
		Columns: []string{"Id", "Name", "Industry", "BillingCity", "LastModifiedDate"},
	}
}

func demoContactRecords() sf.RecordsList {
	first := []string{
		"Alice", "Bram", "Carmen", "Dmitri", "Esther", "Farid", "Greta", "Henrik",
		"Imogen", "Jasper", "Katya", "Lorenzo", "Maeve", "Nikolai", "Oona", "Pavel",
		"Quinn", "Rosa", "Stellan", "Tova",
	}
	last := []string{
		"Vermeer", "Lindqvist", "Moreau", "Kowalski", "Berg", "Castellano", "Novak", "Eriksen",
		"Dubois", "Schmidt", "Petrov", "Janssen", "Olsen", "Ferreira", "Weber", "Andersen",
		"Visser", "Marchetti", "Holm", "Bakker",
	}
	titles := []string{
		"Logistics Manager", "Procurement Lead", "Operations Director", "Warehouse Supervisor",
		"Supply Chain Analyst", "Customs Coordinator", "Fleet Manager", "Head of Distribution",
	}
	recs := make([]map[string]any, 0, 100)
	for i := 0; i < 100; i++ {
		f, l := first[i%len(first)], last[(i*7+i/20)%len(last)]
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("003DM00000DM%04dAAA", i+1),
			"Name":             f + " " + l,
			"Title":            titles[i%len(titles)],
			"Email":            fmt.Sprintf("%s.%s@example.com", strings.ToLower(f), strings.ToLower(l)),
			"LastModifiedDate": demoStamp(i + 3),
		})
	}
	return sf.RecordsList{
		SObject: "Contact", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, Title, Email, LastModifiedDate FROM Contact ORDER BY LastModifiedDate DESC",
		Columns: []string{"Id", "Name", "Title", "Email", "LastModifiedDate"},
	}
}

func demoOpportunityRecords() sf.RecordsList {
	stages := []string{"Prospecting", "Qualification", "Proposal", "Negotiation", "Closed Won", "Closed Lost"}
	kinds := []string{"Renewal", "Expansion", "Pilot", "New Lane"}
	recs := make([]map[string]any, 0, 80)
	for i := 0; i < 80; i++ {
		acct := demoAccountNames[(i*3)%len(demoAccountNames)]
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("006DM00000DM%04dAAA", i+1),
			"Name":             fmt.Sprintf("%s - %s FY26", acct, kinds[i%len(kinds)]),
			"StageName":        stages[(i*5)%len(stages)],
			"Amount":           5000 + (i*3571)%90000,
			"LastModifiedDate": demoStamp(i + 5),
		})
	}
	return sf.RecordsList{
		SObject: "Opportunity", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, StageName, Amount, LastModifiedDate FROM Opportunity ORDER BY LastModifiedDate DESC",
		Columns: []string{"Id", "Name", "StageName", "Amount", "LastModifiedDate"},
	}
}

func demoCaseRecords() sf.RecordsList {
	subjects := []string{
		"Shipment arrived short two pallets", "POD missing for delivery",
		"Temperature excursion alert on reefer", "Customs hold - missing tariff code",
		"Damaged goods on receipt", "Late delivery - penalty clause query",
		"Carrier invoice does not match rate card", "Dock booking double-allocated",
		"Wrong incoterms on confirmation", "Container demurrage dispute",
	}
	statuses := []string{"New", "Working", "Escalated", "Closed"}
	priorities := []string{"Low", "Medium", "High"}
	recs := make([]map[string]any, 0, 60)
	for i := 0; i < 60; i++ {
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("500DM00000DM%04dAAA", i+1),
			"Name":             fmt.Sprintf("%08d", 1041+i),
			"Subject":          subjects[i%len(subjects)],
			"Status":           statuses[(i*3)%len(statuses)],
			"Priority":         priorities[(i*7)%len(priorities)],
			"LastModifiedDate": demoStamp(i + 2),
		})
	}
	return sf.RecordsList{
		SObject: "Case", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, CaseNumber, Subject, Status, Priority, LastModifiedDate FROM Case ORDER BY LastModifiedDate DESC",
		Columns: []string{"Id", "Name", "Subject", "Status", "Priority", "LastModifiedDate"},
	}
}

func demoShipmentRecords() sf.RecordsList {
	statuses := []string{"Draft", "Booked", "In Transit", "At Customs", "Delivered"}
	recs := make([]map[string]any, 0, 120)
	for i := 0; i < 120; i++ {
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("a0ADM00000DM%04dAAA", i+1),
			"Name":             fmt.Sprintf("SHP-%04d", 1180-i),
			"Status__c":        statuses[(i*3)%len(statuses)],
			"LastModifiedDate": demoStamp(i),
		})
	}
	return sf.RecordsList{
		SObject: "Shipment__c", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, Status__c, LastModifiedDate FROM Shipment__c ORDER BY LastModifiedDate DESC",
		Columns: []string{"Id", "Name", "Status__c", "LastModifiedDate"},
	}
}

func demoCarrierRecords() sf.RecordsList {
	names := []string{
		"Albatross Air Freight", "Baltic Bridge Lines", "Cobalt Couriers", "Dover Strait Shipping",
		"Evergreen Overland", "Falcon Express EU", "Gullwing Cargo", "Hanseatic Haulage",
		"Ibex Mountain Transport", "Juniper Road Freight", "Kestrel Logistics", "Lighthouse Lines",
		"Magpie Parcel Network", "Nordkapp Shipping", "Osprey Ocean Freight", "Puffin Short Sea",
		"Quayside Carriers", "Reindeer Express", "Stork Air Cargo", "Tern Coastal Lines",
		"Urchin Last Mile", "Vulcan Heavy Haul", "Wren City Couriers", "Yellowfin Reefer Lines",
		"Zebra Crossing Freight",
	}
	recs := make([]map[string]any, 0, len(names))
	for i, n := range names {
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("a0DDM00000DM%04dAAA", i+1),
			"Name":             n,
			"Active__c":        i%5 != 4,
			"LastModifiedDate": demoStamp(i + 9),
		})
	}
	return sf.RecordsList{
		SObject: "Carrier__c", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, Active__c, LastModifiedDate FROM Carrier__c ORDER BY Name",
		Columns: []string{"Id", "Name", "Active__c", "LastModifiedDate"},
	}
}

func demoWarehouseRecords() sf.RecordsList {
	sites := []string{
		"Rotterdam Central DC", "Hamburg Port Annex", "Leeds North Hub", "Antwerp Cold Store",
		"Lyon Crossdock", "Gdansk Container Yard", "Bilbao Overflow", "Aarhus Reefer Depot",
		"Lille Returns Centre", "Bremen Bonded Store", "Sheffield Parts Hub", "Valencia Citrus DC",
		"Turin Transit Shed", "Porto Atlantic Gate", "Gothenburg Rail Head", "Dublin Air Cargo Shed",
		"Marseille Quay 7", "Stettin River Depot",
	}
	recs := make([]map[string]any, 0, len(sites))
	for i, n := range sites {
		recs = append(recs, map[string]any{
			"Id":               fmt.Sprintf("a0CDM00000DM%04dAAA", i+1),
			"Name":             n,
			"Region__c":        []string{"North", "South", "East", "West", "Central"}[i%5],
			"LastModifiedDate": demoStamp(i + 11),
		})
	}
	return sf.RecordsList{
		SObject: "Warehouse__c", HasName: true, HasModDate: true,
		Records: recs, TotalSize: len(recs), Done: true,
		Query:   "SELECT Id, Name, Region__c, LastModifiedDate FROM Warehouse__c ORDER BY Name",
		Columns: []string{"Id", "Name", "Region__c", "LastModifiedDate"},
	}
}
