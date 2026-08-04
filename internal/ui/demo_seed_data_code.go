package ui

// Fixture source code for the demo's code drill-downs — see
// demo_seed.go for the seeder, demo_seed_data.go for the class /
// trigger / bundle lists, and demo_seed_data_extra.go for the
// operational surfaces. This file generates the bodies behind those
// lists: a plausible, syntactically-valid Apex body for every seeded
// class and trigger, and full file sets (.js / .html / .css /
// .js-meta.xml, .cmp / controller / helper) for every seeded LWC and
// Aura bundle. Template-generated from the same name grids as the
// lists, so a class row and its body can't drift apart.

import (
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// demoApexDomainObjects maps the demoApexClasses domain grid onto the
// custom objects each domain's code works against.
var demoApexDomainObjects = map[string]string{
	"Shipment": "Shipment__c", "Carrier": "Carrier__c", "Route": "Route__c",
	"Customs": "Customs_Declaration__c", "FreightInvoice": "Freight_Invoice__c",
	"Warehouse": "Warehouse__c", "DockBooking": "Dock_Booking__c",
	"Inventory": "Inventory_Adjustment__c", "Supplier": "Supplier_Scorecard__c",
	"Container": "Container__c", "Vessel": "Vessel__c", "Tariff": "Tariff_Code__c",
	"Claim": "Claim__c", "Driver": "Driver__c", "Fleet": "Fleet_Vehicle__c",
}

// demoSpaceCamel inserts spaces into a CamelCase name for prose
// ("FreightInvoice" -> "Freight Invoice").
func demoSpaceCamel(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---------------------------------------------------------------
// Apex class bodies
// ---------------------------------------------------------------

// demoApexClassDetails builds the apex_class:<id> payloads: one
// ApexClassDetail per demoApexClasses row, body derived from the
// class's name (handler / service / selector / test / batch /
// queueable) and domain object.
func demoApexClassDetails() map[string]sf.ApexClassDetail {
	classes := demoApexClasses()
	out := make(map[string]sf.ApexClassDetail, len(classes))
	for _, c := range classes {
		body := demoApexBody(c.Name)
		out[c.ID] = sf.ApexClassDetail{
			ID: c.ID, Name: c.Name, Status: c.Status, Body: body,
			ApiVersion: c.ApiVersion, IsValid: c.IsValid,
			LengthNoComments: len(body),
			LastModifiedDate: c.LastModifiedDate,
		}
	}
	return out
}

// demoApexBody dispatches on the class-name suffix the demo grid
// composes names from.
func demoApexBody(name string) string {
	switch {
	case strings.HasSuffix(name, "Batch"):
		return demoApexBatchBody(name)
	case strings.HasSuffix(name, "Queueable"):
		return demoApexQueueableBody(name)
	case strings.HasSuffix(name, "TriggerHandler"):
		return demoApexRoleBody(strings.TrimSuffix(name, "TriggerHandler"), demoApexHandlerTemplate)
	case strings.HasSuffix(name, "ServiceTest"):
		return demoApexRoleBody(strings.TrimSuffix(name, "ServiceTest"), demoApexTestTemplate)
	case strings.HasSuffix(name, "Service"):
		return demoApexRoleBody(strings.TrimSuffix(name, "Service"), demoApexServiceTemplate)
	case strings.HasSuffix(name, "Selector"):
		return demoApexRoleBody(strings.TrimSuffix(name, "Selector"), demoApexSelectorTemplate)
	}
	// Unknown shape (shouldn't happen with the current grid): a stub
	// that's still valid Apex.
	return "public with sharing class " + name + " {\n}\n"
}

// demoApexRoleBody fills a role template. Placeholders: %[1]s domain,
// %[2]s object API name, %[3]s human label, %[4]s lower-case label.
func demoApexRoleBody(domain string, tmpl string) string {
	obj := demoApexDomainObjects[domain]
	if obj == "" {
		obj = domain + "__c"
	}
	label := demoSpaceCamel(domain)
	return fmt.Sprintf(tmpl, domain, obj, label, strings.ToLower(label))
}

const demoApexHandlerTemplate = `/**
 * Trigger handler for %[2]s. Routed from the object trigger so the
 * trigger body stays logic-free; all %[3]s writes go through
 * %[1]sService.
 */
public with sharing class %[1]sTriggerHandler {

    private static Boolean bypass = false;

    /** Data-migration escape hatch: skip handler logic for this txn. */
    public static void setBypass(Boolean value) {
        bypass = value;
    }

    public static void beforeInsert(List<%[2]s> newRecords) {
        if (bypass) {
            return;
        }
        applyDefaults(newRecords);
    }

    public static void beforeUpdate(Map<Id, %[2]s> oldMap, List<%[2]s> newRecords) {
        if (bypass) {
            return;
        }
        applyDefaults(newRecords);
    }

    public static void afterInsert(List<%[2]s> newRecords) {
        if (bypass) {
            return;
        }
        %[1]sService.handleStatusChange(newRecords);
    }

    public static void afterUpdate(Map<Id, %[2]s> oldMap, Map<Id, %[2]s> newMap) {
        if (bypass) {
            return;
        }
        List<%[2]s> changed = new List<%[2]s>();
        for (%[2]s rec : newMap.values()) {
            %[2]s prior = oldMap.get(rec.Id);
            if (prior != null && prior.Status__c != rec.Status__c) {
                changed.add(rec);
            }
        }
        if (!changed.isEmpty()) {
            %[1]sService.handleStatusChange(changed);
        }
    }

    private static void applyDefaults(List<%[2]s> records) {
        for (%[2]s rec : records) {
            if (rec.Status__c == null) {
                rec.Status__c = 'Draft';
            }
        }
    }
}
`

const demoApexServiceTemplate = `/**
 * Domain service for %[2]s. Cross-object writes for the %[3]s domain
 * route through here so trigger handlers, flows, and batch jobs share
 * one code path.
 */
public with sharing class %[1]sService {

    public static void handleStatusChange(List<%[2]s> changed) {
        List<Task> followUps = new List<Task>();
        for (%[2]s rec : changed) {
            if (rec.Status__c == 'Exception') {
                followUps.add(new Task(
                    WhatId = rec.Id,
                    Subject = 'Review %[4]s exception',
                    ActivityDate = Date.today().addDays(1)
                ));
            }
        }
        if (!followUps.isEmpty()) {
            insert followUps;
        }
    }

    public static Map<Id, %[2]s> refreshExternalRefs(Set<Id> ids) {
        Map<Id, %[2]s> records = new Map<Id, %[2]s>(%[1]sSelector.byIds(ids));
        List<%[2]s> dirty = new List<%[2]s>();
        for (%[2]s rec : records.values()) {
            if (String.isBlank(rec.External_Ref__c)) {
                rec.External_Ref__c = 'NW-' + String.valueOf(rec.Id).left(15);
                dirty.add(rec);
            }
        }
        if (!dirty.isEmpty()) {
            update dirty;
        }
        return records;
    }

    @AuraEnabled(cacheable=true)
    public static List<%[2]s> recentForDisplay(Integer max) {
        Integer capped = max == null ? 25 : Math.min(max, 100);
        return %[1]sSelector.recentlyModified(capped);
    }
}
`

const demoApexSelectorTemplate = `/**
 * Query layer for %[2]s. Keeps SOQL in one place so field lists and
 * sharing behaviour stay consistent across the %[3]s domain.
 */
public inherited sharing class %[1]sSelector {

    public static List<%[2]s> byIds(Set<Id> ids) {
        return [
            SELECT Id, Name, Status__c, External_Ref__c, LastModifiedDate
            FROM %[2]s
            WHERE Id IN :ids
        ];
    }

    public static List<%[2]s> recentlyModified(Integer maxRows) {
        return [
            SELECT Id, Name, Status__c, External_Ref__c, LastModifiedDate
            FROM %[2]s
            WHERE LastModifiedDate = LAST_N_DAYS:30
            ORDER BY LastModifiedDate DESC
            LIMIT :maxRows
        ];
    }

    public static List<%[2]s> byStatus(String status) {
        return [
            SELECT Id, Name, Status__c, LastModifiedDate
            FROM %[2]s
            WHERE Status__c = :status
            ORDER BY Name
        ];
    }
}
`

const demoApexTestTemplate = `@IsTest
private class %[1]sServiceTest {

    @TestSetup
    static void makeData() {
        List<%[2]s> records = new List<%[2]s>();
        for (Integer i = 0; i < 5; i++) {
            records.add(new %[2]s(Name = 'Test %[3]s ' + i));
        }
        insert records;
    }

    @IsTest
    static void statusChangeCreatesFollowUpTasks() {
        List<%[2]s> records = [SELECT Id, Status__c FROM %[2]s];
        for (%[2]s rec : records) {
            rec.Status__c = 'Exception';
        }

        Test.startTest();
        %[1]sService.handleStatusChange(records);
        Test.stopTest();

        System.assertEquals(records.size(),
            [SELECT COUNT() FROM Task WHERE Subject = 'Review %[4]s exception'],
            'one follow-up task per exception record');
    }

    @IsTest
    static void refreshExternalRefsBackfillsBlanks() {
        Set<Id> ids = new Map<Id, %[2]s>([SELECT Id FROM %[2]s]).keySet();

        Test.startTest();
        Map<Id, %[2]s> refreshed = %[1]sService.refreshExternalRefs(ids);
        Test.stopTest();

        for (%[2]s rec : [SELECT External_Ref__c FROM %[2]s WHERE Id IN :ids]) {
            System.assert(String.isNotBlank(rec.External_Ref__c),
                'external ref backfilled');
        }
        System.assertEquals(ids.size(), refreshed.size(), 'all records returned');
    }
}
`

// demoAsyncApexMeta gives each async worker its object + one-line
// purpose (used in the class-body comment) and how it gets invoked.
var demoAsyncApexMeta = map[string]struct {
	object, purpose, invokedBy string
}{
	"SupplierScorecardBatch": {"Supplier_Scorecard__c",
		"recomputes each supplier's rolling on-time and damage scores",
		"the Supplier Scorecard Refresh scheduled job"},
	"ShipmentEtaRecalcBatch": {"Shipment__c",
		"re-derives expected delivery dates from carrier transit times",
		"ShipmentService after bulk status changes"},
	"NightlyKpiSnapshotBatch": {"KPI_Snapshot__c",
		"writes the daily operations KPI snapshot rows",
		"the Nightly KPI Snapshot scheduled job"},
	"DemurrageAccrualBatch": {"Demurrage_Charge__c",
		"accrues daily demurrage against overstayed containers",
		"the Demurrage Accrual scheduled job"},
	"RateCardSyncQueueable": {"Rate_Card__c",
		"pulls carrier rate updates into the active rate cards",
		"the hourly Rate Card Sync scheduled job"},
	"ColdChainAlertQueueable": {"Cold_Chain_Alert__c",
		"fans out excursion alerts to the owning warehouse team",
		"ColdChainAlertTrigger"},
}

func demoApexBatchBody(name string) string {
	m := demoAsyncApexMeta[name]
	return fmt.Sprintf(`/**
 * Batch job: %[3]s. Enqueued by %[4]s.
 */
public with sharing class %[1]s implements Database.Batchable<SObject>, Database.Stateful {

    private Integer processed = 0;

    public Database.QueryLocator start(Database.BatchableContext ctx) {
        return Database.getQueryLocator(
            'SELECT Id, Name, Status__c FROM %[2]s WHERE Status__c != \'Archived\'');
    }

    public void execute(Database.BatchableContext ctx, List<SObject> scope) {
        List<%[2]s> records = (List<%[2]s>) scope;
        for (%[2]s rec : records) {
            rec.Status__c = 'Refreshed';
        }
        processed += records.size();
        update records;
    }

    public void finish(Database.BatchableContext ctx) {
        System.debug('%[1]s finished; rows=' + processed);
    }
}
`, name, m.object, m.purpose, m.invokedBy)
}

func demoApexQueueableBody(name string) string {
	m := demoAsyncApexMeta[name]
	return fmt.Sprintf(`/**
 * Queueable: %[3]s. Enqueued by %[4]s.
 */
public with sharing class %[1]s implements Queueable {

    private final Set<Id> targetIds;

    public %[1]s(Set<Id> targetIds) {
        this.targetIds = targetIds;
    }

    public void execute(QueueableContext ctx) {
        List<%[2]s> records = [
            SELECT Id, Name, Status__c
            FROM %[2]s
            WHERE Id IN :targetIds
        ];
        for (%[2]s rec : records) {
            rec.Status__c = 'Synced';
        }
        if (!records.isEmpty()) {
            update records;
        }
    }
}
`, name, m.object, m.purpose, m.invokedBy)
}

// ---------------------------------------------------------------
// Trigger bodies
// ---------------------------------------------------------------

// demoTriggerDetails builds the triggerdetail:<id> payloads for every
// demoTriggers row. Domain triggers with a seeded handler class
// delegate to it (the pattern the handler bodies advertise); the rest
// carry small inline logic, the way real orgs mix styles.
func demoTriggerDetails() map[string]sf.TriggerDetail {
	rows := demoTriggers()
	out := make(map[string]sf.TriggerDetail, len(rows))
	for _, t := range rows {
		body := demoTriggerBody(t)
		out[t.ID] = sf.TriggerDetail{
			ID: t.ID, Name: t.Name, Status: t.Status, Body: body,
			ApiVer: t.ApiVer, Valid: t.Valid, Len: len(body), Events: t.Events,
		}
	}
	return out
}

// demoTriggerHandlers maps trigger name -> the seeded handler class
// it delegates to. Triggers not listed here get inline bodies.
var demoTriggerHandlers = map[string]string{
	"ShipmentTrigger":           "ShipmentTriggerHandler",
	"CarrierTrigger":            "CarrierTriggerHandler",
	"RouteTrigger":              "RouteTriggerHandler",
	"CustomsDeclarationTrigger": "CustomsTriggerHandler",
	"FreightInvoiceTrigger":     "FreightInvoiceTriggerHandler",
	"DockBookingTrigger":        "DockBookingTriggerHandler",
	"ClaimTrigger":              "ClaimTriggerHandler",
}

func demoTriggerBody(t sf.TriggerRow) string {
	if handler, ok := demoTriggerHandlers[t.Name]; ok {
		return demoTriggerDispatchBody(t, handler)
	}
	if body, ok := demoInlineTriggerBodies[t.Name]; ok {
		return body
	}
	return fmt.Sprintf("trigger %s on %s (%s) {\n    // no-op\n}\n", t.Name, t.Table, t.Events)
}

// demoTriggerDispatchBody writes the handler-delegation pattern,
// guarding each dispatch with the trigger-context checks for the
// events the row declares.
func demoTriggerDispatchBody(t sf.TriggerRow, handler string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "trigger %s on %s (%s) {\n", t.Name, t.Table, t.Events)
	for _, ev := range strings.Split(t.Events, ", ") {
		switch ev {
		case "before insert":
			fmt.Fprintf(&b, "    if (Trigger.isBefore && Trigger.isInsert) {\n        %s.beforeInsert(Trigger.new);\n    }\n", handler)
		case "before update":
			fmt.Fprintf(&b, "    if (Trigger.isBefore && Trigger.isUpdate) {\n        %s.beforeUpdate(Trigger.oldMap, Trigger.new);\n    }\n", handler)
		case "after insert":
			fmt.Fprintf(&b, "    if (Trigger.isAfter && Trigger.isInsert) {\n        %s.afterInsert(Trigger.new);\n    }\n", handler)
		case "after update":
			fmt.Fprintf(&b, "    if (Trigger.isAfter && Trigger.isUpdate) {\n        %s.afterUpdate(Trigger.oldMap, Trigger.newMap);\n    }\n", handler)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// demoInlineTriggerBodies: hand-written bodies for the triggers with
// no matching handler class. Small, plausible, valid.
var demoInlineTriggerBodies = map[string]string{
	"ShipmentLineTrigger": `trigger ShipmentLineTrigger on Shipment_Line__c (before insert, before update) {
    for (Shipment_Line__c line : Trigger.new) {
        if (line.Quantity__c == null || line.Quantity__c < 1) {
            line.Quantity__c = 1;
        }
    }
}
`,
	"ColdChainAlertTrigger": `trigger ColdChainAlertTrigger on Cold_Chain_Alert__c (after insert) {
    // Fan alert handling out of the transaction: the queueable posts
    // to the warehouse channel and stamps the acknowledgement window.
    System.enqueueJob(new ColdChainAlertQueueable(Trigger.newMap.keySet()));
}
`,
	"GoodsReceiptTrigger": `trigger GoodsReceiptTrigger on Goods_Receipt__c (before insert, before update) {
    for (Goods_Receipt__c receipt : Trigger.new) {
        if (receipt.Status__c == null) {
            receipt.Status__c = 'Draft';
        }
        if (receipt.Status__c == 'Received' && receipt.End_Date__c == null) {
            receipt.End_Date__c = Date.today();
        }
    }
}
`,
	"AccountTrigger": `trigger AccountTrigger on Account (before update) {
    // Credit hold: flag the account when it goes on stop so the
    // shipment flows refuse new bookings.
    for (Account acct : Trigger.new) {
        Account prior = Trigger.oldMap.get(acct.Id);
        if (acct.On_Stop__c && !prior.On_Stop__c) {
            acct.Delivery_Window_Notes__c = 'ON STOP since ' + Date.today().format();
        }
    }
}
`,
	"ContactTrigger": `trigger ContactTrigger on Contact (before insert, before update) {
    for (Contact con : Trigger.new) {
        if (con.Preferred_Contact_Method__c == null && con.Email != null) {
            con.Preferred_Contact_Method__c = 'Email';
        }
    }
}
`,
	"CaseTrigger": `trigger CaseTrigger on Case (after insert) {
    // Shipment-linked cases get a triage task for the logistics team.
    List<Task> triage = new List<Task>();
    for (Case c : Trigger.new) {
        if (c.Related_Shipment__c != null) {
            triage.add(new Task(
                WhatId = c.Id,
                Subject = 'Triage shipment case',
                ActivityDate = Date.today()
            ));
        }
    }
    if (!triage.isEmpty()) {
        insert triage;
    }
}
`,
	"OpportunityTrigger": `trigger OpportunityTrigger on Opportunity (before update, after update) {
    if (Trigger.isBefore) {
        for (Opportunity opp : Trigger.new) {
            if (opp.StageName == 'Closed Won' && opp.NextStep != null) {
                opp.NextStep = null;
            }
        }
    }
}
`,
	"LeadTrigger": `trigger LeadTrigger on Lead (before insert) {
    for (Lead lead : Trigger.new) {
        if (lead.LeadSource == null) {
            lead.LeadSource = 'Web';
        }
    }
}
`,
}

// demoTriggersByTable groups the flat trigger fixture per sObject for
// the object drill's Triggers subtab (cache key triggers:<sobject>).
// Every catalog object gets an entry — empty for the objects with no
// trigger — so no drill lands on a demo-mode fetch error.
func demoTriggersByTable(objs []sf.SObject) map[string][]sf.TriggerRow {
	out := make(map[string][]sf.TriggerRow, len(objs))
	for _, o := range objs {
		out[o.Name] = []sf.TriggerRow{}
	}
	for _, t := range demoTriggers() {
		out[t.Table] = append(out[t.Table], t)
	}
	return out
}

// ---------------------------------------------------------------
// LWC bundle sources
// ---------------------------------------------------------------

// demoLWCBundleDetails builds the lwc_bundle:<id> payloads: the four
// files of each seeded bundle with generated-but-plausible source.
func demoLWCBundleDetails() map[string]sf.LWCBundleDetail {
	bundles := demoLWCBundles()
	specs := demoLWCSpecs()
	out := make(map[string]sf.LWCBundleDetail, len(bundles))
	resSeq := 0
	resID := func() string {
		resSeq++
		return fmt.Sprintf("0RdDM00000DM%03dAAA", resSeq)
	}
	for i, b := range bundles {
		s := specs[i]
		out[b.ID] = sf.LWCBundleDetail{
			Bundle: b,
			Resources: []sf.LWCResource{
				{ID: resID(), FilePath: s.dev + "/" + s.dev + ".js", Format: "js", Source: demoLWCJS(s)},
				{ID: resID(), FilePath: s.dev + "/" + s.dev + ".html", Format: "html", Source: demoLWCHTML(s)},
				{ID: resID(), FilePath: s.dev + "/" + s.dev + ".css", Format: "css", Source: demoLWCCSS()},
				{ID: resID(), FilePath: s.dev + "/" + s.dev + ".js-meta.xml", Format: "xml", Source: demoLWCMeta(s)},
			},
		}
	}
	return out
}

func demoLWCJS(s demoLWCSpec) string {
	className := strings.ToUpper(s.dev[:1]) + s.dev[1:]
	return fmt.Sprintf(`import { LightningElement, api, wire } from 'lwc';
import { refreshApex } from '@salesforce/apex';
import getRows from '@salesforce/apex/%[2]s.%[3]s';

/**
 * %[4]s - %[5]s
 */
export default class %[1]s extends LightningElement {
    @api recordId;
    @api maxRows = 25;

    error;
    wiredResult;

    @wire(getRows, { max: '$maxRows' })
    wired(result) {
        this.wiredResult = result;
        if (result.error) {
            this.error = result.error.body
                ? result.error.body.message
                : 'Unable to load records.';
        } else {
            this.error = undefined;
        }
    }

    get items() {
        return this.wiredResult && this.wiredResult.data
            ? this.wiredResult.data
            : [];
    }

    get hasItems() {
        return this.items.length > 0;
    }

    get cardTitle() {
        return '%[4]s (' + this.items.length + ')';
    }

    handleRefresh() {
        this.error = undefined;
        return refreshApex(this.wiredResult);
    }
}
`, className, s.apexClass, s.apexMethod, s.label, s.desc)
}

func demoLWCHTML(s demoLWCSpec) string {
	return fmt.Sprintf(`<template>
    <lightning-card title={cardTitle} icon-name="%[1]s">
        <lightning-button-icon
            slot="actions"
            icon-name="utility:refresh"
            alternative-text="Refresh"
            onclick={handleRefresh}>
        </lightning-button-icon>
        <div class="slds-card__body slds-card__body_inner">
            <template if:true={error}>
                <div class="error slds-text-color_error">{error}</div>
            </template>
            <template if:true={hasItems}>
                <ul class="item-list">
                    <template for:each={items} for:item="item">
                        <li key={item.Id} class="item-row">
                            <span class="item-name">{item.Name}</span>
                            <span class="item-status">{item.Status__c}</span>
                            <lightning-formatted-date-time
                                class="item-when"
                                value={item.LastModifiedDate}>
                            </lightning-formatted-date-time>
                        </li>
                    </template>
                </ul>
            </template>
            <template if:false={hasItems}>
                <p class="slds-text-body_small empty">%[2]s</p>
            </template>
        </div>
    </lightning-card>
</template>
`, s.icon, s.emptyMsg)
}

func demoLWCCSS() string {
	return `.item-list {
    margin: 0;
    padding: 0;
    list-style: none;
}

.item-row {
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.375rem 0;
    border-bottom: 1px solid var(--slds-g-color-border-base-1, #e5e5e5);
}

.item-row:last-child {
    border-bottom: none;
}

.item-name {
    font-weight: 600;
}

.item-status {
    color: var(--slds-g-color-neutral-base-30, #514f4d);
}

.item-when {
    white-space: nowrap;
}

.empty,
.error {
    padding: 0.5rem 0;
}
`
}

func demoLWCMeta(s demoLWCSpec) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<LightningComponentBundle xmlns="http://soap.sforce.com/2006/04/metadata">
    <apiVersion>62.0</apiVersion>
    <isExposed>%[1]t</isExposed>
    <masterLabel>%[2]s</masterLabel>
    <description>%[3]s</description>
    <targets>
        <target>lightning__RecordPage</target>
        <target>lightning__AppPage</target>
    </targets>
    <targetConfigs>
        <targetConfig targets="lightning__RecordPage">
            <objects>
                <object>%[4]s</object>
            </objects>
        </targetConfig>
    </targetConfigs>
</LightningComponentBundle>
`, s.exposed, s.label, s.desc, s.object)
}

// ---------------------------------------------------------------
// Aura bundle sources
// ---------------------------------------------------------------

// demoAuraBundleDetails builds the aura_bundle:<id> payloads: .cmp +
// controller + helper for each seeded Aura bundle.
func demoAuraBundleDetails() map[string]sf.AuraBundleDetail {
	bundles := demoAuraBundles()
	specs := demoAuraSpecs()
	out := make(map[string]sf.AuraBundleDetail, len(bundles))
	resSeq := 0
	resID := func() string {
		resSeq++
		return fmt.Sprintf("0AdDM00000DM%03dAAA", resSeq)
	}
	for i, b := range bundles {
		s := specs[i]
		out[b.ID] = sf.AuraBundleDetail{
			Bundle: b,
			Resources: []sf.AuraResource{
				{ID: resID(), DefType: "COMPONENT", Format: "XML", Source: demoAuraCmp(s)},
				{ID: resID(), DefType: "CONTROLLER", Format: "JS", Source: demoAuraController()},
				{ID: resID(), DefType: "HELPER", Format: "JS", Source: demoAuraHelper(s)},
			},
		}
	}
	return out
}

func demoAuraCmp(s demoAuraSpec) string {
	return fmt.Sprintf(`<aura:component implements="flexipage:availableForAllPageTypes,force:hasRecordId"
                controller="%[1]s" access="global">
    <!-- %[3]s -->
    <aura:attribute name="rows" type="Object[]" />
    <aura:attribute name="loading" type="Boolean" default="false" />
    <aura:attribute name="errorMessage" type="String" />

    <aura:handler name="init" value="{!this}" action="{!c.doInit}" />

    <lightning:card title="%[2]s" iconName="%[4]s">
        <aura:set attribute="actions">
            <lightning:buttonIcon iconName="utility:refresh"
                                  alternativeText="Refresh"
                                  onclick="{!c.doInit}" />
        </aura:set>
        <div class="slds-card__body_inner">
            <aura:if isTrue="{!v.loading}">
                <lightning:spinner alternativeText="Loading" size="small" />
            </aura:if>
            <aura:if isTrue="{!not(empty(v.errorMessage))}">
                <div class="slds-text-color_error">{!v.errorMessage}</div>
            </aura:if>
            <aura:iteration items="{!v.rows}" var="row">
                <div class="slds-grid slds-p-vertical_xx-small">
                    <div class="slds-col slds-grow">{!row.Name}</div>
                    <div class="slds-col slds-text-align_right">{!row.Status__c}</div>
                </div>
            </aura:iteration>
        </div>
    </lightning:card>
</aura:component>
`, s.apexClass, s.label, s.desc, s.icon)
}

func demoAuraController() string {
	return `({
    doInit: function (component, event, helper) {
        component.set('v.loading', true);
        component.set('v.errorMessage', null);
        helper.loadRows(component);
    },

    handleRowSelect: function (component, event, helper) {
        var rowId = event.currentTarget.dataset.rowId;
        helper.notifySelection(rowId);
    }
})
`
}

func demoAuraHelper(s demoAuraSpec) string {
	return fmt.Sprintf(`({
    loadRows: function (component) {
        var action = component.get('c.%[1]s');
        action.setParams({ max: 25 });
        action.setCallback(this, function (response) {
            component.set('v.loading', false);
            if (response.getState() === 'SUCCESS') {
                component.set('v.rows', response.getReturnValue());
            } else {
                var errors = response.getError();
                var message = errors && errors[0] && errors[0].message
                    ? errors[0].message
                    : 'Unable to load rows.';
                component.set('v.errorMessage', message);
            }
        });
        $A.enqueueAction(action);
    },

    notifySelection: function (rowId) {
        var toast = $A.get('e.force:showToast');
        if (toast) {
            toast.setParams({ message: 'Selected row ' + rowId, type: 'info' });
            toast.fire();
        }
    }
})
`, s.apexMethod)
}
