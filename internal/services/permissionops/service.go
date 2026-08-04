// Package permissionops provides safety-enforced PermissionSet, object
// permission, and field-level-security mutations.
package permissionops

import (
	"context"
	"errors"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type Remote interface {
	UpsertField(target, id, sobject, field, parentID string, read, edit bool) (string, error)
	DeleteField(target, id string) error
	UpsertObject(target, id, parentID, sobject string, read, create, edit, delete, viewAll, modifyAll bool) (string, error)
	DeleteObject(target, id string) error
	SetSystem(target, parentID, field string, value bool) error
}

type liveRemote struct{}

func (liveRemote) UpsertField(target, id, sobject, field, parentID string, read, edit bool) (string, error) {
	return sf.UpsertFieldPermission(target, id, sobject, field, parentID, read, edit)
}
func (liveRemote) DeleteField(target, id string) error {
	return sf.DeleteFieldPermission(target, id)
}
func (liveRemote) UpsertObject(target, id, parentID, sobject string, read, create, edit, delete, viewAll, modifyAll bool) (string, error) {
	return sf.UpsertObjectPermission(target, id, parentID, sobject, read, create, edit, delete, viewAll, modifyAll)
}
func (liveRemote) DeleteObject(target, id string) error {
	return sf.DeleteObjectPermission(target, id)
}
func (liveRemote) SetSystem(target, parentID, field string, value bool) error {
	return sf.TogglePermissionSetBool(target, parentID, field, value)
}

type Service struct {
	gate   *orgwrite.Gate
	remote Remote
}

func New(gate *orgwrite.Gate) *Service { return NewWithRemote(gate, liveRemote{}) }

func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote}
}

type FieldInput struct {
	Target   string
	ID       string
	SObject  string
	Field    string
	ParentID string
	Read     bool
	Edit     bool
}

type ObjectInput struct {
	Target    string
	ID        string
	ParentID  string
	SObject   string
	Read      bool
	Create    bool
	Edit      bool
	Delete    bool
	ViewAll   bool
	ModifyAll bool
}

type SystemInput struct {
	Target   string
	ParentID string
	Field    string
	Value    bool
}

type Result struct {
	Target  orgwrite.Target
	ID      string
	Deleted bool
	Noop    bool
}

func (s *Service) SetField(ctx context.Context, in FieldInput) (Result, error) {
	if strings.TrimSpace(in.SObject) == "" || strings.TrimSpace(in.Field) == "" || strings.TrimSpace(in.ParentID) == "" {
		return Result{}, errors.New("sobject, field, and permission parent id are required")
	}
	target, err := s.require(ctx, in.Target)
	if err != nil {
		return Result{}, err
	}
	if !in.Read && !in.Edit {
		if in.ID == "" {
			return Result{Target: target, Noop: true}, nil
		}
		err := s.remote.DeleteField(target.CLIArg, in.ID)
		return Result{Target: target, ID: in.ID, Deleted: err == nil}, err
	}
	id, err := s.remote.UpsertField(target.CLIArg, in.ID, in.SObject, in.Field, in.ParentID, in.Read, in.Edit)
	return Result{Target: target, ID: id}, err
}

func (s *Service) SetObject(ctx context.Context, in ObjectInput) (Result, error) {
	if strings.TrimSpace(in.SObject) == "" || strings.TrimSpace(in.ParentID) == "" {
		return Result{}, errors.New("sobject and permission parent id are required")
	}
	target, err := s.require(ctx, in.Target)
	if err != nil {
		return Result{}, err
	}
	allFalse := !in.Read && !in.Create && !in.Edit && !in.Delete && !in.ViewAll && !in.ModifyAll
	if allFalse {
		if in.ID == "" {
			return Result{Target: target, Noop: true}, nil
		}
		err := s.remote.DeleteObject(target.CLIArg, in.ID)
		return Result{Target: target, ID: in.ID, Deleted: err == nil}, err
	}
	id, err := s.remote.UpsertObject(target.CLIArg, in.ID, in.ParentID, in.SObject,
		in.Read, in.Create, in.Edit, in.Delete, in.ViewAll, in.ModifyAll)
	return Result{Target: target, ID: id}, err
}

func (s *Service) SetSystem(ctx context.Context, in SystemInput) (Result, error) {
	if strings.TrimSpace(in.ParentID) == "" {
		return Result{}, errors.New("permission parent id is required")
	}
	if !strings.HasPrefix(in.Field, "Permissions") || len(in.Field) == len("Permissions") {
		return Result{}, errors.New("system permission field must start with Permissions")
	}
	target, err := s.require(ctx, in.Target)
	if err != nil {
		return Result{}, err
	}
	err = s.remote.SetSystem(target.CLIArg, in.ParentID, in.Field, in.Value)
	return Result{Target: target, ID: in.ParentID}, err
}

func (s *Service) require(ctx context.Context, target string) (orgwrite.Target, error) {
	if err := ctx.Err(); err != nil {
		return orgwrite.Target{}, err
	}
	if s == nil || s.gate == nil {
		return orgwrite.Target{}, errors.New("permission safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return orgwrite.Target{}, errors.New("permission transport unavailable; write refused")
	}
	return s.gate.Require(target, settings.WriteMetadata)
}
