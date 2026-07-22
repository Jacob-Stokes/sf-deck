package ui

// Fictional Northwind DevProject / bundle / tag / saved-query
// fixtures. Companion to demo_seed_data.go (which holds cache.db
// fixtures); this file owns devprojects.db payloads.
//
// Three projects, two bundles, six tags, five saved queries, four
// apex snippets, and a small SOQL history log. Everything keys off
// the same Northwind universe (demoDev / demoUAT / demoProd) so
// org-hopping in the TUI stays consistent across DBs.

import (
	"path/filepath"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

// demoDevProjects is the project list. IDs are short, deterministic
// strings so the demo replays identically across runs — same row
// order, same tag bindings, same bundle linkage.
func demoDevProjects(now time.Time) []devproject.DevProject {
	return []devproject.DevProject{
		{
			ID:          "dp_shipment_revamp",
			Name:        "Shipment status revamp",
			Description: "Re-do the Shipment__c status picklist + the validation rules that pin it.",
			CreatedAt:   now.AddDate(0, 0, -12),
			TouchedAt:   now.AddDate(0, 0, -1),
		},
		{
			ID:          "dp_carrier_consolidation",
			Name:        "Carrier consolidation",
			Description: "Merge the legacy Carrier__c flows + apex into one rules engine before EOY.",
			CreatedAt:   now.AddDate(0, 0, -28),
			TouchedAt:   now.AddDate(0, 0, -3),
		},
		{
			ID:          "dp_q3_cleanup",
			Name:        "Q3 cleanup",
			Description: "Inactive flows, abandoned chips, dead apex. Ship before the freeze.",
			CreatedAt:   now.AddDate(0, 0, -5),
			TouchedAt:   now.AddDate(0, 0, -1),
		},
	}
}

// demoDevProjectItems is the flat list of (project, item) rows. Each
// project gets a mix of kinds so the kind-filter chip strip on the
// detail view has multiple buckets to filter.
func demoDevProjectItems() []devproject.Item {
	devUser := demoDev
	now := time.Now()
	t := func(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }
	return []devproject.Item{
		// dp_shipment_revamp: 1 sObject + 4 fields + 2 flows + 1 validation rule + 1 apex
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindSObject,
			Ref: "Shipment__c", Name: "Shipment", AddedAt: t(12)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Shipment__c.Status__c", Type: "Shipment__c", Name: "Status", AddedAt: t(11)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Shipment__c.Carrier__c", Type: "Shipment__c", Name: "Carrier", AddedAt: t(11)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Shipment__c.Tracking_Number__c", Type: "Shipment__c", Name: "Tracking Number", AddedAt: t(10)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Shipment__c.Delivered_At__c", Type: "Shipment__c", Name: "Delivered At", AddedAt: t(10)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Shipment_Status_Change", Name: "Shipment Status Change", AddedAt: t(8)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Shipment_Delivered_Notify", Name: "Shipment Delivered Notify", AddedAt: t(7)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindValidationRule,
			Ref: "Shipment_Status_Transitions", Type: "Shipment__c", Name: "Status Transitions", AddedAt: t(6)},
		{DevProjectID: "dp_shipment_revamp", OrgUser: devUser, Kind: devproject.KindApexClass,
			Ref: "ShipmentStatusHelper", Name: "ShipmentStatusHelper", AddedAt: t(4)},

		// dp_carrier_consolidation: heavier — Carrier sObject + fields + 3 flows + 2 apex + 1 LWC + 1 trigger
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindSObject,
			Ref: "Carrier__c", Name: "Carrier", AddedAt: t(28)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Carrier__c.Active__c", Type: "Carrier__c", Name: "Active", AddedAt: t(27)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Carrier__c.Service_Level__c", Type: "Carrier__c", Name: "Service Level", AddedAt: t(27)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Carrier_Activation", Name: "Carrier Activation", AddedAt: t(22)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Carrier_Rate_Update", Name: "Carrier Rate Update", AddedAt: t(20)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Carrier_Holiday_Pause", Name: "Carrier Holiday Pause", AddedAt: t(18)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindApexClass,
			Ref: "CarrierRoutingService", Name: "CarrierRoutingService", AddedAt: t(15)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindApexClass,
			Ref: "CarrierRoutingServiceTest", Name: "CarrierRoutingServiceTest", AddedAt: t(15)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindLWC,
			Ref: "carrierPickerLWC", Type: "carrierPickerLWC", Name: "Carrier Picker", AddedAt: t(10)},
		{DevProjectID: "dp_carrier_consolidation", OrgUser: devUser, Kind: devproject.KindApexTrigger,
			Ref: "CarrierBeforeUpdate", Type: "Carrier__c", Name: "CarrierBeforeUpdate", AddedAt: t(8)},

		// dp_q3_cleanup: mixed bag — a flow, a perm set, a couple of fields, a saved query (org-agnostic)
		{DevProjectID: "dp_q3_cleanup", OrgUser: devUser, Kind: devproject.KindFlow,
			Ref: "Legacy_Order_Approval", Name: "Legacy Order Approval", AddedAt: t(5)},
		{DevProjectID: "dp_q3_cleanup", OrgUser: devUser, Kind: devproject.KindPermissionSet,
			Ref: "Shipment_Admin", Name: "Shipment Admin", AddedAt: t(4)},
		{DevProjectID: "dp_q3_cleanup", OrgUser: devUser, Kind: devproject.KindField,
			Ref: "Account.Legacy_Region__c", Type: "Account", Name: "Legacy Region", AddedAt: t(3)},
		{DevProjectID: "dp_q3_cleanup", OrgUser: devUser, Kind: devproject.KindApexClass,
			Ref: "DeprecatedReportBuilder", Name: "DeprecatedReportBuilder", AddedAt: t(2)},
	}
}

