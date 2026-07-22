package sf

// Page layouts for one sObject — the object-drill Layouts subtab.
// Tooling Layout filters by TableEnumOrId, which is the API name for
// standard objects but the CustomObject's ID for custom ones (the
// classic tooling-API gotcha — verified both shapes on phd
// 2026-06-12).

import (
	"fmt"
	"sort"
	"strings"
)

type PageLayoutRow struct {
	ID      string
	Name    string
	SObject string // owning sObject API name (for the Setup target)
}

// Targets opens Object Manager's Page Layouts list for the owning
// object. Per-layout edit pages are layout-editor iframes that don't
// deep-link reliably, so the list page is the canonical target.
func (r PageLayoutRow) Targets() []OpenTarget {
	if r.SObject == "" {
		return nil
	}
	return []OpenTarget{
		{ID: "layouts", Label: "Page Layouts (Object Manager)",
			Path: "/lightning/setup/ObjectManager/" + r.SObject + "/PageLayouts/view"},
	}
}

// ListObjectLayouts returns the page layouts defined on one sObject.
func ListObjectLayouts(target, sobject string) ([]PageLayoutRow, error) {
	key := sobject
	if strings.HasSuffix(sobject, "__c") {
		id, err := customObjectID(target, sobject)
		if err != nil {
			return nil, err
		}
		key = id
	}
	if !validSOQLIdentifier(sobject) {
		return nil, fmt.Errorf("invalid sobject name %q", sobject)
	}
	soql := fmt.Sprintf(
		"SELECT Id, Name FROM Layout WHERE TableEnumOrId = '%s' ORDER BY Name",
		sqlEscape(key))
	q, err := Query(target, soql, true)
	if err != nil {
		return nil, fmt.Errorf("list layouts: %w", err)
	}
	out := make([]PageLayoutRow, 0, len(q.Records))
	for _, r := range q.Records {
		out = append(out, PageLayoutRow{
			ID:      asString(r["Id"]),
			Name:    asString(r["Name"]),
			SObject: sobject,
		})
	}
	return out, nil
}

// customObjectID resolves a custom object's tooling CustomObject Id
// from its API name. Handles both ns__Dev__c and Dev__c shapes.
func customObjectID(target, sobject string) (string, error) {
	base := strings.TrimSuffix(sobject, "__c")
	ns := ""
	if i := strings.Index(base, "__"); i > 0 {
		ns = base[:i]
		base = base[i+2:]
	}
	soql := "SELECT Id FROM CustomObject WHERE DeveloperName = '" + sqlEscape(base) + "'"
	if ns != "" {
		soql += " AND NamespacePrefix = '" + sqlEscape(ns) + "'"
	} else {
		soql += " AND NamespacePrefix = null"
	}
	soql += " LIMIT 1"
	q, err := Query(target, soql, true)
	if err != nil {
		return "", fmt.Errorf("resolve custom object: %w", err)
	}
	if len(q.Records) == 0 {
		return "", fmt.Errorf("custom object %q not found in tooling CustomObject", sobject)
	}
	return asString(q.Records[0]["Id"]), nil
}

// ObjectFlowRow is one record-triggered flow on an sObject — the
// object-drill Flows subtab. DefinitionID is FlowDefinitionView's
// DurableId (a 300… FlowDefinition id), which is exactly what the
// /flows drill keys on, so Enter can reuse the flow-detail tab.
type ObjectFlowRow struct {
	DefinitionID      string
	ApiName           string
	Label             string
	ProcessType       string
	TriggerType       string // RecordBeforeSave / RecordAfterSave / RecordBeforeDelete / Scheduled…
	RecordTriggerType string // Create / Update / CreateAndUpdate / Delete — which DML fires it
	TriggerOrder      int    // explicit run order within the phase (0 when unset)
	HasTriggerOrder   bool   // TriggerOrder was non-null (distinguishes "order 0" from "no order")
	HasAsyncPath      bool   // after-save flow with an async-after-commit path → Run Asynchronously
	VersionNumber     int    // active version number (0 when none)
	IsOutOfDate       bool   // active version is older than the latest
	IsActive          bool
}

