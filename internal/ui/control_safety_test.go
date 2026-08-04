package ui

import (
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestControlSafetyClearCannotRaiseScratchOrg(t *testing.T) {
	org := sf.Org{Alias: "scratch", Username: "scratch@example.com", IsScratch: true}
	st := &settings.Settings{}
	st.SetOrg(org.Username, settings.SafetyReadOnly, false)
	resolve := func(string) (sf.Org, error) { return org, nil }
	safety := func(o sf.Org) settings.SafetyLevel {
		return st.Resolve(o.Username, settings.OrgKind(o.Kind()), o.Alias)
	}
	state := NewControlState(nil, resolve, safety, st, func() error { return nil })

	_, err := state.OrgSafetySet(control.OrgSafetySetArgs{OrgAlias: org.Alias, Clear: true})
	type coded interface{ Code() string }
	var c coded
	if !errors.As(err, &c) || c.Code() != control.ErrSafetyBlocked {
		t.Fatalf("err = %#v, want safety_blocked", err)
	}
	if got := safety(org); got != settings.SafetyReadOnly {
		t.Fatalf("safety changed after rejected clear: %v", got)
	}
}

func TestControlSafetySaveFailureRollsBackMemory(t *testing.T) {
	org := sf.Org{Alias: "sandbox", Username: "sandbox@example.com", IsSandbox: true}
	st := &settings.Settings{}
	st.SetOrg(org.Username, settings.SafetyMetadata, false)
	resolve := func(string) (sf.Org, error) { return org, nil }
	safety := func(o sf.Org) settings.SafetyLevel {
		return st.Resolve(o.Username, settings.OrgKind(o.Kind()), o.Alias)
	}
	state := NewControlState(nil, resolve, safety, st, func() error { return errors.New("disk changed") })

	_, err := state.OrgSafetySet(control.OrgSafetySetArgs{OrgAlias: org.Alias, Level: "records"})
	if err == nil {
		t.Fatal("save failure was not returned")
	}
	if got := safety(org); got != settings.SafetyMetadata {
		t.Fatalf("safety after failed save = %v, want metadata rollback", got)
	}
}
