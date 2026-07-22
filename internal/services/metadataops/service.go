// Package metadataops provides the safety-enforced Tooling metadata write
// use cases shared by CLI, IPC, and the TUI.
package metadataops

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Remote is the narrow Tooling API capability used by this service. The live
// implementation delegates to internal/sf; tests inject a fake and prove a
// blocked operation never reaches Salesforce.
type Remote interface {
	Create(target, metadataType, fullName string, metadata map[string]any) (string, error)
	Update(target, metadataType, id string, patch map[string]any) error
	Delete(target, metadataType, id string) error
}

type liveRemote struct{}

func (liveRemote) Create(target, metadataType, fullName string, metadata map[string]any) (string, error) {
	return sf.CreateToolingMetadata(target, metadataType, fullName, metadata)
}
func (liveRemote) Update(target, metadataType, id string, patch map[string]any) error {
	return sf.UpdateToolingMetadata(target, metadataType, id, patch)
}
func (liveRemote) Delete(target, metadataType, id string) error {
	return sf.DeleteToolingMetadata(target, metadataType, id)
}

// Service owns metadata write validation, target resolution, and safety.
type Service struct {
	gate   *orgwrite.Gate
	remote Remote
}

func New(gate *orgwrite.Gate) *Service {
	return NewWithRemote(gate, liveRemote{})
}

// NewWithRemote is the test/embedding constructor for a custom Tooling
// transport. A nil remote fails closed when an operation is attempted.
func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote}
}

type CreateInput struct {
	Target   string
	Type     string
	FullName string
	Metadata map[string]any
}

type CreateResult struct {
	Target orgwrite.Target
	ID     string
}

type UpdateInput struct {
	Target string
	Type   string
	ID     string
	Patch  map[string]any
}

type UpdateResult struct {
	Target orgwrite.Target
}

type DeleteInput struct {
	Target string
	Type   string
	ID     string
}

type DeleteResult struct {
	Target orgwrite.Target
}

func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	if err := validateCreate(in); err != nil {
		return CreateResult{}, err
	}
	target, err := s.require(ctx, in.Target, settings.WriteMetadata)
	if err != nil {
		return CreateResult{}, err
	}
	id, err := s.remote.Create(target.CLIArg, in.Type, in.FullName, in.Metadata)
	if err != nil {
		return CreateResult{Target: target}, err
	}
	return CreateResult{Target: target, ID: id}, nil
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (UpdateResult, error) {
	if err := validateUpdate(in); err != nil {
		return UpdateResult{}, err
	}
	target, err := s.require(ctx, in.Target, settings.WriteMetadata)
	if err != nil {
		return UpdateResult{}, err
	}
	if err := s.remote.Update(target.CLIArg, in.Type, in.ID, in.Patch); err != nil {
		return UpdateResult{Target: target}, err
	}
	return UpdateResult{Target: target}, nil
}

func (s *Service) Delete(ctx context.Context, in DeleteInput) (DeleteResult, error) {
	if err := ValidateType(in.Type); err != nil {
		return DeleteResult{}, err
	}
	if strings.TrimSpace(in.ID) == "" {
		return DeleteResult{}, errors.New("metadata id is required")
	}
	// Metadata deletion is destructive and has no undo. Keep it at the
	// full tier until settings grows a dedicated destructive-metadata kind.
	target, err := s.require(ctx, in.Target, settings.WriteAnonymous)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := s.remote.Delete(target.CLIArg, in.Type, in.ID); err != nil {
		return DeleteResult{Target: target}, err
	}
	return DeleteResult{Target: target}, nil
}

func (s *Service) require(ctx context.Context, target string, kind settings.WriteKind) (orgwrite.Target, error) {
	if err := ctx.Err(); err != nil {
		return orgwrite.Target{}, err
	}
	if s == nil || s.gate == nil {
		return orgwrite.Target{}, errors.New("metadata safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return orgwrite.Target{}, errors.New("metadata transport unavailable; write refused")
	}
	return s.gate.Require(target, kind)
}

func validateCreate(in CreateInput) error {
	if err := ValidateType(in.Type); err != nil {
		return err
	}
	if strings.TrimSpace(in.FullName) == "" {
		return errors.New("metadata full name is required")
	}
	if in.Metadata == nil {
		return errors.New("metadata payload is required")
	}
	return nil
}

func validateUpdate(in UpdateInput) error {
	if err := ValidateType(in.Type); err != nil {
		return err
	}
	if strings.TrimSpace(in.ID) == "" {
		return errors.New("metadata id is required")
	}
	if in.Patch == nil {
		return errors.New("metadata patch is required")
	}
	return nil
}

var knownTypes = map[string]struct{}{
	"CustomField": {}, "CustomObject": {}, "ValidationRule": {},
	"RecordType": {}, "ApexTrigger": {}, "FlexiPage": {}, "FieldSet": {},
	"WebLink": {}, "WorkflowRule": {}, "PermissionSet": {}, "CustomTab": {},
	"CustomLabel": {}, "CustomPermission": {}, "FlowDefinition": {}, "Layout": {},
}

// ErrInvalidType is returned before org resolution or Salesforce access.
type ErrInvalidType struct {
	Type string
}

func (e ErrInvalidType) Error() string {
	return fmt.Sprintf("unknown metadata type %q (want one of %s)",
		e.Type, strings.Join(KnownTypes(), ", "))
}

func ValidateType(metadataType string) error {
	if strings.TrimSpace(metadataType) == "" {
		return errors.New("metadata type is required")
	}
	if _, ok := knownTypes[metadataType]; !ok {
		return ErrInvalidType{Type: metadataType}
	}
	return nil
}

func KnownTypes() []string {
	out := make([]string, 0, len(knownTypes))
	for metadataType := range knownTypes {
		out = append(out, metadataType)
	}
	sort.Strings(out)
	return out
}
