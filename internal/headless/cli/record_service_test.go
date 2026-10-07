package cli

import (
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/app"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/services/records"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type cliRecordRemote struct {
	createCalls int
	target      string
	sobject     string
}

func (*cliRecordRemote) ResolveSObject(string, string) (string, error) { return "Account", nil }
func (r *cliRecordRemote) Create(target, sobject string, fields map[string]any) ([]sf.FieldError, string, error) {
	r.createCalls++
	r.target, r.sobject = target, sobject
	return nil, "001-new", nil
}
func (*cliRecordRemote) Update(string, string, string, map[string]any) ([]sf.FieldError, error) {
	return nil, nil
}
func (*cliRecordRemote) Delete(string, string, string) error { return nil }

func TestRecordCreateUsesInjectedService(t *testing.T) {
	remote := &cliRecordRemote{}
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyRecords })
	a := &app.App{Records: records.NewWithRemote(gate, remote)}
	code, got := runWriteCLI(t, a, "--json", "record", "create",
		"--org", "input", "--object", "Account", "--field", "Name=Acme")
	if code != 0 || got["ok"] != true || got["org"] != "u@example.com" || got["target"] != "resolved" {
		t.Fatalf("code=%d response=%#v", code, got)
	}
	data, _ := got["data"].(map[string]any)
	if data["id"] != "001-new" || remote.createCalls != 1 || remote.target != "resolved" || remote.sobject != "Account" {
		t.Fatalf("data=%#v remote=%#v", data, remote)
	}
}
