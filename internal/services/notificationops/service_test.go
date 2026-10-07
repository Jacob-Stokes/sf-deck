package notificationops

import (
	"context"
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeRemote struct {
	calls  []string
	target string
	id     string
}

func (f *fakeRemote) MarkRead(target, id string) error {
	f.calls, f.target, f.id = append(f.calls, "one"), target, id
	return nil
}
func (f *fakeRemote) MarkAllRead(target string) error {
	f.calls, f.target = append(f.calls, "all"), target
	return nil
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(gate, remote)
}

func TestMarkReadRequiresRecordSafetyBeforeRemote(t *testing.T) {
	remote := &fakeRemote{}
	_, err := serviceAt(settings.SafetyReadOnly, remote).MarkRead(context.Background(), MarkReadInput{ID: "notif"})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteRecord {
		t.Fatalf("err=%#v, want record denial", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called on denial: %v", remote.calls)
	}
}

func TestMarkOneAndAllUseResolvedTarget(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyRecords, remote)
	result, err := service.MarkRead(context.Background(), MarkReadInput{Target: "input", ID: "notif"})
	if err != nil || result.Target.CLIArg != "resolved" || remote.target != "resolved" || remote.id != "notif" {
		t.Fatalf("one result=%#v err=%v remote=%#v", result, err, remote)
	}
	result, err = service.MarkRead(context.Background(), MarkReadInput{Target: "input", All: true})
	if err != nil || !result.All || remote.calls[len(remote.calls)-1] != "all" || remote.target != "resolved" {
		t.Fatalf("all result=%#v err=%v remote=%#v", result, err, remote)
	}
}

func TestInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	service := serviceAt(settings.SafetyRecords, &fakeRemote{})
	for _, in := range []MarkReadInput{{}, {ID: "x", All: true}} {
		if _, err := service.MarkRead(context.Background(), in); err == nil {
			t.Fatalf("invalid input accepted: %#v", in)
		}
	}
	valid := MarkReadInput{ID: "x"}
	for _, service := range []*Service{nil, NewWithRemote(nil, &fakeRemote{}), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyRecords }), nil)} {
		if _, err := service.MarkRead(context.Background(), valid); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
}