// demoBundleSeed is a thin shape — we don't want to depend on the
// real Bundle struct here because it has private fields. The seeder
// translates to/from this.
type demoBundleSeed struct {
	DevProjectID    string
	Path            string
	DefaultOrgAlias string
	LastRetrievedAt time.Time
	LastDeployedAt  time.Time
}

// demoDevBundles is two bundles: one fresh + retrieved, one
// deployed-yesterday. Both link to dp_shipment_revamp and
// dp_carrier_consolidation respectively. Paths sit under the
// supplied demo bundle root.
func demoDevBundles(root string) []demoBundleSeed {
	now := time.Now()
	return []demoBundleSeed{
		{
			DevProjectID:    "dp_shipment_revamp",
			Path:            filepath.Join(root, "shipment-revamp-1719504000"),
			DefaultOrgAlias: "northwind-dev",
			LastRetrievedAt: now.Add(-2 * time.Hour),
		},
		{
			DevProjectID:    "dp_carrier_consolidation",
			Path:            filepath.Join(root, "carrier-consolidation-1719417600"),
			DefaultOrgAlias: "northwind-uat",
			LastRetrievedAt: now.AddDate(0, 0, -2),
			LastDeployedAt:  now.AddDate(0, 0, -1),
		},
	}
}

// demoBundleManifestFor returns a one-line package.xml whose Flow
// member matches one of the project's items, so a user drilling
// into the bundle sees something familiar in the Components view.
func demoBundleManifestFor(projectID string) string {
	prelude := `<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
`
	closer := `    <version>62.0</version>
</Package>
`
	switch projectID {
	case "dp_shipment_revamp":
		return prelude + `    <types>
        <members>Shipment__c</members>
        <name>CustomObject</name>
    </types>
    <types>
        <members>Shipment__c.Status__c</members>
        <members>Shipment__c.Tracking_Number__c</members>
        <members>Shipment__c.Delivered_At__c</members>
        <name>CustomField</name>
    </types>
    <types>
        <members>Shipment_Status_Change</members>
        <members>Shipment_Delivered_Notify</members>
        <name>Flow</name>
    </types>
    <types>
        <members>ShipmentStatusHelper</members>
        <name>ApexClass</name>
    </types>
` + closer
	case "dp_carrier_consolidation":
		return prelude + `    <types>
        <members>Carrier__c</members>
        <name>CustomObject</name>
    </types>
    <types>
        <members>Carrier_Activation</members>
        <members>Carrier_Rate_Update</members>
        <members>Carrier_Holiday_Pause</members>
        <name>Flow</name>
    </types>
    <types>
        <members>CarrierRoutingService</members>
        <members>CarrierRoutingServiceTest</members>
        <name>ApexClass</name>
    </types>
    <types>
        <members>carrierPickerLWC</members>
        <name>LightningComponentBundle</name>
    </types>
` + closer
	}
	return prelude + closer
}

