package permissionops

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
}

func (f *fakeRemote) UpsertField(target, id, sobject, field, parentID string, read, edit bool) (string, error) {
	f.calls, f.target = append(f.calls, "upsert-field"), target
	return "field-id", nil
}
func (f *fakeRemote) DeleteField(target, id string) error {
	f.calls, f.target = append(f.calls, "delete-field"), target
	return nil
}
func (f *fakeRemote) UpsertObject(target, id, parentID, sobject string, read, create, edit, delete, viewAll, modifyAll bool) (string, error) {
	f.calls, f.target = append(f.calls, "upsert-object"), target
	return "object-id", nil
}
func (f *fakeRemote) DeleteObject(target, id string) error {
	f.calls, f.target = append(f.calls, "delete-object"), target
	return nil
}
func (f *fakeRemote) SetSystem(target, parentID, field string, value bool) error {
	f.calls, f.target = append(f.calls, "set-system"), target
	return nil
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(gate, remote)
}

func TestEveryMutationRequiresMetadataBeforeRemote(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Service) error
	}{
		{"field", func(s *Service) error {
			_, err := s.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: "0PS", Read: true})
			return err
		}},
		{"object", func(s *Service) error {
			_, err := s.SetObject(context.Background(), ObjectInput{SObject: "Account", ParentID: "0PS", Read: true})
			return err
		}},
		{"system", func(s *Service) error {
			_, err := s.SetSystem(context.Background(), SystemInput{ParentID: "0PS", Field: "PermissionsApiEnabled", Value: true})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remote := &fakeRemote{}
			err := tc.run(serviceAt(settings.SafetyRecords, remote))
			var blocked orgwrite.BlockedError
			if !errors.As(err, &blocked) || blocked.Required != settings.WriteMetadata {
				t.Fatalf("err = %#v, want metadata denial", err)
			}
			if len(remote.calls) != 0 {
				t.Fatalf("remote called on denial: %v", remote.calls)
			}
		})
	}
}

func TestSetOperationsUseResolvedTarget(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyMetadata, remote)
	field, err := service.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: "0PS", Read: true})
	if err != nil || field.ID != "field-id" || remote.target != "resolved" {
		t.Fatalf("field=%#v err=%v remote=%#v", field, err, remote)
	}
	object, err := service.SetObject(context.Background(), ObjectInput{SObject: "Account", ParentID: "0PS", Read: true})
	if err != nil || object.ID != "object-id" || remote.target != "resolved" {
		t.Fatalf("object=%#v err=%v remote=%#v", object, err, remote)
	}
	_, err = service.SetSystem(context.Background(), SystemInput{ParentID: "0PS", Field: "PermissionsApiEnabled", Value: true})
	if err != nil || remote.target != "resolved" {
		t.Fatalf("system err=%v remote=%#v", err, remote)
	}
}

func TestAllFalseDeletesOrNoops(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyMetadata, remote)
	result, err := service.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: "0PS"})
	if err != nil || !result.Noop || len(remote.calls) != 0 {
		t.Fatalf("field result=%#v err=%v calls=%v", result, err, remote.calls)
	}
	result, err = service.SetObject(context.Background(), ObjectInput{ID: "obj", SObject: "Account", ParentID: "0PS"})
	if err != nil || !result.Deleted || len(remote.calls) != 1 || remote.calls[0] != "delete-object" {
		t.Fatalf("object result=%#v err=%v calls=%v", result, err, remote.calls)
	}
}

func TestInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyMetadata, remote)
	if _, err := service.SetSystem(context.Background(), SystemInput{ParentID: "0PS", Field: "Label"}); err == nil || len(remote.calls) != 0 {
		t.Fatalf("invalid system field err=%v calls=%v", err, remote.calls)
	}
	valid := SystemInput{ParentID: "0PS", Field: "PermissionsApiEnabled"}
	for _, service := range []*Service{nil, NewWithRemote(nil, remote), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyMetadata }), nil)} {
		if _, err := service.SetSystem(context.Background(), valid); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
}
