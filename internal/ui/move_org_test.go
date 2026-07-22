package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestMovableKind(t *testing.T) {
	movable := []devproject.ItemKind{
		devproject.KindSObject, devproject.KindField, devproject.KindFlow,
		devproject.KindApexClass, devproject.KindLWC,
	}
	for _, k := range movable {
		if !movableKind(k) {
			t.Errorf("expected %q to be movable", k)
		}
	}
	// Records are Id-keyed and must NOT be movable (Ids don't map
	// across orgs). Flow versions and local snippets likewise.
	notMovable := []devproject.ItemKind{
		devproject.KindRecord, devproject.KindFlowVersion,
		devproject.KindApexSnippet, devproject.KindSOQLQuery,
		// Fast-follow kinds — not in v1 scope.
		devproject.KindApexTrigger, devproject.KindAura,
		devproject.KindPermissionSet, devproject.KindQueue,
	}
	for _, k := range notMovable {
		if movableKind(k) {
			t.Errorf("expected %q to NOT be movable in v1", k)
		}
	}
}

func TestMoveNameOf(t *testing.T) {
	cases := []struct {
		it       ItemIdentity
		wantName string
		wantHint string
	}{
		{ItemIdentity{Kind: devproject.KindSObject, Ref: "Account", Label: "Account"}, "Account", ""},
		{ItemIdentity{Kind: devproject.KindField, Ref: "Account.Industry", Label: "Account.Industry"}, "Account.Industry", "Account"},
		// Id-keyed: the developer name lives in Label, Ref is the org-local Id.
		{ItemIdentity{Kind: devproject.KindFlow, Ref: "301xx0", Label: "My_Flow"}, "My_Flow", ""},
		{ItemIdentity{Kind: devproject.KindApexClass, Ref: "01pxx0", Label: "AccountService"}, "AccountService", ""},
		{ItemIdentity{Kind: devproject.KindLWC, Ref: "0Rbxx0", Label: "myComponent"}, "myComponent", ""},
	}
	for _, c := range cases {
		name, hint := moveNameOf(c.it)
		if name != c.wantName || hint != c.wantHint {
			t.Errorf("moveNameOf(%v) = (%q,%q), want (%q,%q)", c.it.Kind, name, hint, c.wantName, c.wantHint)
		}
	}
}

func TestResolveMoveRef_APINameKinds(t *testing.T) {
	d := &orgData{}
	// Before the object list loads, sObject/field must report not-ready
	// (so callers WAIT rather than concluding "not found" and switching
	// the user into an org that may not have the resource).
	if _, _, ready := resolveMoveRef(d, devproject.KindSObject, "Account", ""); ready {
		t.Error("sObject before object-list load should be not-ready")
	}

	// Seed the target org's browseable object list.
	d.SObjects.Set([]sf.SObject{{Name: "Account"}, {Name: "Contact"}})

	// sObject present → matched, ready.
	ref, found, ready := resolveMoveRef(d, devproject.KindSObject, "account", "")
	if ref != "account" || !found || !ready {
		t.Errorf("sObject present: got (%q,%v,%v), want (account,true,true)", ref, found, ready)
	}
	// sObject absent → NOT found, but ready (so caller flashes + stays put).
	if ref, found, ready := resolveMoveRef(d, devproject.KindSObject, "Widget__c", ""); found || !ready || ref != "" {
		t.Errorf("sObject absent: got (%q,%v,%v), want (\"\",false,true)", ref, found, ready)
	}

	// field: parent object present → matched.
	ref, found, ready = resolveMoveRef(d, devproject.KindField, "Account.Industry", "Account")
	if ref != "Account.Industry" || !found || !ready {
		t.Errorf("field present: got (%q,%v,%v), want (Account.Industry,true,true)", ref, found, ready)
	}
	// field: parent object absent → NOT found.
	if _, found, ready := resolveMoveRef(d, devproject.KindField, "Widget__c.Name", "Widget__c"); found || !ready {
		t.Errorf("field absent parent: found=%v ready=%v, want found=false ready=true", found, ready)
	}
}

func TestResolveMoveRef_ListNotReady(t *testing.T) {
	d := &orgData{} // Flows resource never fetched.
	ref, found, ready := resolveMoveRef(d, devproject.KindFlow, "My_Flow", "")
	if ready {
		t.Errorf("flow before fetch should report ready=false, got ready=true (ref=%q found=%v)", ref, found)
	}
}

// TestResolvePendingMove_MissDoesNotSwitch is the core guarantee: when
// the resource is absent in the target org, resolvePendingMove must NOT
// change the active org — it flashes and clears the pending move.
func TestResolvePendingMove_MissDoesNotSwitch(t *testing.T) {
	target := &orgData{}
	target.Flows.Set([]sf.Flow{{DefinitionID: "301A", DeveloperName: "Alpha_Flow"}}) // loaded, but no "Ghost_Flow"

	m := &Model{
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0, // u@active
			data:     map[string]*orgData{"u@other": target},
		},
	}
	m.move = &pendingMove{
		kind:   devproject.KindFlow,
		name:   "Ghost_Flow",
		label:  "Ghost_Flow",
		target: "u@other",
	}
	cmd := m.resolvePendingMove()
	if cmd != nil {
		t.Error("miss should not return a nav command")
	}
	if m.selected != 0 {
		t.Errorf("miss switched the active org to index %d; must stay on 0", m.selected)
	}
	if m.move != nil {
		t.Error("miss should clear the pending move")
	}
}

// TestResolvePendingMove_NotReadyWaits: while the target list is still
// loading, resolve is a no-op and the move stays armed for a retry.
func TestResolvePendingMove_NotReadyWaits(t *testing.T) {
	target := &orgData{} // Flows never fetched → not ready
	m := &Model{
		modelOrgs: modelOrgs{
			orgs:     []sf.Org{{Username: "u@active"}, {Username: "u@other"}},
			selected: 0,
			data:     map[string]*orgData{"u@other": target},
		},
	}
	m.move = &pendingMove{kind: devproject.KindFlow, name: "Any", label: "Any", target: "u@other"}
	if cmd := m.resolvePendingMove(); cmd != nil {
		t.Error("not-ready should return nil")
	}
	if m.move == nil {
		t.Error("not-ready must keep the move armed for a retry")
	}
	if m.selected != 0 {
		t.Errorf("not-ready must not switch orgs (selected=%d)", m.selected)
	}
}

func TestResolveMoveRef_FlowMatchByName(t *testing.T) {
	d := &orgData{}
	// Seed a fetched Flows resource so FetchedAt() is non-zero, then
	// mirror it into FlowList the way SyncFlowsList does.
	d.Flows.Set([]sf.Flow{
		{DefinitionID: "301A", DeveloperName: "Alpha_Flow"},
		{DefinitionID: "301B", DeveloperName: "Beta_Flow"},
	})
	d.FlowList.Set(d.Flows.Value())

	ref, found, ready := resolveMoveRef(d, devproject.KindFlow, "beta_flow", "")
	if !ready || !found || ref != "301B" {
		t.Errorf("flow match: got (%q,%v,%v), want (301B,true,true)", ref, found, ready)
	}

	ref, found, ready = resolveMoveRef(d, devproject.KindFlow, "Missing_Flow", "")
	if !ready || found || ref != "" {
		t.Errorf("flow miss: got (%q,%v,%v), want (\"\",false,true)", ref, found, ready)
	}
}
