package userops

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
	url    string
}

func (f *fakeRemote) ResetPassword(target, userID string) error {
	f.calls, f.target = append(f.calls, "reset"), target
	return nil
}
func (f *fakeRemote) GenerateResetLink(target, userID string) (string, error) {
	f.calls, f.target = append(f.calls, "link"), target
	return f.url, nil
}
func (f *fakeRemote) SetActive(target, userID string, active bool) error {
	f.calls, f.target = append(f.calls, "active"), target
	return nil
}
func (f *fakeRemote) SetFrozen(target, userID string, frozen bool) error {
	f.calls, f.target = append(f.calls, "frozen"), target
	return nil
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "admin@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(gate, remote)
}

func TestEveryOperationRequiresFullBeforeRemote(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Service) error
	}{
		{"reset", func(s *Service) error {
			_, err := s.ResetPassword(context.Background(), Input{UserID: "005"})
			return err
		}},
		{"link", func(s *Service) error {
			_, err := s.GenerateResetLink(context.Background(), Input{UserID: "005"})
			return err
		}},
		{"active", func(s *Service) error {
			_, err := s.SetActive(context.Background(), Input{UserID: "005"}, true)
			return err
		}},
		{"frozen", func(s *Service) error {
			_, err := s.SetFrozen(context.Background(), Input{UserID: "005"}, true)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remote := &fakeRemote{url: "https://example.test/reset"}
			err := tc.run(serviceAt(settings.SafetyMetadata, remote))
			var blocked orgwrite.BlockedError
			if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
				t.Fatalf("err=%#v, want full denial", err)
			}
			if len(remote.calls) != 0 {
				t.Fatalf("remote called on denial: %v", remote.calls)
			}
		})
	}
}

func TestOperationsUseResolvedTarget(t *testing.T) {
	remote := &fakeRemote{url: "https://example.test/reset"}
	service := serviceAt(settings.SafetyFull, remote)
	if _, err := service.ResetPassword(context.Background(), Input{Target: "input", UserID: "005"}); err != nil || remote.target != "resolved" {
		t.Fatalf("reset err=%v remote=%#v", err, remote)
	}
	result, err := service.GenerateResetLink(context.Background(), Input{Target: "input", UserID: "005"})
	if err != nil || result.URL != remote.url || result.Target.Username != "admin@example.com" {
		t.Fatalf("link result=%#v err=%v", result, err)
	}
	if _, err := service.SetActive(context.Background(), Input{UserID: "005"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFrozen(context.Background(), Input{UserID: "005"}, true); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	if _, err := serviceAt(settings.SafetyFull, &fakeRemote{}).ResetPassword(context.Background(), Input{}); err == nil {
		t.Fatal("empty user id accepted")
	}
	valid := Input{UserID: "005"}
	for _, service := range []*Service{nil, NewWithRemote(nil, &fakeRemote{}), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyFull }), nil)} {
		if _, err := service.ResetPassword(context.Background(), valid); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
	if _, err := serviceAt(settings.SafetyFull, &fakeRemote{}).GenerateResetLink(context.Background(), valid); err == nil {
		t.Fatal("empty reset link accepted")
	}
}