// demoTagSeed and demoTagBindingSeed are thin shapes for the
// fixture spec — the seeder applies them through the real Store
// methods so any constraint changes propagate naturally.
type demoTagSeed struct {
	Name, Color, Icon string
}

type demoTagBindingSeed struct {
	TagName string
	Kind    devproject.ItemKind
	Ref     string
	OrgUser string
}

// demoTagSeeds: small, named-by-purpose so the demo tag picker has
// meaningful choices instead of "tag1/tag2/tag3".
func demoTagSeeds() []demoTagSeed {
	return []demoTagSeed{
		{Name: "to-review", Color: "yellow", Icon: "○"},
		{Name: "in-progress", Color: "blue", Icon: "◐"},
		{Name: "done", Color: "green", Icon: "●"},
		{Name: "blocked", Color: "red", Icon: "✕"},
		{Name: "tech-debt", Color: "magenta", Icon: "△"},
		{Name: "needs-tests", Color: "cyan", Icon: "T"},
	}
}

// demoTagBindings applies tags across the seeded project items so
// the /tags surface has data and the bulk-tag picker on the items
// list shows familiar groups.
func demoTagBindings() []demoTagBindingSeed {
	return []demoTagBindingSeed{
		{TagName: "in-progress", Kind: devproject.KindFlow, Ref: "Shipment_Status_Change", OrgUser: demoDev},
		{TagName: "to-review", Kind: devproject.KindFlow, Ref: "Shipment_Delivered_Notify", OrgUser: demoDev},
		{TagName: "done", Kind: devproject.KindApexClass, Ref: "ShipmentStatusHelper", OrgUser: demoDev},
		{TagName: "in-progress", Kind: devproject.KindFlow, Ref: "Carrier_Activation", OrgUser: demoDev},
		{TagName: "tech-debt", Kind: devproject.KindFlow, Ref: "Legacy_Order_Approval", OrgUser: demoDev},
		{TagName: "needs-tests", Kind: devproject.KindApexClass, Ref: "CarrierRoutingService", OrgUser: demoDev},
		{TagName: "blocked", Kind: devproject.KindApexClass, Ref: "DeprecatedReportBuilder", OrgUser: demoDev},
	}
}

// demoSavedQuerySeed is a thin spec shape.
type demoSavedQuerySeed struct {
	Name, Description, Body string
}

func demoSavedQueries() []demoSavedQuerySeed {
	return []demoSavedQuerySeed{
		{
			Name:        "Open shipments by carrier",
			Description: "Shipments not yet delivered, grouped by carrier",
			Body:        "SELECT Id, Name, Status__c, Carrier__r.Name FROM Shipment__c WHERE Delivered_At__c = NULL ORDER BY Carrier__r.Name, CreatedDate DESC LIMIT 100",
		},
		{
			Name:        "Recently changed accounts",
			Description: "Accounts modified in the last 7 days",
			Body:        "SELECT Id, Name, Owner.Name, LastModifiedDate FROM Account WHERE LastModifiedDate = LAST_N_DAYS:7 ORDER BY LastModifiedDate DESC LIMIT 50",
		},
		{
			Name:        "Flow versions for shipment area",
			Description: "Active vs inactive Flow definitions touching Shipment__c",
			Body:        "SELECT Id, DeveloperName, ApiVersion, Status FROM FlowDefinitionView WHERE DeveloperName LIKE 'Shipment%' ORDER BY DeveloperName",
		},
		{
			Name:        "Apex classes by namespace",
			Description: "Bucket the org's apex by managed vs unmanaged",
			Body:        "SELECT NamespacePrefix, COUNT(Id) classes FROM ApexClass GROUP BY NamespacePrefix ORDER BY COUNT(Id) DESC",
		},
		{
			Name:        "Cases this week",
			Description: "Open cases by priority",
			Body:        "SELECT Id, CaseNumber, Subject, Priority, Status, CreatedDate FROM Case WHERE CreatedDate = THIS_WEEK AND IsClosed = false ORDER BY Priority, CreatedDate DESC LIMIT 100",
		},
	}
}

