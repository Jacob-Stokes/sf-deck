// Package orgwrite owns the shared org-resolution and safety gate used by
// Salesforce-mutating services. It deliberately sits below app, UI, CLI,
// and IPC adapters so every surface can enforce the same policy without
// importing presentation or wire-protocol types.
package orgwrite

import (
	"errors"
	"fmt"

	"github.com/Jacob-Stokes/sf-deck/internal/settings"
	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

// ResolveOrgFunc resolves an alias, username, or empty/default target to the
// canonical org value used for both safety evaluation and execution.
type ResolveOrgFunc func(target string) (sf.Org, error)

// SafetyForFunc returns the effective safety level for a resolved org.
type SafetyForFunc func(org sf.Org) settings.SafetyLevel

// Gate resolves Salesforce targets and enforces the product safety policy.
// A nil dependency fails closed.
type Gate struct {
	resolve   ResolveOrgFunc
	safetyFor SafetyForFunc
}

// Target is the resolved identity a service must use for its Salesforce
// call. CLIArg prefers the user-friendly alias and falls back to username.
type Target struct {
	Org      sf.Org
	CLIArg   string
	Username string
}

// NewGate constructs a shared gate. Dependencies are intentionally accepted
// as functions so app.App can wire its existing resolution methods without a
// package cycle and tests can inject narrow fakes.
func NewGate(resolve ResolveOrgFunc, safetyFor SafetyForFunc) *Gate {
	return &Gate{resolve: resolve, safetyFor: safetyFor}
}

// ReadTarget resolves a target without applying a write policy. Read-only
// service operations use this when they still need canonical org identity.
func (g *Gate) ReadTarget(target string) (Target, error) {
	if g == nil || g.resolve == nil {
		return Target{}, errors.New("org resolution unavailable")
	}
	o, err := g.resolve(target)
	if err != nil {
		return Target{}, ResolutionError{Target: target, Err: err}
	}
	return targetFromOrg(o), nil
}

// ResolutionError distinguishes target lookup failures from Salesforce
// execution failures so adapters can preserve their existing org-not-found
// response shapes without resolving a second time.
type ResolutionError struct {
	Target string
	Err    error
}

func (e ResolutionError) Error() string { return e.Err.Error() }
func (e ResolutionError) Unwrap() error { return e.Err }

// Require resolves target once, checks the requested write kind against that
// exact org, and returns the same canonical target for execution.
func (g *Gate) Require(target string, kind settings.WriteKind) (Target, error) {
	resolved, err := g.ReadTarget(target)
	if err != nil {
		return Target{}, err
	}
	if err := g.check(resolved, kind); err != nil {
		return Target{}, err
	}
	return resolved, nil
}

// Check applies a write policy to an already-resolved org. It exists for
// compatibility with app.CanWrite and TUI affordance checks; new services
// should normally use Require so resolution and execution cannot diverge.
func (g *Gate) Check(org sf.Org, kind settings.WriteKind) error {
	return g.check(targetFromOrg(org), kind)
}

func (g *Gate) check(target Target, kind settings.WriteKind) error {
	if g == nil || g.safetyFor == nil {
		return errors.New("safety policy unavailable; write refused")
	}
	level := g.safetyFor(target.Org)
	if level.Allows(kind) {
		return nil
	}
	return BlockedError{
		Target:   target.CLIArg,
		Username: target.Username,
		Required: kind,
		Actual:   level,
	}
}

func targetFromOrg(o sf.Org) Target {
	arg := o.Alias
	if arg == "" {
		arg = o.Username
	}
	return Target{Org: o, CLIArg: arg, Username: o.Username}
}

// BlockedError is the typed safety denial shared by services and adapters.
// CLI and IPC map it to their stable safety_blocked response code.
type BlockedError struct {
	Target   string
	Username string
	Required settings.WriteKind
	Actual   settings.SafetyLevel
}

func (e BlockedError) Error() string {
	return fmt.Sprintf("%s is %s; requires %s",
		e.Target, e.Actual.String(), writeKindLabel(e.Required))
}

func writeKindLabel(k settings.WriteKind) string {
	switch k {
	case settings.WriteRecord:
		return "records"
	case settings.WriteMetadata:
		return "metadata"
	case settings.WriteAnonymous:
		return "full"
	}
	return "unknown"
}
