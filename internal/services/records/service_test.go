package records

import (
	"context"
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeRemote struct {
	calls       []string
	target      string
	sobject     string
	id          string
	fields      map[string]any
	fieldErrors []sf.FieldError
	newID       string
	err         error
}

func (f *fakeRemote) ResolveSObject(target, id string) (string, error) {
	f.calls = append(f.calls, "resolve")
	f.target, f.id = target, id
	return "Account", f.err
}
func (f *fakeRemote) Create(target, sobject string, fields map[string]any) ([]sf.FieldError, string, error) {
	f.calls = append(f.calls, "create")
	f.target, f.sobject, f.fields = target, sobject, fields
	return f.fieldErrors, f.newID, f.err
}
func (f *fakeRemote) Update(target, sobject, id string, fields map[string]any) ([]sf.FieldError, error) {
	f.calls = append(f.calls, "update")
	f.target, f.sobject, f.id, f.fields = target, sobject, id, fields
	return f.fieldErrors, f.err
}
func (f *fakeRemote) Delete(target, sobject, id string) error {
	f.calls = append(f.calls, "delete")
	f.target, f.sobject, f.id = target, sobject, id
	return f.err
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(gate, remote)
}

func TestMutationsRequireRecordSafetyBeforeRemote(t *testing.T) {
	inputs := []struct {
		name string
		run  func(*Service) error
	}{
		{"create", func(s *Service) error {
			_, err := s.Create(context.Background(), CreateInput{SObject: "Account", Fields: map[string]any{"Name": "x"}})
			return err
		}},
		{"update", func(s *Service) error {
			_, err := s.Update(context.Background(), UpdateInput{ID: "001000000000000", Fields: map[string]any{"Name": "x"}})
			return err
		}},
		{"delete", func(s *Service) error {
			_, err := s.Delete(context.Background(), DeleteInput{ID: "001000000000000"})
			return err
		}},
	}
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			remote := &fakeRemote{}
			err := tc.run(serviceAt(settings.SafetyReadOnly, remote))
			var blocked orgwrite.BlockedError
			if !errors.As(err, &blocked) || blocked.Required != settings.WriteRecord {
				t.Fatalf("err = %#v, want record BlockedError", err)
			}
			if len(remote.calls) != 0 {
				t.Fatalf("remote called on denial: %v", remote.calls)
			}
		})
	}
}

func TestUpdateInfersObjectAfterGateAndUsesResolvedTarget(t *testing.T) {
	remote := &fakeRemote{fieldErrors: []sf.FieldError{{Message: "bad"}}}
	got, err := serviceAt(settings.SafetyRecords, remote).Update(context.Background(), UpdateInput{
		ID: "001000000000000", Fields: map[string]any{"Name": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Target.CLIArg != "resolved" || got.SObject != "Account" || len(got.FieldErrors) != 1 {
		t.Fatalf("result = %#v", got)
	}
	want := []string{"resolve", "update"}
	if len(remote.calls) != len(want) || remote.calls[0] != want[0] || remote.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", remote.calls, want)
	}
}

func TestCreateAndDeleteReturnStructuredResults(t *testing.T) {
	remote := &fakeRemote{newID: "001-new"}
	service := serviceAt(settings.SafetyRecords, remote)
	created, err := service.Create(context.Background(), CreateInput{SObject: "Account", Fields: map[string]any{"Name": "x"}})
	if err != nil || created.ID != "001-new" || created.Target.Username != "u@example.com" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	deleted, err := service.Delete(context.Background(), DeleteInput{SObject: "Account", ID: "001000000000000"})
	if err != nil || deleted.SObject != "Account" || deleted.ID == "" {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
}

func TestInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	if _, err := serviceAt(settings.SafetyRecords, &fakeRemote{}).Update(context.Background(), UpdateInput{ID: "short", Fields: map[string]any{"x": 1}}); err == nil {
		t.Fatal("short id accepted")
	}
	valid := CreateInput{SObject: "Account", Fields: map[string]any{"Name": "x"}}
	for _, service := range []*Service{nil, NewWithRemote(nil, &fakeRemote{}), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyRecords }), nil)} {
		if _, err := service.Create(context.Background(), valid); err == nil {
			t.Fatal("missing dependency must refuse write")
		}
	}
}

func TestPrivilegedSObjectsRequireStrongerSafety(t *testing.T) {
	tests := []struct {
		object   string
		level    settings.SafetyLevel
		required settings.WriteKind
	}{
		{"User", settings.SafetyRecords, settings.WriteAnonymous},
		{"PermissionSetAssignment", settings.SafetyMetadata, settings.WriteAnonymous},
		{"ObjectPermissions", settings.SafetyRecords, settings.WriteMetadata},
		{"AccountShare", settings.SafetyMetadata, settings.WriteAnonymous},
	}
	for _, tc := range tests {
		t.Run(tc.object, func(t *testing.T) {
			remote := &fakeRemote{}
			_, err := serviceAt(tc.level, remote).Create(context.Background(), CreateInput{
				SObject: tc.object,
				Fields:  map[string]any{"Name": "x"},
			})
			var blocked orgwrite.BlockedError
			if !errors.As(err, &blocked) || blocked.Required != tc.required {
				t.Fatalf("err = %#v, want %v BlockedError", err, tc.required)
			}
			if len(remote.calls) != 0 {
				t.Fatalf("remote called on denial: %v", remote.calls)
			}
		})
	}
}

func TestInferredPrivilegedSObjectIsRegatedBeforeMutation(t *testing.T) {
	remote := &fakeRemote{}
	service := serviceAt(settings.SafetyRecords, remote)
	remote.err = nil
	// Override the fake's resolved object through a narrow wrapper.
	wrapped := &resolvedObjectRemote{fakeRemote: remote, object: "User"}
	service.remote = wrapped
	_, err := service.Update(context.Background(), UpdateInput{
		ID: "005000000000000", Fields: map[string]any{"IsActive": false},
	})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
		t.Fatalf("err = %#v, want full BlockedError", err)
	}
	if len(remote.calls) != 1 || remote.calls[0] != "resolve" {
		t.Fatalf("calls = %v, want resolve only", remote.calls)
	}
}

type resolvedObjectRemote struct {
	*fakeRemote
	object string
}

func (r *resolvedObjectRemote) ResolveSObject(target, id string) (string, error) {
	r.calls = append(r.calls, "resolve")
	r.target, r.id = target, id
	return r.object, r.err
}

func TestRecordIdentifiersAreStrictlyValidated(t *testing.T) {
	service := serviceAt(settings.SafetyFull, &fakeRemote{})
	for _, id := range []string{"short", "0010000000000000", "00100000000000/", " 001000000000000"} {
		if _, err := service.Delete(context.Background(), DeleteInput{SObject: "Account", ID: id}); err == nil {
			t.Errorf("invalid id %q accepted", id)
		}
	}
	for _, object := range []string{"Account/001", "../User", "Account?x", ""} {
		if _, err := service.Create(context.Background(), CreateInput{SObject: object, Fields: map[string]any{"Name": "x"}}); err == nil {
			t.Errorf("invalid object %q accepted", object)
		}
	}
}
