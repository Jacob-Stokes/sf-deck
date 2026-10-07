// Package notificationops provides safety-enforced notification state writes.
package notificationops

import (
	"context"
	"errors"
	"strings"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type Remote interface {
	MarkRead(target, id string) error
	MarkAllRead(target string) error
}

type liveRemote struct{}

func (liveRemote) MarkRead(target, id string) error { return sf.MarkNotificationRead(target, id) }
func (liveRemote) MarkAllRead(target string) error  { return sf.MarkAllNotificationsRead(target) }

type Service struct {
	gate   *orgwrite.Gate
	remote Remote
}

func New(gate *orgwrite.Gate) *Service { return NewWithRemote(gate, liveRemote{}) }

func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote}
}

type MarkReadInput struct {
	Target string
	ID     string
	All    bool
}

type MarkReadResult struct {
	Target orgwrite.Target
	ID     string
	All    bool
}

func (s *Service) MarkRead(ctx context.Context, in MarkReadInput) (MarkReadResult, error) {
	if strings.TrimSpace(in.ID) == "" && !in.All {
		return MarkReadResult{}, errors.New("notification id or all is required")
	}
	if strings.TrimSpace(in.ID) != "" && in.All {
		return MarkReadResult{}, errors.New("notification id and all are mutually exclusive")
	}
	if err := ctx.Err(); err != nil {
		return MarkReadResult{}, err
	}
	if s == nil || s.gate == nil {
		return MarkReadResult{}, errors.New("notification safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return MarkReadResult{}, errors.New("notification transport unavailable; write refused")
	}
	target, err := s.gate.Require(in.Target, settings.WriteRecord)
	if err != nil {
		return MarkReadResult{}, err
	}
	result := MarkReadResult{Target: target, ID: in.ID, All: in.All}
	if in.All {
		err = s.remote.MarkAllRead(target.CLIArg)
	} else {
		err = s.remote.MarkRead(target.CLIArg, in.ID)
	}
	return result, err
}
