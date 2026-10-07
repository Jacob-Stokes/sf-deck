package cli

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/services/notificationops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type cliNotificationRemote struct {
	calls  int
	target string
	id     string
}

func (r *cliNotificationRemote) MarkRead(target, id string) error {
	r.calls++
	r.target, r.id = target, id
	return nil
}
func (*cliNotificationRemote) MarkAllRead(string) error { return nil }

func TestNotificationMarkReadUsesInjectedService(t *testing.T) {
	remote := &cliNotificationRemote{}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyRecords })
	a := &app.App{Notifications: notificationops.NewWithRemote(gate, remote)}
	code, got := runNotifCLI(t, a, "--json", "notification", "mark-read",
		"--org", "input", "--id", "notif")
	if code != 0 || got["ok"] != true || got["org"] != "u@example.com" || got["target"] != "resolved" {
		t.Fatalf("code=%d response=%#v", code, got)
	}
	if remote.calls != 1 || remote.target != "resolved" || remote.id != "notif" {
		t.Fatalf("remote=%#v", remote)
	}
}