// demoSavedApexSeed is a thin spec shape for apex snippets.
type demoSavedApexSeed struct {
	Name, Description, Body string
}

func demoSavedApex() []demoSavedApexSeed {
	return []demoSavedApexSeed{
		{
			Name:        "Bounce a flow",
			Description: "Deactivate then reactivate a flow to clear cached interview state",
			Body: `String developerName = 'Shipment_Status_Change';
List<FlowDefinitionView> defs = [SELECT DurableId FROM FlowDefinitionView WHERE DeveloperName = :developerName];
System.debug('Found flow def: ' + defs);
// FlowDefinition.ActiveVersionId can be toggled via Metadata API; this is the inspect-only side.`,
		},
		{
			Name:        "Recalculate sharing for an sobject",
			Description: "Trigger sharing recalculation on a custom object",
			Body:        `Database.executeBatch(new SharingRecalcBatch('Shipment__c'), 200);`,
		},
		{
			Name:        "Count records by status",
			Description: "Quick aggregate to verify shipment status distribution",
			Body: `AggregateResult[] groups = [SELECT Status__c, COUNT(Id) cnt FROM Shipment__c GROUP BY Status__c];
for (AggregateResult ag : groups) {
    System.debug(ag.get('Status__c') + ': ' + ag.get('cnt'));
}`,
		},
		{
			Name:        "Force user license check",
			Description: "Show licence usage breakdown for the running user's profile",
			Body: `Id pid = UserInfo.getProfileId();
List<UserLicense> ls = [SELECT Name, TotalLicenses, UsedLicenses FROM UserLicense];
for (UserLicense l : ls) { System.debug(l.Name + ': ' + l.UsedLicenses + '/' + l.TotalLicenses); }`,
		},
	}
}

// demoSOQLHistorySeed pre-populates the /soql History subtab so it
// isn't empty on a fresh demo launch.
type demoSOQLHistorySeed struct {
	OrgUser    string
	Body       string
	DurationMs int
	RowCount   int
	Error      string
}

func demoSOQLHistory(_ time.Time) []demoSOQLHistorySeed {
	return []demoSOQLHistorySeed{
		{OrgUser: demoDev, Body: "SELECT Id, Name FROM Account LIMIT 20", DurationMs: 142, RowCount: 20},
		{OrgUser: demoDev, Body: "SELECT Id, Name, Status__c FROM Shipment__c WHERE Delivered_At__c = NULL ORDER BY CreatedDate DESC LIMIT 50", DurationMs: 287, RowCount: 17},
		{OrgUser: demoDev, Body: "SELECT Id, Name FROM Carrier__c", DurationMs: 91, RowCount: 6},
		{OrgUser: demoUAT, Body: "SELECT Id, DeveloperName FROM FlowDefinitionView WHERE DeveloperName LIKE 'Shipment%'", DurationMs: 312, RowCount: 4},
		{OrgUser: demoDev, Body: "SELECT COUNT() FROM Shipment__c WHERE Status__c = 'In Transit'", DurationMs: 158, RowCount: 0},
		{OrgUser: demoDev, Body: "SELECT Id, NAme FROM Accouont", DurationMs: 0, RowCount: 0, Error: "MALFORMED_QUERY: sObject type 'Accouont' is not supported."},
	}
}
