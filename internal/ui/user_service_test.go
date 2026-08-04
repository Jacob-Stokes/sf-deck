package ui

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/userops"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type uiUserRemote struct {
	calls  int
	target string
	active bool
}

func (*uiUserRemote) ResetPassword(string, string) error { return nil }
func (*uiUserRemote) GenerateResetLink(string, string) (string, error) {
	return "https://example.test/reset", nil
}
func (r *uiUserRemote) SetActive(target, userID string, active bool) error {
	r.calls++
	r.target, r.active = target, active
	return nil
}
func (*uiUserRemote) SetFrozen(string, string, bool) error { return nil }

func TestUserSetActiveCmdUsesInjectedService(t *testing.T) {
	remote := &uiUserRemote{}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "admin@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyFull })
	m := Model{modelServices: modelServices{users: userops.NewWithRemote(gate, remote)}}
	msg, ok := userSetActiveCmd(&m, "input", "005-user", true)().(userActionDoneMsg)
	if !ok || msg.Err != nil || msg.Action != "activate" {
		t.Fatalf("message=%#v", msg)
	}
	if remote.calls != 1 || remote.target != "resolved" || !remote.active {
		t.Fatalf("remote=%#v", remote)
	}
}
