package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestErrEmptyMessage(t *testing.T) {
	// Known SF error code -> raw error + a human hint.
	apiOff := &sf.SFError{Code: "API_DISABLED_FOR_ORG", Message: "API is not enabled"}
	got := errEmptyMessage(apiOff)
	if !strings.Contains(got, "couldn't load") {
		t.Errorf("missing headline: %q", got)
	}
	if !strings.Contains(got, "API access enabled") {
		t.Errorf("missing API-disabled hint: %q", got)
	}

	// Permission error -> its hint.
	perm := &sf.SFError{Code: "INSUFFICIENT_ACCESS_OR_READONLY", Message: "no access"}
	if !strings.Contains(errEmptyMessage(perm), "permission") {
		t.Errorf("missing permission hint for INSUFFICIENT_ACCESS")
	}

	// Unknown/plain error -> raw text, no invented hint (still shown, not swallowed).
	plain := errors.New("dial tcp: connection refused")
	g := errEmptyMessage(plain)
	if !strings.Contains(g, "connection refused") {
		t.Errorf("plain error text must be surfaced, got %q", g)
	}
	// Only one line (headline) when there's no hint.
	if strings.Count(g, "\n") != 0 {
		t.Errorf("plain error should be single-line, got %q", g)
	}
}
