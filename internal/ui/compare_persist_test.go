package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/diff"
)

func TestSerializeDeserializeCompareRun(t *testing.T) {
	run := &compareRun{
		Source: orgEndpoint("acme-test"),
		Target: orgEndpoint("acme-dev"),
		Scope:  []string{"ApexClass", "Flow"},
		Method: compareMethodAuto,
		snapA: diff.Snapshot{
			"ApexClass": {"Foo": "class Foo {}", "Bar": "class Bar {}"},
		},
		snapB: diff.Snapshot{
			"ApexClass": {"Foo": "class Foo { /*changed*/ }"},
		},
		Inv: diff.Inventory{
			Rows: []diff.Row{
				{Type: "ApexClass", Key: "Foo", Status: diff.StatusDifferent, AID: "Foo", BID: "Foo"},
				{Type: "ApexClass", Key: "Bar", Status: diff.StatusAOnly, AID: "Bar"},
			},
		},
	}

	blob, err := serializeCompareRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) == 0 {
		t.Fatal("empty blob")
	}

	// Round-trip through a SavedComparison shell.
	sc := devproject.SavedComparison{
		Source: "acme-test", Target: "acme-dev",
		Scope: "ApexClass, Flow", Method: "Auto", Blob: blob,
	}
	got, err := deserializeCompareRun(sc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != comparePhaseInventory {
		t.Errorf("phase = %v, want inventory", got.Phase)
	}
	if got.Source.OrgRef() != "acme-test" || got.Target.OrgRef() != "acme-dev" {
		t.Errorf("endpoints lost: %+v %+v", got.Source, got.Target)
	}
	if len(got.Scope) != 2 || got.Scope[0] != "ApexClass" || got.Scope[1] != "Flow" {
		t.Errorf("scope lost: %v", got.Scope)
	}
	if got.Method != compareMethodAuto {
		t.Errorf("method lost: %v", got.Method)
	}
	if len(got.Inv.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(got.Inv.Rows))
	}
	// Snapshots preserved → drill-in diff works offline.
	if got.snapA["ApexClass"]["Foo"] != "class Foo {}" {
		t.Errorf("snapA Foo lost: %q", got.snapA["ApexClass"]["Foo"])
	}
	res := diff.BodyDiffFromSnapshots(got.Inv.Rows[0], got.snapA, got.snapB)
	if !res.Changed() {
		t.Error("offline body diff should show a change for Foo")
	}
}

func TestSplitScope(t *testing.T) {
	if got := splitScope("ApexClass, Flow ,  RecordType"); len(got) != 3 || got[2] != "RecordType" {
		t.Errorf("splitScope = %v", got)
	}
	if splitScope("") != nil {
		t.Error("empty scope should be nil")
	}
}
