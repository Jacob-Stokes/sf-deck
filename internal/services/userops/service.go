// Package userops provides full-safety user administration operations.
package userops

import (
	"context"
	"errors"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type Remote interface {
	ResetPassword(target, userID string) error
	GenerateResetLink(target, userID string) (string, error)
	SetActive(target, userID string, active bool) error
	SetFrozen(target, userID string, frozen bool) error
}

type liveRemote struct{}

func (liveRemote) ResetPassword(target, userID string) error {
	return sf.ResetUserPassword(target, userID)
}
func (liveRemote) GenerateResetLink(target, userID string) (string, error) {
	return sf.GenerateUserPasswordResetLink(target, userID)
}
func (liveRemote) SetActive(target, userID string, active bool) error {
	return sf.SetUserActive(target, userID, active)
}
func (liveRemote) SetFrozen(target, userID string, frozen bool) error {
	return sf.SetUserFrozen(target, userID, frozen)
}

type Service struct {
	gate   *orgwrite.Gate
	remote Remote
}

func New(gate *orgwrite.Gate) *Service { return NewWithRemote(gate, liveRemote{}) }

func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote}
}

type Input struct {
	Target string
	UserID string
}

type Result struct {
	Target orgwrite.Target
	URL    string
}

func (s *Service) ResetPassword(ctx context.Context, in Input) (Result, error) {
	target, err := s.require(ctx, in)
	if err != nil {
		return Result{}, err
	}
	err = s.remote.ResetPassword(target.CLIArg, in.UserID)
	return Result{Target: target}, err
}

func (s *Service) GenerateResetLink(ctx context.Context, in Input) (Result, error) {
	target, err := s.require(ctx, in)
	if err != nil {
		return Result{}, err
	}
	url, err := s.remote.GenerateResetLink(target.CLIArg, in.UserID)
	if err == nil && strings.TrimSpace(url) == "" {
		err = errors.New("password reset link response was empty")
	}
	return Result{Target: target, URL: url}, err
}

func (s *Service) SetActive(ctx context.Context, in Input, active bool) (Result, error) {
	target, err := s.require(ctx, in)
	if err != nil {
		return Result{}, err
	}
	err = s.remote.SetActive(target.CLIArg, in.UserID, active)
	return Result{Target: target}, err
}

func (s *Service) SetFrozen(ctx context.Context, in Input, frozen bool) (Result, error) {
	target, err := s.require(ctx, in)
	if err != nil {
		return Result{}, err
	}
	err = s.remote.SetFrozen(target.CLIArg, in.UserID, frozen)
	return Result{Target: target}, err
}

func (s *Service) require(ctx context.Context, in Input) (orgwrite.Target, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return orgwrite.Target{}, errors.New("user id is required")
	}
	if err := ctx.Err(); err != nil {
		return orgwrite.Target{}, err
	}
	if s == nil || s.gate == nil {
		return orgwrite.Target{}, errors.New("user administration safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return orgwrite.Target{}, errors.New("user administration transport unavailable; write refused")
	}
	return s.gate.Require(in.Target, settings.WriteAnonymous)
}
