package devproject

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
)

func TestWritePackageXML_GroupsByType(t *testing.T) {
	items := []devproject.Item{
		{Kind: devproject.KindSObject, Ref: "Account", Name: "Account"},
		{Kind: devproject.KindSObject, Ref: "Contact", Name: "Contact"},
		{Kind: devproject.KindField, Ref: "Account.Phone", Type: "Account", Name: "Phone"},
		{Kind: devproject.KindFlow, Ref: "0F1abc", Name: "AccountRouter"},
		{Kind: devproject.KindApexClass, Ref: "01p123", Name: "AccountUtil"},
	}

	var buf strings.Builder
	res, err := WritePackageXML(&buf, items, PackageXMLOptions{})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if res.IncludedCount != 5 {
		t.Errorf("IncludedCount: got %d, want 5", res.IncludedCount)
	}

	got := buf.String()

	// XML header
	if !strings.HasPrefix(got, "<?xml") {
		t.Errorf("missing XML header:\n%s", got)
	}
	// Both Account and Contact are members of CustomObject
	if !strings.Contains(got, "<members>Account</members>") {
		t.Errorf("missing Account member:\n%s", got)
	}
	if !strings.Contains(got, "<members>Contact</members>") {
		t.Errorf("missing Contact member:\n%s", got)
	}
	if !strings.Contains(got, "<name>CustomObject</name>") {
		t.Errorf("missing CustomObject type:\n%s", got)
	}
	// Field uses CustomField
	if !strings.Contains(got, "<members>Account.Phone</members>") {
		t.Errorf("missing field member:\n%s", got)
	}
	if !strings.Contains(got, "<name>CustomField</name>") {
		t.Errorf("missing CustomField type:\n%s", got)
	}
	// Flow uses Name (DeveloperName)
	if !strings.Contains(got, "<members>AccountRouter</members>") {
		t.Errorf("missing Flow member:\n%s", got)
	}
	// API version pinned
	if !strings.Contains(got, "<version>62.0</version>") {
		t.Errorf("missing or wrong api version:\n%s", got)
	}
}

func TestWritePackageXML_RecordsExcluded(t *testing.T) {
	items := []devproject.Item{
		{Kind: devproject.KindSObject, Ref: "Account", Name: "Account"},
		{Kind: devproject.KindRecord, Ref: "001abc", Type: "Account", Name: "Acme Inc."},
		{Kind: devproject.KindRecord, Ref: "001def", Type: "Account", Name: "Globex"},
	}
	var buf strings.Builder
	res, err := WritePackageXML(&buf, items, PackageXMLOptions{})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if res.IncludedCount != 1 {
		t.Errorf("IncludedCount: got %d, want 1 (Account only)", res.IncludedCount)
	}
	if len(res.Records) != 2 {
		t.Errorf("Records bucket: got %d, want 2", len(res.Records))
	}
	if strings.Contains(buf.String(), "001abc") {
		t.Errorf("manifest should not contain record id:\n%s", buf.String())
	}
}

func TestWritePackageXML_DeterministicOrder(t *testing.T) {
	// Same items in different input order should produce identical
	// output — types sorted alphabetically, members sorted within
	// each type.
	itemsA := []devproject.Item{
		{Kind: devproject.KindSObject, Ref: "Contact", Name: "Contact"},
		{Kind: devproject.KindFlow, Ref: "0F1", Name: "FlowB"},
		{Kind: devproject.KindSObject, Ref: "Account", Name: "Account"},
		{Kind: devproject.KindFlow, Ref: "0F2", Name: "FlowA"},
	}
	itemsB := []devproject.Item{
		{Kind: devproject.KindFlow, Ref: "0F2", Name: "FlowA"},
		{Kind: devproject.KindSObject, Ref: "Account", Name: "Account"},
		{Kind: devproject.KindFlow, Ref: "0F1", Name: "FlowB"},
		{Kind: devproject.KindSObject, Ref: "Contact", Name: "Contact"},
	}
	var bufA, bufB strings.Builder
	if _, err := WritePackageXML(&bufA, itemsA, PackageXMLOptions{}); err != nil {
		t.Fatalf("A: %v", err)
	}
	if _, err := WritePackageXML(&bufB, itemsB, PackageXMLOptions{}); err != nil {
		t.Fatalf("B: %v", err)
	}
	if bufA.String() != bufB.String() {
		t.Errorf("output not deterministic across input order\n--A--\n%s\n--B--\n%s", bufA.String(), bufB.String())
	}
}

