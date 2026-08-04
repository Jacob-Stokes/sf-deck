package sf

import "testing"

func yankByID(ts []YankTarget, id string) (YankTarget, bool) {
	for _, t := range ts {
		if t.ID == id {
			return t, true
		}
	}
	return YankTarget{}, false
}

func TestSObjectYankTargets(t *testing.T) {
	so := SObject{Name: "Account", Label: "Account", KeyPrefix: "001"}
	ts := so.YankTargets()
	if got, ok := yankByID(ts, "api"); !ok || got.Value != "Account" {
		t.Errorf("api target = %+v, ok=%v", got, ok)
	}
	if got, ok := yankByID(ts, "soql"); !ok || got.Value != "SELECT Id FROM Account" {
		t.Errorf("soql target = %+v, ok=%v", got, ok)
	}
	if got, ok := yankByID(ts, "prefix"); !ok || got.Value != "001" {
		t.Errorf("prefix target = %+v, ok=%v", got, ok)
	}
	// Label equal to Name is omitted (no redundant entry).
	plain := SObject{Name: "Account", Label: "Account"}
	if _, ok := yankByID(plain.YankTargets(), "label"); ok {
		t.Error("label should be omitted when equal to Name")
	}
	// A distinct label is included.
	custom := SObject{Name: "Application__c", Label: "Application"}
	if got, ok := yankByID(custom.YankTargets(), "label"); !ok || got.Value != "Application" {
		t.Errorf("distinct label target = %+v, ok=%v", got, ok)
	}
}

func TestFieldRefYankTargets(t *testing.T) {
	fr := FieldRef{SObjectName: "Account", Field: Field{Name: "Industry", Label: "Industry"}}
	ts := fr.YankTargets()
	if got, ok := yankByID(ts, "api"); !ok || got.Value != "Industry" {
		t.Errorf("api = %+v", got)
	}
	if got, ok := yankByID(ts, "qualified"); !ok || got.Value != "Account.Industry" {
		t.Errorf("qualified = %+v", got)
	}
}

func TestRecordRefYankTargets(t *testing.T) {
	rec := RecordRef{Record: map[string]any{
		"attributes": map[string]any{"type": "Account"},
		"Id":         "001AAA",
		"Name":       "Acme Corp",
	}}
	ts := rec.YankTargets()
	if got, ok := yankByID(ts, "id"); !ok || got.Value != "001AAA" {
		t.Errorf("id = %+v", got)
	}
	if got, ok := yankByID(ts, "name"); !ok || got.Value != "Acme Corp" {
		t.Errorf("name = %+v", got)
	}
	if got, ok := yankByID(ts, "soql"); !ok || got.Value != "SELECT Id FROM Account WHERE Id = '001AAA'" {
		t.Errorf("soql = %+v", got)
	}

	// A record with no Name field omits the Name target rather than
	// yielding an empty one.
	noName := RecordRef{Record: map[string]any{
		"attributes": map[string]any{"type": "Case"},
		"Id":         "500AAA",
	}}
	if _, ok := yankByID(noName.YankTargets(), "name"); ok {
		t.Error("Name target should be omitted when the record has no Name")
	}
}

func TestFlowYankTargets(t *testing.T) {
	f := Flow{
		DefinitionID: "301AAA", DeveloperName: "My_Flow", MasterLabel: "My Flow",
		ActiveVersionID: "300AAA",
	}
	ts := f.YankTargets()
	if got, ok := yankByID(ts, "api"); !ok || got.Value != "My_Flow" {
		t.Errorf("api = %+v", got)
	}
	if got, ok := yankByID(ts, "defid"); !ok || got.Value != "301AAA" {
		t.Errorf("defid = %+v", got)
	}
	if got, ok := yankByID(ts, "verid"); !ok || got.Value != "300AAA" {
		t.Errorf("active verid = %+v", got)
	}
	// Falls back to latest version when there's no active one.
	inactive := Flow{DefinitionID: "301B", DeveloperName: "F2", LatestVersionID: "300B"}
	if got, ok := yankByID(inactive.YankTargets(), "verid"); !ok || got.Value != "300B" {
		t.Errorf("latest verid = %+v", got)
	}
}

func TestDeployRowYankTargets(t *testing.T) {
	d := DeployRow{ID: "0AfAAA"}
	ts := d.YankTargets()
	if got, ok := yankByID(ts, "id"); !ok || got.Value != "0AfAAA" {
		t.Errorf("id = %+v", got)
	}
	if got, ok := yankByID(ts, "report"); !ok || got.Value != "sf project deploy report --job-id 0AfAAA" {
		t.Errorf("report cmd = %+v", got)
	}
	empty := DeployRow{}
	if empty.YankTargets() != nil {
		t.Error("empty DeployRow should yield no yank targets")
	}
}

func TestInstalledPackageYankTargets(t *testing.T) {
	p := InstalledPackage{
		SubscriberPackageNamespace:     "acme",
		SubscriberPackageVersionNumber: "1.2.3",
		SubscriberPackageVersionID:     "04tAAA",
	}
	ts := p.YankTargets()
	if got, ok := yankByID(ts, "ns"); !ok || got.Value != "acme" {
		t.Errorf("ns = %+v", got)
	}
	if got, ok := yankByID(ts, "ver"); !ok || got.Value != "1.2.3" {
		t.Errorf("ver = %+v", got)
	}
}
