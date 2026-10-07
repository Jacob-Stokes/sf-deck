package metadataops

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeRemote struct {
	calls    []string
	target   string
	typeName string
	ref      string
	payload  map[string]any
	err      error
}

func (f *fakeRemote) Create(target, metadataType, fullName string, metadata map[string]any) (string, error) {
	f.record("create", target, metadataType, fullName, metadata)
	return "01I-new", f.err
}
func (f *fakeRemote) Update(target, metadataType, id string, patch map[string]any) error {
	f.record("update", target, metadataType, id, patch)
	return f.err
}
func (f *fakeRemote) Delete(target, metadataType, id string) error {
	f.record("delete", target, metadataType, id, nil)
	return f.err
}
func (f *fakeRemote) record(call, target, metadataType, ref string, payload map[string]any) {
	f.calls = append(f.calls, call)
	f.target, f.typeName, f.ref, f.payload = target, metadataType, ref, payload
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	g := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(g, remote)
}

func TestCreateRequiresMetadataAndUsesResolvedTarget(t *testing.T) {
	remote := &fakeRemote{}
	s := serviceAt(settings.SafetyMetadata, remote)
	payload := map[string]any{"label": "Test"}
	got, err := s.Create(context.Background(), CreateInput{
		Target: "input", Type: "CustomField", FullName: "Account.Test__c", Metadata: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "01I-new" || got.Target.CLIArg != "resolved" {
		t.Fatalf("result = %#v", got)
	}
	if !reflect.DeepEqual(remote.calls, []string{"create"}) || remote.target != "resolved" ||
		remote.typeName != "CustomField" || remote.ref != "Account.Test__c" ||
		!reflect.DeepEqual(remote.payload, payload) {
		t.Fatalf("remote = %#v", remote)
	}
}

func TestCreateBlockedBeforeRemote(t *testing.T) {
	remote := &fakeRemote{}
	s := serviceAt(settings.SafetyRecords, remote)
	_, err := s.Create(context.Background(), CreateInput{
		Type: "CustomField", FullName: "Account.Test__c", Metadata: map[string]any{"label": "Test"},
	})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteMetadata {
		t.Fatalf("err = %#v, want metadata BlockedError", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called on denial: %v", remote.calls)
	}
}

func TestUpdateRequiresMetadata(t *testing.T) {
	remote := &fakeRemote{}
	s := serviceAt(settings.SafetyMetadata, remote)
	got, err := s.Update(context.Background(), UpdateInput{
		Type: "ValidationRule", ID: "03d000000000001", Patch: map[string]any{"active": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Target.Username != "u@example.com" || !reflect.DeepEqual(remote.calls, []string{"update"}) {
		t.Fatalf("result=%#v remote=%#v", got, remote)
	}
}

func TestDeleteRequiresFull(t *testing.T) {
	for _, tc := range []struct {
		level   settings.SafetyLevel
		allowed bool
	}{
		{settings.SafetyMetadata, false},
		{settings.SafetyFull, true},
	} {
		remote := &fakeRemote{}
		s := serviceAt(tc.level, remote)
		got, err := s.Delete(context.Background(), DeleteInput{Type: "CustomField", ID: "01I000000000001"})
		if tc.allowed {
			if err != nil || got.Target.CLIArg != "resolved" || !reflect.DeepEqual(remote.calls, []string{"delete"}) {
				t.Fatalf("full delete result=%#v err=%v remote=%#v", got, err, remote)
			}
			continue
		}
		var blocked orgwrite.BlockedError
		if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
			t.Fatalf("metadata delete err=%#v, want full BlockedError", err)
		}
		if len(remote.calls) != 0 {
			t.Fatalf("remote called on denial: %v", remote.calls)
		}
	}
}

func TestValidationPrecedesResolutionAndRemote(t *testing.T) {
	resolveCalls := 0
	remote := &fakeRemote{}
	g := orgwrite.NewGate(func(string) (sf.Org, error) {
		resolveCalls++
		return sf.Org{}, nil
	}, func(sf.Org) settings.SafetyLevel { return settings.SafetyFull })
	s := NewWithRemote(g, remote)
	_, err := s.Delete(context.Background(), DeleteInput{Type: "NotAType", ID: "x"})
	var invalid ErrInvalidType
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %T %v, want ErrInvalidType", err, err)
	}
	if resolveCalls != 0 || len(remote.calls) != 0 {
		t.Fatalf("validation reached dependencies: resolve=%d remote=%v", resolveCalls, remote.calls)
	}
}

func TestMissingDependenciesFailClosed(t *testing.T) {
	valid := DeleteInput{Type: "CustomField", ID: "01I000000000001"}
	for _, s := range []*Service{nil, NewWithRemote(nil, &fakeRemote{}), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil },
			func(sf.Org) settings.SafetyLevel { return settings.SafetyFull }), nil)} {
		if _, err := s.Delete(context.Background(), valid); err == nil {
			t.Fatal("missing dependency must fail closed")
		}
	}
}

func TestContextCancellationPreventsRemote(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	remote := &fakeRemote{}
	_, err := serviceAt(settings.SafetyFull, remote).Delete(ctx, DeleteInput{Type: "CustomField", ID: "01I000000000001"})
	if !errors.Is(err, context.Canceled) || len(remote.calls) != 0 {
		t.Fatalf("err=%v remote=%v", err, remote.calls)
	}
}

func TestValidationBranchesAndKnownTypes(t *testing.T) {
	for _, in := range []CreateInput{
		{},
		{Type: "CustomField", Metadata: map[string]any{}},
		{Type: "CustomField", FullName: "Account.Test__c"},
	} {
		if err := validateCreate(in); err == nil {
			t.Errorf("invalid create accepted: %#v", in)
		}
	}
	for _, in := range []UpdateInput{
		{},
		{Type: "CustomField", ID: "bad", Patch: map[string]any{}},
		{Type: "CustomField", ID: "01I000000000001"},
	} {
		if err := validateUpdate(in); err == nil {
			t.Errorf("invalid update accepted: %#v", in)
		}
	}
	types := KnownTypes()
	if len(types) == 0 || !sort.StringsAreSorted(types) {
		t.Fatalf("KnownTypes not sorted: %v", types)
	}
	if got := (ErrInvalidType{Type: "Bad"}).Error(); !strings.Contains(got, "Bad") {
		t.Fatalf("ErrInvalidType.Error() = %q", got)
	}
}

func TestRemoteErrorsReturnResolvedTarget(t *testing.T) {
	boom := errors.New("remote failed")
	remote := &fakeRemote{err: boom}
	s := serviceAt(settings.SafetyFull, remote)
	created, err := s.Create(context.Background(), CreateInput{Type: "CustomField", FullName: "Account.Test__c", Metadata: map[string]any{}})
	if !errors.Is(err, boom) || created.Target.CLIArg != "resolved" {
		t.Fatalf("create result=%#v err=%v", created, err)
	}
	updated, err := s.Update(context.Background(), UpdateInput{Type: "CustomField", ID: "01I000000000001", Patch: map[string]any{}})
	if !errors.Is(err, boom) || updated.Target.CLIArg != "resolved" {
		t.Fatalf("update result=%#v err=%v", updated, err)
	}
	deleted, err := s.Delete(context.Background(), DeleteInput{Type: "CustomField", ID: "01I000000000001"})
	if !errors.Is(err, boom) || deleted.Target.CLIArg != "resolved" {
		t.Fatalf("delete result=%#v err=%v", deleted, err)
	}
}
