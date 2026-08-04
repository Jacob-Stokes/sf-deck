package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// These tests lock in the per-org safety gate on the TUI's two
// highest-risk write paths (record-edit save and anonymous-Apex exec).
// Both bypassed the gate before — record save checked only field-level
// Updateable, and exec branched on "is this prod?" instead of the
// WriteAnonymous level — so a read-only org accepted writes and a
// sandbox (which resolves to `records`, not `full`) ran arbitrary Apex.
// The gate is m.canWriteOrg(org, kind); these assert its verdict for the
// org-kind / level combinations those paths rely on.

func safetyGateModel(orgs []sf.Org, overrides map[string]settings.SafetyLevel) Model {
	st := &settings.Settings{}
	for user, lvl := range overrides {
		// Resolve honors per-username overrides written to settings.toml.
		st.SetOrg(user, lvl, false)
	}
	return Model{
		modelOrgs:     modelOrgs{orgs: orgs, selected: 0},
		modelServices: modelServices{settings: st},
	}
}

func TestRecordEditSaveGate(t *testing.T) {
	prod := sf.Org{Alias: "prod", Username: "boss@example.com"} // not sandbox → read_only default
	sandbox := sf.Org{Alias: "sbx", Username: "qa@example.com.sbx", IsSandbox: true}

	// Production at its default (read_only) must NOT allow a record write.
	m := safetyGateModel([]sf.Org{prod}, nil)
	if ok, _ := m.canWriteOrg(prod, settings.WriteRecord); ok {
		t.Error("read-only production allowed a record write — gate bypassed")
	}

	// A sandbox (defaults to `records`) DOES allow record DML.
	m = safetyGateModel([]sf.Org{sandbox}, nil)
	if ok, reason := m.canWriteOrg(sandbox, settings.WriteRecord); !ok {
		t.Errorf("sandbox should allow record write, got blocked: %q", reason)
	}

	// Production explicitly raised to `records` allows it; lowering it
	// back to read_only blocks it again.
	m = safetyGateModel([]sf.Org{prod}, map[string]settings.SafetyLevel{prod.Username: settings.SafetyRecords})
	if ok, reason := m.canWriteOrg(prod, settings.WriteRecord); !ok {
		t.Errorf("production at records should allow record write: %q", reason)
	}
}

func TestAnonymousApexGate(t *testing.T) {
	// The C2 regression: a sandbox resolves to `records`, which does NOT
	// permit anonymous Apex (needs `full`). The old code ran it anyway
	// because it only asked "is this prod?".
	sandbox := sf.Org{Alias: "sbx", Username: "qa@example.com.sbx", IsSandbox: true}
	m := safetyGateModel([]sf.Org{sandbox}, nil)
	if ok, _ := m.canWriteOrg(sandbox, settings.WriteAnonymous); ok {
		t.Error("sandbox at records allowed anonymous Apex — needs full")
	}

	// Only `full` unlocks anonymous Apex.
	m = safetyGateModel([]sf.Org{sandbox}, map[string]settings.SafetyLevel{sandbox.Username: settings.SafetyFull})
	if ok, reason := m.canWriteOrg(sandbox, settings.WriteAnonymous); !ok {
		t.Errorf("org at full should allow anonymous Apex: %q", reason)
	}

	// Production left at read_only must never run anon Apex, confirmation
	// modal or not.
	prod := sf.Org{Alias: "prod", Username: "boss@example.com"}
	m = safetyGateModel([]sf.Org{prod}, nil)
	if ok, _ := m.canWriteOrg(prod, settings.WriteAnonymous); ok {
		t.Error("read-only production allowed anonymous Apex — gate bypassed")
	}
}

// TestBundleMetadataGate covers the gate that startBundleValidate /
// startBundleDeploy share: a deploy/validate against a read-only org
// must be blocked. Validate doesn't commit but is still a privileged
// WriteMetadata op, so it's gated the same as deploy.
func TestBundleMetadataGate(t *testing.T) {
	prod := sf.Org{Alias: "prod", Username: "boss@example.com"} // read_only default
	sandbox := sf.Org{Alias: "sbx", Username: "qa@example.com.sbx", IsSandbox: true}

	// Read-only production blocks metadata deploy/validate.
	m := safetyGateModel([]sf.Org{prod}, nil)
	if ok, _ := m.canWriteOrg(prod, settings.WriteMetadata); ok {
		t.Error("read-only production allowed a metadata deploy/validate — gate bypassed")
	}

	// A sandbox defaults to `records`, which does NOT include metadata —
	// so deploy/validate is blocked there too until raised to `metadata`.
	m = safetyGateModel([]sf.Org{sandbox}, nil)
	if ok, _ := m.canWriteOrg(sandbox, settings.WriteMetadata); ok {
		t.Error("sandbox at records allowed a metadata deploy/validate — needs metadata")
	}
	m = safetyGateModel([]sf.Org{sandbox}, map[string]settings.SafetyLevel{sandbox.Username: settings.SafetyMetadata})
	if ok, reason := m.canWriteOrg(sandbox, settings.WriteMetadata); !ok {
		t.Errorf("org at metadata should allow deploy/validate: %q", reason)
	}
}

func TestFlowCommitRechecksCurrentSafety(t *testing.T) {
	o := sf.Org{Alias: "sandbox", Username: "sandbox@example.com", IsSandbox: true}
	m := safetyGateModel([]sf.Org{o}, map[string]settings.SafetyLevel{o.Username: settings.SafetyMetadata})
	if err := requireFlowMetadataWrite(m, o); err != nil {
		t.Fatalf("metadata safety unexpectedly blocked: %v", err)
	}
	// The modal captures Model by value, but Settings is shared by pointer.
	// Lowering over IPC while the modal is open must therefore block Save.
	m.settings.SetOrg(o.Username, settings.SafetyReadOnly, false)
	if err := requireFlowMetadataWrite(m, o); err == nil {
		t.Fatal("flow commit accepted a stale pre-modal safety decision")
	}
}
