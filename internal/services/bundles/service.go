package bundles

import (
	"context"
	"errors"

	"github.com/Jacob-Stokes/sf-deck/internal/devproject"
	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Remote is the narrow sf CLI transport used by bundle operations.
type Remote interface {
	Retrieve(path, target string) ([]byte, error)
	Deploy(path, target string, opts sf.DeployOpts) ([]byte, error)
	DeployAsync(path, target string, opts sf.DeployOpts) ([]byte, error)
	Validate(path, target string, opts sf.DeployOpts) ([]byte, error)
	ValidateAsync(path, target string, opts sf.DeployOpts) ([]byte, error)
	Report(path, target, jobID string) ([]byte, error)
}

type liveRemote struct{}

func (liveRemote) Retrieve(path, target string) ([]byte, error) {
	return sf.RetrieveProject(path, target)
}
func (liveRemote) Deploy(path, target string, opts sf.DeployOpts) ([]byte, error) {
	return sf.DeployProject(path, target, opts)
}
func (liveRemote) DeployAsync(path, target string, opts sf.DeployOpts) ([]byte, error) {
	return sf.DeployProjectAsync(path, target, opts)
}
func (liveRemote) Validate(path, target string, opts sf.DeployOpts) ([]byte, error) {
	return sf.ValidateDeployProject(path, target, opts)
}
func (liveRemote) ValidateAsync(path, target string, opts sf.DeployOpts) ([]byte, error) {
	return sf.ValidateDeployProjectAsync(path, target, opts)
}
func (liveRemote) Report(path, target, jobID string) ([]byte, error) {
	return sf.DeployReport(path, target, jobID)
}

// Service owns target resolution and safety for bundle operations that
// contact Salesforce. Local list/create/link/delete functions remain free
// functions because they do not mutate an org.
type Service struct {
	store  *devproject.Store
	gate   *orgwrite.Gate
	remote Remote
}

func New(store *devproject.Store, gate *orgwrite.Gate) *Service {
	return NewWithRemote(store, gate, liveRemote{})
}

func NewWithRemote(store *devproject.Store, gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{store: store, gate: gate, remote: remote}
}

type OperationInput struct {
	BundleID string
	Target   string
	Opts     sf.DeployOpts
}

type ReportInput struct {
	BundleID string
	Target   string
	JobID    string
}

type OperationResult struct {
	Target orgwrite.Target
	Output []byte
}

func (s *Service) Retrieve(ctx context.Context, in OperationInput) (OperationResult, error) {
	bundle, target, err := s.resolve(ctx, in.BundleID, in.Target, false)
	if err != nil {
		return OperationResult{}, err
	}
	out, err := s.remote.Retrieve(bundle.Path, target.CLIArg)
	result := OperationResult{Target: target, Output: out}
	if err == nil {
		_ = s.store.MarkRetrieved(in.BundleID)
	}
	return result, err
}

func (s *Service) Deploy(ctx context.Context, in OperationInput) (OperationResult, error) {
	return s.deploy(ctx, in, false)
}

func (s *Service) DeployAsync(ctx context.Context, in OperationInput) (OperationResult, error) {
	return s.deploy(ctx, in, true)
}

func (s *Service) deploy(ctx context.Context, in OperationInput, async bool) (OperationResult, error) {
	bundle, target, err := s.resolve(ctx, in.BundleID, in.Target, true)
	if err != nil {
		return OperationResult{}, err
	}
	var out []byte
	if async {
		out, err = s.remote.DeployAsync(bundle.Path, target.CLIArg, in.Opts)
	} else {
		out, err = s.remote.Deploy(bundle.Path, target.CLIArg, in.Opts)
	}
	result := OperationResult{Target: target, Output: out}
	if err == nil {
		_ = s.store.MarkDeployed(in.BundleID)
	}
	return result, err
}

func (s *Service) Validate(ctx context.Context, in OperationInput) (OperationResult, error) {
	return s.validate(ctx, in, false)
}

func (s *Service) ValidateAsync(ctx context.Context, in OperationInput) (OperationResult, error) {
	return s.validate(ctx, in, true)
}

func (s *Service) validate(ctx context.Context, in OperationInput, async bool) (OperationResult, error) {
	bundle, target, err := s.resolve(ctx, in.BundleID, in.Target, true)
	if err != nil {
		return OperationResult{}, err
	}
	var out []byte
	if async {
		out, err = s.remote.ValidateAsync(bundle.Path, target.CLIArg, in.Opts)
	} else {
		out, err = s.remote.Validate(bundle.Path, target.CLIArg, in.Opts)
	}
	return OperationResult{Target: target, Output: out}, err
}

func (s *Service) Report(ctx context.Context, in ReportInput) (OperationResult, error) {
	if in.JobID == "" {
		return OperationResult{}, errors.New("job-id is required")
	}
	bundle, target, err := s.resolve(ctx, in.BundleID, in.Target, false)
	if err != nil {
		return OperationResult{}, err
	}
	out, err := s.remote.Report(bundle.Path, target.CLIArg, in.JobID)
	return OperationResult{Target: target, Output: out}, err
}

func (s *Service) resolve(ctx context.Context, bundleID, requestedTarget string, write bool) (devproject.Bundle, orgwrite.Target, error) {
	if err := ctx.Err(); err != nil {
		return devproject.Bundle{}, orgwrite.Target{}, err
	}
	if s == nil || s.store == nil {
		return devproject.Bundle{}, orgwrite.Target{}, errors.New("bundle store unavailable")
	}
	if s.gate == nil {
		return devproject.Bundle{}, orgwrite.Target{}, errors.New("bundle safety gate unavailable; operation refused")
	}
	if s.remote == nil {
		return devproject.Bundle{}, orgwrite.Target{}, errors.New("bundle transport unavailable; operation refused")
	}
	bundle, err := fetchBundle(s.store, bundleID)
	if err != nil {
		return devproject.Bundle{}, orgwrite.Target{}, err
	}
	if bundle.Stale() {
		return devproject.Bundle{}, orgwrite.Target{}, ErrStale{ID: bundleID, Path: bundle.Path}
	}
	effectiveTarget := requestedTarget
	if effectiveTarget == "" {
		effectiveTarget = bundle.DefaultOrgAlias
	}
	if effectiveTarget == "" {
		return devproject.Bundle{}, orgwrite.Target{}, errors.New("no org alias (bundle has no default and none supplied)")
	}
	var target orgwrite.Target
	if write {
		target, err = s.gate.Require(effectiveTarget, settings.WriteMetadata)
	} else {
		target, err = s.gate.ReadTarget(effectiveTarget)
	}
	if err != nil {
		return devproject.Bundle{}, orgwrite.Target{}, err
	}
	return bundle, target, nil
}
