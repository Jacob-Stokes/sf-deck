package metadataops

import (
	"context"
	"errors"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeEditorRemote struct {
	calls  []string
	target string
}

func (f *fakeEditorRemote) UpdateTrigger(target, id string, patch map[string]any) error {
	f.calls, f.target = append(f.calls, "trigger"), target
	return nil
}
func (f *fakeEditorRemote) DeployObject(target, apiName string, patch sf.CustomObjectPatch) (*sf.DeployResult, error) {
	f.calls, f.target = append(f.calls, "object"), target
	return &sf.DeployResult{Success: true}, nil
}
func (f *fakeEditorRemote) DeployObjectWithBaseline(target, apiName string, patch sf.CustomObjectPatch, baseline *sf.CustomObjectBaseline) (*sf.DeployResult, error) {
	f.calls, f.target = append(f.calls, "object-baseline"), target
	return &sf.DeployResult{Success: true}, nil
}

func editorAt(level settings.SafetyLevel, remote EditorRemote) *EditorService {
	gate := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewEditorWithRemote(gate, remote)
}

func TestEditorWritesRequireMetadataBeforeRemote(t *testing.T) {
	remote := &fakeEditorRemote{}
	service := editorAt(settings.SafetyRecords, remote)
	_, err := service.UpdateTrigger(context.Background(), TriggerUpdateInput{ID: "01q", Patch: map[string]any{"status": "Active"}})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || len(remote.calls) != 0 {
		t.Fatalf("trigger err=%#v calls=%v", err, remote.calls)
	}
	patch := sf.CustomObjectPatch{Label: "Updated"}
	_, err = service.DeployObject(context.Background(), ObjectDeployInput{APIName: "Thing__c", Patch: patch})
	if !errors.As(err, &blocked) || len(remote.calls) != 0 {
		t.Fatalf("object err=%#v calls=%v", err, remote.calls)
	}
}

func TestEditorWritesUseResolvedTarget(t *testing.T) {
	remote := &fakeEditorRemote{}
	service := editorAt(settings.SafetyMetadata, remote)
	if _, err := service.UpdateTrigger(context.Background(), TriggerUpdateInput{ID: "01q", Patch: map[string]any{"body": "trigger X"}}); err != nil || remote.target != "resolved" {
		t.Fatalf("trigger err=%v remote=%#v", err, remote)
	}
	result, err := service.DeployObject(context.Background(), ObjectDeployInput{
		APIName: "Thing__c", Patch: sf.CustomObjectPatch{Label: "Updated"}, Baseline: &sf.CustomObjectBaseline{},
	})
	if err != nil || result.Target.CLIArg != "resolved" || remote.calls[len(remote.calls)-1] != "object-baseline" {
		t.Fatalf("object result=%#v err=%v calls=%v", result, err, remote.calls)
	}
}

func TestEditorInvalidAndMissingDependenciesFailClosed(t *testing.T) {
	if _, err := editorAt(settings.SafetyMetadata, &fakeEditorRemote{}).UpdateTrigger(context.Background(), TriggerUpdateInput{}); err == nil {
		t.Fatal("empty trigger update accepted")
	}
	valid := TriggerUpdateInput{ID: "01q", Patch: map[string]any{"status": "Active"}}
	for _, service := range []*EditorService{nil, NewEditorWithRemote(nil, &fakeEditorRemote{}), NewEditorWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil }, func(sf.Org) settings.SafetyLevel { return settings.SafetyMetadata }), nil)} {
		if _, err := service.UpdateTrigger(context.Background(), valid); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
}
