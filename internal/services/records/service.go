// Package records provides safety-enforced record mutations shared by the
// CLI, IPC, and TUI adapters.
package records

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Remote is the narrow Salesforce capability required by record writes.
// ResolveSObject is used only when update/delete callers omit an object name;
// it runs after the write gate so a denied operation cannot make a read call.
type Remote interface {
	ResolveSObject(target, recordID string) (string, error)
	Create(target, sobject string, fields map[string]any) ([]sf.FieldError, string, error)
	Update(target, sobject, id string, fields map[string]any) ([]sf.FieldError, error)
	Delete(target, sobject, id string) error
}

type liveRemote struct{}

func (liveRemote) ResolveSObject(target, recordID string) (string, error) {
	all, err := sf.ListSObjects(target)
	if err != nil {
		return "", err
	}
	if sobject, ok := sf.SObjectByKeyPrefix(all, recordID); ok {
		return sobject.Name, nil
	}
	return "", fmt.Errorf("unable to resolve sObject from key prefix %q; pass --object explicitly", recordID[:3])
}
func (liveRemote) Create(target, sobject string, fields map[string]any) ([]sf.FieldError, string, error) {
	return sf.CreateRecordAlias(target, sobject, fields)
}
func (liveRemote) Update(target, sobject, id string, fields map[string]any) ([]sf.FieldError, error) {
	return sf.UpdateRecordAlias(target, sobject, id, fields)
}
func (liveRemote) Delete(target, sobject, id string) error {
	return sf.DeleteRecordAlias(target, sobject, id)
}

type Service struct {
	gate   *orgwrite.Gate
	remote Remote
}

func New(gate *orgwrite.Gate) *Service { return NewWithRemote(gate, liveRemote{}) }

func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote}
}

type CreateInput struct {
	Target  string
	SObject string
	Fields  map[string]any
}

type CreateResult struct {
	Target      orgwrite.Target
	SObject     string
	ID          string
	FieldErrors []sf.FieldError
}

type UpdateInput struct {
	Target  string
	SObject string
	ID      string
	Fields  map[string]any
}

type UpdateResult struct {
	Target      orgwrite.Target
	SObject     string
	ID          string
	FieldErrors []sf.FieldError
}

type DeleteInput struct {
	Target  string
	SObject string
	ID      string
}

