// Package apexops provides safety-enforced anonymous Apex execution shared
// by the TUI, CLI, and IPC adapters.
package apexops

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// Remote is the narrow Salesforce capability needed for anonymous Apex and
// optional debug-log capture.
type Remote interface {
	Execute(target, body string) (sf.ExecuteAnonymousResult, error)
	EnsureTraceFlag(target, userID string) error
	FetchLatestLog(target, userID string, since time.Time) (logID, body string, err error)
}

type liveRemote struct{}

func (liveRemote) Execute(target, body string) (sf.ExecuteAnonymousResult, error) {
	return sf.ExecuteAnonymousAlias(target, body)
}
func (liveRemote) EnsureTraceFlag(target, userID string) error {
	_, err := sf.EnsureTraceFlagForUser(target, userID)
	return err
}
func (liveRemote) FetchLatestLog(target, userID string, since time.Time) (string, string, error) {
	return sf.FetchLatestApexLog(target, userID, since)
}

type Service struct {
	gate      *orgwrite.Gate
	remote    Remote
	pollDelay time.Duration
}

func New(gate *orgwrite.Gate) *Service {
	return NewWithRemote(gate, liveRemote{})
}

func NewWithRemote(gate *orgwrite.Gate, remote Remote) *Service {
	return &Service{gate: gate, remote: remote, pollDelay: time.Second}
}

type ExecuteInput struct {
	Target     string
	Body       string
	CaptureLog bool
	UserID     string
}

type ExecuteResult struct {
	Target    orgwrite.Target
	Execution sf.ExecuteAnonymousResult
}

// Execute requires full safety, optionally establishes the trace flag, runs
// the Apex, and best-effort attaches the resulting debug log. Trace/log
// failures remain non-fatal to preserve the TUI's existing behavior.
func (s *Service) Execute(ctx context.Context, in ExecuteInput) (ExecuteResult, error) {
	if strings.TrimSpace(in.Body) == "" {
		return ExecuteResult{}, errors.New("apex body required")
	}
	if err := ctx.Err(); err != nil {
		return ExecuteResult{}, err
	}
	if s == nil || s.gate == nil {
		return ExecuteResult{}, errors.New("apex safety gate unavailable; write refused")
	}
	if s.remote == nil {
		return ExecuteResult{}, errors.New("apex transport unavailable; write refused")
	}
	target, err := s.gate.Require(in.Target, settings.WriteAnonymous)
	if err != nil {
		return ExecuteResult{}, err
	}

	if in.CaptureLog && in.UserID != "" {
		_ = s.remote.EnsureTraceFlag(target.CLIArg, in.UserID)
	}
	since := time.Now().Add(-time.Second)
	execution, err := s.remote.Execute(target.CLIArg, in.Body)
	result := ExecuteResult{Target: target, Execution: execution}
	if err != nil {
		return result, err
	}
	if in.CaptureLog && execution.Compiled {
		for attempt := 0; attempt < 3; attempt++ {
			logID, logBody, logErr := s.remote.FetchLatestLog(target.CLIArg, in.UserID, since)
			if logErr == nil && logID != "" {
				result.Execution.LogID = logID
				result.Execution.LogBody = logBody
				break
			}
			if attempt < 2 {
				if err := wait(ctx, s.pollDelay); err != nil {
					return result, err
				}
			}
		}
	}
	return result, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
