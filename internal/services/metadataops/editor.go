package metadataops

import (
	"context"
	"errors"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// EditorRemote contains specialized metadata transports that cannot use the
// generic Tooling Metadata envelope.
type EditorRemote interface {
	UpdateTrigger(target, id string, patch map[string]any) error
	DeployObject(target, apiName string, patch sf.CustomObjectPatch) (*sf.DeployResult, error)
	DeployObjectWithBaseline(target, apiName string, patch sf.CustomObjectPatch, baseline *sf.CustomObjectBaseline) (*sf.DeployResult, error)
}

type liveEditorRemote struct{}

func (liveEditorRemote) UpdateTrigger(target, id string, patch map[string]any) error {
	return sf.UpdateTriggerMetadata(target, id, patch)
}
func (liveEditorRemote) DeployObject(target, apiName string, patch sf.CustomObjectPatch) (*sf.DeployResult, error) {
	return sf.DeployCustomObjectPatch(target, apiName, patch)
}
func (liveEditorRemote) DeployObjectWithBaseline(target, apiName string, patch sf.CustomObjectPatch, baseline *sf.CustomObjectBaseline) (*sf.DeployResult, error) {
	return sf.DeployCustomObjectPatchWithBaseline(target, apiName, patch, baseline)
}

type EditorService struct {
	gate   *orgwrite.Gate
	remote EditorRemote
}

func NewEditor(gate *orgwrite.Gate) *EditorService {
	return NewEditorWithRemote(gate, liveEditorRemote{})
}

func NewEditorWithRemote(gate *orgwrite.Gate, remote EditorRemote) *EditorService {
	return &EditorService{gate: gate, remote: remote}
}

type TriggerUpdateInput struct {
	Target string
	ID     string
	Patch  map[string]any
}

type ObjectDeployInput struct {
	Target   string
	APIName  string
	Patch    sf.CustomObjectPatch
	Baseline *sf.CustomObjectBaseline
}

type ObjectDeployResult struct {
	Target orgwrite.Target
	Deploy *sf.DeployResult
}

func (s *EditorService) UpdateTrigger(ctx context.Context, in TriggerUpdateInput) (orgwrite.Target, error) {
	if strings.TrimSpace(in.ID) == "" || len(in.Patch) == 0 {
		return orgwrite.Target{}, errors.New("trigger id and patch are required")
	}
	target, err := s.require(ctx, in.Target)
	if err != nil {
		return orgwrite.Target{}, err
	}
	return target, s.remote.UpdateTrigger(target.CLIArg, in.ID, in.Patch)
}

func (s *EditorService) DeployObject(ctx context.Context, in ObjectDeployInput) (ObjectDeployResult, error) {
	if strings.TrimSpace(in.APIName) == "" || !in.Patch.HasChanges() {
		return ObjectDeployResult{}, errors.New("custom object api name and patch are required")
	}
	target, err := s.require(ctx, in.Target)
	if err != nil {
		return ObjectDeployResult{}, err
	}
	var deploy *sf.DeployResult
	if in.Baseline != nil {
		deploy, err = s.remote.DeployObjectWithBaseline(target.CLIArg, in.APIName, in.Patch, in.Baseline)
	} else {
		deploy, err = s.remote.DeployObject(target.CLIArg, in.APIName, in.Patch)
	}
	if err == nil && deploy == nil {
		err = errors.New("custom object deploy returned no result")
	}
	return ObjectDeployResult{Target: target, Deploy: deploy}, err
}

func (s *EditorService) require(ctx context.Context, target string) (orgwrite.Target, error) {
	if err := ctx.Err(); err != nil {
		return orgwrite.Target{}, err
	}
	if s == nil || s.gate == nil {
		return orgwrite.Target{}, errors.New("metadata editor safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return orgwrite.Target{}, errors.New("metadata editor transport unavailable; write refused")
	}
	return s.gate.Require(target, settings.WriteMetadata)
}
