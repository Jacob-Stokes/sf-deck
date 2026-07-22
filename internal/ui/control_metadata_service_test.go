package ui

import (
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/control"
	"github.com/Jacob-Stokes/sf-deck/internal/services/metadataops"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type controlMetadataRemote struct {
	calls  []string
	target string
}

func (r *controlMetadataRemote) Create(target, metadataType, fullName string, metadata map[string]any) (string, error) {
	r.calls = append(r.calls, "create")
	r.target = target
	return "01I-new", nil
}
func (r *controlMetadataRemote) Update(target, metadataType, id string, patch map[string]any) error {
	r.calls = append(r.calls, "update")
	r.target = target
	return nil
}
func (r *controlMetadataRemote) Delete(target, metadataType, id string) error {
	r.calls = append(r.calls, "delete")
	r.target = target
	return nil
}

func metadataControlState(level settings.SafetyLevel, remote metadataops.Remote) *ControlState {
	resolve := func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}
	safety := func(sf.Org) settings.SafetyLevel { return level }
	service := metadataops.NewWithRemote(orgwrite.NewGate(resolve, safety), remote)
	return NewControlState(nil, resolve, safety, &settings.Settings{}, func() error { return nil },
		ControlServices{Metadata: service})
}

func TestControlMetadataDeleteUsesServiceFullGate(t *testing.T) {
	remote := &controlMetadataRemote{}
	s := metadataControlState(settings.SafetyMetadata, remote)
	_, err := s.MetadataDelete(control.MetadataDeleteArgs{
		OrgAlias: "input", Type: "CustomField", ID: "01I-field",
	})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
		t.Fatalf("err = %#v, want full BlockedError", err)
	}
	type coded interface{ Code() string }
	var c coded
	if !errors.As(err, &c) || c.Code() != control.ErrSafetyBlocked {
		t.Fatalf("coded error = %#v", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called despite safety denial: %v", remote.calls)
	}
}

func TestControlMetadataUpdateUsesResolvedServiceTarget(t *testing.T) {
	remote := &controlMetadataRemote{}
	s := metadataControlState(settings.SafetyMetadata, remote)
	_, err := s.MetadataUpdate(control.MetadataUpdateArgs{
		OrgAlias: "input", Type: "CustomField", ID: "01I-field",
		Patch: map[string]any{"description": "updated"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.target != "resolved" || len(remote.calls) != 1 || remote.calls[0] != "update" {
		t.Fatalf("remote target=%q calls=%v", remote.target, remote.calls)
	}
}

func TestControlMetadataRejectsUnknownTypeBeforeRemote(t *testing.T) {
	remote := &controlMetadataRemote{}
	s := metadataControlState(settings.SafetyFull, remote)
	_, err := s.MetadataCreate(control.MetadataCreateArgs{
		Type: "Anything__c", FullName: "X", Patch: map[string]any{"label": "x"},
	})
	type coded interface{ Code() string }
	var c coded
	if !errors.As(err, &c) || c.Code() != control.ErrInvalidArgument {
		t.Fatalf("err = %#v, want invalid_argument", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called for invalid type: %v", remote.calls)
	}
}
