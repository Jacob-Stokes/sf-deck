package ui

import (
	"strings"
	"testing"

	"github.com/Jacob-Stokes/sf-deck/internal/sf"
)

func TestSOQLResultsTitle_AllRows(t *testing.T) {
	res := sf.QueryResult{TotalSize: 50, Done: true}
	got := soqlResultsTitle(res, 50)
	want := "RESULTS · 50 rows"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSOQLResultsTitle_TruncatedDoneFalse(t *testing.T) {
	// 2000-row sync limit hit, more results available.
	res := sf.QueryResult{TotalSize: 12000, Done: false}
	got := soqlResultsTitle(res, 2000)
	if !strings.Contains(got, "2000 of 12000") {
		t.Errorf("missing count: %q", got)
	}
	if !strings.Contains(got, "capped") {
		t.Errorf("missing cap warning: %q", got)
	}
}

func TestSOQLResultsTitle_PartialButDone(t *testing.T) {
	// Edge case: client truncated the slice but server says it's
	// done. Show "X of Y" without the cap warning.
	res := sf.QueryResult{TotalSize: 100, Done: true}
	got := soqlResultsTitle(res, 50)
	if !strings.Contains(got, "50 of 100") {
		t.Errorf("missing count: %q", got)
	}
	if strings.Contains(got, "capped") {
		t.Errorf("shouldn't show cap when Done=true: %q", got)
	}
}
