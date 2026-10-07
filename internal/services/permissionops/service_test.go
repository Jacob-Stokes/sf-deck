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
	err    error
}

const (
	testParentID = "0PS000000000001"
	testObjectID = "110000000000001"
)

func (f *fakeRemote) UpsertField(target, id, sobject, field, parentID string, read, edit bool) (string, error) {
	f.calls, f.target = append(f.calls, "upsert-field"), target
	return "field-id", f.err
}
func (f *fakeRemote) DeleteField(target, id string) error {
	f.calls, f.target = append(f.calls, "delete-field"), target
	return f.err
}
func (f *fakeRemote) UpsertObject(target, id, parentID, sobject string, read, create, edit, delete, viewAll, modifyAll bool) (string, error) {
	f.calls, f.target = append(f.calls, "upsert-object"), target
	return "object-id", f.err
}
func (f *fakeRemote) DeleteObject(target, id string) error {
	f.calls, f.target = append(f.calls, "delete-object"), target
	return f.err
}
func (f *fakeRemote) SetSystem(target, parentID, field string, value bool) error {
	f.calls, f.target = append(f.calls, "set-system"), target
	return f.err
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
			_, err := s.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: testParentID, Read: true})
			return err
		}},
		{"object", func(s *Service) error {
			_, err := s.SetObject(context.Background(), ObjectInput{SObject: "Account", ParentID: testParentID, Read: true})
			return err
		}},
		{"system", func(s *Service) error {
			_, err := s.SetSystem(context.Background(), SystemInput{ParentID: testParentID, Field: "PermissionsApiEnabled", Value: true})
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
	field, err := service.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: testParentID, Read: true})
	if err != nil || field.ID != "field-id" || remote.target != "resolved" {
		t.Fatalf("field=%#v err=%v remote=%#v", field, err, remote)
	}
	object, err := service.SetObject(context.Background(), ObjectInput{SObject: "Account", ParentID: testParentID, Read: true})
	if err != nil || object.ID != "object-id" || remote.target != "resolved" {
		t.Fatalf("object=%#v err=%v remote=%#v", object, err, remote)
	}
	_, err = service.SetSystem(context.Background(), SystemInput{ParentID: testParentID, Field: "PermissionsApiEnabled", Value: true})
	if err != nil || remote.target != "resolved" {
		t.Fatalf("system err=%v remote=%#v", err, remote)
	}
}

func TestAllFalseDeletesOrNoops(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyMetadata, remote)
	result, err := service.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: testParentID})
	if err != nil || !result.Noop || len(remote.calls) != 0 {
		t.Fatalf("field result=%#v err=%v calls=%v", result, err, remote.calls)
	}
	result, err = service.SetObject(context.Background(), ObjectInput{ID: testObjectID, SObject: "Account", ParentID: testParentID})
	if err != nil || !result.Deleted || len(remote.calls) != 1 || remote.calls[0] != "delete-object" {
		t.Fatalf("object result=%#v err=%v calls=%v", result, err, remote.calls)
	}
}

func TestInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyMetadata, remote)
	if _, err := service.SetSystem(context.Background(), SystemInput{ParentID: testParentID, Field: "Label"}); err == nil || len(remote.calls) != 0 {
		t.Fatalf("invalid system field err=%v calls=%v", err, remote.calls)
	}
	valid := SystemInput{ParentID: testParentID, Field: "PermissionsApiEnabled"}
	for _, service := range []*Service{nil, NewWithRemote(nil, remote), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyMetadata }), nil)} {
		if _, err := service.SetSystem(context.Background(), valid); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
}

func TestPermissionValidationAndDeleteBranches(t *testing.T) {
	s := serviceAt(settings.SafetyMetadata, &fakeRemote{})
	for _, in := range []FieldInput{
		{},
		{SObject: "Account", Field: "Account.Name", ParentID: "bad", Read: true},
		{SObject: "Account", Field: "Account.Name", ParentID: testParentID, ID: "bad", Read: true},
		{SObject: "Account/evil", Field: "Account.Name", ParentID: testParentID, Read: true},
	} {
		if _, err := s.SetField(context.Background(), in); err == nil {
			t.Errorf("invalid field input accepted: %#v", in)
		}
	}
	if _, err := s.SetObject(context.Background(), ObjectInput{}); err == nil {
		t.Fatal("empty object input accepted")
	}
	if _, err := s.SetSystem(context.Background(), SystemInput{}); err == nil {
		t.Fatal("empty system input accepted")
	}
	remote := &fakeRemote{}
	s = serviceAt(settings.SafetyMetadata, remote)
	result, err := s.SetField(context.Background(), FieldInput{ID: testObjectID, SObject: "Account", Field: "Account.Name", ParentID: testParentID})
	if err != nil || !result.Deleted || remote.calls[0] != "delete-field" {
		t.Fatalf("field delete result=%#v err=%v calls=%v", result, err, remote.calls)
	}
	result, err = s.SetObject(context.Background(), ObjectInput{SObject: "Account", ParentID: testParentID})
	if err != nil || !result.Noop {
		t.Fatalf("object noop result=%#v err=%v", result, err)
	}
}

func TestPermissionContextAndRemoteErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	remote := &fakeRemote{}
	if _, err := serviceAt(settings.SafetyMetadata, remote).SetSystem(ctx, SystemInput{ParentID: testParentID, Field: "PermissionsApiEnabled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request err=%v", err)
	}
	boom := errors.New("remote failed")
	remote.err = boom
	s := serviceAt(settings.SafetyMetadata, remote)
	if _, err := s.SetField(context.Background(), FieldInput{SObject: "Account", Field: "Account.Name", ParentID: testParentID, Read: true}); !errors.Is(err, boom) {
		t.Fatalf("upsert error=%v", err)
	}
	if _, err := s.SetSystem(context.Background(), SystemInput{ParentID: testParentID, Field: "PermissionsApiEnabled"}); !errors.Is(err, boom) {
		t.Fatalf("system error=%v", err)
	}
}