func TestMetadataMember_AllSupportedKinds(t *testing.T) {
	cases := []struct {
		name     string
		item     devproject.Item
		wantType string
		wantMem  string
		wantOK   bool
	}{
		{"sObject", devproject.Item{Kind: devproject.KindSObject, Ref: "Account"}, "CustomObject", "Account", true},
		{"field", devproject.Item{Kind: devproject.KindField, Ref: "Account.Phone", Type: "Account"}, "CustomField", "Account.Phone", true},
		{"flow", devproject.Item{Kind: devproject.KindFlow, Ref: "0F1", Name: "AccountRouter"}, "Flow", "AccountRouter", true},
		// Blank-name imported flow whose Ref is the DeveloperName → use
		// the Ref (fixes the old silent-drop). Ref must NOT look like a
		// DefinitionId, or it'd be an invalid manifest member.
		{"flow blank-name devname-ref", devproject.Item{Kind: devproject.KindFlow, Ref: "Summer_School_Reinstate"}, "Flow", "Summer_School_Reinstate", true},
		// Blank-name flow whose Ref IS a DefinitionId → cannot emit (a
		// 300-id is not a valid member); honestly dropped, not guessed.
		{"flow blank-name defid-ref", devproject.Item{Kind: devproject.KindFlow, Ref: "300UE00000UDIxDYAX"}, "", "", false},
		{"flow_version", devproject.Item{Kind: devproject.KindFlowVersion, Ref: "vId", Type: "0F1"}, "", "", false}, // intentionally unsupported
		{"apex_class", devproject.Item{Kind: devproject.KindApexClass, Ref: "01p", Name: "AccountUtil"}, "ApexClass", "AccountUtil", true},
		{"apex_trigger", devproject.Item{Kind: devproject.KindApexTrigger, Ref: "01q", Name: "AccountTrigger"}, "ApexTrigger", "AccountTrigger", true},
		{"validation_rule", devproject.Item{Kind: devproject.KindValidationRule, Ref: "vrId", Type: "Account", Name: "Active"}, "ValidationRule", "Account.Active", true},
		{"record_type", devproject.Item{Kind: devproject.KindRecordType, Ref: "rtId", Type: "Account", Name: "Customer"}, "RecordType", "Account.Customer", true},
		{"lwc", devproject.Item{Kind: devproject.KindLWC, Ref: "lwcId", Type: "myCmp"}, "LightningComponentBundle", "myCmp", true},
		{"aura", devproject.Item{Kind: devproject.KindAura, Ref: "auraId", Type: "myAura"}, "AuraDefinitionBundle", "myAura", true},
		{"profile", devproject.Item{Kind: devproject.KindProfile, Ref: "pId", Name: "System Administrator"}, "Profile", "System Administrator", true},
		{"permset", devproject.Item{Kind: devproject.KindPermissionSet, Ref: "psId", Type: "Sales_User", Name: "Sales User"}, "PermissionSet", "Sales_User", true},
		{"psg", devproject.Item{Kind: devproject.KindPermissionSetGroup, Ref: "psgId", Type: "Sales_PSG", Name: "Sales PSG"}, "PermissionSetGroup", "Sales_PSG", true},
		{"queue", devproject.Item{Kind: devproject.KindQueue, Ref: "qId", Type: "Support_Queue"}, "Queue", "Support_Queue", true},
		{"public_group", devproject.Item{Kind: devproject.KindPublicGroup, Ref: "pgId", Type: "All_Sales"}, "Group", "All_Sales", true},
		{"record", devproject.Item{Kind: devproject.KindRecord, Ref: "001", Type: "Account"}, "", "", false}, // intentionally excluded
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotMem, gotOK := metadataMember(tc.item)
			if gotType != tc.wantType || gotMem != tc.wantMem || gotOK != tc.wantOK {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					gotType, gotMem, gotOK, tc.wantType, tc.wantMem, tc.wantOK)
			}
		})
	}
}

