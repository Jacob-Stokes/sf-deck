package apexops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jacob-Stokes/sf-deck/internal/services/orgwrite"
	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

type fakeRemote struct {
	calls      []string
	target     string
	body       string
	execution  sf.ExecuteAnonymousResult
	executeErr error
	traceErr   error
	logID      string
	logBody    string
	logErr     error
}

func (f *fakeRemote) Execute(target, body string) (sf.ExecuteAnonymousResult, error) {
	f.calls = append(f.calls, "execute")
	f.target, f.body = target, body
	return f.execution, f.executeErr
}
func (f *fakeRemote) EnsureTraceFlag(target, userID string) error {
	f.calls = append(f.calls, "trace")
	f.target = target
	return f.traceErr
}
func (f *fakeRemote) FetchLatestLog(target, userID string, since time.Time) (string, string, error) {
	f.calls = append(f.calls, "log")
	return f.logID, f.logBody, f.logErr
}

func serviceAt(level settings.SafetyLevel, remote Remote) *Service {
	g := orgwrite.NewGate(func(string) (sf.Org, error) {
		return sf.Org{Alias: "resolved", Username: "u@example.com"}, nil
	}, func(sf.Org) settings.SafetyLevel { return level })
	return NewWithRemote(g, remote)
}

func TestExecuteRequiresFullBeforeRemote(t *testing.T) {
	remote := &fakeRemote{}
	_, err := serviceAt(settings.SafetyMetadata, remote).Execute(context.Background(), ExecuteInput{Body: "System.debug('x');"})
	var blocked orgwrite.BlockedError
	if !errors.As(err, &blocked) || blocked.Required != settings.WriteAnonymous {
		t.Fatalf("err = %#v, want full BlockedError", err)
	}
	if len(remote.calls) != 0 {
		t.Fatalf("remote called on denial: %v", remote.calls)
	}
}

func TestExecuteUsesResolvedTarget(t *testing.T) {
	remote := &fakeRemote{execution: sf.ExecuteAnonymousResult{Compiled: true, Success: true}}
	got, err := serviceAt(settings.SafetyFull, remote).Execute(context.Background(), ExecuteInput{Body: "System.debug('x');"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Target.CLIArg != "resolved" || remote.target != "resolved" || remote.body != "System.debug('x');" {
		t.Fatalf("result=%#v remote=%#v", got, remote)
	}
}

func TestExecuteCapturesLogInsideGate(t *testing.T) {
	remote := &fakeRemote{
		execution: sf.ExecuteAnonymousResult{Compiled: true, Success: true},
		traceErr:  errors.New("trace unavailable"), // non-fatal
		logID:     "07L-log",
		logBody:   "USER_DEBUG hello",
	}
	got, err := serviceAt(settings.SafetyFull, remote).Execute(context.Background(), ExecuteInput{
		Body: "System.debug('hello');", CaptureLog: true, UserID: "005-user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Execution.LogID != "07L-log" || got.Execution.LogBody != "USER_DEBUG hello" {
		t.Fatalf("execution = %#v", got.Execution)
	}
	if want := []string{"trace", "execute", "log"}; len(remote.calls) != len(want) {
		t.Fatalf("calls = %v", remote.calls)
	} else {
		for i := range want {
			if remote.calls[i] != want[i] {
				t.Fatalf("calls = %v, want %v", remote.calls, want)
			}
		}
	}
}

func TestExecuteRejectsEmptyAndMissingDependencies(t *testing.T) {
	if _, err := serviceAt(settings.SafetyFull, &fakeRemote{}).Execute(context.Background(), ExecuteInput{}); err == nil {
		t.Fatal("empty body accepted")
	}
	valid := ExecuteInput{Body: "x;"}
	for _, s := range []*Service{nil, NewWithRemote(nil, &fakeRemote{}), NewWithRemote(
		orgwrite.NewGate(func(string) (sf.Org, error) { return sf.Org{}, nil },
			func(sf.Org) settings.SafetyLevel { return settings.SafetyFull }), nil)} {
		if _, err := s.Execute(context.Background(), valid); err == nil {
			t.Fatal("missing dependency must fail closed")
		}
	}
}

func TestExecutePropagatesRemoteErrorWithTarget(t *testing.T) {
	want := errors.New("network failed")
	remote := &fakeRemote{executeErr: want}
	got, err := serviceAt(settings.SafetyFull, remote).Execute(context.Background(), ExecuteInput{Body: "x;"})
	if !errors.Is(err, want) || got.Target.Username != "u@example.com" {
		t.Fatalf("result=%#v err=%v", got, err)
	}
}
