package ui

import (
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type uiRecordRemote struct {
	calls  int
	target string
}

func (*uiRecordRemote) ResolveSObject(string, string) (string, error) { return "Account", nil }
func (r *uiRecordRemote) Create(target, sobject string, fields map[string]any) ([]sf.FieldError, string, error) {
	r.calls++
	r.target = target
	return nil, "001-new", nil
}
func (*uiRecordRemote) Update(string, string, string, map[string]any) ([]sf.FieldError, error) {
	return nil, nil
}
func (*uiRecordRemote) Delete(string, string, string) error { return nil }

func TestControlRecordCreateUsesSharedGateAndService(t *testing.T) {
	remote := &uiRecordRemote{}
	level := settings.SafetyReadOnly
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	service := records.NewWithRemote(gate, remote)
	state := NewControlState(nil, nil, nil, &settings.Settings{}, func() error { return nil },
		ControlServices{Records: service})

	_, err := state.RecordCreate(control.RecordCreateArgs{
		OrgAlias: "input", SObject: "Account", Fields: map[string]any{"Name": "Acme"},
	})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || remote.calls != 0 {
		t.Fatalf("err=%#v remote calls=%d", err, remote.calls)
	}
	type coded interface{ Code() string }
	var c coded
	if !errors.As(err, &c) || c.Code() != control.ErrSafetyBlocked {
		t.Fatalf("coded error = %#v", err)
	}

	level = settings.SafetyRecords
	got, err := state.RecordCreate(control.RecordCreateArgs{
		OrgAlias: "input", SObject: "Account", Fields: map[string]any{"Name": "Acme"},
	})
	if err != nil || remote.calls != 1 || remote.target != "resolved" {
		t.Fatalf("result=%#v err=%v remote=%#v", got, err, remote)
	}
}
