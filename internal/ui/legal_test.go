package ui

import (
	"testing"
	"time"

	productlegal "github.com/Jacob-Stokes/sf-deck/internal/legal"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
)

func TestLegalTriggerBlocksRealOrgStartupUntilAccepted(t *testing.T) {
	oldDemo := Demo
	Demo = false
	t.Cleanup(func() { Demo = oldDemo })

	st := &settings.Settings{}
	m := Model{modelServices: modelServices{settings: st}}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("fresh real-org launch must request acknowledgement")
	}
	if _, ok := cmd().(legalModalMsg); !ok {
		t.Fatalf("trigger returned %T", cmd())
	}

	st.AcceptLegal(productlegal.PolicyVersion, time.Now())
	if cmd := m.legalTriggerCmd(); cmd != nil {
		t.Fatal("accepted current revision should not prompt")
	}
}

func TestLegalTriggerSkipsOfflineDemo(t *testing.T) {
	oldDemo := Demo
	Demo = true
	t.Cleanup(func() { Demo = oldDemo })
	m := Model{modelServices: modelServices{settings: &settings.Settings{}}}
	if cmd := m.legalTriggerCmd(); cmd != nil {
		t.Fatal("demo cannot contact Salesforce and should not require acceptance")
	}
}

func TestRecordShapedResourcesAreMemoryOnly(t *testing.T) {
	d := newOrgData("user@example.com", "dev", nil, &settings.Settings{})
	if !d.RecentlyViewed.NoCache {
		t.Fatal("global RecentlyViewed rows must be memory-only")
	}
	if r := d.EnsureRecords("dev", "Account"); r == nil || !r.NoCache {
		t.Fatal("record lists must be memory-only")
	}
	if r := d.EnsureRecordDetail("dev", "Account", "001000000000001"); r == nil || !r.NoCache {
		t.Fatal("record details must be memory-only")
	}
	if r := d.EnsureReportRun("dev", "00O000000000001"); r == nil || !r.NoCache {
		t.Fatal("report results must be memory-only")
	}
}
