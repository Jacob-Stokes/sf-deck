package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/apexops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type uiApexRemote struct {
	calls  int
	target string
	body   string
	result sf.ExecuteAnonymousResult
}

func (r *uiApexRemote) Execute(target, body string) (sf.ExecuteAnonymousResult, error) {
	r.calls++
	r.target, r.body = target, body
	return r.result, nil
}
func (*uiApexRemote) EnsureTraceFlag(string, string) error { return nil }
func (*uiApexRemote) FetchLatestLog(string, string, time.Time) (string, string, error) {
	return "", "", nil
}

func uiApexService(level settings.SafetyLevel, remote apexops.Remote) *apexops.Service {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return apexops.NewWithRemote(gate, remote)
}

func TestControlApexUsesSharedFullGate(t *testing.T) {
	remote := &uiApexRemote{}
	service := uiApexService(settings.SafetyMetadata, remote)
	resolve := func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}
	safety := func(sf.Org) settings.SafetyLevel { return settings.SafetyMetadata }
	s := NewControlState(nil, resolve, safety, &settings.Settings{}, func() error { return nil },
		ControlServices{Apex: service})
	_, err := s.ApexRun(control.ApexRunArgs{OrgAlias: "input", Body: "System.debug('x');"})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
		t.Fatalf("err = %#v, want full BlockedError", err)
	}
	type coded interface{ Code() string }
	var c coded
	if !errors.As(err, &c) || c.Code() != control.ErrSafetyBlocked {
		t.Fatalf("coded error = %#v", err)
	}
	if remote.calls != 0 {
		t.Fatalf("remote called despite denial: %d", remote.calls)
	}
}

func TestRunExecCmdUsesInjectedService(t *testing.T) {
	remote := &uiApexRemote{result: sf.ExecuteAnonymousResult{Compiled: true, Success: true}}
	service := uiApexService(settings.SafetyFull, remote)
	m := Model{modelServices: modelServices{settings: &settings.Settings{}, apex: service}}
	o := sf.Org{Alias: "input", Username: "original@example.com", IsScratch: true}
	msg, ok := m.runExecCmd(o, "System.debug('x');", false, "")().(execResultMsg)
	if !ok {
		t.Fatalf("message type = %T, want execResultMsg", msg)
	}
	if msg.err != nil || !msg.data.Success || msg.orgUser != "u@example.com" {
		t.Fatalf("message = %#v", msg)
	}
	if remote.calls != 1 || remote.target != "resolved" || remote.body != "System.debug('x');" {
		t.Fatalf("remote calls=%d target=%q body=%q", remote.calls, remote.target, remote.body)
	}
}