func TestSuggestedReadme_ManifestOnly(t *testing.T) {
	res := PackageXMLResult{
		IncludedCount: 5,
		Records:       []devproject.Item{{}},
		Unsupported:   []devproject.Item{{}, {}},
	}
	got := SuggestedReadme("Q2 migration", "dev@example.com", res, false)
	if !strings.Contains(got, "# Q2 migration") {
		t.Errorf("missing project header:\n%s", got)
	}
	if !strings.Contains(got, "5 component") {
		t.Errorf("missing included count:\n%s", got)
	}
	if !strings.Contains(got, "records.csv") {
		t.Errorf("missing records.csv mention:\n%s", got)
	}
	if !strings.Contains(got, "2 item(s) skipped") {
		t.Errorf("missing skipped count:\n%s", got)
	}
	if !strings.Contains(got, "sf project retrieve") {
		t.Errorf("missing sfdx instructions:\n%s", got)
	}
	if !strings.Contains(got, "dev@example.com") {
		t.Errorf("missing org context:\n%s", got)
	}
	// Manifest-only mode: README should explain the three workflows
	// (drop-in / generate / non-sfdx)
	if !strings.Contains(got, "Drop into an existing sfdx project") {
		t.Errorf("manifest-only README missing drop-in instructions:\n%s", got)
	}
	if !strings.Contains(got, "Generate a new sfdx project") {
		t.Errorf("manifest-only README missing generate instructions:\n%s", got)
	}
	// Should NOT mention sfdx-project.json (that's the full-project mode)
	if strings.Contains(got, "sfdx-project.json") {
		t.Errorf("manifest-only README shouldn't mention sfdx-project.json:\n%s", got)
	}
}

func TestSuggestedReadme_FullProject(t *testing.T) {
	res := PackageXMLResult{IncludedCount: 5}
	got := SuggestedReadme("Q2 migration", "dev@example.com", res, true)
	if !strings.Contains(got, "self-contained sfdx project") {
		t.Errorf("full-project README missing self-contained intro:\n%s", got)
	}
	if !strings.Contains(got, "sfdx-project.json") {
		t.Errorf("full-project README should mention sfdx-project.json:\n%s", got)
	}
	if !strings.Contains(got, "force-app/main/default") {
		t.Errorf("full-project README should mention force-app:\n%s", got)
	}
	// Should NOT include the manifest-only "drop into existing project"
	// instructions
	if strings.Contains(got, "Drop into an existing sfdx project") {
		t.Errorf("full-project README shouldn't have manifest-only steps:\n%s", got)
	}
}

func TestSfdxProjectJSON(t *testing.T) {
	got := SfdxProjectJSON("Q2 migration", "")
	if !strings.Contains(got, `"name": "Q2 migration"`) {
		t.Errorf("missing project name: %s", got)
	}
	if !strings.Contains(got, `"sourceApiVersion": "62.0"`) {
		t.Errorf("missing default api version: %s", got)
	}
	if !strings.Contains(got, `"force-app"`) {
		t.Errorf("missing force-app package directory: %s", got)
	}

	// Empty project name falls back to a sensible default.
	got = SfdxProjectJSON("", "63.0")
	if !strings.Contains(got, `"name": "sf-deck-export"`) {
		t.Errorf("missing default project name: %s", got)
	}
	if !strings.Contains(got, `"sourceApiVersion": "63.0"`) {
		t.Errorf("explicit api version not respected: %s", got)
	}
}