// Targets opens the flow in Flow Builder.
func (r ObjectFlowRow) Targets() []OpenTarget {
	if r.DefinitionID == "" {
		return nil
	}
	return []OpenTarget{
		{ID: "builder", Label: "Flow Builder",
			Path: "/builder_platform_interaction/flowBuilder.app?flowDefId=" + r.DefinitionID},
	}
}

// ListObjectFlows returns the flows whose trigger object is the
// given sObject. FlowDefinitionView.TriggerObjectOrEventId carries
// the API name for standard objects but the CustomObject Id for
// custom ones — the same TableEnumOrId-style duality as layouts,
// resolved the same way (verified on phd 2026-06-13).
func ListObjectFlows(target, sobject string) ([]ObjectFlowRow, error) {
	key := sobject
	if strings.HasSuffix(sobject, "__c") {
		id, err := customObjectID(target, sobject)
		if err != nil {
			return nil, err
		}
		// FlowDefinitionView stores the 15-char id.
		if len(id) == 18 {
			id = id[:15]
		}
		key = id
	}
	soql := fmt.Sprintf(
		"SELECT DurableId, ApiName, Label, ProcessType, TriggerType, "+
			"RecordTriggerType, TriggerOrder, HasAsyncAfterCommitPath, "+
			"VersionNumber, IsOutOfDate, IsActive "+
			"FROM FlowDefinitionView WHERE TriggerObjectOrEventId = '%s' "+
			"ORDER BY Label", sqlEscape(key))
	q, err := Query(target, soql, false)
	if err != nil {
		return nil, fmt.Errorf("list object flows: %w", err)
	}
	out := make([]ObjectFlowRow, 0, len(q.Records))
	for _, r := range q.Records {
		row := ObjectFlowRow{
			DefinitionID:      asString(r["DurableId"]),
			ApiName:           asString(r["ApiName"]),
			Label:             asString(r["Label"]),
			ProcessType:       asString(r["ProcessType"]),
			TriggerType:       asString(r["TriggerType"]),
			RecordTriggerType: asString(r["RecordTriggerType"]),
			VersionNumber:     asInt(r["VersionNumber"]),
		}
		if n, ok := r["TriggerOrder"]; ok && n != nil {
			row.TriggerOrder = asInt(n)
			row.HasTriggerOrder = true
		}
		if b, ok := r["HasAsyncAfterCommitPath"].(bool); ok {
			row.HasAsyncPath = b
		}
		if b, ok := r["IsOutOfDate"].(bool); ok {
			row.IsOutOfDate = b
		}
		if b, ok := r["IsActive"].(bool); ok {
			row.IsActive = b
		}
		out = append(out, row)
	}
	// Order by execution phase, then by TriggerOrder within the phase
	// (the real run order — 10/20/50…), then by label as a tiebreak. The
	// UI renders Salesforce's Flow Trigger Explorer grouping by walking
	// the slice and inserting a header at each phase boundary. Stable to
	// keep the SOQL's alphabetical order where TriggerOrder ties.
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := FlowPhaseRank(out[i]), FlowPhaseRank(out[j])
		if pi != pj {
			return pi < pj
		}
		return out[i].TriggerOrder < out[j].TriggerOrder
	})
	return out, nil
}

// FlowPhaseRank orders record-triggered flow phases the way Salesforce's
// Flow Trigger Explorer does: before-save (Fast Field Updates) →
// after-save synchronous (Actions and Related Records) → after-save
// async (Run Asynchronously) → before-delete → scheduled → platform
// event → other. The async split uses HasAsyncPath, which is why this
// takes the whole row rather than just the trigger type.
func FlowPhaseRank(r ObjectFlowRow) int {
	switch r.TriggerType {
	case "RecordBeforeSave":
		return 0
	case "RecordAfterSave":
		if r.HasAsyncPath {
			return 2 // Run Asynchronously — after the sync after-save group
		}
		return 1
	case "RecordBeforeDelete":
		return 3
	case "Scheduled":
		return 4
	case "PlatformEvent":
		return 5
	default:
		return 6
	}
}