type DeleteResult struct {
	Target  orgwrite.Target
	SObject string
	ID      string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	sobject := strings.TrimSpace(in.SObject)
	if err := validateSObject(sobject); err != nil {
		return CreateResult{}, err
	}
	if len(in.Fields) == 0 {
		return CreateResult{}, errors.New("at least one field is required")
	}
	target, err := s.require(ctx, in.Target, writeKindForSObject(sobject))
	if err != nil {
		return CreateResult{}, err
	}
	fieldErrors, id, err := s.remote.Create(target.CLIArg, sobject, in.Fields)
	result := CreateResult{Target: target, SObject: sobject, ID: id, FieldErrors: fieldErrors}
	return result, err
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (UpdateResult, error) {
	if err := validateID(in.ID); err != nil {
		return UpdateResult{}, err
	}
	if len(in.Fields) == 0 {
		return UpdateResult{}, errors.New("at least one field is required")
	}
	sobject := strings.TrimSpace(in.SObject)
	if sobject != "" {
		if err := validateSObject(sobject); err != nil {
			return UpdateResult{}, err
		}
	}
	target, err := s.require(ctx, in.Target, writeKindForSObject(sobject))
	if err != nil {
		return UpdateResult{}, err
	}
	sobject, err = s.resolveSObject(target, sobject, in.ID)
	if err != nil {
		return UpdateResult{Target: target, ID: in.ID}, err
	}
	if err := validateSObject(sobject); err != nil {
		return UpdateResult{Target: target, ID: in.ID}, err
	}
	if err := s.requireResolved(target, writeKindForSObject(sobject)); err != nil {
		return UpdateResult{Target: target, SObject: sobject, ID: in.ID}, err
	}
	fieldErrors, err := s.remote.Update(target.CLIArg, sobject, in.ID, in.Fields)
	result := UpdateResult{Target: target, SObject: sobject, ID: in.ID, FieldErrors: fieldErrors}
	return result, err
}

func (s *Service) Delete(ctx context.Context, in DeleteInput) (DeleteResult, error) {
	if err := validateID(in.ID); err != nil {
		return DeleteResult{}, err
	}
	sobject := strings.TrimSpace(in.SObject)
	if sobject != "" {
		if err := validateSObject(sobject); err != nil {
			return DeleteResult{}, err
		}
	}
	target, err := s.require(ctx, in.Target, writeKindForSObject(sobject))
	if err != nil {
		return DeleteResult{}, err
	}
	sobject, err = s.resolveSObject(target, sobject, in.ID)
	if err != nil {
		return DeleteResult{Target: target, ID: in.ID}, err
	}
	if err := validateSObject(sobject); err != nil {
		return DeleteResult{Target: target, ID: in.ID}, err
	}
	if err := s.requireResolved(target, writeKindForSObject(sobject)); err != nil {
		return DeleteResult{Target: target, SObject: sobject, ID: in.ID}, err
	}
	result := DeleteResult{Target: target, SObject: sobject, ID: in.ID}
	if err := s.remote.Delete(target.CLIArg, sobject, in.ID); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) require(ctx context.Context, target string, kind settings.WriteKind) (orgwrite.Target, error) {
	if err := ctx.Err(); err != nil {
		return orgwrite.Target{}, err
	}
	if s == nil || s.gate == nil {
		return orgwrite.Target{}, errors.New("record safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return orgwrite.Target{}, errors.New("record transport unavailable; write refused")
	}
	return s.gate.Require(target, kind)
}

// requireResolved reapplies the final object's policy to the exact org that
// was already resolved. This is needed when update/delete infer the sObject
// from an ID: resolving the object is read-only, but the mutation must not
// remain authorized at the provisional records tier if the result is a
// privileged setup object.
func (s *Service) requireResolved(target orgwrite.Target, kind settings.WriteKind) error {
	return s.gate.Check(target.Org, kind)
}

func (s *Service) resolveSObject(target orgwrite.Target, sobject, id string) (string, error) {
	if sobject != "" {
		return sobject, nil
	}
	return s.remote.ResolveSObject(target.CLIArg, id)
}

func validateID(id string) error {
	if id == "" {
		return errors.New("record id is required")
	}
	if len(id) != 15 && len(id) != 18 {
		return fmt.Errorf("invalid record id %q (must be 15 or 18 chars)", id)
	}
	for _, c := range []byte(id) {
		if !isASCIIAlpha(c) && (c < '0' || c > '9') {
			return fmt.Errorf("invalid record id %q (must be ASCII alphanumeric)", id)
		}
	}
	return nil
}

func validateSObject(name string) error {
	if name == "" {
		return errors.New("sobject is required")
	}
	if len(name) > 255 || !isASCIIAlpha(name[0]) {
		return fmt.Errorf("invalid sobject API name %q", name)
	}
	for _, c := range []byte(name[1:]) {
		if !isASCIIAlpha(c) && (c < '0' || c > '9') && c != '_' {
			return fmt.Errorf("invalid sobject API name %q", name)
		}
	}
	return nil
}

func isASCIIAlpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// writeKindForSObject prevents the generic record API from becoming a
// lower-tier escape hatch around the dedicated administration and metadata
// services. Exact matches are case-insensitive because Salesforce API names
// are case-insensitive. Share tables are also privileged: writing one grants
// or revokes record access even when the parent is an ordinary data object.
func writeKindForSObject(name string) settings.WriteKind {
	low := strings.ToLower(name)
	if strings.HasSuffix(low, "share") {
		return settings.WriteAnonymous
	}
	if _, ok := fullSafetySObjects[low]; ok {
		return settings.WriteAnonymous
	}
	if _, ok := metadataSafetySObjects[low]; ok {
		return settings.WriteMetadata
	}
	return settings.WriteRecord
}

var fullSafetySObjects = map[string]struct{}{
	"authsession":                {},
	"externaldatauserauth":       {},
	"groupmember":                {},
	"oauthcustomscopeapp":        {},
	"oauthtoken":                 {},
	"permissionsetassignment":    {},
	"permissionsetlicenseassign": {},
	"sessionpermsetactivation":   {},
	"twofactormethodsinfo":       {},
	"user":                       {},
	"userlogin":                  {},
	"userprovisioningrequest":    {},
	"userterritory2association":  {},
}

var metadataSafetySObjects = map[string]struct{}{
	"apexclass":                   {},
	"apexcomponent":               {},
	"apexpage":                    {},
	"apextrigger":                 {},
	"fieldpermissions":            {},
	"flow":                        {},
	"flowdefinition":              {},
	"group":                       {},
	"mutingpermissionset":         {},
	"objectpermissions":           {},
	"permissionset":               {},
	"permissionsetgroup":          {},
	"permissionsetgroupcomponent": {},
	"queuesobject":                {},
	"setupentityaccess":           {},
	"territory2":                  {},
	"territory2model":             {},
	"territory2type":              {},
}
